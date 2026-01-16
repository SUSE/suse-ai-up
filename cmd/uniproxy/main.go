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
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	_ "suse-ai-up/docs"
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

// generateID generates a random hex ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// parseAndUploadRegistryYAML parses YAML data and uploads MCP servers to registry
func parseAndUploadRegistryYAML(data []byte, registryManager *handlers.DefaultRegistryManager, source string) error {
	var servers []map[string]interface{}
	if err := yaml.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("could not parse registry YAML from %s: %w", source, err)
	}

	log.Printf("Loading %d MCP servers from %s", len(servers), source)

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

		// Include all additional fields from YAML in meta
		if about, ok := serverData["about"].(map[string]interface{}); ok {
			server.Meta["about"] = about
		}
		if sourceInfo, ok := serverData["source"].(map[string]interface{}); ok {
			server.Meta["source_info"] = sourceInfo
		}
		if config, ok := serverData["config"].(map[string]interface{}); ok {
			server.Meta["config"] = config
		}
		if serverType, ok := serverData["type"].(string); ok {
			server.Meta["type"] = serverType
		}

		mcpServers = append(mcpServers, server)
	}

	// Use the registry manager to upload all servers
	log.Printf("DEBUG: Uploading %d MCP servers to registry", len(mcpServers))
	if err := registryManager.UploadRegistryEntries(mcpServers); err != nil {
		return fmt.Errorf("could not upload registry entries: %w", err)
	}
	log.Printf("DEBUG: Successfully uploaded MCP servers")

	return nil
}

// loadRegistryFromURL loads MCP servers from a URL
func loadRegistryFromURL(registryManager *handlers.DefaultRegistryManager, url string, timeout time.Duration) error {
	log.Printf("Loading MCP registry from URL: %s", url)

	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch from URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("URL returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	return parseAndUploadRegistryYAML(data, registryManager, url)
}

// loadRegistryFromFile loads MCP servers from URL or config/mcp_registry.yaml
func loadRegistryFromFile(registryManager *handlers.DefaultRegistryManager, cfg *config.Config) {
	log.Printf("DEBUG: loadRegistryFromFile called")

	// Clear existing registry for clean startup state
	if err := registryManager.Clear(); err != nil {
		log.Printf("Warning: Failed to clear registry at startup: %v", err)
	}

	// Try to load from URL first if configured
	if cfg.MCPRegistryURL != "" {
		timeout, err := time.ParseDuration(cfg.RegistryTimeout)
		if err != nil {
			log.Printf("Warning: Invalid registry timeout %s, using 30s: %v", cfg.RegistryTimeout, err)
			timeout = 30 * time.Second
		}

		if err := loadRegistryFromURL(registryManager, cfg.MCPRegistryURL, timeout); err != nil {
			log.Printf("Warning: Failed to load registry from URL %s: %v, falling back to local file", cfg.MCPRegistryURL, err)
			// Fall through to local file loading
		} else {
			log.Printf("Successfully loaded MCP registry from URL: %s", cfg.MCPRegistryURL)
			return
		}
	}

	// Fallback to local file
	registryFile := "config/mcp_registry.yaml"
	data, err := os.ReadFile(registryFile)
	if err != nil {
		log.Printf("Warning: Could not read registry file %s: %v", registryFile, err)
		return
	}

	if err := parseAndUploadRegistryYAML(data, registryManager, registryFile); err != nil {
		log.Printf("Warning: Failed to parse and upload registry from file %s: %v", registryFile, err)
		return
	}

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

// @title SUSE AI Uniproxy API
// @version 1.0
// @description A comprehensive, modular MCP (Model Context Protocol) proxy system
// @termsOfService http://swagger.io/terms/

// @contact.name SUSE
// @contact.url https://github.com/suse/suse-ai-up
// @contact.email info@suse.ai

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8911
// @BasePath /

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key

// RunUniproxy starts the SUSE AI Uniproxy service
func RunUniproxy() {
	log.Printf("MAIN FUNCTION STARTED")
	// Load configuration
	cfg := config.LoadConfig()
	log.Printf("Config loaded: Port=%s", cfg.Port)

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

	// Create user auth configuration
	userAuthConfig := &models.UserAuthConfig{
		Mode:    cfg.AuthMode,
		DevMode: cfg.DevMode,
		Local: &models.LocalAuthConfig{
			DefaultAdminPassword: cfg.AdminPassword,
			ForcePasswordChange:  cfg.ForcePasswordChange,
			PasswordMinLength:    cfg.PasswordMinLength,
		},
		GitHub: &models.GitHubAuthConfig{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubClientSecret,
			RedirectURI:  cfg.GitHubRedirectURI,
			AllowedOrgs:  cfg.GitHubAllowedOrgs,
			AdminTeams:   cfg.GitHubAdminTeams,
		},
		Rancher: &models.RancherAuthConfig{
			IssuerURL:     cfg.RancherIssuerURL,
			ClientID:      cfg.RancherClientID,
			ClientSecret:  cfg.RancherClientSecret,
			RedirectURI:   cfg.RancherRedirectURI,
			AdminGroups:   cfg.RancherAdminGroups,
			FallbackLocal: cfg.RancherFallbackLocal,
		},
	}

	// Initialize auth service
	userAuthService := auth.NewUserAuthService(userStore, tokenManager, userAuthConfig)

	// Create initial groups
	log.Printf("DEBUG: CreateInitialGroups: %v, Groups count: %d", cfg.CreateInitialGroups, len(cfg.InitialGroups))
	if cfg.CreateInitialGroups {
		log.Printf("DEBUG: Creating %d initial groups", len(cfg.InitialGroups))
		for _, initialGroup := range cfg.InitialGroups {
			log.Printf("Processing group: %s", initialGroup.ID)
			group := models.Group{
				ID:          initialGroup.ID,
				Name:        initialGroup.Name,
				Description: initialGroup.Description,
				Permissions: strings.Split(initialGroup.Permissions, ","),
			}
			if err := userGroupService.CreateGroup(context.Background(), group); err != nil {
				// Group might already exist, log but continue
				log.Printf("Note: Could not create initial group %s: %v", initialGroup.ID, err)
			} else {
				log.Printf("Created initial group: %s", initialGroup.ID)
			}
		}
	} else {
		log.Printf("CreateInitialGroups is disabled")
	}

	// Create initial users
	log.Printf("DEBUG: CreateInitialUsers: %v, Users count: %d", cfg.CreateInitialUsers, len(cfg.InitialUsers))
	if cfg.CreateInitialUsers {
		log.Printf("DEBUG: Creating %d initial users", len(cfg.InitialUsers))
		for _, initialUser := range cfg.InitialUsers {
			log.Printf("Processing user: %s", initialUser.ID)
			user := models.User{
				ID:           initialUser.ID,
				Name:         initialUser.Name,
				Email:        initialUser.Email,
				Groups:       strings.Split(initialUser.Groups, ","),
				AuthProvider: initialUser.AuthProvider,
			}

			password := initialUser.Password
			if password == "" && initialUser.AuthProvider == string(models.UserAuthProviderLocal) {
				password = cfg.AdminPassword
			}

			if _, err := userGroupService.GetUser(context.Background(), initialUser.ID); err != nil {
				if err := userAuthService.CreateUser(context.Background(), user, password); err != nil {
					log.Printf("Warning: Failed to create initial user %s: %v", initialUser.ID, err)
				} else {
					log.Printf("Created initial user: %s", initialUser.ID)
				}
			} else {
				log.Printf("User %s already exists", initialUser.ID)
			}
		}
	} else {
		log.Printf("CreateInitialUsers is disabled")
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

	// Load MCP registry from URL or config file
	loadRegistryFromFile(registryManager, cfg)

	// Initialize Kubernetes client for ConfigMap updates (optional)
	var k8sClient kubernetes.Interface
	if config, err := rest.InClusterConfig(); err == nil {
		k8sClient, _ = kubernetes.NewForConfig(config)
		log.Printf("Kubernetes client initialized for ConfigMap updates")
	} else {
		log.Printf("Not running in Kubernetes cluster, ConfigMap updates disabled")
	}

	registryHandler := handlers.NewRegistryHandler(registryStore, registryManager, adapterStore, userGroupService, cfg, k8sClient)

	// Initialize user/group and route assignment handlers
	userGroupHandler := handlers.NewUserGroupHandler(userGroupService)
	authHandler := handlers.NewAuthHandler(userAuthService)
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
		if origin != "" && (strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") || strings.Contains(origin, "192.168.") || strings.Contains(origin, "10.")) {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, MCP-Protocol-Version, Mcp-Session-Id, X-User-Id, x-user-id")

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

	// Swagger UI - use relative URL for deployment compatibility
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/docs/doc.json")))

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
			discovery.GET("/servers/:id", discoveryHandler.GetDeprecatedServer)
			discovery.GET("/results", discoveryHandler.GetAllScanResults)
			discovery.GET("/results/:id", discoveryHandler.GetServerFromResults)
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
			adapters.POST("/:name/health", ginToHTTPHandler(adapterHandler.CheckAdapterHealth))

			// Token management
			adapters.GET("/:name/token", tokenHandler.GetAdapterToken)
			adapters.POST("/:name/token/validate", tokenHandler.ValidateToken)
			adapters.POST("/:name/token/refresh", tokenHandler.RefreshToken)

			// Authentication
			adapters.GET("/:name/client-token", mcpAuthHandler.GetClientToken)
			adapters.POST("/:name/validate-auth", mcpAuthHandler.ValidateAuthConfig)
			adapters.POST("/:name/test-auth", mcpAuthHandler.TestAuthConnection)

			// Adapter management
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
			registry.POST("/reload", registryHandler.ReloadRegistry)
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

		// Authentication routes
		authRoutes := v1.Group("/auth")
		{
			authRoutes.POST("/login", authHandler.Login)
			authRoutes.POST("/oauth/login", authHandler.OAuthLogin)
			authRoutes.POST("/oauth/callback", authHandler.OAuthCallback)
			authRoutes.PUT("/password", authHandler.ChangePassword)
			authRoutes.POST("/logout", authHandler.Logout)
		}

		// Unauthenticated auth mode endpoint
		r.GET("/auth/mode", authHandler.GetAuthMode)

		// User/Group management routes (unauthenticated for read operations)
		logging.ProxyLogger.Info("Registering user/group routes")
		users := v1.Group("/users")
		{
			logging.ProxyLogger.Info("Users group created: %v", users != nil)
			// Read operations - no auth required
			users.GET("", ginToHTTPHandler(userGroupHandler.ListUsers))
			users.GET("/:id", ginToHTTPHandler(userGroupHandler.GetUser))

			// Write operations - require authentication
			protectedUsers := users.Group("")
			protectedUsers.Use(auth.UserAuthMiddleware(userAuthService))
			{
				protectedUsers.POST("", ginToHTTPHandler(userGroupHandler.HandleUsers))
				protectedUsers.PUT("/:id", ginToHTTPHandler(userGroupHandler.UpdateUser))
				protectedUsers.DELETE("/:id", ginToHTTPHandler(userGroupHandler.DeleteUser))
			}
		}

		groups := v1.Group("/groups")
		{
			// Read operations - no auth required
			groups.GET("", ginToHTTPHandler(userGroupHandler.HandleGroups))
			groups.GET("/:id", ginToHTTPHandler(userGroupHandler.GetGroup))

			// Write operations - require authentication
			protectedGroups := groups.Group("")
			protectedGroups.Use(auth.UserAuthMiddleware(userAuthService))
			{
				protectedGroups.POST("", ginToHTTPHandler(userGroupHandler.HandleGroups))
				protectedGroups.PUT("/:id", ginToHTTPHandler(userGroupHandler.UpdateGroup))
				protectedGroups.DELETE("/:id", ginToHTTPHandler(userGroupHandler.DeleteGroup))
				protectedGroups.POST("/:id/members", ginToHTTPHandler(userGroupHandler.AddUserToGroup))
				protectedGroups.DELETE("/:id/members/:userId", ginToHTTPHandler(userGroupHandler.RemoveUserFromGroup))
			}
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

// makeMCPRequestWithSession establishes MCP session and makes request
func makeMCPRequestWithSession(ctx context.Context, mcpURL string, request mcp.MCPMessage, auth *models.AdapterAuthConfig) (*http.Response, error) {
	fmt.Printf("DEBUG: Starting MCP session establishment for URL: %s\n", mcpURL)

	// First, initialize the MCP session
	initRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "suse-ai-up-rest-api",
				"version": "1.0.0",
			},
		},
	}

	fmt.Printf("DEBUG: Sending initialize request\n")
	initResp, err := makeRawMCPRequest(ctx, mcpURL, initRequest, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP session: %w", err)
	}
	defer initResp.Body.Close()

	// Read initialize response
	initBody, err := io.ReadAll(initResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read initialize response: %w", err)
	}

	if initResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("initialize failed with status %d: %s", initResp.StatusCode, string(initBody))
	}

	fmt.Printf("DEBUG: Initialize response received: %s\n", string(initBody))

	// Parse initialize response (handle SSE format: "event: message\ndata: {...}")
	responseBody := string(initBody)
	var jsonData string

	if strings.Contains(responseBody, "event: message\ndata: ") {
		// Extract JSON from SSE format
		lines := strings.Split(responseBody, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				jsonData = strings.TrimPrefix(line, "data: ")
				break
			}
		}
	} else {
		jsonData = responseBody
	}

	fmt.Printf("DEBUG: Extracted JSON data: %s\n", jsonData)

	// Parse initialize response to get session info
	var initResult map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &initResult); err != nil {
		return nil, fmt.Errorf("failed to parse initialize response: %w", err)
	}

	// Extract session ID from the response or use a default
	sessionID := "rest-api-session"
	if result, ok := initResult["result"].(map[string]interface{}); ok {
		if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
			if name, ok := serverInfo["name"].(string); ok {
				sessionID = fmt.Sprintf("rest-api-%s", name)
			}
		}
	}

	fmt.Printf("DEBUG: Using session ID: %s\n", sessionID)

	// Send initialized notification
	initializedRequest := mcp.MCPMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  map[string]interface{}{},
	}

	fmt.Printf("DEBUG: Sending initialized notification\n")
	// Add session ID to the notification
	initializedResp, err := makeRawMCPRequestWithSession(ctx, mcpURL, initializedRequest, auth, sessionID)
	if err != nil {
		// Notification failure is not fatal, continue with the main request
		fmt.Printf("Warning: initialized notification failed: %v\n", err)
	} else {
		fmt.Printf("DEBUG: Initialized notification sent successfully\n")
	}

	if initializedResp != nil {
		initializedResp.Body.Close()
	}

	fmt.Printf("DEBUG: Making main request with session ID: %s\n", sessionID)
	// Now make the actual request with the established session
	return makeRawMCPRequestWithSession(ctx, mcpURL, request, auth, sessionID)
}

// makeRawMCPRequest makes HTTP request to MCP endpoint
func makeRawMCPRequest(ctx context.Context, mcpURL string, request mcp.MCPMessage, auth *models.AdapterAuthConfig) (*http.Response, error) {
	return makeRawMCPRequestWithSession(ctx, mcpURL, request, auth, "")
}

// makeRawMCPRequestWithSession makes HTTP request with session ID
func makeRawMCPRequestWithSession(ctx context.Context, mcpURL string, request mcp.MCPMessage, auth *models.AdapterAuthConfig, sessionID string) (*http.Response, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mcpURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Add session ID if provided
	if sessionID != "" {
		req.Header.Set("mcp-session-id", sessionID)
		// Also add as query parameter for compatibility
		values := url.Values{}
		values.Set("sessionId", sessionID)
		req.URL.RawQuery = values.Encode()
	}

	// Add authentication if required
	if auth != nil && auth.BearerToken != nil && auth.BearerToken.Token != "" {
		req.Header.Set("Authorization", "Bearer "+auth.BearerToken.Token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// handleMCPToolsList handles GET /adapters/{name}/tools - REST-style tools/list
// @Summary List MCP tools
// @Description Get the list of tools available from the MCP server
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 200 {object} map[string]interface{} "MCP response with tools list"
// @Failure 404 {object} handlers.ErrorResponse "Adapter not found"
// @Failure 401 {object} handlers.ErrorResponse "Authentication required"
// @Failure 500 {object} handlers.ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/tools [get]
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

	// Make HTTP request to the adapter's /mcp endpoint with full session management
	fmt.Printf("REST_API_DEBUG: About to call makeMCPRequestWithSession for adapter %s, URL: %s\n", adapter.Name, adapter.URL)
	resp, err := makeMCPRequestWithSession(c.Request.Context(), adapter.URL, toolsListRequest, adapter.Authentication)
	fmt.Printf("REST_API_DEBUG: makeMCPRequestWithSession returned, err: %v\n", err)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("MCP request failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	// Read and return the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.Data(resp.StatusCode, "application/json", body)
		return
	}

	c.Data(http.StatusOK, "application/json", body)
}

// handleMCPToolCall handles POST /adapters/{name}/tools/{toolName}/call - REST-style tools/call
// @Summary Call MCP tool
// @Description Execute a specific MCP tool with given arguments
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Param toolName path string true "Tool name"
// @Param request body mcp.MCPMessage true "Tool call request with arguments"
// @Success 200 {object} map[string]interface{} "MCP response with tool result"
// @Failure 404 {object} handlers.ErrorResponse "Adapter or tool not found"
// @Failure 401 {object} handlers.ErrorResponse "Authentication required"
// @Failure 400 {object} handlers.ErrorResponse "Invalid request parameters"
// @Failure 500 {object} handlers.ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/tools/{toolName}/call [post]
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
// @Summary List MCP resources
// @Description Get the list of resources available from the MCP server
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 200 {object} map[string]interface{} "MCP response with resources list"
// @Failure 404 {object} handlers.ErrorResponse "Adapter not found"
// @Failure 401 {object} handlers.ErrorResponse "Authentication required"
// @Failure 500 {object} handlers.ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/resources [get]
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
// @Summary Read MCP resource
// @Description Read the content of a specific MCP resource by URI
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Param uri path string true "Resource URI"
// @Success 200 {object} map[string]interface{} "MCP response with resource content"
// @Failure 404 {object} handlers.ErrorResponse "Adapter or resource not found"
// @Failure 401 {object} handlers.ErrorResponse "Authentication required"
// @Failure 500 {object} handlers.ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/resources/{uri} [get]
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
// @Summary List MCP prompts
// @Description Get the list of prompts available from the MCP server
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 200 {object} map[string]interface{} "MCP response with prompts list"
// @Failure 404 {object} handlers.ErrorResponse "Adapter not found"
// @Failure 401 {object} handlers.ErrorResponse "Authentication required"
// @Failure 500 {object} handlers.ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/prompts [get]
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
// @Summary Get MCP prompt
// @Description Get the content of a specific MCP prompt by name
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Param promptName path string true "Prompt name"
// @Success 200 {object} map[string]interface{} "MCP response with prompt content"
// @Failure 404 {object} handlers.ErrorResponse "Adapter or prompt not found"
// @Failure 401 {object} handlers.ErrorResponse "Authentication required"
// @Failure 500 {object} handlers.ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/prompts/{promptName} [get]
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

// main is the entry point for the SUSE AI Uniproxy service
func main() {
	RunUniproxy()
}
