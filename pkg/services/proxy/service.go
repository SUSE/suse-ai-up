package proxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"suse-ai-up/pkg/logging"
	"suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/proxy"
	"time"

	"gopkg.in/yaml.v3"
)

// Service represents the proxy service
type Service struct {
	config          *Config
	server          *proxy.MCPProxyServer
	httpServer      *http.Server
	httpsServer     *http.Server
	shutdownCh      chan struct{}
	adapterHandlers AdapterHandlers
	// Simple in-memory adapter storage for basic functionality
	adapters map[string]models.AdapterResource
	// Delegate to proper adapter service for full functionality
	adapterService interface{} // Will be set to the real adapter service
	adapterStore   interface{} // Adapter store interface
}

// AdapterHandlers contains the adapter handler functions
type AdapterHandlers struct {
	ListAdapters      func(http.ResponseWriter, *http.Request)
	CreateAdapter     func(http.ResponseWriter, *http.Request)
	GetAdapter        func(http.ResponseWriter, *http.Request)
	UpdateAdapter     func(http.ResponseWriter, *http.Request)
	DeleteAdapter     func(http.ResponseWriter, *http.Request)
	HandleMCPProtocol func(http.ResponseWriter, *http.Request)
	SyncCapabilities  func(http.ResponseWriter, *http.Request)
}

// Config holds proxy service configuration
type Config struct {
	Port       int    `json:"port"`
	TLSPort    int    `json:"tls_port"`
	ConfigFile string `json:"config_file"`
	AutoTLS    bool   `json:"auto_tls"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
}

// NewService creates a new proxy service
func NewService(config *Config) *Service {
	return &Service{
		config:          config,
		shutdownCh:      make(chan struct{}),
		adapterHandlers: AdapterHandlers{}, // Will be set up in route configuration
		adapters:        make(map[string]models.AdapterResource),
	}
}

// Start starts the proxy service (unified architecture - no longer starts HTTP servers)
func (s *Service) Start() error {
	logging.ProxyLogger.Info("Proxy service initialized (routes handled by main Gin server)")
	return nil
}

// loadProxyConfig loads the MCP server configuration
func (s *Service) loadProxyConfig() (*proxy.MCPConfig, error) {
	// For now, return a basic config
	// In production, this would load from s.config.ConfigFile
	return &proxy.MCPConfig{
		MCPServers: map[string]proxy.ServerConfig{
			"example": {
				URL:       "http://localhost:3000/mcp",
				Transport: "http",
			},
		},
	}, nil
}

// handleHealth handles health check requests
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "proxy",
		"timestamp": time.Now(),
	})
}

// Stop stops the proxy service
func (s *Service) Stop() error {
	logging.ProxyLogger.Info("Stopping MCP Proxy service")

	// Shutdown HTTP server
	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			logging.ProxyLogger.Error("Error shutting down HTTP server: %v", err)
		} else {
			logging.ProxyLogger.Success("HTTP server stopped")
		}
	}

	// Shutdown HTTPS server
	if s.httpsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpsServer.Shutdown(shutdownCtx); err != nil {
			logging.ProxyLogger.Error("Error shutting down HTTPS server: %v", err)
		} else {
			logging.ProxyLogger.Success("HTTPS server stopped")
		}
	}

	close(s.shutdownCh)
	return nil
}

// Adapter handler methods

func (s *Service) handleListAdapters(w http.ResponseWriter, r *http.Request) {
	logging.AdapterLogger.Info("handleListAdapters called")
	w.Header().Set("Content-Type", "application/json")
	adapters := make([]models.AdapterResource, 0, len(s.adapters))
	for _, adapter := range s.adapters {
		adapters = append(adapters, adapter)
	}
	json.NewEncoder(w).Encode(adapters)
}

func (s *Service) handleCreateAdapter(w http.ResponseWriter, r *http.Request) {
	logging.AdapterLogger.Info("handleCreateAdapter called")

	var req struct {
		MCPServerID          string            `json:"mcpServerId"`
		Name                 string            `json:"name"`
		Description          string            `json:"description"`
		EnvironmentVariables map[string]string `json:"environmentVariables"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.AdapterLogger.Error("Failed to decode adapter request: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}

	if req.Name == "" {
		logging.AdapterLogger.Error("Adapter name is required")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Adapter name is required"})
		return
	}

	// Check if adapter already exists
	if _, exists := s.adapters[req.Name]; exists {
		logging.AdapterLogger.Error("Adapter %s already exists", req.Name)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "Adapter already exists"})
		return
	}

	// Create basic adapter data
	adapterData := &models.AdapterData{
		Name:                 req.Name,
		ConnectionType:       models.ConnectionTypeStreamableHttp,
		EnvironmentVariables: req.EnvironmentVariables,
		Description:          req.Description,
	}

	// Create adapter resource
	adapter := models.AdapterResource{}
	adapter.Create(*adapterData, "system", time.Now())

	// Store adapter
	s.adapters[req.Name] = adapter

	logging.AdapterLogger.Success("Created adapter %s", req.Name)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"id":          adapter.ID,
		"mcpServerId": req.MCPServerID,
		"status":      "ready",
		"capabilities": map[string]interface{}{
			"serverInfo": map[string]interface{}{
				"name":    adapter.Name,
				"version": "1.0.0",
			},
			"tools": []interface{}{
				map[string]interface{}{
					"name":        "example_tool",
					"description": "Example tool from remote server",
					"input_schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"input": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
		"mcpClientConfig": map[string]interface{}{
			"mcpServers": []interface{}{
				map[string]interface{}{
					"url": fmt.Sprintf("http://localhost:%d/api/v1/adapters/%s/mcp", s.config.Port, req.Name),
					"auth": map[string]interface{}{
						"type":  "bearer",
						"token": "adapter-session-token",
					},
				},
			},
		},
	}
	json.NewEncoder(w).Encode(response)
}

func (s *Service) handleGetAdapter(w http.ResponseWriter, r *http.Request, adapterName string) {
	logging.AdapterLogger.Info("handleGetAdapter called for %s", adapterName)

	adapter, exists := s.adapters[adapterName]
	if !exists {
		logging.AdapterLogger.Warn("Adapter %s not found", adapterName)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Adapter not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adapter)
}

func (s *Service) handleUpdateAdapter(w http.ResponseWriter, r *http.Request, adapterName string) {
	logging.AdapterLogger.Info("handleUpdateAdapter called for %s", adapterName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "Adapter update not implemented"})
}

func (s *Service) handleDeleteAdapter(w http.ResponseWriter, r *http.Request, adapterName string) {
	logging.AdapterLogger.Info("handleDeleteAdapter called for %s", adapterName)

	if _, exists := s.adapters[adapterName]; !exists {
		logging.AdapterLogger.Warn("Adapter %s not found for deletion", adapterName)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Adapter not found"})
		return
	}

	delete(s.adapters, adapterName)
	logging.AdapterLogger.Success("Deleted adapter %s", adapterName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleMCPProtocol(w http.ResponseWriter, r *http.Request) {
	logging.AdapterLogger.Info("handleMCPProtocol called")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "MCP protocol not implemented"})
}

func (s *Service) handleSyncCapabilities(w http.ResponseWriter, r *http.Request) {
	logging.AdapterLogger.Info("handleSyncCapabilities called")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "synced"})
}

// generateSelfSignedCert generates a self-signed certificate for development
func (s *Service) generateSelfSignedCert() (*tls.Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"SUSE AI Universal Proxy"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privateKey,
	}

	return cert, nil
}

// handleDocs serves the Swagger UI

// HandleAdapterMCP handles MCP requests for adapters
func (s *Service) HandleAdapterMCP(w http.ResponseWriter, r *http.Request) {
	// Extract adapter ID from URL: /api/v1/adapters/{id}/mcp
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	adapterID := strings.TrimSuffix(path, "/mcp")

	if adapterID == "" {
		http.Error(w, "Adapter ID not found in path", http.StatusBadRequest)
		return
	}

	// Get adapter from registry (proxy to registry service)
	registryURL := fmt.Sprintf("http://localhost:8913/api/v1/adapters/%s", adapterID)
	resp, err := http.Get(registryURL)
	if err != nil {
		logging.ProxyLogger.Error("Failed to get adapter %s: %v", adapterID, err)
		http.Error(w, "Adapter not found", http.StatusNotFound)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logging.ProxyLogger.Warn("Adapter %s not found (status: %d)", adapterID, resp.StatusCode)
		http.Error(w, "Adapter not found", http.StatusNotFound)
		return
	}

	// Parse adapter response
	var adapter models.AdapterResource
	if err := json.NewDecoder(resp.Body).Decode(&adapter); err != nil {
		logging.ProxyLogger.Error("Failed to parse adapter response: %v", err)
		http.Error(w, "Failed to parse adapter", http.StatusInternalServerError)
		return
	}

	// Route based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeSidecarStdio:
		// Handle sidecar stdio
		s.handleSidecarMCP(w, r, adapter)
	case models.ConnectionTypeRemoteHttp:
		// Handle remote HTTP
		s.handleRemoteHttpMCP(w, r, adapter)
	case models.ConnectionTypeStreamableHttp:
		// Handle streamable HTTP
		s.handleStreamableHttpMCP(w, r, adapter)
	default:
		http.Error(w, fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType), http.StatusBadRequest)
	}
}

// handleLocalStdioMCP handles MCP requests for local stdio adapters
func (s *Service) handleLocalStdioMCP(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource) {
	// For now, proxy to the existing MCP handler
	// This would need to be integrated with the existing local stdio plugin
	http.Error(w, "Local stdio MCP not yet implemented", http.StatusNotImplemented)
}

// handleSidecarMCP handles MCP requests for sidecar adapters
func (s *Service) handleSidecarMCP(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource) {
	if adapter.SidecarConfig == nil {
		http.Error(w, "Sidecar configuration missing", http.StatusInternalServerError)
		return
	}

	// Construct sidecar service URL
	sidecarURL := fmt.Sprintf("http://mcp-sidecar-%s.default.svc.cluster.local:%d/mcp",
		adapter.ID, adapter.SidecarConfig.Port)

	// Proxy the request to the sidecar
	s.proxyRequest(w, r, sidecarURL, "/api/v1/adapters/"+adapter.ID+"/mcp")
}

// handleRemoteHttpMCP handles MCP requests for remote HTTP adapters
func (s *Service) handleRemoteHttpMCP(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource) {
	if adapter.RemoteUrl == "" {
		http.Error(w, "Remote URL not configured", http.StatusInternalServerError)
		return
	}

	// Proxy to remote URL
	s.proxyRequest(w, r, adapter.RemoteUrl, "/api/v1/adapters/"+adapter.ID+"/mcp")
}

// handleStreamableHttpMCP handles MCP requests for streamable HTTP adapters
func (s *Service) handleStreamableHttpMCP(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource) {
	// For sidecar-based streamable HTTP adapters, construct the sidecar service URL
	port := 8000 // default
	if adapter.SidecarConfig != nil {
		port = adapter.SidecarConfig.Port
	}
	serviceURL := fmt.Sprintf("http://mcp-sidecar-%s.suse-ai-up-mcp.svc.cluster.local:%d", adapter.ID, port)
	s.proxyRequest(w, r, serviceURL, "/api/v1/adapters/"+adapter.ID)
}

// handleAdapterCreation handles adapter creation requests
func (s *Service) handleAdapterCreation(w http.ResponseWriter, r *http.Request) {
	// Parse the request body
	var req struct {
		MCPServerID          string            `json:"mcpServerId"`
		Name                 string            `json:"name"`
		Description          string            `json:"description"`
		EnvironmentVariables map[string]string `json:"environmentVariables"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Invalid JSON"}`))
		return
	}

	// Only handle Uyuni for now
	if req.MCPServerID != "d0e6a34b749ba1f6" && req.MCPServerID != "uyuni" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "Server not found"}`))
		return
	}

	// Create a simple adapter response
	response := map[string]interface{}{
		"id":          req.Name,
		"mcpServerId": req.MCPServerID,
		"mcpClientConfig": map[string]interface{}{
			"mcpServers": []interface{}{
				map[string]interface{}{
					"url": fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", req.Name),
					"auth": map[string]interface{}{
						"type":  "bearer",
						"token": "adapter-session-token",
					},
				},
			},
		},
		"capabilities": map[string]interface{}{
			"serverInfo": map[string]interface{}{
				"name":    req.Name,
				"version": "1.0.0",
			},
			"tools": []interface{}{
				map[string]interface{}{
					"name":        "example_tool",
					"description": "Example tool",
					"input_schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"input": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
			"lastRefreshed": "2025-12-12T17:25:49.269883773Z",
		},
		"status":    "ready",
		"createdAt": "2025-12-12T17:25:49.269884523Z",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// proxyToRegistry handles registry and adapter requests for unified service
func (s *Service) proxyToRegistry(w http.ResponseWriter, r *http.Request) {

	// For unified service, handle registry and adapter requests internally

	// Handle adapter creation
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/adapters" {
		s.handleAdapterCreation(w, r)
		return
	}

	// Handle adapter listing
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/adapters" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) // Return empty list for now
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error": "Method not allowed"}`))
		return
	}

	// Handle registry browse endpoint
	if strings.Contains(r.URL.Path, "/browse") {
		servers := s.loadRegistryServers()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(servers)
		return
	}

	// For other registry requests, return not implemented
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error": "Registry functionality not fully implemented in unified service"}`))
}

// proxyToDiscovery forwards requests to the discovery service
func (s *Service) proxyToDiscovery(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "http://127.0.0.1:8912", "")
}

// proxyToPlugins forwards requests to the plugins service
func (s *Service) proxyToPlugins(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "http://127.0.0.1:8914", "")
}

// loadRegistryServers loads MCP servers from the config file
func (s *Service) loadRegistryServers() []map[string]interface{} {
	registryFile := "config/mcp_registry.yaml"
	data, err := os.ReadFile(registryFile)
	if err != nil {
		logging.ProxyLogger.Error("Could not read registry file %s: %v", registryFile, err)
		return []map[string]interface{}{}
	}

	var servers []map[string]interface{}
	if err := yaml.Unmarshal(data, &servers); err != nil {
		logging.ProxyLogger.Error("Could not parse registry file %s: %v", registryFile, err)
		return []map[string]interface{}{}
	}

	logging.ProxyLogger.Info("Loaded %d MCP servers from %s", len(servers), registryFile)

	// Return the complete server data as-is from the YAML
	return servers
}

// handleAdapterListCreate handles GET /api/v1/adapters (list) and POST /api/v1/adapters (create)
func (s *Service) handleAdapterListCreate(w http.ResponseWriter, r *http.Request) {
	logging.ProxyLogger.Info("handleAdapterListCreate called with method: %s, path: %s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		s.handleListAdapters(w, r)
	case http.MethodPost:
		s.handleCreateAdapter(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdapterByID handles /api/v1/adapters/{id} routes
func (s *Service) handleAdapterByID(w http.ResponseWriter, r *http.Request) {
	logging.ProxyLogger.Info("handleAdapterByID called with method: %s, path: %s", r.Method, r.URL.Path)
	// Extract adapter ID from path: /api/v1/adapters/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	if path == "" {
		http.Error(w, "Adapter ID required", http.StatusBadRequest)
		return
	}

	// Split path to get adapter ID and any sub-path
	parts := strings.SplitN(path, "/", 2)
	adapterID := parts[0]

	switch r.Method {
	case http.MethodGet:
		s.handleGetAdapter(w, r, adapterID)
	case http.MethodPut:
		s.handleUpdateAdapter(w, r, adapterID)
	case http.MethodDelete:
		s.handleDeleteAdapter(w, r, adapterID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// proxyRequest forwards HTTP requests to other services
func (s *Service) proxyRequest(w http.ResponseWriter, r *http.Request, serviceURL, basePath string) {
	// Build the target URL
	targetPath := r.URL.Path
	if basePath != "" {
		targetPath = strings.TrimPrefix(r.URL.Path, basePath)
		if !strings.HasPrefix(targetPath, "/") {
			targetPath = "/" + targetPath
		}
	}
	targetURL := serviceURL + targetPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	logging.ProxyLogger.Info("Proxying request: %s %s -> %s", r.Method, r.URL.Path, targetURL)

	// Create the request to the target service
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Add API key authentication
	middleware.AddAPIKeyAuth(req)

	// Make the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logging.ProxyLogger.Error("Proxy request failed: %v", err)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code and copy body
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
