package service

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/plugins"
)

// DynamicRouter handles dynamic routing to plugin services
type DynamicRouter struct {
	serviceManager plugins.PluginServiceManager
}

// NewDynamicRouter creates a new dynamic router
func NewDynamicRouter(serviceManager plugins.PluginServiceManager) *DynamicRouter {
	return &DynamicRouter{
		serviceManager: serviceManager,
	}
}

// Middleware returns a Gin middleware that handles dynamic routing
func (dr *DynamicRouter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method

		log.Printf("DEBUG: DynamicRouter processing path: %s %s", method, path)

		// Skip plugin management endpoints - these are handled by the proxy itself
		if dr.isPluginManagementEndpoint(path) {
			log.Printf("DEBUG: Skipping plugin management endpoint: %s", path)
			c.Next()
			return
		}

		// Check if this path should be routed to a plugin service
		service, exists := dr.serviceManager.GetServiceForPath(path)
		if !exists {
			// Not a plugin service route, continue to next handler
			c.Next()
			return
		}

		// Check if the service is healthy
		health, _ := dr.serviceManager.GetServiceHealth(service.ServiceID)
		if health.Status != "healthy" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Plugin service temporarily unavailable",
				"service": service.ServiceID,
				"status":  health.Status,
			})
			c.Abort()
			return
		}

		// Check if the method is supported for this path
		if !dr.isMethodSupported(service, path, method) {
			c.JSON(http.StatusMethodNotAllowed, gin.H{
				"error":   "Method not allowed for this endpoint",
				"service": service.ServiceID,
				"path":    path,
				"method":  method,
			})
			c.Abort()
			return
		}

		// Proxy the request to the plugin service
		dr.proxyToService(c, service, path, method)
		c.Abort() // Prevent further processing
	}
}

// isMethodSupported checks if the HTTP method is supported for the given path
func (dr *DynamicRouter) isMethodSupported(service *plugins.ServiceRegistration, path, method string) bool {
	for _, capability := range service.Capabilities {
		if dr.pathMatchesCapability(path, capability.Path) {
			for _, supportedMethod := range capability.Methods {
				if supportedMethod == method {
					return true
				}
			}
		}
	}
	return false
}

// pathMatchesCapability checks if a path matches a capability pattern
func (dr *DynamicRouter) pathMatchesCapability(path, pattern string) bool {
	// Simple prefix matching for now
	// Could be enhanced with more sophisticated pattern matching
	if pattern == "" {
		return false
	}

	// Handle wildcard patterns like "/v1/*"
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}

	// Handle path parameters like "/agents/{id}"
	if strings.Contains(pattern, "{") && strings.Contains(pattern, "}") {
		// Convert pattern to regex-like matching
		// For now, do simple prefix matching before parameters
		paramStart := strings.Index(pattern, "{")
		if paramStart > 0 {
			prefix := pattern[:paramStart]
			return len(path) >= len(prefix) && path[:len(prefix)] == prefix
		}
	}

	// Exact match
	return path == pattern
}

// isPluginManagementEndpoint checks if the path is a plugin management endpoint
func (dr *DynamicRouter) isPluginManagementEndpoint(path string) bool {
	// Plugin management endpoints are handled by the proxy itself
	pluginPaths := []string{
		"/plugins/register",
		"/plugins/services",
	}

	for _, pluginPath := range pluginPaths {
		if strings.HasPrefix(path, pluginPath) {
			return true
		}
	}
	return false
}

// proxyToService proxies the request to the plugin service
func (dr *DynamicRouter) proxyToService(c *gin.Context, service *plugins.ServiceRegistration, path, method string) {
	// For now, create a simple reverse proxy
	// In a production system, you might want to use a more sophisticated proxy

	// Store original request info
	originalHost := c.Request.Host

	// Set the target URL
	targetURL := service.ServiceURL + path
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// Create a new request to the plugin service
	req, err := http.NewRequestWithContext(c.Request.Context(), method, targetURL, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request"})
		return
	}

	// Copy headers from original request
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Add service information headers
	req.Header.Set("X-Forwarded-Host", originalHost)
	req.Header.Set("X-Forwarded-Proto", c.Request.Proto)
	req.Header.Set("X-Plugin-Service", service.ServiceID)
	req.Header.Set("X-Plugin-Type", string(service.ServiceType))

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "Failed to proxy request to plugin service",
			"service": service.ServiceID,
		})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Set status code
	c.Status(resp.StatusCode)

	// Copy response body
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			c.Writer.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
