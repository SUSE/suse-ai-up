package service

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/plugins"
)

// RouteManager handles dynamic route registration for plugin services
type RouteManager struct {
	router         *gin.Engine
	serviceManager plugins.PluginServiceManager
	routeMappings  map[string]*RouteMapping
	mu             sync.RWMutex
}

// RouteMapping represents a mapping between a proxy route and a plugin service
type RouteMapping struct {
	Path         string
	ServiceID    string
	ServiceURL   string
	Methods      []string
	ReverseProxy *httputil.ReverseProxy
}

// NewRouteManager creates a new route manager
func NewRouteManager(router *gin.Engine, serviceManager plugins.PluginServiceManager) *RouteManager {
	return &RouteManager{
		router:         router,
		serviceManager: serviceManager,
		routeMappings:  make(map[string]*RouteMapping),
	}
}

// RegisterServiceRoutes registers routes for a plugin service
func (rm *RouteManager) RegisterServiceRoutes(service *plugins.ServiceRegistration) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	log.Printf("Registering routes for plugin service: %s (%s)",
		service.ServiceID, service.ServiceType)

	// Parse the service URL
	serviceURL, err := url.Parse(service.ServiceURL)
	if err != nil {
		return fmt.Errorf("invalid service URL %s: %w", service.ServiceURL, err)
	}

	// Create reverse proxy
	reverseProxy := httputil.NewSingleHostReverseProxy(serviceURL)

	// Register routes for each capability
	for _, capability := range service.Capabilities {
		routeKey := rm.generateRouteKey(capability.Path, capability.Methods)

		// Create route mapping
		mapping := &RouteMapping{
			Path:         capability.Path,
			ServiceID:    service.ServiceID,
			ServiceURL:   service.ServiceURL,
			Methods:      capability.Methods,
			ReverseProxy: reverseProxy,
		}

		// Store mapping
		rm.routeMappings[routeKey] = mapping

		// Register the route with Gin
		for _, method := range capability.Methods {
			rm.registerRouteWithGin(capability.Path, method, mapping)
		}

		log.Printf("Registered route: %s %s -> %s",
			strings.Join(capability.Methods, ","), capability.Path, service.ServiceID)
	}

	return nil
}

// UnregisterServiceRoutes removes routes for a plugin service
func (rm *RouteManager) UnregisterServiceRoutes(serviceID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	log.Printf("Unregistering routes for plugin service: %s", serviceID)

	// Find and remove all routes for this service
	routesToRemove := make([]string, 0)
	for routeKey, mapping := range rm.routeMappings {
		if mapping.ServiceID == serviceID {
			routesToRemove = append(routesToRemove, routeKey)
		}
	}

	// Remove from our mappings
	for _, routeKey := range routesToRemove {
		delete(rm.routeMappings, routeKey)
	}

	// Note: Gin doesn't provide a way to remove routes dynamically
	// The routes will remain but will return 404 since the mapping is gone
	// In a production system, you might need to restart or use a more sophisticated routing system

	log.Printf("Unregistered %d routes for service: %s", len(routesToRemove), serviceID)
	return nil
}

// GetRouteMapping returns the route mapping for a given path and method
func (rm *RouteManager) GetRouteMapping(path, method string) (*RouteMapping, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	routeKey := rm.generateRouteKey(path, []string{method})
	mapping, exists := rm.routeMappings[routeKey]
	return mapping, exists
}

// GetAllRouteMappings returns all route mappings
func (rm *RouteManager) GetAllRouteMappings() []*RouteMapping {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	mappings := make([]*RouteMapping, 0, len(rm.routeMappings))
	for _, mapping := range rm.routeMappings {
		mappings = append(mappings, mapping)
	}
	return mappings
}

// generateRouteKey creates a unique key for a route
func (rm *RouteManager) generateRouteKey(path string, methods []string) string {
	return strings.Join(methods, ",") + ":" + path
}

// registerRouteWithGin registers a route with the Gin router
func (rm *RouteManager) registerRouteWithGin(path, method string, mapping *RouteMapping) {
	// Create a handler that uses the reverse proxy
	handler := func(c *gin.Context) {
		// Check if the service is still registered and healthy
		service, exists := rm.serviceManager.GetService(mapping.ServiceID)
		if !exists {
			log.Printf("Service %s no longer registered, returning 404", mapping.ServiceID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Service not available"})
			return
		}

		// Check service health
		health, _ := rm.serviceManager.GetServiceHealth(mapping.ServiceID)
		if health.Status != "healthy" {
			log.Printf("Service %s health check failed: %s", mapping.ServiceID, health.Message)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service temporarily unavailable"})
			return
		}

		// Add service information to headers for debugging
		c.Header("X-Plugin-Service", service.ServiceID)
		c.Header("X-Plugin-Type", string(service.ServiceType))

		// Proxy the request
		mapping.ReverseProxy.ServeHTTP(c.Writer, c.Request)
	}

	// Register the route with Gin
	switch strings.ToUpper(method) {
	case "GET":
		rm.router.GET(path, handler)
	case "POST":
		rm.router.POST(path, handler)
	case "PUT":
		rm.router.PUT(path, handler)
	case "DELETE":
		rm.router.DELETE(path, handler)
	case "PATCH":
		rm.router.PATCH(path, handler)
	case "HEAD":
		rm.router.HEAD(path, handler)
	case "OPTIONS":
		rm.router.OPTIONS(path, handler)
	default:
		log.Printf("Unsupported HTTP method: %s", method)
	}
}

// UpdateRoutes updates routes when services change
func (rm *RouteManager) UpdateRoutes() error {
	log.Println("Updating plugin service routes...")

	// Get all registered services
	services := rm.serviceManager.GetAllServices()

	// Clear existing routes
	rm.mu.Lock()
	rm.routeMappings = make(map[string]*RouteMapping)
	rm.mu.Unlock()

	// Re-register all routes
	for _, service := range services {
		if err := rm.RegisterServiceRoutes(service); err != nil {
			log.Printf("Failed to register routes for service %s: %v", service.ServiceID, err)
			continue
		}
	}

	log.Printf("Updated routes for %d plugin services", len(services))
	return nil
}
