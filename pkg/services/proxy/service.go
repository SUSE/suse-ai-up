package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/proxy"
	"time"
)

// Service represents the proxy service
type Service struct {
	config *Config
	server *proxy.MCPProxyServer
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
		config: config,
	}
}

// Start starts the proxy service
func (s *Service) Start() error {
	log.Printf("Starting MCP Proxy service on port %d", s.config.Port)

	// Load proxy configuration
	config, err := s.loadProxyConfig()
	if err != nil {
		return fmt.Errorf("failed to load proxy config: %w", err)
	}

	// Create proxy server
	s.server = proxy.AsProxyFromConfig(config, "MCPProxy")

	// Create HTTP handler
	handler := proxy.NewMCPProxyHandler(s.server)

	// Setup routes with CORS middleware
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", middleware.CORSMiddleware(handler.HandleMCP))
	mux.HandleFunc("/mcp/tools", middleware.CORSMiddleware(handler.HandleToolsList))
	mux.HandleFunc("/mcp/tools/", middleware.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			handler.HandleToolCall(w, r)
		} else {
			http.NotFound(w, r)
		}
	}))
	mux.HandleFunc("/mcp/resources", middleware.CORSMiddleware(handler.HandleResourcesList))
	mux.HandleFunc("/mcp/resources/", middleware.CORSMiddleware(handler.HandleResourceRead))
	mux.HandleFunc("/health", middleware.CORSMiddleware(s.handleHealth))
	mux.HandleFunc("/docs", middleware.CORSMiddleware(s.handleDocs))
	mux.HandleFunc("/swagger.json", middleware.CORSMiddleware(s.handleSwaggerJSON))

	// Proxy routes for other services
	mux.HandleFunc("/api/v1/registry/", middleware.CORSMiddleware(s.proxyToRegistry))
	mux.HandleFunc("/api/v1/scan/", middleware.CORSMiddleware(s.proxyToDiscovery))
	mux.HandleFunc("/api/v1/servers", middleware.CORSMiddleware(s.proxyToDiscovery))
	mux.HandleFunc("/api/v1/plugins/", middleware.CORSMiddleware(s.proxyToPlugins))

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	// Start HTTP server in goroutine
	go func() {
		log.Printf("MCP Proxy HTTP server listening on port %d", s.config.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
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
				log.Printf("Failed to load TLS certificates: %v", err)
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
			log.Printf("Generated self-signed TLS certificate for proxy service")
		}

		if len(tlsConfig.Certificates) > 0 {
			httpsServer := &http.Server{
				Addr:      fmt.Sprintf(":%d", s.config.TLSPort),
				Handler:   mux,
				TLSConfig: tlsConfig,
			}

			go func() {
				log.Printf("MCP Proxy HTTPS server listening on port %d", s.config.TLSPort)
				if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					log.Printf("HTTPS server error: %v", err)
				}
			}()
		}
	}

	log.Printf("MCP Proxy service started successfully")
	// Keep the service running
	select {}
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
	log.Println("Stopping MCP Proxy service")
	return nil
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
	swaggerJSON := `{
  "swagger": "2.0",
  "info": {
    "title": "SUSE AI Universal Proxy API",
    "description": "Complete API documentation for the SUSE AI Universal Proxy - A comprehensive MCP proxy system with registry, discovery, and plugin management",
    "version": "1.0.0",
    "contact": {
      "name": "SUSE AI Team",
      "email": "ai@suse.com"
    }
  },
  "host": "localhost:8911",
  "basePath": "/",
  "schemes": ["http", "https"],
  "consumes": ["application/json"],
  "produces": ["application/json"],
  "tags": [
    {"name": "Proxy", "description": "MCP proxy endpoints"},
    {"name": "Registry", "description": "MCP server registry management"},
    {"name": "Discovery", "description": "Network discovery and server scanning"},
    {"name": "Plugins", "description": "Plugin management and registration"},
    {"name": "Health", "description": "Health check endpoints"}
  ],
  "paths": {
    "/health": {
      "get": {
        "tags": ["Health"],
        "summary": "Proxy Health Check",
        "description": "Check the health status of the proxy service",
        "responses": {
          "200": {
            "description": "Service is healthy",
            "schema": {
              "type": "object",
              "properties": {
                "service": {"type": "string", "example": "proxy"},
                "status": {"type": "string", "example": "healthy"},
                "timestamp": {"type": "string", "format": "date-time"}
              }
            }
          }
        }
      }
    },
    "/mcp": {
      "post": {
        "tags": ["Proxy"],
        "summary": "MCP JSON-RPC Endpoint",
        "description": "Main Model Context Protocol JSON-RPC endpoint for tool calls and resource access",
        "parameters": [
          {
            "in": "body",
            "name": "request",
            "description": "JSON-RPC 2.0 request",
            "required": true,
            "schema": {
              "type": "object",
              "properties": {
                "jsonrpc": {"type": "string", "example": "2.0"},
                "id": {"type": "integer", "example": 1},
                "method": {"type": "string", "example": "tools/call"},
                "params": {"type": "object"}
              }
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Successful MCP response",
            "schema": {
              "type": "object",
              "properties": {
                "jsonrpc": {"type": "string"},
                "id": {"type": "integer"},
                "result": {"type": "object"}
              }
            }
          }
        }
      }
    },
    "/mcp/tools": {
      "get": {
        "tags": ["Proxy"],
        "summary": "List Available Tools",
        "description": "Get a list of all available MCP tools",
        "responses": {
          "200": {
            "description": "List of tools",
            "schema": {
              "type": "object",
              "properties": {
                "tools": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "name": {"type": "string"},
                      "description": {"type": "string"},
                      "inputSchema": {"type": "object"}
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/mcp/resources": {
      "get": {
        "tags": ["Proxy"],
        "summary": "List Available Resources",
        "description": "Get a list of all available MCP resources",
        "responses": {
          "200": {
            "description": "List of resources",
            "schema": {
              "type": "object",
              "properties": {
                "resources": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "uri": {"type": "string"},
                      "name": {"type": "string"},
                      "description": {"type": "string"},
                      "mimeType": {"type": "string"}
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/registry/browse": {
      "get": {
        "tags": ["Registry"],
        "summary": "Browse MCP Server Registry",
        "description": "Get a filtered list of MCP servers from the registry",
        "parameters": [
          {"name": "q", "in": "query", "description": "Search query", "type": "string"},
          {"name": "transport", "in": "query", "description": "Transport type filter", "type": "string"},
          {"name": "registryType", "in": "query", "description": "Registry type filter", "type": "string"},
          {"name": "validationStatus", "in": "query", "description": "Validation status filter", "type": "string"}
        ],
        "responses": {
          "200": {"description": "List of MCP servers", "schema": {"type": "array", "items": {"$ref": "#/definitions/MCPServer"}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/registry/{id}": {
      "get": {
        "tags": ["Registry"],
        "summary": "Get MCP Server by ID",
        "description": "Retrieve a specific MCP server from the registry",
        "parameters": [{"name": "id", "in": "path", "required": true, "type": "string"}],
        "responses": {
          "200": {"description": "MCP server details", "schema": {"$ref": "#/definitions/MCPServer"}},
          "404": {"description": "Server not found"}
        },
        "security": [{"apiKey": []}]
      },
      "put": {
        "tags": ["Registry"],
        "summary": "Update MCP Server",
        "description": "Update an existing MCP server in the registry",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "type": "string"},
          {"name": "server", "in": "body", "required": true, "schema": {"$ref": "#/definitions/MCPServer"}}
        ],
        "responses": {
          "200": {"description": "Updated server", "schema": {"$ref": "#/definitions/MCPServer"}},
          "404": {"description": "Server not found"}
        },
        "security": [{"apiKey": []}]
      },
      "delete": {
        "tags": ["Registry"],
        "summary": "Delete MCP Server",
        "description": "Remove an MCP server from the registry",
        "parameters": [{"name": "id", "in": "path", "required": true, "type": "string"}],
        "responses": {
          "204": {"description": "Server deleted"},
          "404": {"description": "Server not found"}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/registry/upload": {
      "post": {
        "tags": ["Registry"],
        "summary": "Upload Single MCP Server",
        "description": "Add a single MCP server to the registry",
        "parameters": [{"name": "server", "in": "body", "required": true, "schema": {"$ref": "#/definitions/MCPServer"}}],
        "responses": {
          "201": {"description": "Server created", "schema": {"$ref": "#/definitions/MCPServer"}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/registry/upload/bulk": {
      "post": {
        "tags": ["Registry"],
        "summary": "Bulk Upload MCP Servers",
        "description": "Add multiple MCP servers to the registry",
        "parameters": [{"name": "servers", "in": "body", "required": true, "schema": {"type": "array", "items": {"$ref": "#/definitions/MCPServer"}}}],
        "responses": {
          "201": {"description": "Servers created", "schema": {"type": "array", "items": {"$ref": "#/definitions/MCPServer"}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/registry/sync/official": {
      "post": {
        "tags": ["Registry"],
        "summary": "Sync Official Registry",
        "description": "Trigger synchronization with the official MCP registry",
        "responses": {
          "200": {"description": "Sync started", "schema": {"type": "object", "properties": {"status": {"type": "string"}, "source": {"type": "string"}}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/registry/sync/docker": {
      "post": {
        "tags": ["Registry"],
        "summary": "Sync Docker Registry",
        "description": "Trigger synchronization with Docker MCP registry",
        "responses": {
          "200": {"description": "Sync started", "schema": {"type": "object", "properties": {"status": {"type": "string"}, "source": {"type": "string"}}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/scan": {
      "post": {
        "tags": ["Discovery"],
        "summary": "Start Network Scan",
        "description": "Initiate a network scan for MCP servers",
        "parameters": [{"name": "config", "in": "body", "required": true, "schema": {"$ref": "#/definitions/ScanConfig"}}],
        "responses": {
          "200": {"description": "Scan started", "schema": {"type": "object", "properties": {"scan_id": {"type": "string"}}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/scan/{id}": {
      "get": {
        "tags": ["Discovery"],
        "summary": "Get Scan Status",
        "description": "Check the status of a running or completed scan",
        "parameters": [{"name": "id", "in": "path", "required": true, "type": "string"}],
        "responses": {
          "200": {"description": "Scan status", "schema": {"$ref": "#/definitions/ScanJob"}},
          "404": {"description": "Scan not found"}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/servers": {
      "get": {
        "tags": ["Discovery"],
        "summary": "List Discovered Servers",
        "description": "Get a list of all discovered MCP servers",
        "responses": {
          "200": {"description": "List of discovered servers", "schema": {"type": "array", "items": {"$ref": "#/definitions/DiscoveredServer"}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/plugins": {
      "get": {
        "tags": ["Plugins"],
        "summary": "List Plugins",
        "description": "Get a list of all registered plugins",
        "responses": {
          "200": {"description": "List of plugins", "schema": {"type": "array", "items": {"$ref": "#/definitions/Plugin"}}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/plugins/register": {
      "post": {
        "tags": ["Plugins"],
        "summary": "Register Plugin",
        "description": "Register a new plugin",
        "parameters": [{"name": "plugin", "in": "body", "required": true, "schema": {"$ref": "#/definitions/Plugin"}}],
        "responses": {
          "201": {"description": "Plugin registered", "schema": {"$ref": "#/definitions/Plugin"}}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/plugins/{id}": {
      "get": {
        "tags": ["Plugins"],
        "summary": "Get Plugin by ID",
        "description": "Retrieve details of a specific plugin",
        "parameters": [{"name": "id", "in": "path", "required": true, "type": "string"}],
        "responses": {
          "200": {"description": "Plugin details", "schema": {"$ref": "#/definitions/Plugin"}},
          "404": {"description": "Plugin not found"}
        },
        "security": [{"apiKey": []}]
      },
      "put": {
        "tags": ["Plugins"],
        "summary": "Update Plugin",
        "description": "Update an existing plugin",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "type": "string"},
          {"name": "plugin", "in": "body", "required": true, "schema": {"$ref": "#/definitions/Plugin"}}
        ],
        "responses": {
          "200": {"description": "Plugin updated", "schema": {"$ref": "#/definitions/Plugin"}},
          "404": {"description": "Plugin not found"}
        },
        "security": [{"apiKey": []}]
      },
      "delete": {
        "tags": ["Plugins"],
        "summary": "Unregister Plugin",
        "description": "Remove a plugin from the registry",
        "parameters": [{"name": "id", "in": "path", "required": true, "type": "string"}],
        "responses": {
          "204": {"description": "Plugin unregistered"},
          "404": {"description": "Plugin not found"}
        },
        "security": [{"apiKey": []}]
      }
    },
    "/api/v1/health/{pluginId}": {
      "get": {
        "tags": ["Plugins"],
        "summary": "Get Plugin Health",
        "description": "Check the health status of a specific plugin",
        "parameters": [{"name": "pluginId", "in": "path", "required": true, "type": "string"}],
        "responses": {
          "200": {"description": "Plugin health status", "schema": {"$ref": "#/definitions/HealthStatus"}},
          "404": {"description": "Plugin not found"}
        },
        "security": [{"apiKey": []}]
      }
    }
  },
  "definitions": {
    "MCPServer": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "name": {"type": "string"},
        "description": {"type": "string"},
        "packages": {"type": "array", "items": {"$ref": "#/definitions/Package"}},
        "validationStatus": {"type": "string"},
        "discoveredAt": {"type": "string", "format": "date-time"},
        "meta": {"type": "object"}
      }
    },
    "Package": {
      "type": "object",
      "properties": {
        "name": {"type": "string"},
        "version": {"type": "string"},
        "transport": {"$ref": "#/definitions/Transport"},
        "registryType": {"type": "string"}
      }
    },
    "Transport": {
      "type": "object",
      "properties": {
        "type": {"type": "string"},
        "config": {"type": "object"}
      }
    },
    "ScanConfig": {
      "type": "object",
      "properties": {
        "networks": {"type": "array", "items": {"type": "string"}},
        "ports": {"type": "array", "items": {"type": "integer"}},
        "timeout": {"type": "integer"},
        "maxConcurrency": {"type": "integer"}
      }
    },
    "ScanJob": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "config": {"$ref": "#/definitions/ScanConfig"},
        "startTime": {"type": "string", "format": "date-time"},
        "status": {"type": "string"},
        "results": {"type": "array", "items": {"$ref": "#/definitions/DiscoveredServer"}},
        "error": {"type": "string"}
      }
    },
    "DiscoveredServer": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "address": {"type": "string"},
        "port": {"type": "integer"},
        "protocol": {"type": "string"},
        "discoveredAt": {"type": "string", "format": "date-time"},
        "lastSeen": {"type": "string", "format": "date-time"}
      }
    },
    "Plugin": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "name": {"type": "string"},
        "description": {"type": "string"},
        "version": {"type": "string"},
        "status": {"type": "string"},
        "config": {"type": "object"}
      }
    },
    "HealthStatus": {
      "type": "object",
      "properties": {
        "status": {"type": "string"},
        "lastChecked": {"type": "string", "format": "date-time"},
        "responseTime": {"type": "integer"},
        "error": {"type": "string"}
      }
    }
  },
  "securityDefinitions": {
    "apiKey": {
      "type": "apiKey",
      "name": "X-API-Key",
      "in": "header",
      "description": "API key authentication"
    }
  },
  "security": [
    {
      "apiKey": []
    }
  ]
}`
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(swaggerJSON))
}

// proxyToRegistry forwards requests to the registry service
func (s *Service) proxyToRegistry(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "http://suse-ai-up-service.suse-ai-up.svc.cluster.local:8913", "")
}

// proxyToDiscovery forwards requests to the discovery service
func (s *Service) proxyToDiscovery(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "http://suse-ai-up-service.suse-ai-up.svc.cluster.local:8912", "")
}

// proxyToPlugins forwards requests to the plugins service
func (s *Service) proxyToPlugins(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "http://suse-ai-up-service.suse-ai-up.svc.cluster.local:8914", "")
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

	log.Printf("Proxying request: %s %s -> %s", r.Method, r.URL.Path, targetURL)

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
