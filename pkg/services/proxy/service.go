package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
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

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", handler.HandleMCP)
	mux.HandleFunc("/mcp/tools", handler.HandleToolsList)
	mux.HandleFunc("/mcp/tools/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			handler.HandleToolCall(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/mcp/resources", handler.HandleResourcesList)
	mux.HandleFunc("/mcp/resources/", handler.HandleResourceRead)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/docs", s.handleDocs)
	mux.HandleFunc("/swagger.json", s.handleSwaggerJSON)

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
                layout: "StandaloneLayout"
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
        "summary": "Browse Registry",
        "description": "Browse available MCP servers in the registry",
        "responses": {
          "200": {
            "description": "Registry browse results",
            "schema": {
              "type": "object",
              "properties": {
                "servers": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "id": {"type": "string"},
                      "name": {"type": "string"},
                      "description": {"type": "string"},
                      "endpoint": {"type": "string"},
                      "capabilities": {"type": "array", "items": {"type": "string"}}
                    }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/registry/{id}": {
      "get": {
        "tags": ["Registry"],
        "summary": "Get Registry Server",
        "description": "Get details of a specific MCP server from the registry",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "type": "string",
            "description": "Server ID"
          }
        ],
        "responses": {
          "200": {
            "description": "Server details",
            "schema": {
              "type": "object",
              "properties": {
                "id": {"type": "string"},
                "name": {"type": "string"},
                "description": {"type": "string"},
                "endpoint": {"type": "string"},
                "capabilities": {"type": "array", "items": {"type": "string"}}
              }
            }
          },
          "404": {
            "description": "Server not found"
          }
        }
      }
    },
    "/api/v1/registry/upload": {
      "post": {
        "tags": ["Registry"],
        "summary": "Upload Server to Registry",
        "description": "Upload a new MCP server configuration to the registry",
        "parameters": [
          {
            "in": "body",
            "name": "server",
            "description": "Server configuration",
            "required": true,
            "schema": {
              "type": "object",
              "properties": {
                "name": {"type": "string"},
                "description": {"type": "string"},
                "endpoint": {"type": "string"},
                "capabilities": {"type": "array", "items": {"type": "string"}}
              }
            }
          }
        ],
        "responses": {
          "201": {
            "description": "Server uploaded successfully"
          }
        }
      }
    },
    "/api/v1/registry/upload/bulk": {
      "post": {
        "tags": ["Registry"],
        "summary": "Bulk Upload Servers",
        "description": "Upload multiple MCP server configurations to the registry",
        "parameters": [
          {
            "in": "body",
            "name": "servers",
            "description": "Array of server configurations",
            "required": true,
            "schema": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "name": {"type": "string"},
                  "description": {"type": "string"},
                  "endpoint": {"type": "string"},
                  "capabilities": {"type": "array", "items": {"type": "string"}}
                }
              }
            }
          }
        ],
        "responses": {
          "201": {
            "description": "Servers uploaded successfully"
          }
        }
      }
    },
    "/api/v1/registry/sync/official": {
      "post": {
        "tags": ["Registry"],
        "summary": "Sync Official Registry",
        "description": "Synchronize with the official MCP server registry",
        "responses": {
          "200": {
            "description": "Sync completed successfully"
          }
        }
      }
    },
    "/api/v1/registry/sync/docker": {
      "post": {
        "tags": ["Registry"],
        "summary": "Sync Docker Registry",
        "description": "Synchronize with Docker MCP images registry",
        "responses": {
          "200": {
            "description": "Sync completed successfully"
          }
        }
      }
    },
    "/api/v1/scan": {
      "post": {
        "tags": ["Discovery"],
        "summary": "Start Network Scan",
        "description": "Start a network scan to discover MCP servers",
        "parameters": [
          {
            "in": "body",
            "name": "config",
            "description": "Scan configuration",
            "required": true,
            "schema": {
              "type": "object",
              "properties": {
                "cidr": {"type": "string", "example": "192.168.1.0/24"},
                "ports": {"type": "array", "items": {"type": "integer"}, "example": [8080, 8911]},
                "timeout": {"type": "string", "example": "30s"}
              }
            }
          }
        ],
        "responses": {
          "202": {
            "description": "Scan started",
            "schema": {
              "type": "object",
              "properties": {
                "scan_id": {"type": "string"}
              }
            }
          }
        }
      }
    },
    "/api/v1/scan/{id}": {
      "get": {
        "tags": ["Discovery"],
        "summary": "Get Scan Status",
        "description": "Get the status of a running or completed scan",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "type": "string",
            "description": "Scan ID"
          }
        ],
        "responses": {
          "200": {
            "description": "Scan status",
            "schema": {
              "type": "object",
              "properties": {
                "id": {"type": "string"},
                "status": {"type": "string", "enum": ["running", "completed", "failed"]},
                "progress": {"type": "number"},
                "results": {"type": "array", "items": {"type": "object"}}
              }
            }
          }
        }
      }
    },
    "/api/v1/servers": {
      "get": {
        "tags": ["Discovery"],
        "summary": "List Discovered Servers",
        "description": "Get a list of all discovered MCP servers",
        "responses": {
          "200": {
            "description": "List of discovered servers",
            "schema": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "address": {"type": "string"},
                  "port": {"type": "integer"},
                  "server_type": {"type": "string"},
                  "last_seen": {"type": "string", "format": "date-time"}
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/plugins": {
      "get": {
        "tags": ["Plugins"],
        "summary": "List Plugins",
        "description": "Get a list of all registered plugins",
        "responses": {
          "200": {
            "description": "List of plugins",
            "schema": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "id": {"type": "string"},
                  "name": {"type": "string"},
                  "description": {"type": "string"},
                  "endpoint": {"type": "string"},
                  "capabilities": {"type": "array", "items": {"type": "string"}},
                  "status": {"type": "string", "enum": ["active", "inactive", "error"]}
                }
              }
            }
          }
        }
      },
      "post": {
        "tags": ["Plugins"],
        "summary": "Register Plugin",
        "description": "Register a new plugin with the system",
        "parameters": [
          {
            "in": "body",
            "name": "plugin",
            "description": "Plugin configuration",
            "required": true,
            "schema": {
              "type": "object",
              "properties": {
                "name": {"type": "string"},
                "description": {"type": "string"},
                "endpoint": {"type": "string"},
                "capabilities": {"type": "array", "items": {"type": "string"}}
              }
            }
          }
        ],
        "responses": {
          "201": {
            "description": "Plugin registered successfully"
          }
        }
      }
    },
    "/api/v1/plugins/{id}": {
      "get": {
        "tags": ["Plugins"],
        "summary": "Get Plugin",
        "description": "Get details of a specific plugin",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "type": "string",
            "description": "Plugin ID"
          }
        ],
        "responses": {
          "200": {
            "description": "Plugin details",
            "schema": {
              "type": "object",
              "properties": {
                "id": {"type": "string"},
                "name": {"type": "string"},
                "description": {"type": "string"},
                "endpoint": {"type": "string"},
                "capabilities": {"type": "array", "items": {"type": "string"}},
                "status": {"type": "string"}
              }
            }
          },
          "404": {
            "description": "Plugin not found"
          }
        }
      },
      "delete": {
        "tags": ["Plugins"],
        "summary": "Unregister Plugin",
        "description": "Remove a plugin from the system",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "type": "string",
            "description": "Plugin ID"
          }
        ],
        "responses": {
          "204": {
            "description": "Plugin unregistered successfully"
          }
        }
      }
    },
    "/api/v1/health/{id}": {
      "get": {
        "tags": ["Plugins"],
        "summary": "Plugin Health Check",
        "description": "Check the health status of a specific plugin",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "required": true,
            "type": "string",
            "description": "Plugin ID"
          }
        ],
        "responses": {
          "200": {
            "description": "Plugin is healthy",
            "schema": {
              "type": "object",
              "properties": {
                "status": {"type": "string", "example": "healthy"},
                "timestamp": {"type": "string", "format": "date-time"}
              }
            }
          },
          "503": {
            "description": "Plugin is unhealthy"
          }
        }
      }
    }
  },
  "definitions": {},
  "securityDefinitions": {
    "bearerAuth": {
      "type": "apiKey",
      "name": "Authorization",
      "in": "header",
      "description": "Bearer token authentication (e.g., 'Bearer <token>')"
    }
  },
  "security": [
    {
      "bearerAuth": []
    }
  ]
}`
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(swaggerJSON))
}
