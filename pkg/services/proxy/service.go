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

// Start starts the proxy service
func (s *Service) Start() error {
	logging.ProxyLogger.Info("Initializing MCP Proxy service on port %d", s.config.Port)

	// Load proxy configuration
	config, err := s.loadProxyConfig()
	if err != nil {
		return fmt.Errorf("failed to load proxy config: %w", err)
	}

	// Create proxy server
	s.server = proxy.AsProxyFromConfig(config, "MCPProxy")

	// Create HTTP handler
	handler := proxy.NewMCPProxyHandler(s.server)

	// Create mux with specific route handlers
	mux := http.NewServeMux()

	// MCP routes (both /mcp/* and /api/v1/mcp/* for compatibility)
	mux.HandleFunc("/mcp", middleware.CORSMiddleware(handler.HandleMCP))
	mux.HandleFunc("/api/v1/mcp", middleware.CORSMiddleware(handler.HandleMCP))
	mux.HandleFunc("/mcp/tools", middleware.CORSMiddleware(handler.HandleToolsList))
	mux.HandleFunc("/api/v1/mcp/tools", middleware.CORSMiddleware(handler.HandleToolsList))
	mux.HandleFunc("/mcp/tools/", middleware.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			handler.HandleToolCall(w, r)
		} else {
			http.NotFound(w, r)
		}
	}))
	mux.HandleFunc("/api/v1/mcp/tools/", middleware.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			handler.HandleToolCall(w, r)
		} else {
			http.NotFound(w, r)
		}
	}))
	mux.HandleFunc("/mcp/resources", middleware.CORSMiddleware(handler.HandleResourcesList))
	mux.HandleFunc("/api/v1/mcp/resources", middleware.CORSMiddleware(handler.HandleResourcesList))
	mux.HandleFunc("/mcp/resources/", middleware.CORSMiddleware(handler.HandleResourceRead))
	mux.HandleFunc("/api/v1/mcp/resources/", middleware.CORSMiddleware(handler.HandleResourceRead))

	// Health and docs
	mux.HandleFunc("/docs", middleware.CORSMiddleware(s.handleDocs))
	mux.HandleFunc("/swagger.json", middleware.CORSMiddleware(s.handleSwaggerJSON))

	// Registry routes - handle properly instead of hardcoded responses
	mux.HandleFunc("/api/v1/registry/", middleware.CORSMiddleware(s.proxyToRegistry))
	mux.HandleFunc("/api/v1/registry/upload", middleware.CORSMiddleware(s.proxyToRegistry))
	mux.HandleFunc("/api/v1/registry/upload/bulk", middleware.CORSMiddleware(s.proxyToRegistry))
	// Adapter routes with inline handlers
	mux.HandleFunc("/api/v1/adapters", middleware.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Handle both GET (list) and POST (create) for /api/v1/adapters
		switch r.Method {
		case http.MethodGet:
			s.handleListAdapters(w, r)
		case http.MethodPost:
			s.handleCreateAdapter(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/adapters/", middleware.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Route based on HTTP method and path
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters")
		if path == "" || path == "/" {
			// Root adapters endpoint
			switch r.Method {
			case http.MethodGet:
				s.handleListAdapters(w, r)
			case http.MethodPost:
				s.handleCreateAdapter(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		} else if strings.Contains(path, "/mcp") {
			// MCP protocol endpoint
			s.handleMCPProtocol(w, r)
		} else if strings.Contains(path, "/sync") {
			// Sync capabilities endpoint
			s.handleSyncCapabilities(w, r)
		} else {
			// Individual adapter endpoints - extract adapter name
			parts := strings.Split(strings.Trim(path, "/"), "/")
			if len(parts) > 0 {
				adapterName := parts[0]
				switch r.Method {
				case http.MethodGet:
					s.handleGetAdapter(w, r, adapterName)
				case http.MethodPut:
					s.handleUpdateAdapter(w, r, adapterName)
				case http.MethodDelete:
					s.handleDeleteAdapter(w, r, adapterName)
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			} else {
				http.Error(w, "Invalid adapter path", http.StatusBadRequest)
			}
		}
	}))
	mux.HandleFunc("/api/v1/scan", middleware.CORSMiddleware(s.proxyToDiscovery))
	mux.HandleFunc("/api/v1/scan/", middleware.CORSMiddleware(s.proxyToDiscovery))
	mux.HandleFunc("/api/v1/servers", middleware.CORSMiddleware(s.proxyToDiscovery))
	mux.HandleFunc("/api/v1/plugins", middleware.CORSMiddleware(s.proxyToPlugins))
	mux.HandleFunc("/api/v1/plugins/", middleware.CORSMiddleware(s.proxyToPlugins))
	mux.HandleFunc("/api/v1/health/", middleware.CORSMiddleware(s.proxyToPlugins))

	// Health check endpoint for Kubernetes probes
	mux.HandleFunc("/health", middleware.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy", "timestamp": "` + time.Now().UTC().Format(time.RFC3339) + `", "version": "1.0.0"}`))
	}))

	// Start HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", s.config.Port),
		Handler: mux,
	}

	// Start HTTP server in goroutine
	go func() {
		logging.ProxyLogger.Success("HTTP server listening on port %d", s.config.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.ProxyLogger.Error("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server if TLS is configured
	if s.config.TLSPort > 0 {
		tlsConfig := &tls.Config{
			ServerName: "localhost",
		}

		// Try to load certificates
		if s.config.CertFile != "" && s.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
			if err != nil {
				logging.ProxyLogger.Error("Failed to load TLS certificates: %v", err)
				if !s.config.AutoTLS {
					return fmt.Errorf("TLS certificates required but failed to load: %w", err)
				}
			} else {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		// Generate self-signed certificate if AutoTLS is enabled and no certs loaded
		if s.config.AutoTLS && len(tlsConfig.Certificates) == 0 {
			cert, err := s.generateSelfSignedCert()
			if err != nil {
				return fmt.Errorf("failed to generate self-signed certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{*cert}
			logging.ProxyLogger.Info("Generated self-signed TLS certificate for proxy service")
		}

		if len(tlsConfig.Certificates) > 0 {
			s.httpsServer = &http.Server{
				Addr:      fmt.Sprintf("0.0.0.0:%d", s.config.TLSPort),
				Handler:   mux,
				TLSConfig: tlsConfig,
			}

			go func() {
				logging.ProxyLogger.Success("HTTPS server listening on port %d", s.config.TLSPort)
				if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					logging.ProxyLogger.Error("HTTPS server error: %v", err)
				}
			}()
		}
	}

	logging.ProxyLogger.Success("MCP Proxy service started successfully")

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
func (s *Service) handleDocs(w http.ResponseWriter, r *http.Request) {
	swaggerHTML := `<!DOCTYPE html>
<html>
<head>
    <title>SUSE AI Universal Proxy API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: '/swagger.json',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                servers: [
                    {
                        url: 'http://localhost:8911',
                        description: 'Proxy Service'
                    }
                ],
                onComplete: function() {
                    // Add custom server selection for different operations
                    setTimeout(function() {
                        // Find all operations and set appropriate servers
                        const operations = document.querySelectorAll('.opblock-summary-method');
                        operations.forEach(function(op) {
                            const path = op.closest('.opblock').querySelector('.opblock-summary-path').textContent.trim();
                            if (path.startsWith('/api/v1/registry')) {
                                // This should use registry service
                                console.log('Registry operation:', path);
                            } else if (path.startsWith('/api/v1/scan') || path.startsWith('/api/v1/servers')) {
                                // This should use discovery service
                                console.log('Discovery operation:', path);
                            } else if (path.startsWith('/api/v1/plugins')) {
                                // This should use plugins service
                                console.log('Plugins operation:', path);
                            }
                        });
                    }, 1000);
                }
            });
        };
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(swaggerHTML))
}

// handleSwaggerJSON serves the Swagger JSON specification
func (s *Service) handleSwaggerJSON(w http.ResponseWriter, r *http.Request) {
	// Determine the host dynamically based on the request
	host := r.Host
	if host == "" {
		host = "localhost:8911"
	}

	// Create the swagger spec as a Go map for easier manipulation
	swagger := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":       "SUSE AI Universal Proxy API",
			"description": "Complete API documentation for the SUSE AI Universal Proxy - A comprehensive MCP proxy system with registry, discovery, and plugin management",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name":  "SUSE AI Team",
				"email": "ai@suse.com",
			},
		},
		"host":     host,
		"basePath": "/",
		"schemes":  []string{"http", "https"},
		"consumes": []string{"application/json"},
		"produces": []string{"application/json"},
		"tags": []map[string]interface{}{
			{"name": "Proxy", "description": "MCP proxy endpoints (Port 8911)"},
			{"name": "Registry", "description": "MCP server registry management (Port 8913)"},
			{"name": "Discovery", "description": "Network discovery and server scanning (Port 8912)"},
			{"name": "Plugins", "description": "Plugin management and registration (Port 8914)"},
			{"name": "Health", "description": "Health check endpoints"},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Health"},
					"summary":     "Proxy Health Check",
					"description": "Check the health status of the proxy service",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Service is healthy",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"status":    map[string]interface{}{"type": "string", "example": "healthy"},
									"service":   map[string]interface{}{"type": "string", "example": "proxy"},
									"timestamp": map[string]interface{}{"type": "string", "format": "date-time"},
								},
							},
						},
					},
				},
			},
			"/mcp": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Proxy"},
					"summary":     "MCP JSON-RPC Endpoint",
					"description": "Main Model Context Protocol JSON-RPC endpoint for tool calls and resource access",
					"parameters": []map[string]interface{}{
						{
							"in":          "body",
							"name":        "request",
							"description": "JSON-RPC 2.0 request",
							"required":    true,
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"jsonrpc": map[string]interface{}{"type": "string", "example": "2.0"},
									"id":      map[string]interface{}{"type": "integer", "example": 1},
									"method":  map[string]interface{}{"type": "string", "example": "tools/call"},
									"params":  map[string]interface{}{"type": "object"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful MCP response",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"jsonrpc": map[string]interface{}{"type": "string"},
									"id":      map[string]interface{}{"type": "integer"},
									"result":  map[string]interface{}{"type": "object"},
								},
							},
						},
					},
				},
			},
			"/mcp/tools": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Proxy"},
					"summary":     "List Available Tools",
					"description": "Get a list of all available MCP tools",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of tools",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"tools": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"type": "object",
											"properties": map[string]interface{}{
												"name":        map[string]interface{}{"type": "string"},
												"description": map[string]interface{}{"type": "string"},
												"inputSchema": map[string]interface{}{"type": "object"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"/mcp/tools/{toolName}": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Proxy"},
					"summary":     "Call MCP Tool",
					"description": "Execute a specific MCP tool",
					"parameters": []map[string]interface{}{
						{"name": "toolName", "in": "path", "required": true, "type": "string"},
						{"name": "params", "in": "body", "required": true, "schema": map[string]interface{}{"type": "object"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Tool execution result",
							"schema":      map[string]interface{}{"type": "object"},
						},
					},
				},
			},
			"/mcp/resources": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Proxy"},
					"summary":     "List Available Resources",
					"description": "Get a list of all available MCP resources",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of resources",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"resources": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"type": "object",
											"properties": map[string]interface{}{
												"uri":         map[string]interface{}{"type": "string"},
												"name":        map[string]interface{}{"type": "string"},
												"description": map[string]interface{}{"type": "string"},
												"mimeType":    map[string]interface{}{"type": "string"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"/mcp/resources/{resourceUri}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Proxy"},
					"summary":     "Read MCP Resource",
					"description": "Read content from a specific MCP resource",
					"parameters": []map[string]interface{}{
						{"name": "resourceUri", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Resource content",
							"schema":      map[string]interface{}{"type": "object"},
						},
					},
				},
			},
			"/api/v1/registry/browse": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Browse MCP Server Registry",
					"description": "Get a filtered list of MCP servers from the registry (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "q", "in": "query", "description": "Search query", "type": "string"},
						{"name": "category", "in": "query", "description": "Category filter (development, productivity, etc.)", "type": "string"},
						{"name": "transport", "in": "query", "description": "Transport type filter", "type": "string"},
						{"name": "registryType", "in": "query", "description": "Registry type filter", "type": "string"},
						{"name": "validationStatus", "in": "query", "description": "Validation status filter", "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of MCP servers",
							"schema": map[string]interface{}{
								"type":  "array",
								"items": map[string]interface{}{"$ref": "#/definitions/MCPServer"},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/registry/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Get MCP Server by ID",
					"description": "Retrieve a specific MCP server from the registry (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "MCP server details",
							"schema":      map[string]interface{}{"$ref": "#/definitions/MCPServer"},
						},
						"404": map[string]interface{}{
							"description": "Server not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Update MCP Server",
					"description": "Update an existing MCP server in the registry (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
						{"name": "server", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/MCPServer"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Updated server",
							"schema":      map[string]interface{}{"$ref": "#/definitions/MCPServer"},
						},
						"404": map[string]interface{}{
							"description": "Server not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Delete MCP Server",
					"description": "Remove an MCP server from the registry (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "Server deleted",
						},
						"404": map[string]interface{}{
							"description": "Server not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/registry/upload": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Upload Single MCP Server",
					"description": "Add a single MCP server to the registry (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "server", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/MCPServer"}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Server created",
							"schema":      map[string]interface{}{"$ref": "#/definitions/MCPServer"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/registry/upload/bulk": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Bulk Upload MCP Servers",
					"description": "Add multiple MCP servers to the registry (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "servers", "in": "body", "required": true, "schema": map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/MCPServer"}}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Servers created",
							"schema":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/MCPServer"}},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/registry/reload": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Reload Remote Servers",
					"description": "Reload MCP servers from remote configuration files (Registry Service - Port 8913)",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Reload completed",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"status":  map[string]interface{}{"type": "string"},
									"message": map[string]interface{}{"type": "string"},
								},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/adapters": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "List Adapters",
					"description": "Get a list of all adapters for the current user (Registry Service - Port 8913)",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of adapters",
							"schema": map[string]interface{}{
								"type":  "array",
								"items": map[string]interface{}{"$ref": "#/definitions/Adapter"},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Create Adapter",
					"description": "Create a new adapter from a registry server (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "adapter", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/CreateAdapterRequest"}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Adapter created",
							"schema":      map[string]interface{}{"$ref": "#/definitions/CreateAdapterResponse"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/adapters/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Get Adapter by ID",
					"description": "Retrieve details of a specific adapter (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Adapter details",
							"schema":      map[string]interface{}{"$ref": "#/definitions/Adapter"},
						},
						"404": map[string]interface{}{
							"description": "Adapter not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Update Adapter",
					"description": "Update an existing adapter (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
						{"name": "adapter", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/Adapter"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Adapter updated",
							"schema":      map[string]interface{}{"$ref": "#/definitions/Adapter"},
						},
						"404": map[string]interface{}{
							"description": "Adapter not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Delete Adapter",
					"description": "Remove an adapter (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "Adapter deleted",
						},
						"404": map[string]interface{}{
							"description": "Adapter not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/adapters/{id}/sync": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Registry"},
					"summary":     "Sync Adapter Capabilities",
					"description": "Synchronize capabilities for a specific adapter (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Capabilities synced",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"status":  map[string]interface{}{"type": "string"},
									"message": map[string]interface{}{"type": "string"},
								},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/scan": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Discovery"},
					"summary":     "Start Network Scan",
					"description": "Initiate a network scan for MCP servers (Discovery Service - Port 8912)",
					"parameters": []map[string]interface{}{
						{"name": "config", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/ScanConfig"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Scan started",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"scanId":    map[string]interface{}{"type": "string"},
									"status":    map[string]interface{}{"type": "string"},
									"config":    map[string]interface{}{"$ref": "#/definitions/ScanConfig"},
									"startTime": map[string]interface{}{"type": "string", "format": "date-time"},
								},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/scan/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Discovery"},
					"summary":     "Get Scan Status",
					"description": "Check the status of a running or completed scan (Discovery Service - Port 8912)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Scan status",
							"schema":      map[string]interface{}{"$ref": "#/definitions/ScanJob"},
						},
						"404": map[string]interface{}{
							"description": "Scan not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/servers": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Discovery"},
					"summary":     "List Discovered Servers",
					"description": "Get a list of all discovered MCP servers (Discovery Service - Port 8912)",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of discovered servers",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"servers": map[string]interface{}{
										"type":  "array",
										"items": map[string]interface{}{"$ref": "#/definitions/DiscoveredServer"},
									},
									"totalCount": map[string]interface{}{"type": "integer"},
									"scanCount":  map[string]interface{}{"type": "integer"},
								},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/plugins": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Plugins"},
					"summary":     "List Plugins",
					"description": "Get a list of all registered plugins (Plugins Service - Port 8914)",
					"parameters": []map[string]interface{}{
						{"name": "type", "in": "query", "description": "Filter by service type", "type": "string", "enum": []string{"smartagents", "registry", "virtualmcp"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of plugins",
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"plugins": map[string]interface{}{
										"type":  "array",
										"items": map[string]interface{}{"$ref": "#/definitions/Plugin"},
									},
									"totalCount": map[string]interface{}{"type": "integer"},
								},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/plugins/register": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Plugins"},
					"summary":     "Register Plugin",
					"description": "Register a new plugin (Plugins Service - Port 8914)",
					"parameters": []map[string]interface{}{
						{"name": "plugin", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/ServiceRegistration"}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Plugin registered",
							"schema":      map[string]interface{}{"$ref": "#/definitions/ServiceRegistration"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/plugins/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Plugins"},
					"summary":     "Get Plugin by ID",
					"description": "Retrieve details of a specific plugin (Plugins Service - Port 8914)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Plugin details",
							"schema":      map[string]interface{}{"$ref": "#/definitions/ServiceRegistration"},
						},
						"404": map[string]interface{}{
							"description": "Plugin not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Plugins"},
					"summary":     "Unregister Plugin",
					"description": "Remove a plugin from the registry (Plugins Service - Port 8914)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "Plugin unregistered",
						},
						"404": map[string]interface{}{
							"description": "Plugin not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/health/{pluginId}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Plugins"},
					"summary":     "Get Plugin Health",
					"description": "Check the health status of a specific plugin (Plugins Service - Port 8914)",
					"parameters": []map[string]interface{}{
						{"name": "pluginId", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Plugin health status",
							"schema":      map[string]interface{}{"$ref": "#/definitions/HealthStatus"},
						},
						"404": map[string]interface{}{
							"description": "Plugin not found",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
		},
		"definitions": map[string]interface{}{
			"MCPServer": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":               map[string]interface{}{"type": "string"},
					"name":             map[string]interface{}{"type": "string"},
					"description":      map[string]interface{}{"type": "string"},
					"packages":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/Package"}},
					"validationStatus": map[string]interface{}{"type": "string"},
					"discoveredAt":     map[string]interface{}{"type": "string", "format": "date-time"},
					"meta":             map[string]interface{}{"type": "object"},
				},
			},
			"Package": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":         map[string]interface{}{"type": "string"},
					"version":      map[string]interface{}{"type": "string"},
					"transport":    map[string]interface{}{"$ref": "#/definitions/Transport"},
					"registryType": map[string]interface{}{"type": "string"},
				},
			},
			"Transport": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type":   map[string]interface{}{"type": "string"},
					"config": map[string]interface{}{"type": "object"},
				},
			},
			"Adapter": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":                   map[string]interface{}{"type": "string", "example": "my-adapter"},
					"name":                 map[string]interface{}{"type": "string", "example": "my-adapter"},
					"imageName":            map[string]interface{}{"type": "string", "example": "nginx"},
					"imageVersion":         map[string]interface{}{"type": "string", "example": "latest"},
					"protocol":             map[string]interface{}{"type": "string", "example": "MCP"},
					"connectionType":       map[string]interface{}{"type": "string", "example": "StreamableHttp"},
					"environmentVariables": map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
					"replicaCount":         map[string]interface{}{"type": "integer", "example": 1},
					"description":          map[string]interface{}{"type": "string", "example": "My MCP adapter"},
					"useWorkloadIdentity":  map[string]interface{}{"type": "boolean", "example": false},
					"remoteUrl":            map[string]interface{}{"type": "string", "example": "https://remote-mcp.example.com"},
					"apiBaseUrl":           map[string]interface{}{"type": "string", "example": "http://localhost:8000"},
					"tools":                map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}},
					"command":              map[string]interface{}{"type": "string", "example": "python"},
					"args":                 map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"mcpClientConfig":      map[string]interface{}{"type": "object"},
					"authentication":       map[string]interface{}{"$ref": "#/definitions/AdapterAuthConfig"},
					"mcpFunctionality":     map[string]interface{}{"$ref": "#/definitions/MCPFunctionality"},
					"createdBy":            map[string]interface{}{"type": "string", "example": "user@example.com"},
					"createdAt":            map[string]interface{}{"type": "string", "format": "date-time"},
					"lastUpdatedAt":        map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"CreateAdapterRequest": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":                 map[string]interface{}{"type": "string", "example": "my-adapter"},
					"imageName":            map[string]interface{}{"type": "string", "example": "nginx"},
					"imageVersion":         map[string]interface{}{"type": "string", "example": "latest"},
					"protocol":             map[string]interface{}{"type": "string", "example": "MCP"},
					"connectionType":       map[string]interface{}{"type": "string", "example": "StreamableHttp"},
					"environmentVariables": map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
					"replicaCount":         map[string]interface{}{"type": "integer", "example": 1},
					"description":          map[string]interface{}{"type": "string", "example": "My MCP adapter"},
					"useWorkloadIdentity":  map[string]interface{}{"type": "boolean", "example": false},
					"remoteUrl":            map[string]interface{}{"type": "string", "example": "https://remote-mcp.example.com"},
					"apiBaseUrl":           map[string]interface{}{"type": "string", "example": "http://localhost:8000"},
					"tools":                map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}},
					"command":              map[string]interface{}{"type": "string", "example": "python"},
					"args":                 map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"mcpClientConfig":      map[string]interface{}{"type": "object"},
					"authentication":       map[string]interface{}{"$ref": "#/definitions/AdapterAuthConfig"},
				},
				"required": []string{"name"},
			},
			"CreateAdapterResponse": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":                   map[string]interface{}{"type": "string", "example": "my-adapter"},
					"name":                 map[string]interface{}{"type": "string", "example": "my-adapter"},
					"imageName":            map[string]interface{}{"type": "string", "example": "nginx"},
					"imageVersion":         map[string]interface{}{"type": "string", "example": "latest"},
					"protocol":             map[string]interface{}{"type": "string", "example": "MCP"},
					"connectionType":       map[string]interface{}{"type": "string", "example": "StreamableHttp"},
					"environmentVariables": map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
					"replicaCount":         map[string]interface{}{"type": "integer", "example": 1},
					"description":          map[string]interface{}{"type": "string", "example": "My MCP adapter"},
					"useWorkloadIdentity":  map[string]interface{}{"type": "boolean", "example": false},
					"remoteUrl":            map[string]interface{}{"type": "string", "example": "https://remote-mcp.example.com"},
					"apiBaseUrl":           map[string]interface{}{"type": "string", "example": "http://localhost:8000"},
					"tools":                map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}},
					"command":              map[string]interface{}{"type": "string", "example": "python"},
					"args":                 map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"mcpClientConfig":      map[string]interface{}{"type": "object"},
					"authentication":       map[string]interface{}{"$ref": "#/definitions/AdapterAuthConfig"},
					"mcpFunctionality":     map[string]interface{}{"$ref": "#/definitions/MCPFunctionality"},
					"createdBy":            map[string]interface{}{"type": "string", "example": "user@example.com"},
					"createdAt":            map[string]interface{}{"type": "string", "format": "date-time"},
					"lastUpdatedAt":        map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"ScanConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"scanRanges":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"192.168.1.0/24", "10.0.0.1-10.0.0.10"}},
					"ports":            map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"8000", "8001", "9000-9100"}},
					"timeout":          map[string]interface{}{"type": "string", "example": "30s"},
					"maxConcurrent":    map[string]interface{}{"type": "integer", "example": 10},
					"excludeProxy":     map[string]interface{}{"type": "boolean", "example": true},
					"excludeAddresses": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				},
			},
			"ScanJob": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]interface{}{"type": "string", "example": "scan-12345"},
					"status":    map[string]interface{}{"type": "string", "example": "running"},
					"startTime": map[string]interface{}{"type": "string", "format": "date-time"},
					"config":    map[string]interface{}{"$ref": "#/definitions/ScanConfig"},
					"results":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/DiscoveredServer"}},
					"error":     map[string]interface{}{"type": "string"},
				},
			},
			"DiscoveredServer": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":                  map[string]interface{}{"type": "string", "example": "server-123"},
					"name":                map[string]interface{}{"type": "string", "example": "MCP Example Server"},
					"address":             map[string]interface{}{"type": "string", "example": "http://192.168.1.100:8000"},
					"protocol":            map[string]interface{}{"type": "string", "example": "MCP"},
					"connection":          map[string]interface{}{"type": "string", "example": "StreamableHttp"},
					"status":              map[string]interface{}{"type": "string", "example": "healthy"},
					"lastSeen":            map[string]interface{}{"type": "string", "format": "date-time"},
					"metadata":            map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
					"vulnerability_score": map[string]interface{}{"type": "string", "example": "high"},
				},
			},
			"Plugin": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service_id":     map[string]interface{}{"type": "string"},
					"service_type":   map[string]interface{}{"type": "string", "enum": []string{"smartagents", "registry", "virtualmcp"}},
					"service_url":    map[string]interface{}{"type": "string"},
					"capabilities":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/ServiceCapability"}},
					"version":        map[string]interface{}{"type": "string"},
					"registered_at":  map[string]interface{}{"type": "string", "format": "date-time"},
					"last_heartbeat": map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"ServiceRegistration": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service_id":     map[string]interface{}{"type": "string"},
					"service_type":   map[string]interface{}{"type": "string", "enum": []string{"smartagents", "registry", "virtualmcp"}},
					"service_url":    map[string]interface{}{"type": "string"},
					"capabilities":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/ServiceCapability"}},
					"version":        map[string]interface{}{"type": "string"},
					"registered_at":  map[string]interface{}{"type": "string", "format": "date-time"},
					"last_heartbeat": map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"ServiceCapability": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":        map[string]interface{}{"type": "string", "example": "/v1/*"},
					"methods":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"GET", "POST"}},
					"description": map[string]interface{}{"type": "string"},
				},
			},
			"HealthStatus": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status":        map[string]interface{}{"type": "string", "enum": []string{"healthy", "unhealthy", "unknown"}},
					"message":       map[string]interface{}{"type": "string"},
					"last_checked":  map[string]interface{}{"type": "string", "format": "date-time"},
					"response_time": map[string]interface{}{"type": "integer", "description": "Response time in nanoseconds"},
				},
			},
			"AdapterAuthConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"required":    map[string]interface{}{"type": "boolean"},
					"type":        map[string]interface{}{"type": "string", "enum": []string{"bearer", "oauth", "basic", "apikey", "none"}},
					"bearerToken": map[string]interface{}{"$ref": "#/definitions/BearerTokenConfig"},
					"oauth":       map[string]interface{}{"$ref": "#/definitions/OAuthConfig"},
					"basic":       map[string]interface{}{"$ref": "#/definitions/BasicAuthConfig"},
					"apiKey":      map[string]interface{}{"$ref": "#/definitions/APIKeyConfig"},
				},
			},
			"BearerTokenConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"token":     map[string]interface{}{"type": "string"},
					"dynamic":   map[string]interface{}{"type": "boolean"},
					"expiresAt": map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"OAuthConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"clientId":     map[string]interface{}{"type": "string"},
					"clientSecret": map[string]interface{}{"type": "string"},
					"authUrl":      map[string]interface{}{"type": "string"},
					"tokenUrl":     map[string]interface{}{"type": "string"},
					"scopes":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"redirectUri":  map[string]interface{}{"type": "string"},
				},
			},
			"BasicAuthConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"username": map[string]interface{}{"type": "string"},
					"password": map[string]interface{}{"type": "string"},
				},
			},
			"APIKeyConfig": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key":      map[string]interface{}{"type": "string"},
					"location": map[string]interface{}{"type": "string", "enum": []string{"header", "query", "cookie"}},
					"name":     map[string]interface{}{"type": "string"},
				},
			},
			"MCPFunctionality": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"serverInfo":    map[string]interface{}{"$ref": "#/definitions/MCPServerInfo"},
					"tools":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/MCPTool"}},
					"prompts":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/MCPPrompt"}},
					"resources":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/MCPResource"}},
					"lastRefreshed": map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"MCPServerInfo": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":         map[string]interface{}{"type": "string"},
					"version":      map[string]interface{}{"type": "string"},
					"protocol":     map[string]interface{}{"type": "string"},
					"capabilities": map[string]interface{}{"type": "object"},
				},
			},
			"MCPTool": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":         map[string]interface{}{"type": "string"},
					"description":  map[string]interface{}{"type": "string"},
					"input_schema": map[string]interface{}{"type": "object"},
					"source_type":  map[string]interface{}{"type": "string", "enum": []string{"api", "database", "graphql"}},
					"config":       map[string]interface{}{"type": "object"},
				},
			},
			"MCPPrompt": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"arguments":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/definitions/MCPArgument"}},
				},
			},
			"MCPArgument": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"required":    map[string]interface{}{"type": "boolean"},
				},
			},
			"MCPResource": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"uri":         map[string]interface{}{"type": "string"},
					"name":        map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"mimeType":    map[string]interface{}{"type": "string"},
				},
			},
		},
		"securityDefinitions": map[string]interface{}{
			"apiKey": map[string]interface{}{
				"type":        "apiKey",
				"name":        "X-API-Key",
				"in":          "header",
				"description": "API key authentication",
			},
		},
		"security": []map[string]interface{}{
			{"apiKey": []interface{}{}},
		},
	}

	// Convert to JSON
	responseData, err := json.Marshal(swagger)
	if err != nil {
		logging.ProxyLogger.Error("Failed to marshal swagger JSON: %v", err)
		http.Error(w, "Swagger documentation not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseData)
}

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

// proxyToRegistry handles registry requests for unified service
func (s *Service) proxyToRegistry(w http.ResponseWriter, r *http.Request) {

	// For unified service, handle registry requests internally

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
