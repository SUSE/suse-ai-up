package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "suse-ai-up/docs"
	"suse-ai-up/internal/config"
	"suse-ai-up/internal/handlers"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/proxy"
	"suse-ai-up/pkg/scanner"
	"suse-ai-up/pkg/session"
)

// @title SUSE AI Universal Proxy API
// @version 1.0
// @description Comprehensive MCP proxy with discovery, registry, and deployment capabilities
// @host localhost:8911
// @BasePath /

// generateID generates a random hex ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

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
	// sessionStore := session.NewInMemorySessionStore()
	tokenManager, err := auth.NewTokenManager("mcp-gateway")
	if err != nil {
		log.Fatalf("Failed to create token manager: %v", err)
	}

	// Initialize MCP components
	// capabilityCache := mcp.NewCapabilityCache()
	// cache := mcp.NewMCPCache(nil)     // Use default config
	// monitor := mcp.NewMCPMonitor(nil) // Use default config
	// protocolHandler := mcp.NewProtocolHandler(sessionStore, capabilityCache)
	// messageRouter := mcp.NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	// streamableTransport := mcp.NewStreamableHTTPTransport(sessionStore, protocolHandler, messageRouter)

	// Initialize stdio proxy plugin for local stdio adapters
	// stdioProxy := proxy.NewLocalStdioProxyPlugin()
	// log.Printf("stdioProxy initialized: %v", stdioProxy != nil)

	// Initialize stdio-to-HTTP adapter
	// stdioToHTTPAdapter := proxy.NewStdioToHTTPAdapter(stdioProxy, messageRouter, sessionStore, protocolHandler, capabilityCache)
	// log.Printf("stdioToHTTPAdapter initialized: %v", stdioToHTTPAdapter != nil)

	// Initialize remote HTTP proxy adapter
	// remoteHTTPAdapter := proxy.NewRemoteHTTPProxyAdapter(sessionStore, messageRouter, protocolHandler, capabilityCache)
	// log.Printf("remoteHTTPAdapter initialized: %v", remoteHTTPAdapter != nil)

	// Initialize remote HTTP proxy plugin
	// remoteHTTPPlugin := proxy.NewRemoteHttpProxyPlugin()
	// log.Printf("remoteHTTPPlugin initialized: %v", remoteHTTPPlugin != nil)

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
	registryHandler := handlers.NewRegistryHandler(registryStore, registryManager)

	kubeClient, err := clients.NewKubernetesClient()
	if err != nil {
		log.Printf("Warning: Failed to create Kubernetes client: %v", err)
		kubeClient = nil
	}
	kubeWrapper := clients.NewKubeClientWrapper(kubeClient, "default")
	deploymentHandler := handlers.NewDeploymentHandler(registryHandler, kubeWrapper)

	// registrationHandler := handlers.NewRegistrationHandler(networkScanner, adapterStore, tokenManager, cfg)

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

	// Swagger documentation
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/swagger/doc.json", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
	v1 := r.Group("/api/v1")
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
		adapters := v1.Group("/adapters")
		{
			// CRUD operations
			adapters.GET("", func(c *gin.Context) {
				// List all adapters
				allAdapters := adapterStore.List()
				c.JSON(http.StatusOK, allAdapters)
			})
			adapters.POST("", func(c *gin.Context) {
				// Create adapter
				var data models.AdapterData
				if err := c.ShouldBindJSON(&data); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				adapter := &models.AdapterResource{}
				adapter.Create(data, "system", time.Now())
				if err := adapterStore.Create(adapter); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusCreated, gin.H{"status": "ok", "id": adapter.ID})
			})
			adapters.GET("/:name", func(c *gin.Context) {
				// Get adapter
				adapter, err := adapterStore.TryGetAsync(c.Param("name"), c.Request.Context())
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
					return
				}
				c.JSON(http.StatusOK, adapter)
			})
			adapters.PUT("/:name", func(c *gin.Context) {
				// Update adapter
				var data models.AdapterData
				if err := c.ShouldBindJSON(&data); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				adapter, err := adapterStore.TryGetAsync(c.Param("name"), c.Request.Context())
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
					return
				}
				adapter.AdapterData = data
				adapter.LastUpdatedAt = time.Now()
				if err := adapterStore.Update(adapter); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, adapter)
			})
			adapters.DELETE("/:name", func(c *gin.Context) {
				// Delete adapter
				if err := adapterStore.Delete(c.Param("name")); err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
					return
				}
				c.JSON(http.StatusNoContent, nil)
			})

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
			// adapters.Any("/:name/mcp", func(c *gin.Context) {
			// 	handleMCPProxy(c, adapterStore, streamableTransport, stdioToHTTPAdapter, remoteHTTPPlugin, sessionStore)
			// })
		}

		// Registry routes
		registry := v1.Group("/registry")
		{
			registry.GET("", func(c *gin.Context) {
				// Browse registry servers
				servers := registryStore.ListMCPServers()
				c.JSON(http.StatusOK, servers)
			})
			registry.GET("/public", registryHandler.PublicList)
			registry.POST("/sync/official", registryHandler.SyncOfficialRegistry)
			registry.POST("/upload", registryHandler.UploadRegistryEntry)
			registry.POST("/upload/bulk", registryHandler.UploadBulkRegistryEntries)
			registry.POST("/upload/local-mcp", registryHandler.UploadLocalMCP)
			registry.GET("/browse", registryHandler.BrowseRegistry)
			registry.GET("/:id", registryHandler.GetMCPServer)
			registry.PUT("/:id", registryHandler.UpdateMCPServer)
			registry.DELETE("/:id", registryHandler.DeleteMCPServer)
		}

		// Deployment routes
		deployment := v1.Group("/deployment")
		{
			deployment.GET("/config/:serverId", deploymentHandler.GetMCPConfig)
			deployment.POST("/deploy", deploymentHandler.DeployMCP)
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
	expectedToken := auth.Token

	// Check new bearer token config
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
func handleMCPProxy(c *gin.Context, adapterStore clients.AdapterResourceStore, transport *mcp.StreamableHTTPTransport, stdioToHTTPAdapter *proxy.StdioToHTTPAdapter, remoteHTTPPlugin *proxy.RemoteHttpProxyPlugin, sessionStore session.SessionStore) {
	adapterName := c.Param("name")

	// Get adapter
	adapter, err := adapterStore.TryGetAsync(adapterName, c.Request.Context())
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
