// Package main SUSE AI Universal Proxy Service
//
//	@title			SUSE AI Universal Proxy - Control Plane
//	@version		1.0.0
//	@description	SUSE AI Universal Proxy provides RESTful APIs for managing MCP (Model Context Protocol) server deployments and proxying requests to MCP servers running in Kubernetes.
//
//	@contact.name	SUSE AI Universal Proxy Team
//	@contact.url	https://github.com/SUSE/suse-ai-up
//
//	@license.name	MIT
//	@license.url	https://github.com/SUSE/suse-ai-up/blob/main/LICENSE
//
//	@host		localhost:8911
//	@BasePath	/
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Bearer token for authentication
//
//	@externalDocs.description	OpenAPI Specification
//	@externalDocs.url			https://github.com/SUSE/suse-ai-up/openapi/mcp-gateway.openapi.json
package main

import (
	"context"
	"log"
	"os"
	"time"

	_ "suse-ai-up/docs" // This is required for swagger
	"suse-ai-up/internal/config"
	"suse-ai-up/internal/handlers"
	"suse-ai-up/internal/service"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/plugins"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	// Get port early for banner
	port := os.Getenv("PORT")
	if port == "" {
		port = "8911"
	}

	log.Printf("SUSE AI Universal Proxy starting on port %s", port)
	log.Printf("Version: 1.0.0")
	log.Printf("")

	// Load configuration first
	cfg := config.LoadConfig()

	// Initialize clients (e.g., Kubernetes, Cosmos)
	kubeClient, err := clients.NewKubernetesClient()
	var kubeWrapper *clients.KubeClientWrapper
	if err != nil {
		log.Printf("Warning: Failed to create Kubernetes client: %v. Running in local mode without Kubernetes deployment.", err)
		kubeWrapper = nil // Will be handled in services
	} else {
		// Test connectivity by trying to list namespaces
		_, testErr := kubeClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{Limit: 1})
		if testErr != nil {
			log.Printf("Warning: Kubernetes client created but connectivity test failed: %v. Running in local mode without Kubernetes deployment.", testErr)
			kubeWrapper = nil
		} else {
			kubeWrapper = clients.NewKubeClientWrapper(kubeClient, "adapter")
			log.Printf("Successfully connected to Kubernetes cluster")
		}
	}

	// Initialize services
	sessionStore := session.NewInMemorySessionStore()
	store := clients.NewAdapterResourceStore() // in-memory for now
	managementService := service.NewManagementService(kubeWrapper, store, sessionStore)

	// Start periodic session cleanup (every 30 minutes, cleanup sessions inactive for 1 hour)
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := sessionStore.CleanupExpired(1 * time.Hour); err != nil {
				log.Printf("Error cleaning up expired sessions: %v", err)
			} else {
				log.Println("Cleaned up expired sessions")
			}
		}
	}()
	log.Printf("✓ Session cleanup service started (30min intervals)")

	// Initialize authorization service
	baseURL := os.Getenv("PROXY_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8911"
	}

	// Initialize token manager for OAuth 2.1 compliant token handling
	tokenManager, err := auth.NewTokenManager(baseURL)
	if err != nil {
		log.Printf("⚠ Token manager initialization failed: %v. Using legacy token validation.", err)
		tokenManager = nil
	} else {
		log.Printf("✓ Token manager initialized successfully")
	}

	proxyHandler := service.NewProxyHandler(sessionStore, kubeWrapper, store)
	log.Printf("✓ Proxy handler initialized")

	log.Printf("✓ Creating discovery service...")
	discoveryService := service.NewDiscoveryService(managementService, tokenManager)
	log.Printf("✓ Discovery service created successfully")

	authorizationService := service.NewAuthorizationService(sessionStore, baseURL)
	log.Printf("✓ Authorization service initialized")

	// Initialize MCP auth integration service
	mcpAuthIntegration := service.NewMCPAuthIntegrationService(tokenManager)
	mcpAuthHandler := handlers.NewMCPAuthHandler(store, mcpAuthIntegration)
	log.Printf("✓ MCP authentication integration initialized")

	// Initialize MCP registry services
	mcpStore := service.NewInMemoryMCPServerStore()
	registryManager := service.NewRegistryManager(mcpStore, true, 24*time.Hour, []string{})
	registryHandler := handlers.NewRegistryHandler(mcpStore, registryManager)
	deploymentHandler := handlers.NewDeploymentHandler(registryHandler, kubeWrapper)
	log.Printf("✓ Registry services initialized")

	// Initialize plugin service framework
	serviceManager := plugins.NewServiceManager(cfg)
	pluginRegistrationService := service.NewPluginRegistrationService(serviceManager)
	dynamicRouter := service.NewDynamicRouter(serviceManager)

	// Start health checks for plugin services
	// go serviceManager.StartHealthChecks(context.Background(), 30*time.Second)
	log.Printf("✓ Plugin service framework initialized")

	// Set up Gin router
	r := gin.New()
	r.Use(gin.Recovery())

	// Add debug middleware to trace requests
	r.Use(func(c *gin.Context) {
		log.Printf("DEBUG: Incoming request: %s %s", c.Request.Method, c.Request.URL.Path)
		c.Next()
		log.Printf("DEBUG: Request completed with status: %d", c.Writer.Status())
	})

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Configure authentication based on environment
	authMode := os.Getenv("AUTH_MODE")
	switch authMode {
	case "oauth":
		// Configure OAuth middleware (placeholder config)
		oauthConfig := &auth.ExternalOAuthConfig{
			Provider: "azure", // or "oauth"
			Required: false,   // Set to true for production
		}
		oauthMiddleware := auth.NewOAuthMiddleware(oauthConfig)
		r.Use(oauthMiddleware.Middleware())
		log.Printf("✓ Authentication: OAuth mode (Azure provider)")
	default:
		// Default to development auth
		r.Use(auth.DevelopmentAuthMiddleware())
		log.Printf("✓ Authentication: Development mode")
	}

	// Create adapter authentication middleware (needed for v1 routes)
	adapterAuthMiddleware := auth.NewAdapterAuthMiddleware(store, tokenManager)

	// Create token handler for token management APIs (needed for v1 routes)
	tokenHandler := handlers.NewTokenHandler(store, tokenManager)

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		// Discovery routes
		discovery := v1.Group("/discovery")
		{
			discovery.POST("/scan", discoveryService.StartScan)
			discovery.GET("/scan", discoveryService.ListScans) // List all scans
			discovery.GET("/scan/:scanId", discoveryService.GetScanStatus)
			discovery.GET("/servers", discoveryService.ListDiscoveredServers)
			discovery.POST("/register", discoveryService.RegisterServer)
		}

		// Registry routes
		registry := v1.Group("/registry")
		{
			registry.GET("/public", registryHandler.PublicList)
			registry.GET("/browse", registryHandler.BrowseRegistry)
			registry.POST("/sync/official", registryHandler.SyncOfficialRegistry)
			registry.POST("/upload", registryHandler.UploadRegistryEntry)
			registry.POST("/upload/bulk", registryHandler.UploadBulkRegistryEntries)
			registry.POST("/upload/local-mcp", registryHandler.UploadLocalMCP)
			registry.GET("/:id", registryHandler.GetMCPServer)
			registry.PUT("/:id", registryHandler.UpdateMCPServer)
			registry.DELETE("/:id", registryHandler.DeleteMCPServer)
		}

		// Deployment routes (protected)
		deployment := v1.Group("/deployment")
		deployment.Use(auth.DevelopmentAuthMiddleware()) // TODO: Use proper auth based on authMode
		{
			deployment.GET("/config/*serverId", deploymentHandler.GetMCPConfig)
			deployment.POST("/deploy", deploymentHandler.DeployMCP)
		}

		// Adapters routes
		adapters := v1.Group("/adapters")
		{
			adapters.POST("", managementService.CreateAdapter)
			adapters.GET("", managementService.ListAdapters)
			adapters.GET("/:name", managementService.GetAdapter)
			adapters.PUT("/:name", managementService.UpdateAdapter)
			adapters.DELETE("/:name", managementService.DeleteAdapter)
			adapters.GET("/:name/status", managementService.GetAdapterStatus)
			adapters.GET("/:name/logs", managementService.GetAdapterLogs)

			// Proxy routes with adapter-specific authentication
			adapters.POST("/:name/mcp", adapterAuthMiddleware.Middleware(), proxyHandler.ForwardStreamableHttp)
			adapters.POST("/:name/messages", adapterAuthMiddleware.Middleware(), proxyHandler.ForwardMessages)
			adapters.GET("/:name/sse", adapterAuthMiddleware.Middleware(), proxyHandler.ForwardSSE)

			// Authorization routes
			adapters.GET("/:name/auth/status", authorizationService.GetAuthorizationStatus)
			adapters.POST("/:name/auth/authorize", authorizationService.AuthorizeAdapter)
			adapters.DELETE("/:name/auth/tokens", authorizationService.RevokeTokens)
			adapters.GET("/:name/sessions", authorizationService.ListSessions)
			adapters.POST("/:name/sessions", authorizationService.CreateSession)

			// Session management routes
			adapters.DELETE("/:name/sessions/:sessionId", authorizationService.DeleteSession)
			adapters.DELETE("/:name/sessions", authorizationService.DeleteAllSessions)

			// Token management routes
			adapters.GET("/:name/token", tokenHandler.GetAdapterToken)
			adapters.GET("/:name/token/validate", tokenHandler.ValidateToken)
			adapters.POST("/:name/token/refresh", tokenHandler.RefreshToken)

			// MCP client authentication routes
			adapters.GET("/:name/client-token", mcpAuthHandler.GetClientToken)
			adapters.POST("/:name/validate-auth", mcpAuthHandler.ValidateAuthConfig)
			adapters.POST("/:name/test-auth", mcpAuthHandler.TestAuthConnection)
		}
	}

	// Plugin service routes (outside API v1)
	pluginsGroup := r.Group("/plugins")
	{
		pluginsGroup.POST("/register", pluginRegistrationService.RegisterService)
		pluginsGroup.DELETE("/register/:serviceId", pluginRegistrationService.UnregisterService)
		pluginsGroup.GET("/services", pluginRegistrationService.ListServices)
		pluginsGroup.GET("/services/:serviceId", pluginRegistrationService.GetService)
		pluginsGroup.GET("/services/:serviceId/health", pluginRegistrationService.GetServiceHealth)
		pluginsGroup.GET("/services/type/:serviceType", pluginRegistrationService.ListServicesByType)
	}

	// Test route (outside API v1)
	r.GET("/ping", func(c *gin.Context) {
		log.Printf("DEBUG: Ping route called")
		c.JSON(200, gin.H{"message": "pong"})
	})

	// OAuth callback routes
	r.GET("/oauth/callback", authorizationService.OAuthCallback)
	r.POST("/oauth/callback", authorizationService.OAuthCallback)
	// discovery := r.Group("/discovery")
	// {
	// 	discovery.POST("/test", service.StartScan)
	// 	discovery.GET("/scan/:scanId", discoveryService.GetScanStatus)
	// 	discovery.GET("/servers", discoveryService.ListDiscoveredServers)
	// 	discovery.POST("/register", discoveryService.RegisterServer)
	// }

	// Swagger routes
	r.GET("/docs", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Dynamic routing catch-all for plugin services
	// This must be registered last to not interfere with specific routes
	r.NoRoute(dynamicRouter.Middleware())

	// Display available endpoints
	log.Printf("Available endpoints:")
	log.Printf("")

	log.Printf("API v1 Endpoints:")
	log.Printf("")

	log.Printf("Discovery:")
	log.Printf("  - POST /api/v1/discovery/scan - Start network scan for MCP servers")
	log.Printf("  - GET  /api/v1/discovery/scan - List all scans")
	log.Printf("  - GET  /api/v1/discovery/scan/:scanId - Get scan status")
	log.Printf("  - GET  /api/v1/discovery/servers - List discovered servers")
	log.Printf("  - POST /api/v1/discovery/register - Register discovered server")
	log.Printf("")

	log.Printf("Registry:")
	log.Printf("  - GET  /api/v1/registry/public - List public registry")
	log.Printf("  - GET  /api/v1/registry/browse - Browse registry")
	log.Printf("  - POST /api/v1/registry/sync/official - Sync official registry")
	log.Printf("  - POST /api/v1/registry/upload - Upload registry entry")
	log.Printf("  - POST /api/v1/registry/upload/bulk - Upload bulk registry entries")
	log.Printf("  - POST /api/v1/registry/upload/local-mcp - Upload local MCP")
	log.Printf("  - GET  /api/v1/registry/:id - Get MCP server")
	log.Printf("  - PUT  /api/v1/registry/:id - Update MCP server")
	log.Printf("  - DELETE /api/v1/registry/:id - Delete MCP server")
	log.Printf("")

	log.Printf("Adapters:")
	log.Printf("  - POST /api/v1/adapters - Create adapter")
	log.Printf("  - GET  /api/v1/adapters - List adapters")
	log.Printf("  - GET  /api/v1/adapters/:name - Get adapter")
	log.Printf("  - PUT  /api/v1/adapters/:name - Update adapter")
	log.Printf("  - DELETE /api/v1/adapters/:name - Delete adapter")
	log.Printf("  - GET  /api/v1/adapters/:name/status - Get adapter status")
	log.Printf("  - GET  /api/v1/adapters/:name/logs - Get adapter logs")
	log.Printf("  - POST /api/v1/adapters/:name/mcp - Proxy MCP requests")
	log.Printf("  - POST /api/v1/adapters/:name/messages - Forward messages")
	log.Printf("  - GET  /api/v1/adapters/:name/sse - Forward SSE")
	log.Printf("")

	log.Printf("Authentication & Authorization:")
	log.Printf("  - GET  /api/v1/adapters/:name/auth/status - Get authorization status")
	log.Printf("  - POST /api/v1/adapters/:name/auth/authorize - Authorize adapter")
	log.Printf("  - DELETE /api/v1/adapters/:name/auth/tokens - Revoke tokens")
	log.Printf("  - GET  /api/v1/adapters/:name/sessions - List sessions")
	log.Printf("  - POST /api/v1/adapters/:name/sessions - Create session")
	log.Printf("  - DELETE /api/v1/adapters/:name/sessions/:sessionId - Delete session")
	log.Printf("  - DELETE /api/v1/adapters/:name/sessions - Delete all sessions")
	log.Printf("")

	log.Printf("Token Management:")
	log.Printf("  - GET  /api/v1/adapters/:name/token - Get adapter token")
	log.Printf("  - GET  /api/v1/adapters/:name/token/validate - Validate token")
	log.Printf("  - POST /api/v1/adapters/:name/token/refresh - Refresh token")
	log.Printf("  - GET  /api/v1/adapters/:name/client-token - Get client token")
	log.Printf("  - POST /api/v1/adapters/:name/validate-auth - Validate auth config")
	log.Printf("  - POST /api/v1/adapters/:name/test-auth - Test auth connection")
	log.Printf("")

	log.Printf("Deployment:")
	log.Printf("  - GET  /api/v1/deployment/config/*serverId - Get MCP config")
	log.Printf("  - POST /api/v1/deployment/deploy - Deploy MCP")
	log.Printf("")

	log.Printf("Non-API Endpoints:")
	log.Printf("")

	log.Printf("Plugin Services:")
	log.Printf("  - POST /plugins/register - Register service")
	log.Printf("  - DELETE /plugins/register/:serviceId - Unregister service")
	log.Printf("  - GET  /plugins/services - List services")
	log.Printf("  - GET  /plugins/services/:serviceId - Get service")
	log.Printf("  - GET  /plugins/services/:serviceId/health - Get service health")
	log.Printf("  - GET  /plugins/services/type/:serviceType - List services by type")
	log.Printf("")

	log.Printf("OAuth:")
	log.Printf("  - GET  /oauth/callback - OAuth callback")
	log.Printf("  - POST /oauth/callback - OAuth callback")
	log.Printf("")

	log.Printf("Documentation:")
	log.Printf("  - GET  /docs - Swagger documentation")
	log.Printf("  - GET  /docs/*any - Swagger documentation")
	log.Printf("")

	log.Printf("Health & Ping:")
	log.Printf("  - GET  /ping - Ping endpoint")
	log.Printf("")

	// Run server
	bindAddr := "0.0.0.0:" + port
	log.Printf("")
	log.Printf("🚀 SUSE AI Universal Proxy is now running!")
	log.Printf("   Local access: http://localhost:%s", port)
	log.Printf("   Network access: http://%s", bindAddr)
	log.Printf("   Swagger docs: http://localhost:%s/docs", port)
	log.Printf("")

	log.Fatal(r.Run(bindAddr))
}
