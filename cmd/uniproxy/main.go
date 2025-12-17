package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"suse-ai-up/internal/config"
	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/logging"
	"suse-ai-up/pkg/mcp"

	// "suse-ai-up/pkg/middleware"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/plugins"
	"suse-ai-up/pkg/proxy"
	"suse-ai-up/pkg/scanner"
	"suse-ai-up/pkg/services"
	adaptersvc "suse-ai-up/pkg/services/adapters"
	"suse-ai-up/pkg/session"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

//go:generate swag init -g main.go -o ../../docs

// @title SUSE AI Uniproxy API
// @version 1.0
// @description Comprehensive MCP proxy with discovery, registry, and deployment capabilities
// @host localhost:8911
// @BasePath /
// @schemes http

// generateID generates a random hex ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// loadRegistryFromFile loads MCP servers from config/mcp_registry.yaml
func loadRegistryFromFile(registryManager *handlers.DefaultRegistryManager) {
	log.Printf("DEBUG: loadRegistryFromFile called")
	registryFile := "config/mcp_registry.yaml"
	data, err := os.ReadFile(registryFile)
	if err != nil {
		log.Printf("Warning: Could not read registry file %s: %v", registryFile, err)
		return
	}

	var servers []map[string]interface{}
	if err := yaml.Unmarshal(data, &servers); err != nil {
		log.Printf("Warning: Could not parse registry file %s: %v", registryFile, err)
		return
	}

	log.Printf("Loading %d MCP servers from %s", len(servers), registryFile)

	var mcpServers []*models.MCPServer
	log.Printf("DEBUG: Processing %d servers from YAML", len(servers))
	for i, serverData := range servers {
		log.Printf("DEBUG: Server %d data: %+v", i, serverData)
		// Convert to models.MCPServer format
		server := &models.MCPServer{}

		if name, ok := serverData["name"].(string); ok {
			server.ID = name
			server.Name = name
			log.Printf("DEBUG: Server name/ID: %s", name)
		} else {
			log.Printf("Warning: Server missing name field, skipping: %+v", serverData)
			continue
		}

		if desc, ok := serverData["description"].(string); ok {
			server.Description = desc
		}

		if image, ok := serverData["image"].(string); ok {
			server.Packages = []models.Package{
				{
					Identifier: image,
					Transport: models.Transport{
						Type: "stdio",
					},
				},
			}
		}

		// Handle meta field
		if meta, ok := serverData["meta"].(map[string]interface{}); ok {
			server.Meta = meta
			log.Printf("DEBUG: Loaded meta for server %s: %+v", server.Name, meta)
		} else {
			server.Meta = make(map[string]interface{})
			log.Printf("DEBUG: No meta field found for server %s", server.Name)
		}

		// Set source to distinguish from external registries
		server.Meta["source"] = "yaml"

		mcpServers = append(mcpServers, server)
	}

	// Use the registry manager to upload all servers
	log.Printf("DEBUG: Uploading %d MCP servers to registry", len(mcpServers))
	if err := registryManager.UploadRegistryEntries(mcpServers); err != nil {
		log.Printf("Warning: Could not upload registry entries: %v", err)
		return
	}
	log.Printf("DEBUG: Successfully uploaded MCP servers")

	log.Printf("Successfully loaded MCP registry from %s", registryFile)
}

// isVirtualMCPAdapter checks if an adapter is configured for VirtualMCP
func isVirtualMCPAdapter(data *models.AdapterData) bool {
	log.Printf("Checking adapter for VirtualMCP: %s (type: %s)", data.Name, data.ConnectionType)

	// Check if any MCP server config references a VirtualMCP package
	for serverName, serverConfig := range data.MCPClientConfig.MCPServers {
		log.Printf("Checking server config: %s", serverName)
		log.Printf("  Command: %s", serverConfig.Command)
		log.Printf("  Args: %v", serverConfig.Args)

		// Check command
		cmdLower := strings.ToLower(serverConfig.Command)
		if strings.Contains(cmdLower, "@suse") ||
			strings.Contains(cmdLower, "virtual-mcp") ||
			strings.Contains(cmdLower, "virtualmcp") ||
			strings.Contains(cmdLower, "virtual") {
			log.Printf("Detected VirtualMCP package in command: %s", serverConfig.Command)
			return true
		}

		// Check all args
		for i, arg := range serverConfig.Args {
			log.Printf("  Arg[%d]: %s", i, arg)
			argLower := strings.ToLower(arg)
			if strings.Contains(argLower, "@suse") ||
				strings.Contains(argLower, "virtual-mcp") ||
				strings.Contains(argLower, "virtualmcp") ||
				strings.Contains(argLower, "virtual") {
				log.Printf("Detected VirtualMCP package in args: %s", arg)
				return true
			}
		}

		// Check env vars
		for envKey, envValue := range serverConfig.Env {
			log.Printf("  Env[%s]: %s", envKey, envValue)
			envLower := strings.ToLower(envValue)
			if strings.Contains(envLower, "@suse") ||
				strings.Contains(envLower, "virtual-mcp") ||
				strings.Contains(envLower, "virtualmcp") ||
				strings.Contains(envLower, "virtual") {
				log.Printf("Detected VirtualMCP package in env: %s", envValue)
				return true
			}
		}
	}

	// Check adapter metadata for VirtualMCP indicators
	nameLower := strings.ToLower(data.Name)
	descLower := strings.ToLower(data.Description)
	if strings.Contains(nameLower, "virtual") ||
		strings.Contains(descLower, "virtual") ||
		strings.Contains(nameLower, "suse") ||
		strings.Contains(descLower, "suse") ||
		strings.Contains(nameLower, "mcp") ||
		strings.Contains(descLower, "mcp") {
		log.Printf("Detected VirtualMCP by name/description: %s - %s", data.Name, data.Description)
		return true
	}

	log.Printf("Adapter %s is not VirtualMCP", data.Name)
	return false
}

// reconfigureVirtualMCPAdapter reconfigures a VirtualMCP adapter to run VirtualMCP server locally via stdio
func reconfigureVirtualMCPAdapter(data *models.AdapterData) {
	log.Printf("Reconfiguring VirtualMCP adapter: %s", data.Name)
	log.Printf("Original connection type: %s", data.ConnectionType)

	// Skip reconfiguration if this is already an HTTP-based VirtualMCP adapter (from registry spawning)
	if data.ConnectionType == models.ConnectionTypeStreamableHttp {
		log.Printf("Skipping reconfiguration for HTTP-based VirtualMCP adapter: %s", data.Name)
		return
	}

	// Get API base URL from adapter configuration or default
	apiBaseUrl := data.ApiBaseUrl
	if apiBaseUrl == "" {
		apiBaseUrl = "http://localhost:8000"
	}

	// Get tools config from Tools field or default to empty
	toolsConfig := "[]"
	if len(data.Tools) > 0 {
		if toolsJSON, err := json.Marshal(data.Tools); err == nil {
			toolsConfig = string(toolsJSON)
		}
	}

	// Keep connection type as LocalStdio for stdio communication
	data.ConnectionType = models.ConnectionTypeLocalStdio

	// Modify MCPClientConfig to run VirtualMCP server locally via stdio
	log.Printf("Modifying MCPClientConfig for VirtualMCP adapter")
	data.MCPClientConfig = models.MCPClientConfig{
		MCPServers: map[string]models.MCPServerConfig{
			"virtualmcp": {
				Command: "tsx",
				Args:    []string{"templates/virtualmcp-server.ts"}, // No --transport flag = stdio mode
				Env: map[string]string{
					"SERVER_NAME":  data.Name,
					"TOOLS_CONFIG": toolsConfig, // Use tools from Tools field
					"API_BASE_URL": apiBaseUrl,  // Use configured API base URL
				},
			},
		},
	}

	// Add authentication for VirtualMCP (required for MCP protocol)
	data.Authentication = &models.AdapterAuthConfig{
		Required: true,
		Type:     "bearer",
		BearerToken: &models.BearerTokenConfig{
			Token:     "virtualmcp-token",
			Dynamic:   false,
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour), // Long expiry
		},
	}

	// Update description
	data.Description = fmt.Sprintf("VirtualMCP adapter: %s", data.Description)

	log.Printf("Reconfigured VirtualMCP adapter: connectionType=%s, tools=%s, apiBaseUrl=%s",
		data.ConnectionType, toolsConfig, apiBaseUrl)
}

// initOTEL initializes OpenTelemetry tracing and metrics
func initOTEL(ctx context.Context, cfg *config.Config) error {
	log.Printf("DEBUG: initOTEL called")

	// Create OTLP trace exporter
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OtelEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create OTLP metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OtelEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("suse-ai-up"),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace provider
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	// Create meter provider
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	log.Println("OpenTelemetry initialized successfully")
	return nil
}

// RunUniproxy starts the SUSE AI Uniproxy service
func RunUniproxy() {
	log.Printf("MAIN FUNCTION STARTED")
	// Load configuration
	cfg := config.LoadConfig()
	log.Printf("Config loaded: Port=%s", cfg.Port)

	// Update swagger host dynamically
	// docs.SwaggerInfo.Host = cfg.GetSwaggerHost() // Not used in current implementation

	// Initialize OpenTelemetry (if enabled)
	if cfg.OtelEnabled {
		ctx := context.Background()
		if err := initOTEL(ctx, cfg); err != nil {
			log.Printf("Failed to initialize OpenTelemetry: %v", err)
			// Continue without OTEL rather than failing
		}
	}

	// Initialize Gin
	if cfg.AuthMode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// Custom recovery middleware to handle panics properly
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if err, ok := recovered.(string); ok {
			log.Printf("Panic recovered: %s", err)
		} else {
			log.Printf("Panic recovered: %v", recovered)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Internal server error",
			"message": "An unexpected error occurred",
		})
	}))

	// Add OTEL Gin middleware (if enabled)
	if cfg.OtelEnabled {
		r.Use(otelgin.Middleware("suse-ai-up"))
	}

	// Initialize stores
	// Use file-based adapter store for persistence
	adapterStore := clients.NewFileAdapterStore("/tmp/adapters.json")
	tokenManager, err := auth.NewTokenManager("mcp-gateway")
	if err != nil {
		log.Fatalf("Failed to create token manager: %v", err)
	}

	// Initialize user/group system with admin defaults
	userStore := clients.NewInMemoryUserStore()
	groupStore := clients.NewInMemoryGroupStore()
	userGroupService := services.NewUserGroupService(userStore, groupStore)

	// Initialize default groups and admin user
	if err := userGroupService.InitializeDefaultGroups(context.Background()); err != nil {
		log.Printf("Warning: Failed to initialize default groups: %v", err)
	}

	// Create default admin user if not exists
	adminUser := models.User{
		ID:     "admin",
		Name:   "System Administrator",
		Email:  "admin@suse.ai",
		Groups: []string{"mcp-admins"},
	}
	if _, err := userGroupService.GetUser(context.Background(), "admin"); err != nil {
		if err := userGroupService.CreateUser(context.Background(), adminUser); err != nil {
			log.Printf("Warning: Failed to create admin user: %v", err)
		}
	}

	// Initialize MCP components
	capabilityCache := mcp.NewCapabilityCache()
	cache := mcp.NewMCPCache(nil)     // Use default config
	monitor := mcp.NewMCPMonitor(nil) // Use default config
	sessionStore := session.NewInMemorySessionStore()
	protocolHandler := mcp.NewProtocolHandler(sessionStore, capabilityCache)
	messageRouter := mcp.NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)

	// Initialize stdio proxy plugin for local stdio adapters
	stdioProxy := proxy.NewLocalStdioProxyPlugin()
	log.Printf("stdioProxy initialized: %v", stdioProxy != nil)

	// Initialize stdio-to-HTTP adapter
	stdioToHTTPAdapter := proxy.NewStdioToHTTPAdapter(stdioProxy, messageRouter, sessionStore, protocolHandler, capabilityCache)
	log.Printf("stdioToHTTPAdapter initialized: %v", stdioToHTTPAdapter != nil)

	// Initialize remote HTTP proxy adapter
	remoteHTTPAdapter := proxy.NewRemoteHTTPProxyAdapter(sessionStore, messageRouter, protocolHandler, capabilityCache)
	log.Printf("remoteHTTPAdapter initialized: %v", remoteHTTPAdapter != nil)

	// Initialize remote HTTP proxy plugin
	remoteHTTPPlugin := proxy.NewRemoteHttpProxyPlugin()
	log.Printf("remoteHTTPPlugin initialized: %v", remoteHTTPPlugin != nil)

	// Initialize Kubernetes client and SidecarManager
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get in-cluster config, trying kubeconfig: %v", err)
		// Try to load from kubeconfig file
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		if err != nil {
			log.Printf("Failed to get Kubernetes config: %v", err)
			log.Printf("Sidecar functionality will not be available")
		}
	}

	var sidecarManager *proxy.SidecarManager
	if kubeConfig != nil {
		kubeClient, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			log.Printf("Failed to create Kubernetes client: %v", err)
		} else {
			sidecarManager = proxy.NewSidecarManager(kubeClient, "default")
			log.Printf("SidecarManager initialized successfully")
		}
	}

	// Initialize discovery components
	scanConfig := &models.ScanConfig{
		ScanRanges:    []string{"192.168.1.0/24"},
		Ports:         []string{"8000", "8001", "9000"},
		Timeout:       "30s",
		MaxConcurrent: 10,
		ExcludeProxy:  func() *bool { b := true; return &b }(),
	}
	networkScanner := scanner.NewNetworkScanner(scanConfig)
	discoveryStore := scanner.NewInMemoryDiscoveryStore()
	scanManager := scanner.NewScanManager(networkScanner, discoveryStore)
	discoveryHandler := handlers.NewDiscoveryHandler(scanManager, discoveryStore)
	tokenHandler := handlers.NewTokenHandler(adapterStore, tokenManager)
	// mcpAuthIntegration := service.NewMCPAuthIntegrationService(tokenManager)
	mcpAuthHandler := handlers.NewMCPAuthHandler(adapterStore, nil)

	// Initialize missing handlers
	registryStore := clients.NewInMemoryMCPServerStore()
	registryManager := handlers.NewDefaultRegistryManager(registryStore)

	// Initialize AdapterService with SidecarManager
	logging.ProxyLogger.Info("Initializing AdapterService with SidecarManager")
	adapterService := adaptersvc.NewAdapterService(adapterStore, registryStore, sidecarManager)
	logging.ProxyLogger.Info("AdapterService created: %v", adapterService != nil)
	adapterHandler := handlers.NewAdapterHandler(adapterService, userGroupService)
	logging.ProxyLogger.Info("AdapterHandler created: %v", adapterHandler != nil)
	logging.ProxyLogger.Success("AdapterService and AdapterHandler initialized")

	// Adapter handlers are now used directly in Gin routes

	// Helper function to convert Gin context to standard HTTP handler
	ginToHTTPHandler := func(handler func(http.ResponseWriter, *http.Request)) gin.HandlerFunc {
		return func(c *gin.Context) {
			log.Printf("GIN HANDLER CALLED for path: %s", c.Request.URL.Path)
			handler(c.Writer, c.Request)
		}
	}

	// Load MCP registry from config file
	loadRegistryFromFile(registryManager)

	registryHandler := handlers.NewRegistryHandler(registryStore, registryManager, adapterStore, userGroupService)

	// Initialize user/group and route assignment handlers
	userGroupHandler := handlers.NewUserGroupHandler(userGroupService)
	routeAssignmentHandler := handlers.NewRouteAssignmentHandler(userGroupService, registryStore)
	logging.ProxyLogger.Info("UserGroupHandler created: %v", userGroupHandler != nil)
	logging.ProxyLogger.Info("RouteAssignmentHandler created: %v", routeAssignmentHandler != nil)

	// Initialize plugin service manager
	serviceManager := plugins.NewServiceManager(cfg, registryManager)
	pluginHandler := handlers.NewPluginHandler(serviceManager)

	// registrationHandler := handlers.NewRegistrationHandler(networkScanner, adapterStore, tokenManager, cfg)

	// Request/Response logging middleware
	// logger := middleware.NewRequestResponseLogger()
	// r.Use(logger.GinMiddleware())

	// CORS middleware
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1")) {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, MCP-Protocol-Version, Mcp-Session-Id")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC(),
			"version":   "1.0.0",
		})
	})

	// Test endpoint
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "SUSE AI Universal Proxy",
			"time":    time.Now().UTC(),
		})
	})

	// Test route
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"test": "ok"})
	})

	// Swagger UI
	r.GET("/swagger/index.html", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(200, `<!DOCTYPE html>
<html>
<head>
  <title>SUSE AI Universal Proxy API</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin:0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: '/swagger/doc.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIBundle.presets.standalone
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        validatorUrl: null,
        tryItOutEnabled: true
      });
    };
  </script>
</body>
</html>`)
	})
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"test": "ok"})
	})
	r.GET("/swagger/doc.json", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"swagger": "2.0",
			"info": gin.H{
				"title":   "SUSE AI Universal Proxy API",
				"version": "1.0",
			},
		})
	})

	// Monitoring endpoints
	r.GET("/api/v1/monitoring/metrics", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": "Monitoring not enabled",
		})
	})

	r.GET("/api/v1/monitoring/logs", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": "Monitoring not enabled",
		})
	})

	r.GET("/api/v1/monitoring/cache", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": "Cache not available",
		})
	})

	// API v1 routes
	logging.ProxyLogger.Info("Setting up API v1 routes")
	v1 := r.Group("/api/v1")
	logging.ProxyLogger.Info("V1 group created: %v", v1 != nil)
	{
		// Discovery routes
		discovery := v1.Group("/discovery")
		{
			discovery.POST("/scan", discoveryHandler.ScanForMCPServers)
			discovery.GET("/scan", discoveryHandler.ListScanJobs)
			discovery.GET("/scan/:jobId", discoveryHandler.GetScanJob)
			discovery.DELETE("/scan/:jobId", discoveryHandler.CancelScanJob)
			discovery.GET("/servers", discoveryHandler.ListDiscoveredServers)
			discovery.GET("/servers/:id", discoveryHandler.GetDiscoveredServer)
			// discovery.POST("/register", registrationHandler.RegisterDiscoveredServer)
		}

		// Adapter routes
		logging.ProxyLogger.Info("Setting up adapter routes")
		adapters := v1.Group("/adapters")
		{
			logging.ProxyLogger.Info("Adapter handler initialized: %v", adapterHandler != nil)
			// CRUD operations using AdapterHandler
			logging.ProxyLogger.Info("Registering adapter GET route")
			adapters.GET("", ginToHTTPHandler(adapterHandler.ListAdapters))
			logging.ProxyLogger.Info("Registering adapter POST route")
			adapters.POST("", ginToHTTPHandler(adapterHandler.CreateAdapter))
			adapters.GET("/:name", ginToHTTPHandler(adapterHandler.GetAdapter))
			adapters.PUT("/:name", ginToHTTPHandler(adapterHandler.UpdateAdapter))
			adapters.DELETE("/:name", ginToHTTPHandler(adapterHandler.DeleteAdapter))

			// Token management
			adapters.GET("/:name/token", tokenHandler.GetAdapterToken)
			adapters.POST("/:name/token/validate", tokenHandler.ValidateToken)
			adapters.POST("/:name/token/refresh", tokenHandler.RefreshToken)

			// Authentication
			adapters.GET("/:name/client-token", mcpAuthHandler.GetClientToken)
			adapters.POST("/:name/validate-auth", mcpAuthHandler.ValidateAuthConfig)
			adapters.POST("/:name/test-auth", mcpAuthHandler.TestAuthConnection)

			// Adapter management
			adapters.GET("/:name/logs", func(c *gin.Context) {
				// Get adapter logs
				c.JSON(http.StatusOK, gin.H{
					"logs":  []string{"Adapter logs not yet implemented"},
					"count": 1,
				})
			})
			adapters.GET("/:name/status", func(c *gin.Context) {
				// Get adapter status
				c.JSON(http.StatusOK, gin.H{
					"readyReplicas":     1,
					"updatedReplicas":   1,
					"availableReplicas": 1,
					"image":             "nginx:latest",
					"replicaStatus":     "Healthy",
				})
			})

			// Session management
			adapters.GET("/:name/sessions", func(c *gin.Context) {
				// List sessions for adapter
				c.JSON(http.StatusOK, gin.H{
					"sessions": []interface{}{},
					"count":    0,
				})
			})
			adapters.POST("/:name/sessions", func(c *gin.Context) {
				// Reinitialize session
				c.JSON(http.StatusOK, gin.H{
					"sessionId": "session-" + generateID(),
					"status":    "initialized",
				})
			})
			adapters.DELETE("/:name/sessions", func(c *gin.Context) {
				// Delete all sessions
				c.JSON(http.StatusOK, gin.H{
					"deleted": 0,
					"message": "All sessions deleted",
				})
			})
			adapters.GET("/:name/sessions/:sessionId", func(c *gin.Context) {
				// Get session details
				c.JSON(http.StatusOK, gin.H{
					"sessionId": c.Param("sessionId"),
					"status":    "active",
					"createdAt": time.Now(),
				})
			})
			adapters.DELETE("/:name/sessions/:sessionId", func(c *gin.Context) {
				// Delete specific session
				c.JSON(http.StatusOK, gin.H{
					"sessionId": c.Param("sessionId"),
					"deleted":   true,
				})
			})

			// MCP proxy endpoint - this is the main integration point
			adapters.Any("/:name/mcp", ginToHTTPHandler(adapterHandler.HandleMCPProtocol))

			// Sync capabilities
			adapters.POST("/:name/sync", ginToHTTPHandler(adapterHandler.SyncAdapterCapabilities))

			// REST-style MCP endpoints
			adapters.GET("/:name/tools", func(c *gin.Context) {
				handleMCPToolsList(c, adapterStore, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			})
			adapters.POST("/:name/tools/:toolName/call", func(c *gin.Context) {
				handleMCPToolCall(c, adapterStore, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			})
			adapters.GET("/:name/resources", func(c *gin.Context) {
				handleMCPResourcesList(c, adapterStore, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			})
			adapters.GET("/:name/resources/*uri", func(c *gin.Context) {
				handleMCPResourceRead(c, adapterStore, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			})
			adapters.GET("/:name/prompts", func(c *gin.Context) {
				handleMCPPromptsList(c, adapterStore, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			})
			adapters.GET("/:name/prompts/:promptName", func(c *gin.Context) {
				handleMCPPromptGet(c, adapterStore, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			})
		}

		// Registry routes
		registry := v1.Group("/registry")
		{
			registry.GET("", ginToHTTPHandler(registryHandler.ListMCPServersFiltered))
			registry.POST("/upload", registryHandler.UploadRegistryEntry)
			registry.POST("/upload/bulk", registryHandler.UploadBulkRegistryEntries)
			registry.POST("/upload/local-mcp", registryHandler.UploadLocalMCP)
			registry.GET("/browse", registryHandler.BrowseRegistry)

			registry.GET("/:id", registryHandler.GetMCPServer)
			registry.PUT("/:id", registryHandler.UpdateMCPServer)
			registry.DELETE("/:id", registryHandler.DeleteMCPServer)
		}

		// Plugin routes
		plugins := v1.Group("/plugins")
		{
			plugins.POST("/register", pluginHandler.RegisterService)
			plugins.DELETE("/register/:serviceId", pluginHandler.UnregisterService)
			plugins.GET("/services", pluginHandler.ListServices)
			plugins.GET("/services/:serviceId", pluginHandler.GetService)
			plugins.GET("/services/type/:serviceType", pluginHandler.ListServicesByType)
			plugins.GET("/services/:serviceId/health", pluginHandler.GetServiceHealth)
		}

		// User/Group management routes
		logging.ProxyLogger.Info("Registering user/group routes")
		users := v1.Group("/users")
		{
			logging.ProxyLogger.Info("Users group created: %v", users != nil)
			users.GET("", func(c *gin.Context) {
				logging.ProxyLogger.Info("Users GET route called")
				c.JSON(http.StatusOK, gin.H{"message": "users endpoint working"})
			})
			users.POST("", ginToHTTPHandler(userGroupHandler.HandleUsers))
			users.GET("/:id", ginToHTTPHandler(userGroupHandler.GetUser))
			users.PUT("/:id", ginToHTTPHandler(userGroupHandler.UpdateUser))
			users.DELETE("/:id", ginToHTTPHandler(userGroupHandler.DeleteUser))
		}

		groups := v1.Group("/groups")
		{
			groups.GET("", ginToHTTPHandler(userGroupHandler.HandleGroups))
			groups.POST("", ginToHTTPHandler(userGroupHandler.HandleGroups))
			groups.GET("/:id", ginToHTTPHandler(userGroupHandler.GetGroup))
			groups.PUT("/:id", ginToHTTPHandler(userGroupHandler.UpdateGroup))
			groups.DELETE("/:id", ginToHTTPHandler(userGroupHandler.DeleteGroup))
			groups.POST("/:id/members", ginToHTTPHandler(userGroupHandler.AddUserToGroup))
			groups.DELETE("/:id/members/:userId", ginToHTTPHandler(userGroupHandler.RemoveUserFromGroup))
		}

		// Route assignment routes (under registry)
		registry.POST("/:id/routes", ginToHTTPHandler(routeAssignmentHandler.CreateRouteAssignment))
		registry.GET("/:id/routes", ginToHTTPHandler(routeAssignmentHandler.ListRouteAssignments))
		registry.PUT("/:id/routes/:assignmentId", ginToHTTPHandler(routeAssignmentHandler.UpdateRouteAssignment))
		registry.DELETE("/:id/routes/:assignmentId", ginToHTTPHandler(routeAssignmentHandler.DeleteRouteAssignment))

	}

	// Start health checks for plugins
	pluginCtx, pluginCancel := context.WithCancel(context.Background())
	defer pluginCancel()
	go serviceManager.StartHealthChecks(pluginCtx, 30*time.Second)

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	log.Printf("DEBUG: About to start Gin HTTP server on port %s", cfg.Port)
	go func() {
		log.Printf("DEBUG: Gin server goroutine started")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: Gin server failed: %v", err)
			log.Fatalf("Failed to start server: %v", err)
		}
	}()
	log.Printf("DEBUG: Gin HTTP server created and goroutine started")

	// Log available server URLs
	serverURLs := cfg.GetServerURLs()
	log.Printf("Server starting on port %s (from config)", cfg.Port)
	log.Printf("PORT env var: %s", os.Getenv("PORT"))
	log.Printf("Service will be accessible at:")
	for _, url := range serverURLs {
		log.Printf("  %s", url)
	}
	log.Printf("Swagger documentation: %s/docs/", serverURLs[0])

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

// handleMCPToolsList handles GET /adapters/{name}/tools - REST-style tools/list
func handleMCPToolsList(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Create tools/list JSON-RPC request
	toolsListRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]interface{}{},
	}

	// Convert to JSON
	requestBody, err := json.Marshal(toolsListRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Create a mock gin context with the JSON-RPC request
	mockContext, _ := gin.CreateTestContext(c.Writer)
	mockContext.Request = c.Request
	mockContext.Request.Method = "POST"
	mockContext.Request.Header.Set("Content-Type", "application/json")
	mockContext.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
	mockContext.Params = c.Params

	// Route to appropriate handler based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(mockContext, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(mockContext, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}

// handleMCPToolCall handles POST /adapters/{name}/tools/{toolName}/call - REST-style tools/call
func handleMCPToolCall(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")
	toolName := c.Param("toolName")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Parse request body for tool arguments
	var requestBody map[string]interface{}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Create tools/call JSON-RPC request
	toolCallRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": requestBody,
		},
	}

	// Convert to JSON
	jsonRequestBody, err := json.Marshal(toolCallRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Create a mock gin context with the JSON-RPC request
	mockContext, _ := gin.CreateTestContext(c.Writer)
	mockContext.Request = c.Request
	mockContext.Request.Method = "POST"
	mockContext.Request.Header.Set("Content-Type", "application/json")
	mockContext.Request.Body = io.NopCloser(bytes.NewReader(jsonRequestBody))
	mockContext.Params = c.Params

	// Route to appropriate handler based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(mockContext, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(mockContext, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}

// handleMCPResourcesList handles GET /adapters/{name}/resources - REST-style resources/list
func handleMCPResourcesList(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Create resources/list JSON-RPC request
	resourcesListRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "resources/list",
		Params:  map[string]interface{}{},
	}

	// Convert to JSON
	requestBody, err := json.Marshal(resourcesListRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Create a mock gin context with the JSON-RPC request
	mockContext, _ := gin.CreateTestContext(c.Writer)
	mockContext.Request = c.Request
	mockContext.Request.Method = "POST"
	mockContext.Request.Header.Set("Content-Type", "application/json")
	mockContext.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
	mockContext.Params = c.Params

	// Route to appropriate handler based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(mockContext, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(mockContext, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}

// handleMCPResourceRead handles GET /adapters/{name}/resources/*uri - REST-style resources/read
func handleMCPResourceRead(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")
	resourceURI := c.Param("uri")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Create resources/read JSON-RPC request
	resourceReadRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "resources/read",
		Params: map[string]interface{}{
			"uri": resourceURI,
		},
	}

	// Convert to JSON
	requestBody, err := json.Marshal(resourceReadRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Create a mock gin context with the JSON-RPC request
	mockContext, _ := gin.CreateTestContext(c.Writer)
	mockContext.Request = c.Request
	mockContext.Request.Method = "POST"
	mockContext.Request.Header.Set("Content-Type", "application/json")
	mockContext.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
	mockContext.Params = c.Params

	// Route to appropriate handler based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(mockContext, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(mockContext, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}

// handleMCPPromptsList handles GET /adapters/{name}/prompts - REST-style prompts/list
func handleMCPPromptsList(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Create prompts/list JSON-RPC request
	promptsListRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "prompts/list",
		Params:  map[string]interface{}{},
	}

	// Convert to JSON
	requestBody, err := json.Marshal(promptsListRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Create a mock gin context with the JSON-RPC request
	mockContext, _ := gin.CreateTestContext(c.Writer)
	mockContext.Request = c.Request
	mockContext.Request.Method = "POST"
	mockContext.Request.Header.Set("Content-Type", "application/json")
	mockContext.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
	mockContext.Params = c.Params

	// Route to appropriate handler based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(mockContext, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(mockContext, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}

// handleMCPPromptGet handles GET /adapters/{name}/prompts/{promptName} - REST-style prompts/get
func handleMCPPromptGet(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")
	promptName := c.Param("promptName")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Parse query parameters for prompt arguments
	args := make(map[string]interface{})
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			args[key] = values[0]
		}
	}

	// Create prompts/get JSON-RPC request
	promptGetRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "prompts/get",
		Params: map[string]interface{}{
			"name":      promptName,
			"arguments": args,
		},
	}

	// Convert to JSON
	requestBody, err := json.Marshal(promptGetRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Create a mock gin context with the JSON-RPC request
	mockContext, _ := gin.CreateTestContext(c.Writer)
	mockContext.Request = c.Request
	mockContext.Request.Method = "POST"
	mockContext.Request.Header.Set("Content-Type", "application/json")
	mockContext.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
	mockContext.Params = c.Params

	// Route to appropriate handler based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(mockContext, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(mockContext, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}

// validateClientAuthentication validates client authentication for adapter access
func validateClientAuthentication(c *gin.Context, auth *models.AdapterAuthConfig) error {
	if auth == nil || !auth.Required {
		return nil // No authentication required
	}

	switch auth.Type {
	case "bearer":
		return validateBearerAuth(c, auth)
	case "basic":
		return validateBasicAuth(c, auth)
	case "apikey":
		return validateAPIKeyAuth(c, auth)
	default:
		return fmt.Errorf("unsupported authentication type: %s", auth.Type)
	}
}

// validateBearerAuth validates Bearer token authentication
func validateBearerAuth(c *gin.Context, auth *models.AdapterAuthConfig) error {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return fmt.Errorf("missing Authorization header")
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return fmt.Errorf("invalid Authorization header format")
	}

	token := strings.TrimPrefix(authHeader, bearerPrefix)
	var expectedToken string

	// Check bearer token config
	if auth.BearerToken != nil && auth.BearerToken.Token != "" {
		expectedToken = auth.BearerToken.Token
	}

	if token != expectedToken {
		return fmt.Errorf("invalid token")
	}

	return nil
}

// validateBasicAuth validates Basic authentication
func validateBasicAuth(c *gin.Context, auth *models.AdapterAuthConfig) error {
	if auth.Basic == nil {
		return fmt.Errorf("basic authentication configuration not found")
	}

	username, password, ok := c.Request.BasicAuth()
	if !ok {
		return fmt.Errorf("missing or invalid Basic authentication header")
	}

	if username != auth.Basic.Username || password != auth.Basic.Password {
		return fmt.Errorf("invalid username or password")
	}

	return nil
}

// validateAPIKeyAuth validates API key authentication
func validateAPIKeyAuth(c *gin.Context, auth *models.AdapterAuthConfig) error {
	if auth.APIKey == nil {
		return fmt.Errorf("API key configuration not found")
	}

	location := strings.ToLower(auth.APIKey.Location)
	name := auth.APIKey.Name
	expectedKey := auth.APIKey.Key

	var providedKey string
	var found bool

	switch location {
	case "header":
		providedKey = c.GetHeader(name)
		found = providedKey != ""
	case "query":
		providedKey = c.Query(name)
		found = providedKey != ""
	case "cookie":
		cookie, err := c.Cookie(name)
		if err == nil {
			providedKey = cookie
			found = true
		}
	default:
		return fmt.Errorf("unsupported API key location: %s", location)
	}

	if !found {
		return fmt.Errorf("API key not found in %s '%s'", location, name)
	}

	if providedKey != expectedKey {
		return fmt.Errorf("invalid API key")
	}

	return nil
}

// handleMCPProxy handles MCP proxy requests using the new MCP infrastructure
func handleMCPProxy(c *gin.Context, adapterStore clients.AdapterResourceStore, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")

	// Get adapter
	adapter, err := adapterStore.Get(c.Request.Context(), adapterName)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Validate client authentication before proxying
	if adapter.Authentication != nil && adapter.Authentication.Required {
		if err := validateClientAuthentication(c, adapter.Authentication); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required: " + err.Error()})
			return
		}
	}

	// Route MCP requests based on connection type
	switch adapter.ConnectionType {
	case models.ConnectionTypeLocalStdio:
		// Handle LocalStdio connections
		if stdioToHTTPAdapter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Stdio to HTTP adapter not initialized"})
			return
		}
		if err := stdioToHTTPAdapter.HandleRequest(c, *adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Stdio adapter error: %v", err)})
			return
		}
	case models.ConnectionTypeRemoteHttp, models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		// Handle remote HTTP connections
		if remoteHTTPPlugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Remote HTTP plugin not initialized"})
			return
		}
		if err := remoteHTTPPlugin.ProxyRequest(c, *adapter, sessionStore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Remote HTTP plugin error: %v", err)})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Unsupported connection type: %s", adapter.ConnectionType)})
		return
	}
}
