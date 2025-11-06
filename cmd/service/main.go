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
	log.Println("Starting SUSE AI Universal Proxy service...")

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

	// Initialize authorization service
	baseURL := os.Getenv("PROXY_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8911"
	}

	// Initialize token manager for OAuth 2.1 compliant token handling
	tokenManager, err := auth.NewTokenManager(baseURL)
	if err != nil {
		log.Printf("Warning: Failed to initialize token manager: %v. Using legacy token validation.", err)
		tokenManager = nil
	} else {
		log.Println("Token manager initialized successfully")
	}

	proxyHandler := service.NewProxyHandler(sessionStore, kubeWrapper, store)
	log.Println("Creating discovery service...")
	discoveryService := service.NewDiscoveryService(managementService, tokenManager)
	log.Println("Discovery service created successfully")

	authorizationService := service.NewAuthorizationService(sessionStore, baseURL)

	// Initialize MCP registry services
	mcpStore := service.NewInMemoryMCPServerStore()
	registryManager := service.NewRegistryManager(mcpStore, true, 24*time.Hour, []string{})
	registryHandler := handlers.NewRegistryHandler(mcpStore, registryManager)
	deploymentHandler := handlers.NewDeploymentHandler(registryHandler, kubeWrapper)

	// Initialize plugin service framework
	serviceManager := plugins.NewServiceManager(cfg)
	pluginRegistrationService := service.NewPluginRegistrationService(serviceManager)
	dynamicRouter := service.NewDynamicRouter(serviceManager)

	// Start health checks for plugin services
	// go serviceManager.StartHealthChecks(context.Background(), 30*time.Second)
	log.Println("Plugin service framework initialized")

	// Set up Gin router
	r := gin.New()
	r.Use(gin.Recovery())

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
		log.Println("Using OAuth authentication")
	default:
		// Default to development auth
		r.Use(auth.DevelopmentAuthMiddleware())
		log.Println("Using development authentication")
	}

	// Discovery routes (register first)
	r.POST("/scan", discoveryService.StartScan)
	r.GET("/scan", discoveryService.ListScans) // List all scans
	r.GET("/scan/:scanId", discoveryService.GetScanStatus)
	r.GET("/servers", discoveryService.ListDiscoveredServers)
	r.POST("/register", discoveryService.RegisterServer)

	// Registry routes
	r.GET("/public/registry", registryHandler.PublicList)
	r.GET("/registry/browse", registryHandler.BrowseRegistry)
	r.POST("/registry/sync/official", registryHandler.SyncOfficialRegistry)
	r.POST("/registry/upload", registryHandler.UploadRegistryEntry)
	r.POST("/registry/upload/bulk", registryHandler.UploadBulkRegistryEntries)
	r.POST("/registry/upload/local-mcp", registryHandler.UploadLocalMCP)
	r.GET("/registry/:id", registryHandler.GetMCPServer)
	r.PUT("/registry/:id", registryHandler.UpdateMCPServer)
	r.DELETE("/registry/:id", registryHandler.DeleteMCPServer)

	// Deployment routes (protected)
	protected := r.Group("/")
	protected.Use(auth.DevelopmentAuthMiddleware()) // TODO: Use proper auth based on authMode
	{
		protected.GET("/deployment/config/*serverId", deploymentHandler.GetMCPConfig)
		protected.POST("/deployment/deploy", deploymentHandler.DeployMCP)
	}

	// Plugin service routes
	pluginsGroup := r.Group("/plugins")
	{
		pluginsGroup.POST("/register", pluginRegistrationService.RegisterService)
		pluginsGroup.DELETE("/register/:serviceId", pluginRegistrationService.UnregisterService)
		pluginsGroup.GET("/services", pluginRegistrationService.ListServices)
		pluginsGroup.GET("/services/:serviceId", pluginRegistrationService.GetService)
		pluginsGroup.GET("/services/:serviceId/health", pluginRegistrationService.GetServiceHealth)
		pluginsGroup.GET("/services/type/:serviceType", pluginRegistrationService.ListServicesByType)
	}

	// Test route
	r.GET("/ping", func(c *gin.Context) {
		log.Printf("DEBUG: Ping route called")
		c.JSON(200, gin.H{"message": "pong"})
	})

	// Create adapter authentication middleware
	adapterAuthMiddleware := auth.NewAdapterAuthMiddleware(store, tokenManager)

	// Create token handler for token management APIs
	tokenHandler := handlers.NewTokenHandler(store, tokenManager)

	// Routes
	api := r.Group("/adapters")
	{
		api.POST("", managementService.CreateAdapter)
		api.GET("", managementService.ListAdapters)
		api.GET("/:name", managementService.GetAdapter)
		api.PUT("/:name", managementService.UpdateAdapter)
		api.DELETE("/:name", managementService.DeleteAdapter)
		api.GET("/:name/status", managementService.GetAdapterStatus)
		api.GET("/:name/logs", managementService.GetAdapterLogs)

		// Proxy routes with adapter-specific authentication
		api.POST("/:name/mcp", adapterAuthMiddleware.Middleware(), proxyHandler.ForwardStreamableHttp)
		api.POST("/:name/messages", adapterAuthMiddleware.Middleware(), proxyHandler.ForwardMessages)
		api.GET("/:name/sse", adapterAuthMiddleware.Middleware(), proxyHandler.ForwardSSE)

		// Authorization routes
		api.GET("/:name/auth/status", authorizationService.GetAuthorizationStatus)
		api.POST("/:name/auth/authorize", authorizationService.AuthorizeAdapter)
		api.DELETE("/:name/auth/tokens", authorizationService.RevokeTokens)
		api.GET("/:name/sessions", authorizationService.ListSessions)
		api.POST("/:name/sessions", authorizationService.CreateSession)

		// Token management routes
		api.GET("/:name/token", tokenHandler.GetAdapterToken)
		api.GET("/:name/token/validate", tokenHandler.ValidateToken)
		api.POST("/:name/token/refresh", tokenHandler.RefreshToken)
	}

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

	// Run server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8911"
	}
	log.Printf("Server listening on http://localhost:%s", port)
	log.Fatal(r.Run(":" + port))
}
