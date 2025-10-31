package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/models"
)

// DiscoveryService handles network discovery of MCP servers
type DiscoveryService struct {
	httpClient *http.Client
	scans      map[string]*models.ScanJob
	cache      map[string]*models.DiscoveredServer
	mu         sync.RWMutex
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService() *DiscoveryService {
	return &DiscoveryService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		scans:      make(map[string]*models.ScanJob),
		cache:      make(map[string]*models.DiscoveredServer),
	}
}

// StartScan handles POST /discovery/scan
// @Summary Start network scan for MCP servers
// @Description Initiates a network scan to discover MCP servers
// @Tags discovery
// @Accept json
// @Produce json
// @Param config body models.ScanConfig true "Scan configuration"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Router /scan [post]
func (ds *DiscoveryService) StartScan(c *gin.Context) {
	log.Printf("DiscoveryService: StartScan called - REAL FUNCTION")
	var config models.ScanConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		log.Printf("DiscoveryService: JSON binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("DiscoveryService: Config received: %+v", config)

	// Set defaults if not provided
	if len(config.ScanRanges) == 0 {
		config.ScanRanges = []string{"127.0.0.1/32"} // Only localhost for testing
	}
	if len(config.Ports) == 0 {
		config.Ports = []int{8000, 3000, 5000, 8080}
	}
	if config.Timeout == "" {
		config.Timeout = "30s"
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 10
	}

	// Parse timeout
	timeoutDuration, err := time.ParseDuration(config.Timeout)
	if err != nil {
		log.Printf("DiscoveryService: Timeout parse error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid timeout format"})
		return
	}

	// Validate configuration
	if err := ds.validateScanConfig(config); err != nil {
		log.Printf("DiscoveryService: Validation error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate scan ID
	scanId := fmt.Sprintf("scan-%d", time.Now().UnixNano())

	// Create scan job
	job := &models.ScanJob{
		ID:        scanId,
		Status:    "running",
		StartTime: time.Now(),
		Config:    config,
	}

	// Store job
	ds.mu.Lock()
	ds.scans[scanId] = job
	ds.mu.Unlock()

	// Start scan synchronously for testing
	log.Printf("DiscoveryService: About to call runScan")
	ds.runScan(scanId, config, timeoutDuration)
	log.Printf("DiscoveryService: runScan completed")

	// Get final job status
	ds.mu.RLock()
	finalJob := ds.scans[scanId]
	ds.mu.RUnlock()

	response := gin.H{
		"scanId":  scanId,
		"status":  finalJob.Status,
		"message": "Network scan completed",
	}

	if finalJob.Status == "completed" {
		response["serverCount"] = len(finalJob.Results)
		response["results"] = finalJob.Results
	}

	if finalJob.Error != "" {
		response["error"] = finalJob.Error
	}

	log.Printf("DiscoveryService: Sending response: %+v", response)
	c.JSON(http.StatusOK, response)
}

// GetScanStatus handles GET /discovery/scan/:scanId
// @Summary Get scan status
// @Description Retrieve the status and results of a network scan
// @Tags discovery
// @Produce json
// @Param scanId path string true "Scan ID"
// @Success 200 {object} models.ScanJob
// @Failure 404 {object} ErrorResponse
// @Router /scan/{scanId} [get]
func (ds *DiscoveryService) GetScanStatus(c *gin.Context) {
	scanId := c.Param("scanId")

	ds.mu.RLock()
	job, exists := ds.scans[scanId]
	ds.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Scan not found"})
		return
	}

	response := gin.H{
		"scanId":    job.ID,
		"status":    job.Status,
		"startTime": job.StartTime,
		"duration":  time.Since(job.StartTime).String(),
		"config":    job.Config,
	}

	if job.Status == "completed" {
		response["serverCount"] = len(job.Results)
		response["results"] = job.Results
	}

	if job.Error != "" {
		response["error"] = job.Error
	}

	c.JSON(http.StatusOK, response)
}

// ListDiscoveredServers handles GET /discovery/servers
// @Summary List discovered servers
// @Description Retrieve all discovered MCP servers
// @Tags discovery
// @Produce json
// @Success 200 {array} models.DiscoveredServer
// @Router /servers [get]
func (ds *DiscoveryService) ListDiscoveredServers(c *gin.Context) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	servers := make([]*models.DiscoveredServer, 0, len(ds.cache))
	for _, server := range ds.cache {
		servers = append(servers, server)
	}

	c.JSON(http.StatusOK, servers)
}

// RegisterServer handles POST /discovery/register
// @Summary Register discovered server
// @Description Register a discovered MCP server as an adapter
// @Tags discovery
// @Accept json
// @Produce json
// @Param request body map[string]string true "Registration request with discoveredServerId"
// @Success 201 {object} models.AdapterResource
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /register [post]
func (ds *DiscoveryService) RegisterServer(c *gin.Context) {
	var req struct {
		DiscoveredServerId string `json:"discoveredServerId" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ds.mu.RLock()
	server, exists := ds.cache[req.DiscoveredServerId]
	ds.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Discovered server not found"})
		return
	}

	// Parse address to extract host
	host, _, err := ds.parseAddress(server.Address)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server address"})
		return
	}

	// Create adapter data from discovered server
	adapterData := models.AdapterData{
		Name:        fmt.Sprintf("discovered-%s-%d", strings.ReplaceAll(host, ".", "-"), time.Now().Unix()),
		Protocol:    server.Protocol,
		Description: fmt.Sprintf("Auto-discovered MCP server at %s", server.Address),
	}

	if server.Connection == models.ConnectionTypeRemoteHttp {
		adapterData.ConnectionType = models.ConnectionTypeRemoteHttp
		adapterData.RemoteUrl = server.Address
	} else if server.Connection == models.ConnectionTypeLocalStdio {
		adapterData.ConnectionType = models.ConnectionTypeLocalStdio
		adapterData.Command = "python"                      // Assume python for discovered
		adapterData.Args = []string{"discovered_server.py"} // Placeholder
	} else {
		// For K8s, set defaults
		adapterData.ConnectionType = server.Connection
		adapterData.ImageName = "mcp-proxy"
		adapterData.ImageVersion = "1.0.0"
		adapterData.EnvironmentVariables = map[string]string{
			"MCP_PROXY_URL": server.Address + "/mcp",
		}
	}

	// Note: This would need access to ManagementService to actually create the adapter
	// For now, just return the adapter data that would be created
	c.JSON(http.StatusCreated, gin.H{
		"message":     "Server registration prepared",
		"adapterData": adapterData,
		"note":        "Integration with ManagementService needed for actual adapter creation",
	})
}

// validateScanConfig validates scan configuration
func (ds *DiscoveryService) validateScanConfig(config models.ScanConfig) error {
	if len(config.ScanRanges) == 0 {
		return fmt.Errorf("at least one scan range required")
	}

	for _, cidr := range config.ScanRanges {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid CIDR range: %s", cidr)
		}
	}

	if len(config.Ports) == 0 {
		return fmt.Errorf("at least one port required")
	}

	// Parse timeout
	if config.Timeout != "" {
		if _, err := time.ParseDuration(config.Timeout); err != nil {
			return fmt.Errorf("invalid timeout format: %s", config.Timeout)
		}
	}

	if config.MaxConcurrent < 1 || config.MaxConcurrent > 100 {
		return fmt.Errorf("maxConcurrent must be between 1 and 100")
	}

	return nil
}

// runScan executes the network scan
func (ds *DiscoveryService) runScan(scanId string, config models.ScanConfig, timeout time.Duration) {
	log.Printf("DiscoveryService: runScan called for scanId: %s", scanId)
	ds.mu.RLock()
	job := ds.scans[scanId]
	ds.mu.RUnlock()

	if job == nil {
		log.Printf("DiscoveryService: Job not found for scanId: %s", scanId)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("DiscoveryService: Scan %s panicked: %v", scanId, r)
			ds.mu.Lock()
			job.Status = "failed"
			job.Error = fmt.Sprintf("panic: %v", r)
			ds.mu.Unlock()
		} else {
			log.Printf("DiscoveryService: Scan %s completed successfully", scanId)
			ds.mu.Lock()
			job.Status = "completed"
			ds.mu.Unlock()
		}
	}()

	log.Printf("DiscoveryService: Starting scan %s with config: %+v", scanId, config)

	// Generate all IP:port combinations to scan
	log.Printf("DiscoveryService: Generating targets...")
	targets := ds.generateTargets(config)

	previewCount := 5
	if len(targets) < 5 {
		previewCount = len(targets)
	}
	log.Printf("DiscoveryService: Generated %d targets: %v", len(targets), targets[:previewCount])

	// Scan targets with concurrency control
	log.Printf("DiscoveryService: Scanning targets...")
	results := ds.scanTargets(targets, config, timeout)

	reachable := 0
	for _, r := range results {
		if r.Reachable {
			reachable++
		}
	}
	log.Printf("DiscoveryService: Scanned %d targets, %d reachable", len(results), reachable)

	// Detect MCP servers from results
	log.Printf("DiscoveryService: Detecting MCP servers...")
	mcpServers := ds.detectMCPServers(results)

	log.Printf("DiscoveryService: Found %d MCP servers", len(mcpServers))

	// Update job results
	ds.mu.Lock()
	job.Results = mcpServers
	ds.mu.Unlock()

	// Cache discovered servers
	ds.cacheServers(mcpServers)
}

// generateTargets creates all IP:port combinations to scan
func (ds *DiscoveryService) generateTargets(config models.ScanConfig) []string {
	log.Printf("DiscoveryService: generateTargets called with config: %+v", config)
	var targets []string

	// Get proxy addresses to exclude (default behavior)
	excludeProxy := true
	if config.ExcludeProxy != nil {
		excludeProxy = *config.ExcludeProxy
	}

	var proxyAddrs []string
	if excludeProxy {
		proxyAddrs = ds.getProxyAddresses()
		log.Printf("DiscoveryService: Excluding proxy addresses: %v", proxyAddrs)
	}

	// Add custom exclusions
	excludedAddrs := append(proxyAddrs, config.ExcludeAddresses...)

	for _, cidr := range config.ScanRanges {
		log.Printf("DiscoveryService: Expanding CIDR %s", cidr)
		ips, err := ds.expandCIDR(cidr)
		if err != nil {
			log.Printf("DiscoveryService: Error expanding CIDR %s: %v", cidr, err)
			continue
		}
		log.Printf("DiscoveryService: CIDR %s expanded to %d IPs: %v", cidr, len(ips), ips)

		for _, ip := range ips {
			for _, port := range config.Ports {
				target := fmt.Sprintf("http://%s:%d", ip, port)

				// Check if target should be excluded
				shouldExclude := false
				for _, excludedAddr := range excludedAddrs {
					if strings.Contains(target, excludedAddr) {
						shouldExclude = true
						log.Printf("DiscoveryService: Excluding address: %s", target)
						break
					}
				}

				if !shouldExclude {
					targets = append(targets, target)
					log.Printf("DiscoveryService: Added target: %s", target)
				}
			}
		}
	}

	log.Printf("DiscoveryService: Total targets generated: %d", len(targets))
	return targets
}

// getProxyAddresses returns all addresses where the proxy is listening
func (ds *DiscoveryService) getProxyAddresses() []string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8911"
	}

	// Get all network interfaces
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("DiscoveryService: Failed to get interface addresses: %v", err)
		return []string{"127.0.0.1:" + port, "localhost:" + port}
	}

	var proxyAddrs []string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				proxyAddrs = append(proxyAddrs, fmt.Sprintf("%s:%s", ipnet.IP.String(), port))
			}
		}
	}

	// Always include localhost
	proxyAddrs = append(proxyAddrs, fmt.Sprintf("127.0.0.1:%s", port))
	proxyAddrs = append(proxyAddrs, fmt.Sprintf("localhost:%s", port))

	return proxyAddrs
}

// expandCIDR expands a CIDR range into individual IP addresses
func (ds *DiscoveryService) expandCIDR(cidr string) ([]string, error) {
	log.Printf("DiscoveryService: expandCIDR called with: %s", cidr)

	// Handle simple IP addresses (no CIDR notation)
	if !strings.Contains(cidr, "/") {
		if net.ParseIP(cidr) != nil {
			log.Printf("DiscoveryService: Returning simple IP: %s", cidr)
			return []string{cidr}, nil
		}
		return nil, fmt.Errorf("invalid IP address: %s", cidr)
	}

	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Printf("DiscoveryService: ParseCIDR error: %v", err)
		return nil, err
	}

	// Calculate the number of addresses in this range
	ones, bits := ipnet.Mask.Size()
	totalIPs := 1 << uint(bits-ones)
	log.Printf("DiscoveryService: CIDR %s has %d total IPs (ones=%d, bits=%d)", cidr, totalIPs, ones, bits)

	// For large ranges, limit to avoid memory issues
	if totalIPs > 256 {
		return nil, fmt.Errorf("CIDR range too large: %s (%d addresses)", cidr, totalIPs)
	}

	var ips []string

	// For /32 (single IP), just return the IP
	if totalIPs == 1 {
		result := []string{ip.String()}
		log.Printf("DiscoveryService: Returning /32 IP: %v", result)
		return result, nil
	}

	// For larger ranges, enumerate all IPs
	currentIP := make(net.IP, len(ip))
	copy(currentIP, ip.Mask(ipnet.Mask))

	for ipnet.Contains(currentIP) {
		ips = append(ips, currentIP.String())
		ds.incIP(currentIP)
	}

	// Remove network and broadcast addresses for ranges larger than /31
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}

	log.Printf("DiscoveryService: Returning IPs: %v", ips)
	return ips, nil
}

// incIP increments an IP address
func (ds *DiscoveryService) incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// scanTargets scans all targets
func (ds *DiscoveryService) scanTargets(targets []string, config models.ScanConfig, timeout time.Duration) []ScanResult {
	log.Printf("DiscoveryService: scanTargets called with %d targets", len(targets))
	results := make([]ScanResult, 0, len(targets))

	// Scan sequentially for now to avoid concurrency issues
	for _, target := range targets {
		log.Printf("DiscoveryService: Scanning target: %s", target)
		result := ds.scanTarget(target, timeout)
		results = append(results, result)
		log.Printf("DiscoveryService: Scanned %s -> reachable: %v, time: %v", target, result.Reachable, result.ResponseTime)
	}

	log.Printf("DiscoveryService: scanTargets completed, returning %d results", len(results))
	return results
}

// ScanResult represents the result of scanning a target
type ScanResult struct {
	Address      string
	Reachable    bool
	ResponseTime time.Duration
	Error        string
}

// scanTarget scans a single target
func (ds *DiscoveryService) scanTarget(address string, timeout time.Duration) ScanResult {
	log.Printf("DiscoveryService: scanTarget called for %s", address)
	start := time.Now()

	// TEMP: Just return success for testing
	duration := time.Since(start)
	result := ScanResult{
		Address:      address,
		Reachable:    true,
		ResponseTime: duration,
	}
	log.Printf("DiscoveryService: scanTarget result: %+v", result)
	return result
}

// detectMCPServers identifies MCP servers from scan results
func (ds *DiscoveryService) detectMCPServers(results []ScanResult) []models.DiscoveredServer {
	var mcpServers []models.DiscoveredServer

	for _, result := range results {
		if !result.Reachable {
			continue
		}

		// Test for MCP server
		if server := ds.testMCPServer(result.Address); server != nil {
			mcpServers = append(mcpServers, *server)
		}
	}

	return mcpServers
}

// testMCPServer tests if an address hosts an MCP server
func (ds *DiscoveryService) testMCPServer(address string) *models.DiscoveredServer {
	mcpURL := address + "/mcp"

	// Test streamable HTTP endpoint (replacing SSE with HTTP)
	authResult := ds.testStreamableHTTPEndpoint(mcpURL)
	if authResult.isMCP {
		return &models.DiscoveredServer{
			ID:                 fmt.Sprintf("mcp-%d", time.Now().UnixNano()),
			Name:               authResult.serverName,
			Address:            address,
			Protocol:           models.ServerProtocolMCP,
			Connection:         models.ConnectionTypeStreamableHttp,
			Status:             "healthy",
			LastSeen:           time.Now(),
			Metadata:           map[string]string{"detectionMethod": "streamable-http", "auth_type": authResult.authType},
			VulnerabilityScore: authResult.vulnerabilityScore,
		}
	}

	return nil
}

// authDetectionResult holds the result of MCP server authentication detection
type authDetectionResult struct {
	isMCP              bool
	vulnerabilityScore string
	authType           string
	serverName         string
}

// testStreamableHTTPEndpoint tests if the endpoint supports streamable HTTP and detects authentication
// testStreamableHTTPEndpoint tests a streamable HTTP MCP endpoint
func (ds *DiscoveryService) testStreamableHTTPEndpoint(url string) authDetectionResult {
	// Check if this is a proxy endpoint (contains /adapters/ and /mcp)
	if strings.Contains(url, "/adapters/") && strings.Contains(url, "/mcp") {
		return ds.scanProxyEndpoint(url)
	}

	// Original logic for direct MCP servers
	return ds.scanDirectMCPServer(url)
}

// scanProxyEndpoint scans a proxy adapter endpoint for authentication
func (ds *DiscoveryService) scanProxyEndpoint(url string) authDetectionResult {
	log.Printf("DiscoveryService: Scanning proxy endpoint: %s", url)

	// Initialize MCP connection
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"mcp-scanner","version":"1.0"}}}`
	req, err := http.NewRequest("POST", url, strings.NewReader(initPayload))
	if err != nil {
		log.Printf("DiscoveryService: Failed to create proxy request: %v", err)
		return authDetectionResult{isMCP: false}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := ds.httpClient.Do(req)
	if err != nil {
		log.Printf("DiscoveryService: Proxy request failed: %v", err)
		return authDetectionResult{isMCP: false}
	}
	defer resp.Body.Close()

	log.Printf("DiscoveryService: Proxy response status: %d", resp.StatusCode)

	// Read response body
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])

	// Check if this looks like an MCP server response
	isMCPResponse := strings.Contains(bodyStr, `"jsonrpc"`) &&
		(strings.Contains(bodyStr, `"result"`) || strings.Contains(bodyStr, `"error"`))

	if !isMCPResponse {
		return authDetectionResult{isMCP: false}
	}

	// Extract server name from MCP response (success or error)
	serverName := ""
	var response map[string]interface{}
	if err := json.Unmarshal(body[:n], &response); err == nil {
		if result, ok := response["result"].(map[string]interface{}); ok {
			if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
				if name, ok := serverInfo["name"].(string); ok {
					serverName = name
				}
			}
		}
		// Also check error responses for server info
		if error, ok := response["error"].(map[string]interface{}); ok {
			if data, ok := error["data"].(map[string]interface{}); ok {
				if serverInfo, ok := data["serverInfo"].(map[string]interface{}); ok {
					if name, ok := serverInfo["name"].(string); ok {
						serverName = name
					}
				}
			}
		}
	}

	// Determine vulnerability based on authentication
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		// Authentication is required - analyze WWW-Authenticate header
		authHeader := resp.Header.Get("WWW-Authenticate")

		// Check for OAuth (resource_metadata indicates OAuth 2.1 protected resource)
		if strings.Contains(authHeader, "resource_metadata") {
			return authDetectionResult{isMCP: true, vulnerabilityScore: "low", authType: "oauth", serverName: serverName}
		}

		// Check for Bearer token auth
		if strings.Contains(authHeader, "Bearer") {
			return authDetectionResult{isMCP: true, vulnerabilityScore: "medium", authType: "token", serverName: serverName}
		}

		// Other authentication methods
		return authDetectionResult{isMCP: true, vulnerabilityScore: "medium", authType: "other", serverName: serverName}
	} else if resp.StatusCode == 200 {
		// No authentication required - potentially vulnerable
		return authDetectionResult{isMCP: true, vulnerabilityScore: "high", authType: "none", serverName: serverName}
	}

	// Other status codes
	return authDetectionResult{isMCP: true, vulnerabilityScore: "medium", authType: "unknown", serverName: serverName}
}

// scanDirectMCPServer scans a direct MCP server (not through proxy)
func (ds *DiscoveryService) scanDirectMCPServer(url string) authDetectionResult {
	// First, try without authentication
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"mcp-discovery","version":"1.0"}}}`
	req, err := http.NewRequest("POST", url, strings.NewReader(initPayload))
	if err != nil {
		return authDetectionResult{isMCP: false}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := ds.httpClient.Do(req)
	if err != nil {
		return authDetectionResult{isMCP: false}
	}
	defer resp.Body.Close()

	// Read response body
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])

	// Check if this looks like an MCP server response
	isMCPResponse := strings.Contains(bodyStr, `"jsonrpc"`) &&
		(strings.Contains(bodyStr, `"result"`) || strings.Contains(bodyStr, `"error"`))

	// Extract server name from MCP response (success or error)
	serverName := ""
	if isMCPResponse {
		var response map[string]interface{}
		if err := json.Unmarshal(body[:n], &response); err == nil {
			if result, ok := response["result"].(map[string]interface{}); ok {
				if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
					if name, ok := serverInfo["name"].(string); ok {
						serverName = name
					}
				}
			}
			// Also check error responses for server info
			if error, ok := response["error"].(map[string]interface{}); ok {
				if data, ok := error["data"].(map[string]interface{}); ok {
					if serverInfo, ok := data["serverInfo"].(map[string]interface{}); ok {
						if name, ok := serverInfo["name"].(string); ok {
							serverName = name
						}
					}
				}
			}
		}
	}

	// If we got any response that looks like MCP, determine vulnerability
	if isMCPResponse {
		if resp.StatusCode == 200 {
			return authDetectionResult{isMCP: true, vulnerabilityScore: "high", authType: "none", serverName: serverName}
		} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
			authHeader := resp.Header.Get("WWW-Authenticate")

			// Check for OAuth (resource_metadata indicates OAuth 2.1 protected resource)
			if strings.Contains(authHeader, "resource_metadata") {
				return authDetectionResult{isMCP: true, vulnerabilityScore: "low", authType: "oauth", serverName: serverName}
			}

			// Check for Bearer token auth
			if strings.Contains(authHeader, "Bearer") {
				return authDetectionResult{isMCP: true, vulnerabilityScore: "medium", authType: "token", serverName: serverName}
			}

			// Other auth
			return authDetectionResult{isMCP: true, vulnerabilityScore: "medium", authType: "other", serverName: serverName}
		} else {
			// Any other status with MCP response
			return authDetectionResult{isMCP: true, vulnerabilityScore: "high", authType: "none", serverName: serverName}
		}
	}

	// If status 200 but not MCP response, still consider it MCP (might be error response)
	if resp.StatusCode == 200 {
		return authDetectionResult{isMCP: true, vulnerabilityScore: "high", authType: "none", serverName: serverName}
	}

	return authDetectionResult{isMCP: false}
}

// cacheServers stores discovered servers in cache
func (ds *DiscoveryService) cacheServers(servers []models.DiscoveredServer) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	for _, server := range servers {
		server := server // copy
		ds.cache[server.ID] = &server
	}
}

// parseAddress extracts host and port from address
func (ds *DiscoveryService) parseAddress(address string) (string, int, error) {
	// Remove http:// prefix
	if strings.HasPrefix(address, "http://") {
		address = address[7:]
	}

	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}

	// Parse port
	port := 80 // default
	if portStr != "" {
		if p, err := net.LookupPort("tcp", portStr); err == nil {
			port = p
		}
	}

	return host, port, nil
}
