package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
)

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// NetworkScanner performs network scanning to discover MCP servers
type NetworkScanner struct {
	config            *models.ScanConfig
	results           chan models.DiscoveredServer
	errors            chan error
	wg                sync.WaitGroup
	ctx               context.Context
	cancel            context.CancelFunc
	discoveredServers []models.DiscoveredServer // Store discovered servers for retrieval
	mu                sync.RWMutex              // Protect discoveredServers slice
	scanCache         *ScanCache                // Cache for incremental scanning
}

// ScanCache tracks recently scanned addresses to enable incremental scanning
type ScanCache struct {
	scannedAddresses map[string]time.Time
	mutex            sync.RWMutex
	maxAge           time.Duration
}

// NewScanCache creates a new scan cache
func NewScanCache(maxAge time.Duration) *ScanCache {
	return &ScanCache{
		scannedAddresses: make(map[string]time.Time),
		maxAge:           maxAge,
	}
}

// IsRecentlyScanned checks if an address was scanned recently
func (sc *ScanCache) IsRecentlyScanned(address string) bool {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	lastScanned, exists := sc.scannedAddresses[address]
	if !exists {
		return false
	}

	return time.Since(lastScanned) < sc.maxAge
}

// MarkScanned marks an address as scanned
func (sc *ScanCache) MarkScanned(address string) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	sc.scannedAddresses[address] = time.Now()
}

// Cleanup removes old entries from the cache
func (sc *ScanCache) Cleanup() {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	cutoff := time.Now().Add(-sc.maxAge)
	for address, timestamp := range sc.scannedAddresses {
		if timestamp.Before(cutoff) {
			delete(sc.scannedAddresses, address)
		}
	}
}

// MCPDetectionConfig holds configuration for MCP server detection
type MCPDetectionConfig struct {
	Endpoints     []string
	Methods       []string
	UserAgent     string
	CustomHeaders map[string]string
	Timeout       time.Duration
}

// NewNetworkScanner creates a new network scanner
func NewNetworkScanner(config *models.ScanConfig) *NetworkScanner {
	ctx, cancel := context.WithCancel(context.Background())
	return &NetworkScanner{
		config:            config,
		results:           make(chan models.DiscoveredServer, 100),
		errors:            make(chan error, 100),
		ctx:               ctx,
		cancel:            cancel,
		discoveredServers: make([]models.DiscoveredServer, 0),
		scanCache:         NewScanCache(5 * time.Minute), // Cache for 5 minutes
	}
}

// Scan performs the network scan
func (ns *NetworkScanner) Scan() ([]models.DiscoveredServer, []error) {
	var discovered []models.DiscoveredServer
	var scanErrors []error

	// Recreate channels in case Scan is called multiple times
	ns.results = make(chan models.DiscoveredServer, 100)
	ns.errors = make(chan error, 100)
	ns.ctx, ns.cancel = context.WithCancel(context.Background())

	// Parse ports from config
	ports, err := ns.expandPorts(ns.config.Ports)
	if err != nil {
		ns.errors <- fmt.Errorf("failed to parse ports: %w", err)
		close(ns.results)
		close(ns.errors)
		return discovered, scanErrors
	}

	// Create worker pool for concurrent scanning
	semaphore := make(chan struct{}, ns.config.MaxConcurrent)

	// Scan each IP range
	for _, scanRange := range ns.config.ScanRanges {
		ips, err := ns.expandIPRange(scanRange)
		if err != nil {
			ns.errors <- fmt.Errorf("failed to parse IP range %s: %w", scanRange, err)
			continue
		}

		for _, ip := range ips {
			for _, port := range ports {
				// Check if we should exclude this address
				if ns.shouldExcludeAddress(ip) {
					continue
				}

				// Check incremental scanning cache
				address := fmt.Sprintf("%s:%d", ip, port)
				if ns.scanCache.IsRecentlyScanned(address) {
					// Skip recently scanned addresses for incremental scanning
					continue
				}

				ns.wg.Add(1)
				go func(ip string, port int, address string) {
					defer ns.wg.Done()

					// Mark as scanned immediately to prevent duplicate scanning
					ns.scanCache.MarkScanned(address)

					// Acquire semaphore
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					// Check if context is cancelled
					select {
					case <-ns.ctx.Done():
						return
					default:
					}

					// Try to detect MCP server
					if server, err := ns.detectMCPServer(ip, port); err == nil && server != nil {
						select {
						case ns.results <- *server:
						case <-ns.ctx.Done():
						}
					}
				}(ip, port, address)
			}
		}
	}

	// Use WaitGroup to wait for result collection to complete
	var collectionWg sync.WaitGroup
	collectionWg.Add(2)

	// Start collecting results and errors
	go func() {
		defer collectionWg.Done()
		for result := range ns.results {
			discovered = append(discovered, result)
			// Also store in scanner for later retrieval
			ns.mu.Lock()
			ns.discoveredServers = append(ns.discoveredServers, result)
			ns.mu.Unlock()
		}
	}()

	go func() {
		defer collectionWg.Done()
		for err := range ns.errors {
			scanErrors = append(scanErrors, err)
		}
	}()

	// Wait for all scans to complete
	ns.wg.Wait()
	close(ns.results)
	close(ns.errors)

	// Wait for result collection to complete
	collectionWg.Wait()

	return discovered, scanErrors
}

// Stop cancels the scanning operation
func (ns *NetworkScanner) Stop() {
	ns.cancel()
}

// expandIPRange converts CIDR notation or IP range to list of IPs
func (ns *NetworkScanner) expandIPRange(ipRange string) ([]string, error) {
	// Check if it's CIDR notation
	if strings.Contains(ipRange, "/") {
		return ns.expandCIDR(ipRange)
	}

	// Check if it's a range (e.g., "192.168.1.1-192.168.1.10")
	if strings.Contains(ipRange, "-") {
		return ns.expandIPRangeNotation(ipRange)
	}

	// Single IP
	if net.ParseIP(ipRange) != nil {
		return []string{ipRange}, nil
	}

	return nil, fmt.Errorf("invalid IP range format: %s", ipRange)
}

// expandCIDR converts CIDR notation to list of IPs
func (ns *NetworkScanner) expandCIDR(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ips := []string{ipnet.IP.String()}
	return ips, nil
}

// expandIPRangeNotation converts "192.168.1.1-192.168.1.10" to list of IPs
func (ns *NetworkScanner) expandIPRangeNotation(ipRange string) ([]string, error) {
	parts := strings.Split(ipRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid IP range format: %s", ipRange)
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("invalid IP addresses in range: %s", ipRange)
	}

	var ips []string
	for ip := startIP; !ip.Equal(ns.incIP(endIP)); ns.incIP(ip) {
		ips = append(ips, ip.String())
	}

	return ips, nil
}

// incIP increments an IP address
func (ns *NetworkScanner) incIP(ip net.IP) net.IP {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
	return ip
}

// expandPorts converts port specifications to list of ports
func (ns *NetworkScanner) expandPorts(portSpecs []string) ([]int, error) {
	var ports []int

	for _, spec := range portSpecs {
		// Check if it's a range (e.g., "8000-8100")
		if strings.Contains(spec, "-") {
			rangePorts, err := ns.expandPortRange(spec)
			if err != nil {
				return nil, err
			}
			ports = append(ports, rangePorts...)
		} else {
			// Single port
			if port, err := strconv.Atoi(spec); err == nil {
				ports = append(ports, port)
			}
		}
	}

	return ports, nil
}

// expandPortRange converts "8000-8100" to list of ports
func (ns *NetworkScanner) expandPortRange(portRange string) ([]int, error) {
	var ports []int

	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port range format: %s", portRange)
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, err
	}

	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, err
	}

	if start > end {
		return nil, fmt.Errorf("invalid port range: start > end")
	}

	for port := start; port <= end; port++ {
		ports = append(ports, port)
	}

	return ports, nil
}

// shouldExcludeAddress checks if an address should be excluded
func (ns *NetworkScanner) shouldExcludeAddress(address string) bool {
	for _, exclude := range ns.config.ExcludeAddresses {
		if address == exclude {
			return true
		}
	}
	return false
}

// detectMCPServer attempts to detect an MCP server at the given address
func (ns *NetworkScanner) detectMCPServer(ip string, port int) (*models.DiscoveredServer, error) {
	address := fmt.Sprintf("%s:%d", ip, port)

	// Parse timeout from config
	timeout := 5 * time.Second
	if ns.config.Timeout != "" {
		if parsedTimeout, err := time.ParseDuration(ns.config.Timeout); err == nil {
			timeout = parsedTimeout
		}
	}

	// MCP detection config with proper JSON-RPC requests
	detectionConfig := &MCPDetectionConfig{
		Endpoints: []string{"/mcp", "/"},
		Methods:   []string{"POST"},
		UserAgent: "MCP-Scanner/1.0",
		CustomHeaders: map[string]string{
			"Content-Type":         "application/json",
			"Accept":               "application/json, text/event-stream",
			"MCP-Protocol-Version": "2025-06-18",
		},
		Timeout: timeout,
	}

	client := &http.Client{
		Timeout: detectionConfig.Timeout,
	}

	// JSON-RPC initialize message for MCP protocol
	initPayload := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2025-06-18",
			"capabilities": {},
			"clientInfo": {
				"name": "mcp-scanner",
				"version": "1.0"
			}
		}
	}`

	for _, endpoint := range detectionConfig.Endpoints {
		for _, method := range detectionConfig.Methods {
			url := fmt.Sprintf("http://%s%s", address, endpoint)

			var body io.Reader
			if method == "POST" {
				body = strings.NewReader(initPayload)
			}

			req, err := http.NewRequestWithContext(ns.ctx, method, url, body)
			if err != nil {
				continue
			}

			req.Header.Set("User-Agent", detectionConfig.UserAgent)
			for key, value := range detectionConfig.CustomHeaders {
				req.Header.Set(key, value)
			}

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
			if server := ns.parseMCPResponse(resp, bodyBytes, address, fmt.Sprintf("%d", port), endpoint); server != nil {
				log.Printf("DEBUG: Detected MCP server at %s, performing deep interrogation", address)
				// Perform deep interrogation if this is a basic MCP detection
				if endpoint == "/" || endpoint == "/mcp" {
					ns.performDeepInterrogation(ns.ctx, server)
				}

				log.Printf("DEBUG: Sending server %s to results channel", server.ID)
				select {
				case ns.results <- *server:
					log.Printf("DEBUG: Successfully sent server %s to results channel", server.ID)
				case <-ns.ctx.Done():
					log.Printf("DEBUG: Context cancelled while sending server %s to results channel", server.ID)
				}
			}
		}
	}

	return nil, fmt.Errorf("no MCP server detected at %s", address)
}

// parseMCPResponse parses a JSON-RPC response to determine if it's from an MCP server
func (ns *NetworkScanner) parseMCPResponse(resp *http.Response, bodyBytes []byte, address, portStr, endpoint string) *models.DiscoveredServer {
	bodyStr := string(bodyBytes)

	// Check if response looks like JSON-RPC (either direct or in SSE format)
	if strings.Contains(bodyStr, `"jsonrpc"`) {
		// Handle JSON-RPC responses (existing logic)
		// Handle Server-Sent Events (SSE) format
		var jsonResponse map[string]interface{}
		if strings.Contains(bodyStr, "event: message") && strings.Contains(bodyStr, "data: ") {
			// Extract JSON from SSE format
			lines := strings.Split(bodyStr, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					jsonData := strings.TrimPrefix(line, "data: ")
					if err := json.Unmarshal([]byte(jsonData), &jsonResponse); err != nil {
						continue
					}
					break
				}
			}
		} else {
			// Direct JSON response
			if err := json.Unmarshal(bodyBytes, &jsonResponse); err != nil {
				return nil
			}
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
			// Error response - analyze the error type
			errorCode, _ := error["code"].(float64)
			errorMessage, _ := error["message"].(string)

			// Check for specific MCP error conditions
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				authType = "required"
				vulnerabilityScore = "low" // Auth required = low vulnerability
			} else if int(errorCode) == -32600 && strings.Contains(strings.ToLower(errorMessage), "not acceptable") {
				// "Not Acceptable: Client must accept text/event-stream" - missing Accept header
				authType = "none"
				vulnerabilityScore = "high"
			} else if int(errorCode) == -32600 && strings.Contains(strings.ToLower(errorMessage), "missing session id") {
				// "Bad Request: Missing session ID" - server requires session initialization
				authType = "session_required"
				vulnerabilityScore = "medium"
			} else if int(errorCode) == -32602 {
				// Invalid params - likely protocol version issue
				authType = "protocol_mismatch"
				vulnerabilityScore = "medium"
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
			// For HTTP-based MCP servers, the transport is always Streamable HTTP
			// The Content-Type indicates whether it uses SSE or direct JSON responses
			connectionType := models.ConnectionTypeStreamableHttp

			serverName := "Unknown MCP Server"
			if name, ok := serverInfo["name"].(string); ok {
				serverName = name
			}

			// Generate unique ID for the discovered server
			serverID := fmt.Sprintf("mcp-%s-%s", strings.ReplaceAll(address, ":", "-"), strings.ReplaceAll(endpoint, "/", "-"))

			server := &models.DiscoveredServer{
				ID:         serverID,
				Address:    fmt.Sprintf("http://%s", address),
				Protocol:   models.ServerProtocolMCP,
				Connection: connectionType,
				Status:     "discovered",
				LastSeen:   time.Now(),
				Metadata: map[string]string{
					"port":                portStr,
					"endpoint":            endpoint,
					"server_name":         serverName,
					"auth_type":           authType,
					"vulnerability_score": vulnerabilityScore,
					"sse_supported":       fmt.Sprintf("%t", resp.Header.Get("Content-Type") == "text/event-stream"),
				},
				VulnerabilityScore: vulnerabilityScore,
			}

			// Set name if available
			if serverName != "Unknown MCP Server" {
				server.Name = serverName
			}

			return server
		}

		return nil
	} else {
		// Check for HTTP errors that might indicate MCP servers
		if (resp.StatusCode >= 400 && resp.StatusCode < 500) && endpoint == "/mcp" {
			// Try to parse as JSON error response
			var errorResponse map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &errorResponse); err == nil {
				if _, hasError := errorResponse["error"]; hasError {
					// Looks like an auth or protocol error for an MCP server
					authType := "required"
					vulnerabilityScore := "low"
					serverName := "Unknown MCP Server (Auth Required)"

					if resp.StatusCode == 401 || resp.StatusCode == 403 {
						authType = "required"
						vulnerabilityScore = "low"
					} else if resp.StatusCode == 400 {
						authType = "protocol_mismatch"
						vulnerabilityScore = "medium"
						serverName = "Unknown MCP Server (Protocol Error)"
					} else {
						authType = "unknown"
						vulnerabilityScore = "medium"
					}

					serverID := fmt.Sprintf("mcp-%s-%s", strings.ReplaceAll(address, ":", "-"), strings.ReplaceAll(endpoint, "/", "-"))

					server := &models.DiscoveredServer{
						ID:         serverID,
						Address:    fmt.Sprintf("http://%s", address),
						Protocol:   models.ServerProtocolMCP,
						Connection: models.ConnectionTypeStreamableHttp,
						Status:     "discovered",
						LastSeen:   time.Now(),
						Name:       serverName,
						Metadata: map[string]string{
							"port":                portStr,
							"endpoint":            endpoint,
							"server_name":         serverName,
							"auth_type":           authType,
							"vulnerability_score": vulnerabilityScore,
							"http_status":         fmt.Sprintf("%d", resp.StatusCode),
						},
						VulnerabilityScore: vulnerabilityScore,
					}
					return server
				}
			} else {
				// Non-JSON error response - could still be MCP server with HTML error page
				// Check for common MCP server error patterns
				bodyLower := strings.ToLower(bodyStr)
				if strings.Contains(bodyLower, "mcp") || strings.Contains(bodyLower, "jsonrpc") ||
					strings.Contains(bodyLower, "protocol") || resp.StatusCode == 405 {
					serverID := fmt.Sprintf("mcp-%s-%s", strings.ReplaceAll(address, ":", "-"), strings.ReplaceAll(endpoint, "/", "-"))

					server := &models.DiscoveredServer{
						ID:         serverID,
						Address:    fmt.Sprintf("http://%s", address),
						Protocol:   models.ServerProtocolMCP,
						Connection: models.ConnectionTypeStreamableHttp,
						Status:     "discovered",
						LastSeen:   time.Now(),
						Name:       "Unknown MCP Server (Error Response)",
						Metadata: map[string]string{
							"port":                portStr,
							"endpoint":            endpoint,
							"server_name":         "Unknown MCP Server (Error Response)",
							"auth_type":           "unknown",
							"vulnerability_score": "medium",
							"http_status":         fmt.Sprintf("%d", resp.StatusCode),
							"error_response":      truncateString(bodyStr, 200), // Truncate for storage
						},
						VulnerabilityScore: "medium",
					}
					return server
				}
			}
		}
		return nil
	}
}

// isMCPServerResponse checks if the HTTP response indicates an MCP server
func (ns *NetworkScanner) isMCPServerResponse(resp *http.Response) bool {
	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	// Check content type for MCP-related responses
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return true
	}

	// Check for MCP-specific headers
	if resp.Header.Get("X-MCP-Protocol") != "" {
		return true
	}

	// Check for Server-Sent Events (SSE) which MCP uses
	if resp.Header.Get("Content-Type") == "text/event-stream" {
		return true
	}

	return false
}

// detectProtocol determines the MCP protocol type
func (ns *NetworkScanner) detectProtocol(resp *http.Response) string {
	contentType := resp.Header.Get("Content-Type")

	if contentType == "text/event-stream" {
		return "sse"
	}

	if strings.Contains(contentType, "application/json") {
		return "http"
	}

	return "unknown"
}

// GetDiscoveredServer retrieves a discovered server by ID
func (ns *NetworkScanner) GetDiscoveredServer(serverID string) *models.DiscoveredServer {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	for _, server := range ns.discoveredServers {
		if server.ID == serverID {
			return &server
		}
	}

	return nil
}

// GetAllDiscoveredServers returns all discovered servers
func (ns *NetworkScanner) GetAllDiscoveredServers() []models.DiscoveredServer {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]models.DiscoveredServer, len(ns.discoveredServers))
	copy(result, ns.discoveredServers)
	return result
}

// performDeepInterrogation performs detailed MCP protocol interrogation
func (ns *NetworkScanner) performDeepInterrogation(ctx context.Context, server *models.DiscoveredServer) {
	// Create MCP client for deep interrogation
	client := mcp.NewClient(server.Address, 10*time.Second)

	// Perform interrogation
	capabilities, serverInfo, tools, resources, prompts, authAnalysis, err := client.InterrogateServer(ctx)
	if err != nil {
		// Log error and update server metadata with failure information
		log.Printf("Deep interrogation failed for %s: %v", server.Address, err)

		// Update server with failure information
		if server.Metadata == nil {
			server.Metadata = make(map[string]string)
		}
		server.Metadata["deep_scan_error"] = err.Error()
		server.Metadata["deep_scan_status"] = "failed"
		server.LastDeepScan = time.Now()

		// If we got auth analysis even on failure, use it
		if authAnalysis != nil {
			server.AuthInfo = authAnalysis
			if authAnalysis.Required {
				server.VulnerabilityScore = "low" // Auth required = lower vulnerability
			} else {
				server.VulnerabilityScore = "high" // No auth = higher vulnerability
			}
		}

		return
	}

	// Perform capability validation
	validation, _ := client.ValidateCapabilities(ctx, capabilities, tools, resources, prompts)

	// Update server with detailed information
	server.Capabilities = capabilities
	server.Tools = tools
	server.Resources = resources
	server.Prompts = prompts
	server.ResourceTemplates = nil // TODO: Add resource templates support
	server.AuthInfo = authAnalysis
	server.LastDeepScan = time.Now()

	if server.Metadata == nil {
		server.Metadata = make(map[string]string)
	}
	server.Metadata["deep_scan_status"] = "success"
	server.Metadata["capability_validation"] = fmt.Sprintf("tools:%t,resources:%t,prompts:%t",
		validation.ToolsValid, validation.ResourcesValid, validation.PromptsValid)

	if len(validation.Issues) > 0 {
		server.Metadata["validation_issues"] = fmt.Sprintf("%d issues found", len(validation.Issues))
	}

	if serverInfo != nil {
		server.ServerVersion = serverInfo.Version
		server.ProtocolVersion = serverInfo.Protocol
	}

	// Update vulnerability score based on auth analysis and validation
	vulnerabilityScore := "medium" // Default

	if authAnalysis != nil {
		if authAnalysis.Required {
			vulnerabilityScore = "low" // Auth required = lower vulnerability
		} else {
			vulnerabilityScore = "high" // No auth = higher vulnerability
		}
	}

	// If validation found issues, increase vulnerability score
	if !validation.ToolsValid || !validation.ResourcesValid || !validation.PromptsValid {
		if vulnerabilityScore == "low" {
			vulnerabilityScore = "medium"
		} else if vulnerabilityScore == "medium" {
			vulnerabilityScore = "high"
		}
	}

	server.VulnerabilityScore = vulnerabilityScore
}
