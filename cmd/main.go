package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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

	"suse-ai-up/pkg/services/discovery"
	"suse-ai-up/pkg/services/plugins"
	"suse-ai-up/pkg/services/proxy"
	"suse-ai-up/pkg/services/registry"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "proxy":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runProxy()
	case "discovery":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runDiscovery()
	case "registry":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runRegistry()
	case "plugins":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runPlugins()
	case "health":
		runHealthServer()
	case "all":
		runAllServices()
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func runProxy() {
	port := 8911    // Default port
	tlsPort := 3911 // Default TLS port

	// Read environment variables if set
	if envPort := os.Getenv("PROXY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		}
	}
	if envTLSPort := os.Getenv("TLS_PORT"); envTLSPort != "" {
		if p, err := strconv.Atoi(envTLSPort); err == nil {
			tlsPort = p
		}
	}

	config := &proxy.Config{
		Port:    port,
		TLSPort: tlsPort,
		AutoTLS: true, // Enable auto-generated TLS certificates
	}
	service := proxy.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start proxy service: %v\n", err)
		os.Exit(1)
	}
}

func runDiscovery() {
	config := &discovery.Config{
		Port:           8912,
		TLSPort:        38912,           // HTTPS port
		DefaultTimeout: 30 * 1000000000, // 30 seconds in nanoseconds
		MaxConcurrency: 10,
		ExcludeProxy:   true,
		AutoTLS:        true, // Enable auto-generated TLS certificates
	}

	service := discovery.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start discovery service: %v\n", err)
		os.Exit(1)
	}
}

func runRegistry() {
	config := &registry.Config{
		Port:              8913,
		TLSPort:           38913, // HTTPS port
		RemoteServersFile: "config/remote_mcp_servers.json",
		AutoTLS:           true, // Enable auto-generated TLS certificates
	}

	service := registry.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start registry service: %v\n", err)
		os.Exit(1)
	}
}

func runPlugins() {
	config := &plugins.Config{
		Port:           8914,
		TLSPort:        38914,           // HTTPS port
		HealthInterval: 30 * 1000000000, // 30 seconds in nanoseconds
		AutoTLS:        true,            // Enable auto-generated TLS certificates
	}

	service := plugins.NewService(config)
	if err := service.Start(); err != nil {
		fmt.Printf("Failed to start plugins service: %v\n", err)
		os.Exit(1)
	}
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
		{Name: "proxy", Port: 8911, Cmd: []string{"./suse-ai-up", "proxy"}},
		{Name: "discovery", Port: 8912, Cmd: []string{"./suse-ai-up", "discovery"}},
		{Name: "registry", Port: 8913, Cmd: []string{"./suse-ai-up", "registry"}},
		{Name: "plugins", Port: 8914, Cmd: []string{"./suse-ai-up", "plugins"}},
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
	fmt.Println("Proxy: http://localhost:8911 (HTTPS: https://localhost:3911)")
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

func startHealthCheckServer(errors chan<- error) error {
	// Simple health check server that checks all services and serves Swagger UI
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Simple health response for the docs server
		healthStatus := map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now(),
			"service":   "docs",
			"message":   "API documentation server is running",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"%s","timestamp":"%s","service":"%s","message":"%s"}`,
			healthStatus["status"],
			healthStatus["timestamp"].(time.Time).Format(time.RFC3339),
			healthStatus["service"],
			healthStatus["message"])
	})

	// Swagger UI endpoint
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// Swagger JSON endpoint
	mux.HandleFunc("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Determine host: query parameter > environment variable > request host > default
		host := r.URL.Query().Get("host")
		if host == "" {
			host = os.Getenv("SWAGGER_HOST")
			if host == "" {
				host = r.Host
				if host == "" {
					host = "localhost:8911"
				}
			}
		}

		swaggerJSON := fmt.Sprintf(`{
  "swagger": "2.0",
  "info": {
    "title": "SUSE AI Universal Proxy API",
    "description": "API documentation for the SUSE AI Universal Proxy - A comprehensive MCP proxy system. Customize host via ?host= parameter or SWAGGER_HOST env var.",
    "version": "1.0.0",
    "contact": {
      "name": "SUSE AI Team",
      "email": "ai@suse.com"
    }
  },
  "host": "%s",
  "basePath": "/",
  "schemes": ["http", "https"],
  "consumes": ["application/json"],
  "produces": ["application/json"],
  "tags": [
    {"name": "Health", "description": "Health check endpoints"},
    {"name": "MCP", "description": "Model Context Protocol endpoints"},
    {"name": "Registry", "description": "MCP server registry management"},
    {"name": "Adapters", "description": "Adapter management"},
    {"name": "Discovery", "description": "Network discovery and scanning"},
    {"name": "Plugins", "description": "Plugin management"}
  ],
  "paths": {
    "/health": {
      "get": {
        "tags": ["Health"],
        "summary": "Health Check",
        "description": "Check the health status of all services",
        "responses": {
          "200": {
            "description": "Services are healthy",
            "schema": {
              "type": "object",
              "properties": {
                "status": {"type": "string"},
                "services": {"type": "object"}
              }
            }
          }
        }
      }
    },
    "/mcp": {
      "post": {
        "tags": ["MCP"],
        "summary": "MCP JSON-RPC",
        "description": "Model Context Protocol JSON-RPC endpoint",
        "parameters": [
          {
            "in": "body",
            "name": "request",
            "required": true,
            "schema": {"type": "object"}
          }
        ],
        "responses": {
          "200": {"description": "Success", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/mcp": {
      "post": {
        "tags": ["MCP"],
        "summary": "MCP JSON-RPC (API v1)",
        "description": "API v1 compatible MCP endpoint",
        "parameters": [
          {
            "in": "body",
            "name": "request",
            "required": true,
            "schema": {"type": "object"}
          }
        ],
        "responses": {
          "200": {"description": "Success", "schema": {"type": "object"}}
        }
      }
    },
    "/mcp/tools": {
      "get": {
        "tags": ["MCP"],
        "summary": "List MCP Tools",
        "description": "Get available MCP tools",
        "responses": {
          "200": {
            "description": "List of tools",
            "schema": {"type": "object"}
          }
        }
      }
    },
    "/api/v1/mcp/tools": {
      "get": {
        "tags": ["MCP"],
        "summary": "List MCP Tools (API v1)",
        "description": "API v1 compatible tools endpoint",
        "responses": {
          "200": {"description": "List of tools", "schema": {"type": "object"}}
        }
      }
    },
    "/mcp/resources": {
      "get": {
        "tags": ["MCP"],
        "summary": "List MCP Resources",
        "description": "Get available MCP resources",
        "responses": {
          "200": {"description": "List of resources", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/mcp/resources": {
      "get": {
        "tags": ["MCP"],
        "summary": "List MCP Resources (API v1)",
        "description": "API v1 compatible resources endpoint",
        "responses": {
          "200": {"description": "List of resources", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/registry/browse": {
      "get": {
        "tags": ["Registry"],
        "summary": "Browse Registry",
        "description": "Browse MCP servers in registry (proxied to registry service)",
        "responses": {
          "200": {"description": "List of servers", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/registry/upload": {
      "post": {
        "tags": ["Registry"],
        "summary": "Upload Server",
        "description": "Upload MCP server to registry (proxied to registry service)",
        "parameters": [
          {
            "in": "body",
            "name": "server",
            "required": true,
            "schema": {"type": "object"}
          }
        ],
        "responses": {
          "201": {"description": "Server uploaded", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/adapters": {
      "get": {
        "tags": ["Adapters"],
        "summary": "List Adapters",
        "description": "Get registered adapters (proxied to registry service)",
        "responses": {
          "200": {"description": "List of adapters", "schema": {"type": "object"}}
        }
      },
      "post": {
        "tags": ["Adapters"],
        "summary": "Create Adapter",
        "description": "Create new adapter (proxied to registry service)",
        "parameters": [
          {
            "in": "body",
            "name": "adapter",
            "required": true,
            "schema": {"type": "object"}
          }
        ],
        "responses": {
          "201": {"description": "Adapter created", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/scan": {
      "post": {
        "tags": ["Discovery"],
        "summary": "Start Scan",
        "description": "Start network scan for MCP servers (proxied to discovery service)",
        "parameters": [
          {
            "in": "body",
            "name": "config",
            "required": true,
            "schema": {"type": "object"}
          }
        ],
        "responses": {
          "202": {"description": "Scan started", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/servers": {
      "get": {
        "tags": ["Discovery"],
        "summary": "List Servers",
        "description": "Get discovered MCP servers (proxied to discovery service)",
        "responses": {
          "200": {"description": "List of servers", "schema": {"type": "object"}}
        }
      }
    },
    "/api/v1/plugins": {
      "get": {
        "tags": ["Plugins"],
        "summary": "List Plugins",
        "description": "Get registered plugins (proxied to plugins service)",
        "responses": {
          "200": {"description": "List of plugins", "schema": {"type": "object"}}
        }
      }
    }
  }
}`, host)
		w.Write([]byte(swaggerJSON))
	})

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    ":8911",
		Handler: mux,
	}

	go func() {
		fmt.Println("[HEALTH] Health check and API docs HTTP server starting on port 8911")
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
		Addr:      ":3911",
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	go func() {
		fmt.Println("[HEALTH] Health check and API docs HTTPS server starting on port 3911")
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
	fmt.Println("SUSE AI Universal Proxy")
	fmt.Println()
	fmt.Println("A modular system for MCP proxying, service discovery, registry management, and plugin handling.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  suse-ai-up <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  proxy      MCP proxy system (HTTP-based MCP server proxying)")
	fmt.Println("  discovery  Network discovery service (CIDR scanning, auto-registration)")
	fmt.Println("  registry   MCP server registry (server management, search, validation)")
	fmt.Println("  plugins    Plugin management (third-party service registration)")
	fmt.Println("  all        Start all services simultaneously")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  suse-ai-up proxy --port 8080")
	fmt.Println("  suse-ai-up discovery --config config/discovery.yaml")
	fmt.Println("  suse-ai-up registry --port 8913")
	fmt.Println("  suse-ai-up plugins --port 8914")
	fmt.Println()
	fmt.Println("For more information about a command, run:")
	fmt.Println("  suse-ai-up <command> --help")
}
