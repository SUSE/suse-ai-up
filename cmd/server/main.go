// @BasePath /api/v1
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/scanner"
)

func main() {
	// Initialize components
	adapterStore := clients.NewAdapterResourceStore()
	tokenManager, err := auth.NewTokenManager("suse-ai-up")
	if err != nil {
		log.Fatalf("Failed to create token manager: %v", err)
	}

	// Initialize network scanner
	scanConfig := &models.ScanConfig{
		ScanRanges:    []string{"192.168.1.74"},
		Ports:         []string{"8002"},
		Timeout:       "30s",
		MaxConcurrent: 10,
		ExcludeProxy:  new(bool),
	}
	*scanConfig.ExcludeProxy = true
	networkScanner := scanner.NewNetworkScanner(scanConfig)

	// Initialize handlers
	registrationHandler := handlers.NewRegistrationHandler(networkScanner, adapterStore, tokenManager, nil)
	registryHandler := handlers.NewRegistryHandler(nil, nil)                 // TODO: Implement proper registry manager
	deploymentHandler := handlers.NewDeploymentHandler(registryHandler, nil) // TODO: Add Kubernetes client
	tokenHandler := handlers.NewTokenHandler(adapterStore, tokenManager)
	mcpAuthHandler := handlers.NewMCPAuthHandler(adapterStore, nil) // TODO: Add auth integration service
	discoveryHandler := handlers.NewDiscoveryHandler(networkScanner)

	// Setup router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Registration endpoints
		v1.POST("/register", registrationHandler.RegisterDiscoveredServer)

		// Adapter endpoints
		adapters := v1.Group("/adapters")
		{
			adapters.GET("", func(c *gin.Context) {
				// TODO: List adapters
				c.JSON(200, gin.H{"adapters": []string{}})
			})
			adapters.POST("", func(c *gin.Context) {
				// TODO: Create adapter
				c.JSON(200, gin.H{"status": "not implemented"})
			})
			adapters.GET("/:name", func(c *gin.Context) {
				// TODO: Get adapter
				c.JSON(200, gin.H{"name": c.Param("name")})
			})
			adapters.DELETE("/:name", func(c *gin.Context) {
				// TODO: Delete adapter
				c.JSON(200, gin.H{"status": "deleted"})
			})

			// Token management
			adapters.GET("/:name/token", tokenHandler.GetAdapterToken)
			adapters.POST("/:name/token/validate", tokenHandler.ValidateToken)
			adapters.POST("/:name/token/refresh", tokenHandler.RefreshToken)

			// Authentication
			adapters.GET("/:name/client-token", mcpAuthHandler.GetClientToken)
			adapters.POST("/:name/validate-auth", mcpAuthHandler.ValidateAuthConfig)
			adapters.POST("/:name/test-auth", mcpAuthHandler.TestAuthConnection)

			// MCP proxy endpoint (this would be handled by adapter middleware)
			adapters.POST("/:name/mcp", func(c *gin.Context) {
				// TODO: Proxy MCP requests
				c.JSON(200, gin.H{"message": "MCP proxy not implemented yet"})
			})
		}

		// Registry endpoints
		registry := v1.Group("/registry")
		{
			registry.GET("", registryHandler.BrowseRegistry)
			registry.GET("/public", registryHandler.PublicList)
			registry.GET("/:id", registryHandler.GetMCPServer)
			registry.PUT("/:id", registryHandler.UpdateMCPServer)
			registry.DELETE("/:id", registryHandler.DeleteMCPServer)
			registry.POST("/upload", registryHandler.UploadRegistryEntry)
			registry.POST("/upload/bulk", registryHandler.UploadBulkRegistryEntries)
			registry.POST("/sync/official", registryHandler.SyncOfficialRegistry)
			registry.GET("/browse", registryHandler.BrowseRegistry)
		}

		// Deployment endpoints
		deployment := v1.Group("/deployment")
		{
			deployment.GET("/config/:serverId", deploymentHandler.GetMCPConfig)
			deployment.POST("/deploy", deploymentHandler.DeployMCP)
		}

		// Discovery endpoints
		discovery := v1.Group("/discovery")
		{
			discovery.POST("/scan", discoveryHandler.ScanForMCPServers)
			discovery.GET("/servers", discoveryHandler.ListDiscoveredServers)
			discovery.GET("/servers/:id", discoveryHandler.GetDiscoveredServer)
		}
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "suse-ai-up",
			"version": "1.0.0",
		})
	})

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8911"
	}

	log.Printf("SUSE AI Universal Proxy starting on port %s", port)
	log.Printf("Available endpoints:")
	log.Printf("  - POST /api/v1/register - Register discovered MCP server")
	log.Printf("  - GET  /api/v1/discovery/servers - List discovered servers")
	log.Printf("  - POST /api/v1/discovery/scan - Scan for MCP servers")
	log.Printf("  - GET  /health - Health check")
	log.Printf("  - GET  /ping - Ping")

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
