package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/models"
)

// NetworkScanner performs network scanning to discover MCP servers
type NetworkScanner struct {
	config  *models.ScanConfig
	results chan models.DiscoveredServer
	errors  chan error
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
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
		config:  config,
		results: make(chan models.DiscoveredServer, 100),
		errors:  make(chan error, 100),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Scan performs the network scan
func (ns *NetworkScanner) Scan() ([]models.DiscoveredServer, []error) {
	var discovered []models.DiscoveredServer
	var scanErrors []error

	// Start collecting results and errors
	go func() {
		for result := range ns.results {
			discovered = append(discovered, result)
		}
	}()

	go func() {
		for err := range ns.errors {
			scanErrors = append(scanErrors, err)
		}
	}()

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

				ns.wg.Add(1)
				go func(ip string, port int) {
					defer ns.wg.Done()

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
				}(ip, port)
			}
		}
	}

	// Wait for all scans to complete
	ns.wg.Wait()
	close(ns.results)
	close(ns.errors)

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

	// MCP detection config with proper JSON-RPC requests
	detectionConfig := &MCPDetectionConfig{
		Endpoints: []string{"/mcp", "/"},
		Methods:   []string{"POST"},
		UserAgent: "MCP-Scanner/1.0",
		CustomHeaders: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json, text/event-stream",
		},
		Timeout: 5 * time.Second,
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
			"protocolVersion": "2024-11-05",
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
				return server, nil
			}
		}
	}

	return nil, fmt.Errorf("no MCP server detected at %s", address)
}

// parseMCPResponse parses a JSON-RPC response to determine if it's from an MCP server
func (ns *NetworkScanner) parseMCPResponse(resp *http.Response, bodyBytes []byte, address, portStr, endpoint string) *models.DiscoveredServer {
	// Check if response looks like JSON-RPC
	if !strings.Contains(string(bodyBytes), `"jsonrpc"`) {
		return nil
	}

	// Parse JSON-RPC response
	var jsonResponse map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &jsonResponse); err != nil {
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
			authType = "required"
			vulnerabilityScore = "low" // Auth required = low vulnerability
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
