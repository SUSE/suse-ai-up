package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/internal/config"
	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/session"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize Gin
	if cfg.AuthMode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Initialize stores
	adapterStore := clients.NewInMemoryAdapterStore()
	sessionStore := session.NewInMemorySessionStore()
	tokenManager, err := auth.NewTokenManager("mcp-gateway")
	if err != nil {
		log.Fatalf("Failed to create token manager: %v", err)
	}

	// Initialize MCP components
	capabilityCache := mcp.NewCapabilityCache()
	cache := mcp.NewMCPCache(nil)     // Use default config
	monitor := mcp.NewMCPMonitor(nil) // Use default config
	protocolHandler := mcp.NewProtocolHandler(sessionStore, capabilityCache)
	messageRouter := mcp.NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	streamableTransport := mcp.NewStreamableHTTPTransport(sessionStore, protocolHandler, messageRouter)

	// Initialize handlers
	discoveryHandler := handlers.NewDiscoveryHandler(nil) // TODO: Add network scanner
	tokenHandler := handlers.NewTokenHandler(adapterStore, tokenManager)
	mcpAuthHandler := handlers.NewMCPAuthHandler(adapterStore, nil) // TODO: Add auth integration

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
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

	// Monitoring endpoints
	r.GET("/api/v1/monitoring/metrics", func(c *gin.Context) {
		if monitor != nil {
			metrics := monitor.GetMetrics()
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"metrics": metrics,
			})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": "Monitoring not enabled",
			})
		}
	})

	r.GET("/api/v1/monitoring/logs", func(c *gin.Context) {
		if monitor != nil {
			limit := 100 // default limit
			if l := c.Query("limit"); l != "" {
				if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
					limit = parsed
				}
			}

			logs := monitor.GetRecentLogs(limit)
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"logs":   logs,
				"count":  len(logs),
			})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": "Monitoring not enabled",
			})
		}
	})

	r.GET("/api/v1/monitoring/cache", func(c *gin.Context) {
		if messageRouter != nil {
			cacheMetrics := messageRouter.GetCacheMetrics()
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"cache":  cacheMetrics,
			})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": "Cache not available",
			})
		}
	})

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		// Discovery routes
		discovery := v1.Group("/discovery")
		{
			discovery.POST("/scan", discoveryHandler.ScanForMCPServers)
			discovery.GET("/servers", discoveryHandler.ListDiscoveredServers)
			discovery.GET("/servers/:id", discoveryHandler.GetDiscoveredServer)
		}

		// Adapter routes
		adapters := v1.Group("/adapters")
		{
			// Token management
			adapters.GET("/:name/token", tokenHandler.GetAdapterToken)
			adapters.POST("/:name/token/validate", tokenHandler.ValidateToken)
			adapters.POST("/:name/token/refresh", tokenHandler.RefreshToken)

			// Authentication
			adapters.GET("/:name/client-token", mcpAuthHandler.GetClientToken)
			adapters.POST("/:name/validate-auth", mcpAuthHandler.ValidateAuthConfig)
			adapters.POST("/:name/test-auth", mcpAuthHandler.TestAuthConnection)

			// MCP proxy endpoint - this is the main integration point
			adapters.Any("/:name/mcp", func(c *gin.Context) {
				handleMCPProxy(c, adapterStore, streamableTransport)
			})
		}
	}

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
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

// handleMCPProxy handles MCP proxy requests using the new MCP infrastructure
func handleMCPProxy(c *gin.Context, adapterStore clients.AdapterResourceStore, transport *mcp.StreamableHTTPTransport) {
	adapterName := c.Param("name")

	// Get adapter
	adapter, err := adapterStore.TryGetAsync(adapterName, c.Request.Context())
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Handle MCP request using the new StreamableHTTPTransport
	transport.HandleRequest(c.Writer, c.Request, *adapter)
}
