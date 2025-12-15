package main

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
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"suse-ai-up/pkg/logging"
	"suse-ai-up/pkg/middleware"
	proxysvc "suse-ai-up/pkg/services/proxy"
)

func main() {
	// Initialize global logging system
	logging.InitGlobalLoggers()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "uniproxy":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runUniproxy()
	case "health":
		runHealthServer()
	case "all":
		runAllServices()
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	default:
		logging.ProxyLogger.Error("Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}
}

func runUniproxy() {
	port := 8911    // Default port
	tlsPort := 3911 // Default TLS port

	// Read environment variables if set
	if envPort := os.Getenv("UNIPROXY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		}
	}
	if envTLSPort := os.Getenv("TLS_PORT"); envTLSPort != "" {
		if p, err := strconv.Atoi(envTLSPort); err == nil {
			tlsPort = p
		}
	}

	logging.ProxyLogger.Info("Starting SUSE AI Universal Proxy service...")
	logging.ProxyLogger.Info("HTTP Port: %d, HTTPS Port: %d", port, tlsPort)

	// Adapter handlers will be implemented in the proxy service

	config := &proxysvc.Config{
		Port:    port,
		TLSPort: tlsPort,
		AutoTLS: true, // Enable auto-generated TLS certificates
	}
	service := proxysvc.NewService(config)
	if err := service.Start(); err != nil {
		logging.ProxyLogger.Error("Failed to start uniproxy service: %v", err)
		os.Exit(1)
	}

	// Display service banner immediately after successful startup
	logging.ServiceBanner(logging.ServiceProxy, "MCP Proxy and Adapter Management", port, tlsPort)
	logging.ProxyLogger.Info("Press Ctrl+C to stop the service")

	// Wait for interrupt signal to gracefully shutdown the service
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logging.ShutdownBanner(logging.ServiceProxy)
	if err := service.Stop(); err != nil {
		logging.ProxyLogger.Error("Error during service shutdown: %v", err)
		os.Exit(1)
	}

	logging.ProxyLogger.Success("Service stopped gracefully")
}

func runHealthServer() {
	// Start only the health check server
	if err := startHealthCheckServer(nil); err != nil {
		fmt.Printf("Failed to start health server: %v\n", err)
		os.Exit(1)
	}

	// Keep the process running
	select {}
}

func runAllServices() {
	fmt.Println("Starting all SUSE AI Universal Proxy services...")

	// Service configurations
	services := []ServiceConfig{
		{Name: "uniproxy", Port: 8911, Cmd: []string{"./suse-ai-up", "uniproxy"}},
		{Name: "discovery", Port: 8912, Cmd: []string{"./suse-ai-up-discovery"}},
		{Name: "registry", Port: 8913, Cmd: []string{"./suse-ai-up-registry"}},
		{Name: "plugins", Port: 8914, Cmd: []string{"./suse-ai-up-plugins"}},
	}

	// Start all services
	var wg sync.WaitGroup
	processes := make(map[string]*os.Process)
	errors := make(chan error, len(services)+1) // +1 for health check server

	// Start health check server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := startHealthCheckServer(errors); err != nil {
			errors <- fmt.Errorf("failed to start health check server: %v", err)
		}
	}()

	// Start each service in a separate process
	for _, svc := range services {
		wg.Add(1)
		go func(service ServiceConfig) {
			defer wg.Done()
			if err := startServiceProcess(service, processes, errors); err != nil {
				errors <- fmt.Errorf("failed to start %s: %v", service.Name, err)
			}
		}(svc)
	}

	// Wait a bit for services to start
	time.Sleep(3 * time.Second)

	// Check if any services failed to start
	select {
	case err := <-errors:
		fmt.Printf("Failed to start services: %v\n", err)
		stopAllServices(processes)
		os.Exit(1)
	default:
		// Services started successfully
	}

	fmt.Println("All services started successfully!")
	fmt.Println("Uniproxy: http://localhost:8911 (HTTPS: https://localhost:3911)")
	fmt.Println("Discovery: http://localhost:8912 (HTTPS: https://localhost:38912)")
	fmt.Println("Registry: http://localhost:8913 (HTTPS: https://localhost:38913)")
	fmt.Println("Plugins: http://localhost:8914 (HTTPS: https://localhost:38914)")
	fmt.Println("Health Check: http://localhost:8911/health")
	fmt.Println("API Documentation: http://localhost:8911/docs (or https://localhost:3911/docs)")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop all services")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down all services...")

	stopAllServices(processes)
	wg.Wait()

	fmt.Println("All services stopped.")
}

type ServiceConfig struct {
	Name string
	Port int
	Cmd  []string
}

func startServiceProcess(svc ServiceConfig, processes map[string]*os.Process, errors chan<- error) error {
	// Create the command
	cmd := exec.Command(svc.Cmd[0], svc.Cmd[1:]...)

	// Inherit environment variables
	cmd.Env = os.Environ()

	// Set up prefixed output
	cmd.Stdout = &prefixedWriter{prefix: fmt.Sprintf("[%s] ", strings.ToUpper(svc.Name)), writer: os.Stdout}
	cmd.Stderr = &prefixedWriter{prefix: fmt.Sprintf("[%s] ", strings.ToUpper(svc.Name)), writer: os.Stderr}

	// Start the process
	if err := cmd.Start(); err != nil {
		return err
	}

	// Store the process
	processes[svc.Name] = cmd.Process

	// Wait for the process to finish (this will block until the process exits)
	go func() {
		if err := cmd.Wait(); err != nil {
			errors <- fmt.Errorf("%s service exited with error: %v", svc.Name, err)
		}
	}()

	// Give the service a moment to start
	time.Sleep(500 * time.Millisecond)

	return nil
}

// handleHealth handles health check requests
func handleHealth(w http.ResponseWriter, r *http.Request) {
	// Simple health response for the proxy health/docs server
	healthStatus := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "proxy",
		"message":   "API proxy server is running",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"%s","timestamp":"%s","service":"%s","message":"%s"}`,
		healthStatus["status"],
		healthStatus["timestamp"].(time.Time).Format(time.RFC3339),
		healthStatus["service"],
		healthStatus["message"])
}

// handleDocs serves the Swagger UI
func handleDocs(w http.ResponseWriter, r *http.Request) {
	// Get service ports from environment variables or use defaults
	proxyPort := os.Getenv("PROXY_PORT")
	if proxyPort == "" {
		proxyPort = "8911"
	}
	registryPort := os.Getenv("REGISTRY_PORT")
	if registryPort == "" {
		registryPort = "8913"
	}
	discoveryPort := os.Getenv("DISCOVERY_PORT")
	if discoveryPort == "" {
		discoveryPort = "8912"
	}
	pluginsPort := os.Getenv("PLUGINS_PORT")
	if pluginsPort == "" {
		pluginsPort = "8914"
	}

	swaggerHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>SUSE AI Universal Proxy API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
        .service-badge {
            font-family: monospace;
            text-transform: uppercase;
            background: #007bff;
            color: white;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 10px;
            font-weight: bold;
            margin-left: 8px;
        }
        .help-section {
            background: #e7f3ff;
            border: 1px solid #b3d9ff;
            border-radius: 5px;
            padding: 15px;
            margin: 20px 0;
        }
        .help-section h4 {
            margin: 0 0 10px 0;
            color: #0056b3;
        }
        .help-section ul {
            margin: 0;
            padding-left: 20px;
            color: #0056b3;
        }
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
                showServersDropdown: true,
                onComplete: function() {
                    // Simple service badge system
                    setTimeout(function() {
                        addServiceBadges();
                        addHelpSection();
                        expandServerDropdown();
                    }, 1000);
                }
            });
        };

        function expandServerDropdown() {
            // Ensure server dropdown is visible and expanded
            const serverWrapper = document.querySelector('.servers');
            if (serverWrapper) {
                serverWrapper.style.display = 'block';
                const title = serverWrapper.querySelector('label');
                if (title) {
                    title.click(); // Expand the server dropdown
                }
            }
        }

        // Service mapping for badges
        const serviceMap = {
            '/adapters': { name: 'Proxy', color: '#007bff' },
            '/registry': { name: 'Registry', color: '#28a745' },
            '/discovery': { name: 'Discovery', color: '#17a2b8' },
            '/plugins': { name: 'Plugins', color: '#ffc107' },
            'default': { name: 'Proxy', color: '#007bff' }
        };

        function getServiceInfo(path) {
            for (const prefix in serviceMap) {
                if (prefix !== 'default' && path.startsWith(prefix)) {
                    return serviceMap[prefix];
                }
            }
            return serviceMap.default;
        }

        function addServiceBadges() {
            const operations = document.querySelectorAll('.opblock');
            operations.forEach(function(op) {
                const pathElement = op.querySelector('.opblock-summary-path');
                if (!pathElement) return;

                const path = pathElement.textContent.trim();
                const serviceInfo = getServiceInfo(path);

                // Add service badge
                const badge = document.createElement('span');
                badge.className = 'service-badge';
                badge.textContent = serviceInfo.name;
                badge.style.cssText = 'background: ' + serviceInfo.color + '; color: white; padding: 2px 6px; border-radius: 3px; font-size: 10px; font-weight: bold; margin-left: 8px;';

                const summary = op.querySelector('.opblock-summary');
                if (summary && !summary.querySelector('.service-badge')) {
                    summary.appendChild(badge);
                }
            });
        }

        function addHelpSection() {
            setTimeout(function() {
                const info = document.querySelector('.info');
                if (info && !document.querySelector('.help-section')) {
                    const helpSection = document.createElement('div');
                    helpSection.className = 'help-section';
                    helpSection.innerHTML = '<h4>How to Use This API Documentation</h4><ul><li><strong>Service Badges:</strong> Each operation shows which service it belongs to</li><li><strong>Automatic Server Selection:</strong> Each operation automatically uses the correct server based on its path</li><li><strong>Try It Out:</strong> Click "Try it out" on any operation to test it</li></ul>';
                    info.appendChild(helpSection);
                }
            }, 1500);
        }
    </script>
</body>
</html>`)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(swaggerHTML))
}

// generateSwaggerSpec generates a Swagger spec for a specific service
// handleSwaggerJSON serves the Swagger JSON specification
func handleSwaggerJSON(w http.ResponseWriter, r *http.Request) {
	// Determine the host dynamically based on the request
	host := r.Host
	if host == "" {
		host = "192.168.64.17:8911"
	}

	// Get service ports from environment variables or use defaults
	proxyPort := os.Getenv("PROXY_PORT")
	if proxyPort == "" {
		proxyPort = "8911"
	}
	registryPort := os.Getenv("REGISTRY_PORT")
	if registryPort == "" {
		registryPort = "8913"
	}
	discoveryPort := os.Getenv("DISCOVERY_PORT")
	if discoveryPort == "" {
		discoveryPort = "8912"
	}
	pluginsPort := os.Getenv("PLUGINS_PORT")
	if pluginsPort == "" {
		pluginsPort = "8914"
	}

	// Extract hostname from request
	hostname := r.Host
	if colonIndex := strings.LastIndex(hostname, ":"); colonIndex != -1 {
		hostname = hostname[:colonIndex]
	}
	if hostname == "" {
		hostname = "192.168.64.17"
	}

	// Create the swagger spec as a Go map for easier manipulation
	swagger := map[string]interface{}{
		"swagger": "2.0",
		"info": map[string]interface{}{
			"title":       "SUSE AI Universal Proxy API",
			"description": "Complete API documentation for the SUSE AI Universal Proxy - A comprehensive MCP proxy system.\n\n**Service Ports:**\n- **Proxy**: Port " + proxyPort + " (MCP communication)\n- **Registry**: Port " + registryPort + " (Server management, adapters, users/groups)\n- **Discovery**: Port " + discoveryPort + " (Network scanning)\n- **Plugins**: Port " + pluginsPort + " (Plugin management)\n\n**Note**: Use the server dropdown to select the appropriate service for testing APIs.",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name":  "SUSE AI Team",
				"email": "ai@suse.com",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "http://" + hostname + ":" + proxyPort,
				"description": "Proxy Service - MCP Communication (Port " + proxyPort + ")",
			},
			{
				"url":         "http://" + hostname + ":" + registryPort,
				"description": "Registry Service - Server Management (Port " + registryPort + ")",
			},
			{
				"url":         "http://" + hostname + ":" + discoveryPort,
				"description": "Discovery Service - Network Scanning (Port " + discoveryPort + ")",
			},
			{
				"url":         "http://" + hostname + ":" + pluginsPort,
				"description": "Plugins Service - Plugin Management (Port " + pluginsPort + ")",
			},
		},
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
			{"name": "User Management", "description": "User account management (Registry Service - Port 8913)"},
			{"name": "Group Management", "description": "User group management (Registry Service - Port 8913)"},
			{"name": "Route Management", "description": "Server access route assignments (Registry Service - Port 8913)"},
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
					"summary":     "Browse MCP Servers",
					"description": "Get a list of all available MCP servers in the registry **(Registry Service - Port 8913)**",
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
			"/api/v1/users": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "List Users",
					"description": "Get a list of all users **(Registry Service - Port 8913)**",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of users",
							"schema": map[string]interface{}{
								"type":  "array",
								"items": map[string]interface{}{"$ref": "#/definitions/User"},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"post": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "Create User",
					"description": "Create a new user (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "user", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/CreateUserRequest"}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "User created",
							"schema":      map[string]interface{}{"$ref": "#/definitions/User"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/users/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "Get User",
					"description": "Get a specific user by ID (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "User details",
							"schema":      map[string]interface{}{"$ref": "#/definitions/User"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"put": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "Update User",
					"description": "Update an existing user (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
						{"name": "user", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/User"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "User updated",
							"schema":      map[string]interface{}{"$ref": "#/definitions/User"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"User Management"},
					"summary":     "Delete User",
					"description": "Delete a user (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "User deleted",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/groups": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "List Groups",
					"description": "Get a list of all groups (Registry Service - Port 8913)",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of groups",
							"schema": map[string]interface{}{
								"type":  "array",
								"items": map[string]interface{}{"$ref": "#/definitions/Group"},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Create Group",
					"description": "Create a new group (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "group", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/CreateGroupRequest"}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Group created",
							"schema":      map[string]interface{}{"$ref": "#/definitions/Group"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/groups/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Get Group",
					"description": "Get a specific group by ID (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Group details",
							"schema":      map[string]interface{}{"$ref": "#/definitions/Group"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"put": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Update Group",
					"description": "Update an existing group (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
						{"name": "group", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/Group"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Group updated",
							"schema":      map[string]interface{}{"$ref": "#/definitions/Group"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Delete Group",
					"description": "Delete a group (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "Group deleted",
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/groups/{id}/members": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Add User to Group",
					"description": "Add a user to a group (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
						{"name": "member", "in": "body", "required": true, "schema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"userId": map[string]interface{}{"type": "string", "example": "user123"},
							},
							"required": []string{"userId"},
						}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "User added to group",
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
			"/api/v1/groups/{id}/members/{userId}": map[string]interface{}{
				"delete": map[string]interface{}{
					"tags":        []string{"Group Management"},
					"summary":     "Remove User from Group",
					"description": "Remove a user from a group (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "type": "string"},
						{"name": "userId", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "User removed from group",
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
			"/api/v1/registry/{serverId}/routes": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Route Management"},
					"summary":     "List Route Assignments",
					"description": "Get route assignments for a server (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "serverId", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of route assignments",
							"schema": map[string]interface{}{
								"type":  "array",
								"items": map[string]interface{}{"$ref": "#/definitions/RouteAssignment"},
							},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"post": map[string]interface{}{
					"tags":        []string{"Route Management"},
					"summary":     "Create Route Assignment",
					"description": "Create a route assignment for a server (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "serverId", "in": "path", "required": true, "type": "string"},
						{"name": "assignment", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/CreateRouteAssignmentRequest"}},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Route assignment created",
							"schema":      map[string]interface{}{"$ref": "#/definitions/RouteAssignment"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
			},
			"/api/v1/registry/{serverId}/routes/{assignmentId}": map[string]interface{}{
				"put": map[string]interface{}{
					"tags":        []string{"Route Management"},
					"summary":     "Update Route Assignment",
					"description": "Update a route assignment for a server (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "serverId", "in": "path", "required": true, "type": "string"},
						{"name": "assignmentId", "in": "path", "required": true, "type": "string"},
						{"name": "assignment", "in": "body", "required": true, "schema": map[string]interface{}{"$ref": "#/definitions/CreateRouteAssignmentRequest"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Route assignment updated",
							"schema":      map[string]interface{}{"$ref": "#/definitions/RouteAssignment"},
						},
					},
					"security": []map[string]interface{}{{"apiKey": []interface{}{}}},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"Route Management"},
					"summary":     "Delete Route Assignment",
					"description": "Delete a route assignment for a server (Registry Service - Port 8913)",
					"parameters": []map[string]interface{}{
						{"name": "serverId", "in": "path", "required": true, "type": "string"},
						{"name": "assignmentId", "in": "path", "required": true, "type": "string"},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "Route assignment deleted",
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
			"User": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]interface{}{"type": "string", "example": "user123"},
					"name":      map[string]interface{}{"type": "string", "example": "John Doe"},
					"email":     map[string]interface{}{"type": "string", "example": "john@example.com"},
					"groups":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"mcp-users"}},
					"createdAt": map[string]interface{}{"type": "string", "format": "date-time"},
					"updatedAt": map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"Group": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":          map[string]interface{}{"type": "string", "example": "mcp-users"},
					"name":        map[string]interface{}{"type": "string", "example": "MCP Users"},
					"description": map[string]interface{}{"type": "string", "example": "Users with access to MCP servers"},
					"members":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"user123"}},
					"permissions": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"server:read"}},
					"createdAt":   map[string]interface{}{"type": "string", "format": "date-time"},
					"updatedAt":   map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"RouteAssignment": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":          map[string]interface{}{"type": "string", "example": "assignment-123"},
					"serverId":    map[string]interface{}{"type": "string", "example": "mcp-bugzilla"},
					"userIds":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"user123"}},
					"groupIds":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"mcp-users"}},
					"autoSpawn":   map[string]interface{}{"type": "boolean", "example": true},
					"permissions": map[string]interface{}{"type": "string", "example": "read"},
					"createdAt":   map[string]interface{}{"type": "string", "format": "date-time"},
					"updatedAt":   map[string]interface{}{"type": "string", "format": "date-time"},
				},
			},
			"CreateUserRequest": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":     map[string]interface{}{"type": "string", "example": "user123"},
					"name":   map[string]interface{}{"type": "string", "example": "John Doe"},
					"email":  map[string]interface{}{"type": "string", "example": "john@example.com"},
					"groups": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"mcp-users"}},
				},
				"required": []string{"id", "name", "email"},
			},
			"CreateGroupRequest": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":          map[string]interface{}{"type": "string", "example": "weather-team"},
					"name":        map[string]interface{}{"type": "string", "example": "Weather Team"},
					"description": map[string]interface{}{"type": "string", "example": "Team with access to weather APIs"},
					"permissions": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"server:weather-*"}},
				},
				"required": []string{"id", "name"},
			},
			"CreateRouteAssignmentRequest": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"userIds":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"user123"}},
					"groupIds":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "example": []string{"weather-team"}},
					"autoSpawn":   map[string]interface{}{"type": "boolean", "example": true},
					"permissions": map[string]interface{}{"type": "string", "example": "read"},
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
		log.Printf("Failed to marshal swagger JSON: %v", err)
		http.Error(w, "Swagger documentation not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseData)
}

func startHealthCheckServer(errors chan<- error) error {
	healthPort := 8915    // Default port (changed to avoid conflict with proxy)
	healthTLSPort := 3915 // Default TLS port

	// Read environment variables if set
	if envPort := os.Getenv("HEALTH_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			healthPort = p
		}
	}
	if envTLSPort := os.Getenv("HEALTH_TLS_PORT"); envTLSPort != "" {
		if p, err := strconv.Atoi(envTLSPort); err == nil {
			healthTLSPort = p
		}
	}

	// Simple health check server that checks all services and serves Swagger UI
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", middleware.CORSMiddleware(handleHealth))

	// Swagger UI endpoint
	mux.HandleFunc("/docs", middleware.CORSMiddleware(handleDocs))

	// Swagger JSON endpoint - use the complete spec from proxy service
	mux.HandleFunc("/swagger.json", middleware.CORSMiddleware(handleSwaggerJSON))

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", healthPort),
		Handler: mux,
	}

	go func() {
		fmt.Printf("[HEALTH] Health check and API docs HTTP server starting on port %d\n", healthPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server
	tlsConfig := &tls.Config{
		ServerName: "localhost",
	}

	// Generate self-signed certificate for health/docs server
	cert, err := generateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("failed to generate self-signed certificate for health server: %w", err)
	}
	tlsConfig.Certificates = []tls.Certificate{*cert}

	httpsServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", healthTLSPort),
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	go func() {
		fmt.Printf("[HEALTH] Health check and API docs HTTPS server starting on port %d\n", healthTLSPort)
		if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTPS server error: %v", err)
		}
	}()

	return nil
}

func checkServiceHealth(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return "healthy"
	}
	return "unhealthy"
}

// generateSelfSignedCert generates a self-signed certificate for development
func generateSelfSignedCert() (*tls.Certificate, error) {
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

func stopAllServices(processes map[string]*os.Process) {
	for name, process := range processes {
		if process != nil {
			fmt.Printf("Stopping %s service...\n", name)
			if err := process.Kill(); err != nil {
				fmt.Printf("Error stopping %s: %v\n", name, err)
			}
		}
	}
}

type prefixedWriter struct {
	prefix string
	writer *os.File
}

func (w *prefixedWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(string(p), "\n")
	for i, line := range lines {
		if line != "" {
			_, err = fmt.Fprintf(w.writer, "%s%s", w.prefix, line)
			if err != nil {
				return n, err
			}
			if i < len(lines)-1 { // Don't add newline after the last line if it was empty
				_, err = fmt.Fprintln(w.writer)
				if err != nil {
					return n, err
				}
			}
		}
	}
	return len(p), nil
}

func printUsage() {
	fmt.Println("SUSE AI Uniproxy")
	fmt.Println()
	fmt.Println("A comprehensive MCP proxy system with integrated registry, discovery, and plugin management.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  suse-ai-up <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  uniproxy   Comprehensive MCP proxy service (port 8911)")
	fmt.Println("  all        Start all services simultaneously")
	fmt.Println("  health     Start health check server")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  suse-ai-up uniproxy")
	fmt.Println("  suse-ai-up all")
	fmt.Println("  suse-ai-up health")
	fmt.Println()
	fmt.Println("For more information about a command, run:")
	fmt.Println("  suse-ai-up <command> --help")
}
