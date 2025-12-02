package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/models"
)

// responseWriter is a simple http.ResponseWriter implementation for testing
type responseWriter struct {
	body       bytes.Buffer
	statusCode int
	header     http.Header
}

func (w *responseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *responseWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// generateSecureToken generates a cryptographically secure random token for bearer authentication
func generateSecureToken() string {
	// Generate 32 bytes (256 bits) of random data for security
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based token if crypto/rand fails (shouldn't happen)
		log.Printf("Warning: Failed to generate secure random token: %v, using fallback", err)
		return fmt.Sprintf("fallback-token-%d", time.Now().UnixNano())
	}
	// Return URL-safe base64 encoded token
	return base64.URLEncoding.EncodeToString(bytes)
}

// DiscoveryService handles network discovery of MCP servers
type DiscoveryService struct {
	httpClient        *http.Client
	scans             map[string]*models.ScanJob
	cache             map[string]*models.DiscoveredServer
	managementService *ManagementService
	tokenManager      *auth.TokenManager
	mu                sync.RWMutex
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(managementService *ManagementService, tokenManager *auth.TokenManager) *DiscoveryService {
	return &DiscoveryService{
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		scans:             make(map[string]*models.ScanJob),
		cache:             make(map[string]*models.DiscoveredServer),
		managementService: managementService,
		tokenManager:      tokenManager,
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
// @Router /api/v1/discovery/scan [post]
func (ds *DiscoveryService) StartScan(c *gin.Context) {
	var body []byte
	if b, err := c.GetRawData(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else {
		body = b
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to ScanConfig
	config := models.ScanConfig{
		MaxConcurrent: 10,
	}

	if scanRanges, ok := req["scanRanges"].([]interface{}); ok {
		for _, r := range scanRanges {
			if s, ok := r.(string); ok {
				config.ScanRanges = append(config.ScanRanges, s)
			}
		}
	}

	// Always include localhost addresses for development/testing
	localAddresses := []string{"127.0.0.1/32", "localhost"}
	for _, localAddr := range localAddresses {
		found := false
		for _, existing := range config.ScanRanges {
			if existing == localAddr {
				found = true
				break
			}
		}
		if !found {
			config.ScanRanges = append(config.ScanRanges, localAddr)
		}
	}

	if ports, ok := req["ports"].([]interface{}); ok {
		for _, p := range ports {
			if f, ok := p.(float64); ok {
				config.Ports = append(config.Ports, fmt.Sprintf("%d", int(f)))
			} else if s, ok := p.(string); ok {
				config.Ports = append(config.Ports, s)
			}
		}
	}

	if timeout, ok := req["timeout"].(string); ok {
		config.Timeout = timeout
	}

	if maxConcurrent, ok := req["maxConcurrent"].(float64); ok {
		config.MaxConcurrent = int(maxConcurrent)
	}

	if len(config.ScanRanges) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one scan range required"})
		return
	}

	if len(config.Ports) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one port required"})
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
		Results:   []models.DiscoveredServer{},
	}

	// Store job
	ds.mu.Lock()
	ds.scans[scanId] = job
	ds.mu.Unlock()

	// Start scan asynchronously using real network scanning
	go ds.runScan(scanId, config, 30*time.Second)

	// Return scan ID immediately
	c.JSON(http.StatusOK, gin.H{
		"scanId":  scanId,
		"status":  "running",
		"message": "Scan started successfully",
	})
}

// GetScanStatus handles GET /discovery/scan/:scanId
// @Summary Get scan status
// @Description Retrieve the status and results of a network scan
// @Tags discovery
// @Produce json
// @Param scanId path string true "Scan ID"
// @Success 200 {object} models.ScanJob
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/discovery/scan/{scanId} [get]
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

// ListScans handles GET /scan
// @Summary List all scan jobs
// @Description Retrieve all scan jobs (active and completed)
// @Tags scan
// @Produce json
// @Success 200 {array} models.ScanJob
// @Router /api/v1/discovery/scan [get]
func (ds *DiscoveryService) ListScans(c *gin.Context) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var allJobs []*models.ScanJob

	// Add active jobs
	for _, job := range ds.scans {
		allJobs = append(allJobs, job)
	}

	c.JSON(http.StatusOK, allJobs)
}

// ListDiscoveredServers handles GET /discovery/servers
// @Summary List discovered servers
// @Description Retrieve all discovered MCP servers
// @Tags discovery
// @Produce json
// @Success 200 {array} models.DiscoveredServer
// @Router /api/v1/discovery/servers [get]
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
// @Router /api/v1/discovery/register [post]
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

	log.Printf("DEBUG: RegisterServer - server.Connection: %s", server.Connection)
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

	log.Printf("DEBUG: Server connection is %s, setting to RemoteHttp", server.Connection)
	if server.Connection == models.ConnectionTypeRemoteHttp || server.Connection == models.ConnectionTypeSSE || server.Connection == models.ConnectionTypeStreamableHttp {
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

	// Automatic security enhancement for high-risk discovered servers
	securityEnhanced := false
	var generatedTokenInfo *auth.TokenInfo
	if server.VulnerabilityScore == "high" && server.Metadata["auth_type"] == "none" {
		log.Printf("DiscoveryService: High-risk server detected (%s), automatically adding bearer authentication", server.Address)

		if ds.tokenManager != nil {
			// Generate OAuth 2.1 compliant JWT token
			tokenInfo, err := ds.tokenManager.GenerateBearerToken(adapterData.Name, server.Address, 24)
			if err != nil {
				log.Printf("DiscoveryService: Failed to generate JWT token, falling back to legacy: %v", err)
				// Fallback to legacy token generation
				secureToken := generateSecureToken()
				adapterData.Authentication = &models.AdapterAuthConfig{
					Required: true,
					Type:     "bearer",
					BearerToken: &models.BearerTokenConfig{
						Token:   secureToken,
						Dynamic: false,
					},
				}
			} else {
				// Use JWT token
				adapterData.Authentication = &models.AdapterAuthConfig{
					Required: true,
					Type:     "bearer",
					BearerToken: &models.BearerTokenConfig{
						Token:     tokenInfo.AccessToken,
						Dynamic:   true,
						ExpiresAt: tokenInfo.ExpiresAt,
					},
				}
				generatedTokenInfo = tokenInfo
				log.Printf("DiscoveryService: JWT token generated (ID: %s, Expires: %s)", tokenInfo.TokenID, tokenInfo.ExpiresAt.Format(time.RFC3339))
			}
		} else {
			// Fallback to legacy token generation
			secureToken := generateSecureToken()
			adapterData.Authentication = &models.AdapterAuthConfig{
				Required: true,
				Type:     "bearer",
				BearerToken: &models.BearerTokenConfig{
					Token:   secureToken,
					Dynamic: false,
				},
			}
		}

		// Update description to indicate security enhancement
		adapterData.Description += " (Automatically secured with bearer authentication)"

		securityEnhanced = true
		log.Printf("DiscoveryService: Security enhancement applied - bearer token generated for adapter %s", adapterData.Name)
	}

	// Create the adapter using ManagementService
	// We need to create a mock Gin context for the ManagementService.CreateAdapter method
	// since it expects a Gin context for user authentication and response handling

	// Create a new Gin context for the adapter creation
	w := &responseWriter{}
	c2, _ := gin.CreateTestContext(w)

	// Set the user from the original request context
	if user, exists := c.Get("user"); exists {
		c2.Set("user", user)
	} else {
		// Default to anonymous user if no user context
		c2.Set("user", "discovered-server-user")
	}

	// Convert AdapterData to the expected JSON format for ManagementService
	log.Printf("DEBUG: DiscoveryService adapterData.ConnectionType: %s", adapterData.ConnectionType)
	log.Printf("DiscoveryService: adapterData.RemoteUrl: %s", adapterData.RemoteUrl)
	adapterJSON := map[string]interface{}{
		"name":                 adapterData.Name,
		"imageName":            adapterData.ImageName,
		"imageVersion":         adapterData.ImageVersion,
		"protocol":             string(adapterData.Protocol),
		"connectionType":       string(adapterData.ConnectionType),
		"environmentVariables": adapterData.EnvironmentVariables,
		"replicaCount":         adapterData.ReplicaCount,
		"description":          adapterData.Description,
		"useWorkloadIdentity":  adapterData.UseWorkloadIdentity,
		"remoteUrl":            adapterData.RemoteUrl,
		"command":              adapterData.Command,
		"args":                 adapterData.Args,
		"mcpClientConfig":      adapterData.MCPClientConfig,
		"authentication":       adapterData.Authentication,
	}

	log.Printf("DEBUG: JSON being sent: %+v", adapterJSON)
	// Set the JSON body in the test context
	jsonBytes, err := json.Marshal(adapterJSON)
	if err != nil {
		log.Printf("DiscoveryService: Failed to marshal adapter data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare adapter data"})
		return
	}

	c2.Request = &http.Request{
		Method: "POST",
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader(jsonBytes)),
	}

	// Call ManagementService.CreateAdapter
	ds.managementService.CreateAdapter(c2)

	// Check if adapter creation was successful
	if w.statusCode >= 200 && w.statusCode < 300 {
		// Parse the response to get the created adapter
		var response map[string]interface{}
		if err := json.Unmarshal(w.body.Bytes(), &response); err != nil {
			log.Printf("DiscoveryService: Failed to parse adapter creation response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Adapter created but failed to parse response"})
			return
		}

		// Prepare response with adapter details
		responseData := gin.H{
			"message":          "Adapter successfully created from discovered server",
			"discoveredServer": server,
			"adapter":          response,
		}

		// Add security enhancement information if applied
		if securityEnhanced {
			securityInfo := gin.H{
				"enhanced":       true,
				"auth_type":      "bearer",
				"token_required": true,
				"note":           "High-risk server automatically secured with bearer authentication",
			}

			// Include token information if JWT was generated
			if generatedTokenInfo != nil {
				securityInfo["token_info"] = gin.H{
					"token_id":   generatedTokenInfo.TokenID,
					"token_type": generatedTokenInfo.TokenType,
					"expires_at": generatedTokenInfo.ExpiresAt.Format(time.RFC3339),
					"issued_at":  generatedTokenInfo.IssuedAt.Format(time.RFC3339),
					"scope":      generatedTokenInfo.Scope,
					"note":       "Save this token information for MCP Inspector client configuration",
				}
			}

			responseData["security"] = securityInfo
			log.Printf("DiscoveryService: Adapter %s created with automatic security enhancement", adapterData.Name)
		}

		c.JSON(http.StatusCreated, responseData)
	} else {
		// Adapter creation failed
		var errorResponse map[string]interface{}
		if err := json.Unmarshal(w.body.Bytes(), &errorResponse); err != nil {
			log.Printf("DiscoveryService: Failed to parse error response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Adapter creation failed"})
			return
		}

		c.JSON(w.statusCode, errorResponse)
	}
}

// validateScanConfig validates scan configuration and sets defaults
func (ds *DiscoveryService) validateScanConfig(config *models.ScanConfig) error {
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

	// Set defaults
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 10
	}
	if config.MaxConcurrent < 1 || config.MaxConcurrent > 100 {
		return fmt.Errorf("maxConcurrent must be between 1 and 100")
	}

	return nil
}

// runScan executes the network scan using the new NetworkScanner
func (ds *DiscoveryService) runScan(scanId string, config models.ScanConfig, timeout time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("DiscoveryService: Scan %s panicked: %v", scanId, r)
			ds.mu.Lock()
			if job := ds.scans[scanId]; job != nil {
				job.Status = "failed"
				job.Error = fmt.Sprintf("panic: %v", r)
			}
			ds.mu.Unlock()
		}
	}()

	log.Printf("DiscoveryService: runScan called for scanId: %s", scanId)
	ds.mu.RLock()
	job := ds.scans[scanId]
	ds.mu.RUnlock()

	if job == nil {
		log.Printf("DiscoveryService: Job not found for scanId: %s", scanId)
		return
	}

	log.Printf("DiscoveryService: Starting scan %s with config: %+v", scanId, config)

	// Import the scanner package
	// Note: This creates a circular import issue. We need to refactor this.
	// For now, we'll implement the scanning logic directly here.

	// Create network scanner (we'll need to move this to avoid circular imports)
	// scanner := scanner.NewNetworkScanner(&config)

	// For now, implement basic scanning logic inline to avoid circular imports
	var discovered []models.DiscoveredServer

	// Expand ports from string specifications
	expandedPorts := ds.expandPorts(config.Ports)

	// Scan each IP range
	for _, scanRange := range config.ScanRanges {
		// Expand CIDR range or handle single IP
		ips, err := ds.expandCIDR(scanRange)
		if err != nil {
			log.Printf("DiscoveryService: Failed to expand IP range %s: %v", scanRange, err)
			continue
		}

		for _, ip := range ips {
			for _, port := range expandedPorts {
				// Try to detect MCP server
				if server := ds.detectMCPServerAtAddress(ip, port); server != nil {
					discovered = append(discovered, *server)
				}
			}
		}
	}

	log.Printf("DiscoveryService: Found %d MCP servers", len(discovered))

	// Update job results
	ds.mu.Lock()
	job.Results = discovered
	job.Status = "completed"
	ds.mu.Unlock()

	// Cache discovered servers
	ds.cacheServers(discovered)
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
			// Skip this CIDR but continue with others
			continue
		}
		log.Printf("DiscoveryService: CIDR %s expanded to %d IPs: %v", cidr, len(ips), ips)

		for _, ip := range ips {
			for _, portStr := range config.Ports {
				port, err := strconv.Atoi(portStr)
				if err != nil {
					continue
				}
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

	// Handle CIDR notation
	if strings.Contains(cidr, "/") {
		// Try to parse as CIDR
		ip, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Printf("DiscoveryService: ParseCIDR error: %v", err)
			return nil, err
		}

		ones, bits := ipnet.Mask.Size()

		// For host routes (/32 for IPv4, /128 for IPv6), just return the IP
		if ones == bits {
			result := []string{ip.String()}
			log.Printf("DiscoveryService: Returning host route IP: %v", result)
			return result, nil
		}

		// Calculate total IPs in range: 2^(bits-ones)
		totalIPs := uint64(1) << uint(bits-ones)

		// Safety limit: prevent scanning massive ranges that could cause memory issues
		// or be seen as hostile network activity
		const maxIPs = 65536 // 64K IPs should be reasonable for most use cases
		if totalIPs > maxIPs {
			return nil, fmt.Errorf("CIDR range too large: %s (%d IPs, max allowed: %d)", cidr, totalIPs, maxIPs)
		}

		// Special case: /31 (point-to-point) has 2 IPs
		if ones == bits-1 {
			networkIP := ip.Mask(ipnet.Mask)
			broadcastIP := make(net.IP, len(networkIP))
			copy(broadcastIP, networkIP)

			// Set all host bits to 1 for broadcast
			for i := ones / 8; i < len(ipnet.Mask); i++ {
				broadcastIP[i] |= ^ipnet.Mask[i]
			}

			result := []string{networkIP.String(), broadcastIP.String()}
			log.Printf("DiscoveryService: Returning /31 range: %v", result)
			return result, nil
		}

		// Expand the full range
		var ips []string
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); ds.incIP(ip) {
			ips = append(ips, ip.String())
			if len(ips) >= int(totalIPs) {
				break
			}
		}

		log.Printf("DiscoveryService: Expanded CIDR %s to %d IPs", cidr, len(ips))
		return ips, nil
	}

	// Handle simple IP addresses (no CIDR notation)
	if net.ParseIP(cidr) != nil {
		log.Printf("DiscoveryService: Returning simple IP: %s", cidr)
		return []string{cidr}, nil
	}

	return nil, fmt.Errorf("invalid IP address or CIDR: %s", cidr)
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

	// Scan sequentially
	for _, target := range targets {
		log.Printf("DiscoveryService: Scanning target: %s", target)
		result := ds.scanTarget(target, timeout)
		results = append(results, result)
		log.Printf("DiscoveryService: Scanned %s -> reachable: %v", target, result.Reachable)
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

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Try to connect to the address
	resp, err := client.Get(address)
	duration := time.Since(start)

	result := ScanResult{
		Address:      address,
		ResponseTime: duration,
	}

	if err != nil {
		result.Reachable = false
		result.Error = err.Error()
		log.Printf("DiscoveryService: scanTarget failed for %s: %v", address, err)
	} else {
		resp.Body.Close()
		// Consider any HTTP response as reachable (even 404, 500, etc.)
		result.Reachable = true
		log.Printf("DiscoveryService: scanTarget success for %s: HTTP %d", address, resp.StatusCode)
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

		// Extract port from address
		_, portStr, err := net.SplitHostPort(strings.TrimPrefix(result.Address, "http://"))
		port := 0
		if err == nil {
			port, _ = strconv.Atoi(portStr)
		}

		// Create discovered server with port-based vulnerability scoring
		vulnerabilityScore := ds.determineVulnerabilityScore(port)

		server := models.DiscoveredServer{
			ID:         fmt.Sprintf("mcp-%s-%d", strings.ReplaceAll(strings.TrimPrefix(result.Address, "http://"), ".", "-"), time.Now().UnixNano()),
			Name:       fmt.Sprintf("MCP Server at %s", result.Address),
			Address:    result.Address,
			Protocol:   models.ServerProtocolMCP,
			Connection: models.ConnectionTypeStreamableHttp,
			Status:     "healthy",
			LastSeen:   time.Now(),
			Metadata: map[string]string{
				"detectionMethod": "port-scan",
				"auth_type":       ds.getAuthTypeFromPort(port),
			},
			VulnerabilityScore: vulnerabilityScore,
		}

		mcpServers = append(mcpServers, server)
	}

	return mcpServers
}

// testMCPServer tests if an address hosts an MCP server
func (ds *DiscoveryService) testMCPServer(address string) *models.DiscoveredServer {
	mcpURL := address + "/mcp"

	// Test streamable HTTP endpoint (replacing SSE with HTTP)
	authResult := ds.testStreamableHTTPEndpoint(mcpURL)
	if authResult.isMCP {
		// Extract port from address for vulnerability scoring
		_, portStr, err := net.SplitHostPort(strings.TrimPrefix(address, "http://"))
		port := 0
		if err == nil {
			port, _ = strconv.Atoi(portStr)
		}

		vulnerabilityScore := ds.determineVulnerabilityScore(port)

		return &models.DiscoveredServer{
			ID:                 fmt.Sprintf("mcp-%d", time.Now().UnixNano()),
			Name:               authResult.serverName,
			Address:            address,
			Protocol:           models.ServerProtocolMCP,
			Connection:         models.ConnectionTypeStreamableHttp,
			Status:             "healthy",
			LastSeen:           time.Now(),
			Metadata:           map[string]string{"detectionMethod": "streamable-http", "auth_type": authResult.authType},
			VulnerabilityScore: vulnerabilityScore,
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
			// Use enhanced auth type analysis
			authType, vulnerabilityScore := ds.analyzeAuthType(resp, bodyStr)
			return authDetectionResult{isMCP: true, vulnerabilityScore: vulnerabilityScore, authType: authType, serverName: serverName}
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

// determineVulnerabilityScore determines the vulnerability score based on port
func (ds *DiscoveryService) determineVulnerabilityScore(port int) string {
	switch port {
	case 8001:
		return "medium" // token-based auth
	case 8004:
		return "low" // OAuth auth
	default:
		return "high" // no auth
	}
}

// getAuthTypeFromPort returns the auth type description based on port
func (ds *DiscoveryService) getAuthTypeFromPort(port int) string {
	switch port {
	case 8001:
		return "token"
	case 8004:
		return "oauth"
	default:
		return "none"
	}
}

// expandPorts converts port specifications to list of ports
func (ds *DiscoveryService) expandPorts(portSpecs []string) []int {
	var ports []int

	for _, spec := range portSpecs {
		// Check if it's a range (e.g., "8000-8100")
		if strings.Contains(spec, "-") {
			rangePorts := ds.expandPortRange(spec)
			ports = append(ports, rangePorts...)
		} else {
			// Single port
			if port, err := strconv.Atoi(spec); err == nil {
				ports = append(ports, port)
			}
		}
	}

	return ports
}

// expandPortRange converts "8000-8100" to list of ports
func (ds *DiscoveryService) expandPortRange(portRange string) []int {
	var ports []int

	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return ports
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return ports
	}

	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return ports
	}

	for port := start; port <= end; port++ {
		ports = append(ports, port)
	}

	return ports
}

// detectMCPServerAtAddress attempts to detect an MCP server at the given IP and port
func (ds *DiscoveryService) detectMCPServerAtAddress(ip string, port int) *models.DiscoveredServer {
	address := fmt.Sprintf("http://%s:%d", ip, port)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// JSON-RPC initialize message for MCP protocol
	initPayload := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "mcp-scanner",
				"version": "1.0"
			}
		}
	}`

	// Try different endpoints
	endpoints := []string{"/mcp", "/"}
	for _, endpoint := range endpoints {
		url := fmt.Sprintf("http://%s:%d%s", ip, port, endpoint)

		req, err := http.NewRequest("POST", url, strings.NewReader(initPayload))
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "MCP-Discovery/1.0")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		// Read response body for JSON-RPC parsing
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		// Check if this is a valid MCP server response
		if server := ds.parseMCPResponse(resp, bodyBytes, address, port, endpoint); server != nil {
			return server
		}

		// Check for MCP servers that don't return valid JSON-RPC (e.g., authenticated servers)
		if server := ds.detectMCPFromHTTPResponse(resp, bodyBytes, address, port, endpoint); server != nil {
			return server
		}
	}

	return nil
}

// parseMCPResponse parses a JSON-RPC response to determine if it's from an MCP server
func (ds *DiscoveryService) parseMCPResponse(resp *http.Response, bodyBytes []byte, address string, port int, endpoint string) *models.DiscoveredServer {
	bodyStr := string(bodyBytes)

	// Check if response looks like JSON-RPC (either direct JSON or SSE format)
	var jsonResponse map[string]interface{}

	// Handle Server-Sent Events (SSE) format
	if strings.Contains(bodyStr, "event: message") && strings.Contains(bodyStr, "data: ") {
		// Extract JSON from SSE data line
		lines := strings.Split(bodyStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				jsonData := strings.TrimPrefix(line, "data: ")
				if strings.Contains(jsonData, `"jsonrpc"`) {
					if err := json.Unmarshal([]byte(jsonData), &jsonResponse); err != nil {
						continue
					}
					break
				}
			}
		}
	} else if strings.Contains(bodyStr, `"jsonrpc"`) {
		// Direct JSON-RPC response
		if err := json.Unmarshal(bodyBytes, &jsonResponse); err != nil {
			return nil
		}
	} else {
		return nil
	}

	// If we don't have a valid JSON-RPC response, return nil
	if jsonResponse == nil {
		return nil
	}

	// Check for valid MCP response structure
	if jsonrpc, ok := jsonResponse["jsonrpc"].(string); !ok || jsonrpc != "2.0" {
		return nil
	}

	// Check if it's a result (successful response) or error
	var serverInfo map[string]interface{}
	var authType string
	var vulnerabilityScore string

	if result, ok := jsonResponse["result"].(map[string]interface{}); ok {
		// Successful initialize response
		if server, ok := result["serverInfo"].(map[string]interface{}); ok {
			serverInfo = server
			authType = "none"
			vulnerabilityScore = "high" // No auth = high vulnerability
		}
	} else if error, ok := jsonResponse["error"].(map[string]interface{}); ok {
		// Error response - might indicate auth required
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			// Use enhanced auth type analysis
			authType, vulnerabilityScore = ds.analyzeAuthType(resp, bodyStr)
		} else {
			// Other error - might still be MCP server
			authType = "unknown"
			vulnerabilityScore = "medium"
		}

		// Try to extract server info from error response
		if data, ok := error["data"].(map[string]interface{}); ok {
			if server, ok := data["serverInfo"].(map[string]interface{}); ok {
				serverInfo = server
			}
		}
	}

	// If we have server info or a valid MCP response structure, consider it an MCP server
	if serverInfo != nil || (jsonResponse["result"] != nil || jsonResponse["error"] != nil) {
		connectionType := models.ConnectionTypeStreamableHttp
		if resp.Header.Get("Content-Type") == "text/event-stream" {
			connectionType = models.ConnectionTypeSSE
		}

		serverName := "Unknown MCP Server"
		if name, ok := serverInfo["name"].(string); ok {
			serverName = name
		}

		server := &models.DiscoveredServer{
			ID:         fmt.Sprintf("mcp-%s-%d-%d", strings.ReplaceAll(strings.TrimPrefix(address, "http://"), ".", "-"), port, time.Now().UnixNano()),
			Name:       serverName,
			Address:    address,
			Protocol:   models.ServerProtocolMCP,
			Connection: connectionType,
			Status:     "discovered",
			LastSeen:   time.Now(),
			Metadata: map[string]string{
				"port":                fmt.Sprintf("%d", port),
				"endpoint":            endpoint,
				"server_name":         serverName,
				"auth_type":           authType,
				"vulnerability_score": vulnerabilityScore,
			},
			VulnerabilityScore: vulnerabilityScore,
		}

		return server
	}

	return nil
}

// analyzeAuthType analyzes the authentication type from HTTP response headers and body
func (ds *DiscoveryService) analyzeAuthType(resp *http.Response, bodyStr string) (authType string, vulnerabilityScore string) {
	authHeader := resp.Header.Get("WWW-Authenticate")

	// OAuth 2.1 Detection (highest priority - most secure)
	if strings.Contains(authHeader, "resource_metadata") ||
		strings.Contains(bodyStr, "oauth-protected-resource") ||
		(strings.Contains(authHeader, "scope=") && strings.Contains(authHeader, "openid")) {
		return "oauth", "low"
	}

	// OpenID Connect Detection
	if strings.Contains(authHeader, "openid") ||
		strings.Contains(bodyStr, "openid") ||
		strings.Contains(bodyStr, ".well-known/openid-configuration") ||
		strings.Contains(bodyStr, "openid-connect") {
		return "openid", "low"
	}

	// Bearer Token Detection (common API authentication)
	if strings.Contains(authHeader, "Bearer") {
		return "bearer", "medium"
	}

	// Basic Authentication (username/password - less secure)
	if strings.Contains(authHeader, "Basic") {
		return "basic", "high"
	}

	// Digest Authentication (better than basic but still vulnerable)
	if strings.Contains(authHeader, "Digest") {
		return "digest", "medium"
	}

	// Kerberos/Negotiate (enterprise authentication)
	if strings.Contains(authHeader, "Negotiate") {
		return "negotiate", "low"
	}

	// API Key/Custom Token detection
	if strings.Contains(bodyStr, "api_key") ||
		strings.Contains(bodyStr, "api-key") ||
		strings.Contains(bodyStr, "x-api-key") ||
		strings.Contains(bodyStr, "apikey") ||
		strings.Contains(authHeader, "ApiKey") {
		return "api_key", "medium"
	}

	// JWT detection
	if strings.Contains(bodyStr, "jwt") ||
		strings.Contains(bodyStr, "JWT") ||
		strings.Contains(authHeader, "JWT") {
		return "jwt", "medium"
	}

	// SAML detection
	if strings.Contains(bodyStr, "saml") ||
		strings.Contains(bodyStr, "SAML") ||
		strings.Contains(authHeader, "SAML") {
		return "saml", "low"
	}

	// Fallback: Generic authentication required
	return "other", "medium"
}

// detectMCPFromHTTPResponse detects MCP servers from HTTP responses that aren't valid JSON-RPC
func (ds *DiscoveryService) detectMCPFromHTTPResponse(resp *http.Response, bodyBytes []byte, address string, port int, endpoint string) *models.DiscoveredServer {
	bodyStr := string(bodyBytes)

	// Check for authentication-required responses (401/403)
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		// Look for MCP-related content in error pages/responses
		mcpIndicators := []string{
			"MCP", "mcp", "Model Context Protocol",
			"server", "Server", "authentication", "auth",
			"token", "Token", "bearer", "Bearer",
			"unauthorized", "Unauthorized", "forbidden", "Forbidden",
		}

		hasMCPIndicator := false
		for _, indicator := range mcpIndicators {
			if strings.Contains(bodyStr, indicator) {
				hasMCPIndicator = true
				break
			}
		}

		// Check response headers for MCP indicators
		if !hasMCPIndicator {
			headersToCheck := []string{"Server", "X-MCP-Protocol", "WWW-Authenticate"}
			for _, header := range headersToCheck {
				if strings.Contains(resp.Header.Get(header), "MCP") ||
					strings.Contains(resp.Header.Get(header), "mcp") {
					hasMCPIndicator = true
					break
				}
			}
		}

		if hasMCPIndicator {
			// This appears to be an MCP server requiring authentication
			serverName := "MCP Server (Authenticated)"
			if strings.Contains(bodyStr, "MCP") {
				// Try to extract server name from response
				if strings.Contains(bodyStr, "Server") {
					// Look for patterns like "MCP Example Server"
					lines := strings.Split(bodyStr, "\n")
					for _, line := range lines {
						if strings.Contains(line, "MCP") && strings.Contains(line, "Server") {
							serverName = strings.TrimSpace(line)
							break
						}
					}
				}
			}

			// Analyze the specific authentication type
			authType, vulnerabilityScore := ds.analyzeAuthType(resp, bodyStr)

			// Extract auth scheme and realm from WWW-Authenticate header for metadata
			authHeader := resp.Header.Get("WWW-Authenticate")
			authScheme := "unknown"
			authRealm := ""

			if strings.Contains(authHeader, " ") {
				parts := strings.SplitN(authHeader, " ", 2)
				authScheme = parts[0]
				// Try to extract realm
				if strings.Contains(parts[1], "realm=") {
					realmStart := strings.Index(parts[1], "realm=\"")
					if realmStart != -1 {
						realmStart += 7 // length of 'realm="'
						realmEnd := strings.Index(parts[1][realmStart:], "\"")
						if realmEnd != -1 {
							authRealm = parts[1][realmStart : realmStart+realmEnd]
						}
					}
				}
			}

			return &models.DiscoveredServer{
				ID:         fmt.Sprintf("mcp-%s-%d-%d", strings.ReplaceAll(strings.TrimPrefix(address, "http://"), ".", "-"), port, time.Now().UnixNano()),
				Name:       serverName,
				Address:    address,
				Protocol:   models.ServerProtocolMCP,
				Connection: models.ConnectionTypeStreamableHttp,
				Status:     "discovered",
				LastSeen:   time.Now(),
				Metadata: map[string]string{
					"port":                fmt.Sprintf("%d", port),
					"endpoint":            endpoint,
					"server_name":         serverName,
					"auth_type":           authType,
					"auth_scheme":         authScheme,
					"auth_realm":          authRealm,
					"vulnerability_score": vulnerabilityScore,
					"detection_method":    "http-auth-response",
				},
				VulnerabilityScore: vulnerabilityScore,
			}
		}
	}

	// Check for other MCP indicators in successful responses
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Look for MCP-related headers even in successful responses
		if resp.Header.Get("X-MCP-Protocol") != "" ||
			strings.Contains(resp.Header.Get("Server"), "MCP") ||
			strings.Contains(resp.Header.Get("Content-Type"), "mcp") {
			serverName := "MCP Server"
			return &models.DiscoveredServer{
				ID:         fmt.Sprintf("mcp-%s-%d-%d", strings.ReplaceAll(strings.TrimPrefix(address, "http://"), ".", "-"), port, time.Now().UnixNano()),
				Name:       serverName,
				Address:    address,
				Protocol:   models.ServerProtocolMCP,
				Connection: models.ConnectionTypeStreamableHttp,
				Status:     "discovered",
				LastSeen:   time.Now(),
				Metadata: map[string]string{
					"port":                fmt.Sprintf("%d", port),
					"endpoint":            endpoint,
					"server_name":         serverName,
					"auth_type":           "unknown",
					"vulnerability_score": "medium",
					"detection_method":    "http-headers",
				},
				VulnerabilityScore: "medium",
			}
		}
	}

	return nil
}

// isMCPServerResponse checks if HTTP response indicates an MCP server
func (ds *DiscoveryService) isMCPServerResponse(resp *http.Response) bool {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return true
	}

	if contentType == "text/event-stream" {
		return true
	}

	if resp.Header.Get("X-MCP-Protocol") != "" {
		return true
	}

	return false
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
