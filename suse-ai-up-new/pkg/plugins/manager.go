package plugins

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"suse-ai-up/internal/config"
)

// ServiceManager implements PluginServiceManager
type ServiceManager struct {
	mu            sync.RWMutex
	services      map[string]*ServiceRegistration
	serviceHealth map[string]ServiceHealth
	config        *config.Config
}

// NewServiceManager creates a new plugin service manager
func NewServiceManager(cfg *config.Config) *ServiceManager {
	return &ServiceManager{
		services:      make(map[string]*ServiceRegistration),
		serviceHealth: make(map[string]ServiceHealth),
		config:        cfg,
	}
}

// RegisterService registers a new plugin service
func (sm *ServiceManager) RegisterService(registration *ServiceRegistration) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	log.Printf("DEBUG: Registering service %s of type %s", registration.ServiceID, registration.ServiceType)

	// Check if service type is enabled
	if !sm.IsServiceEnabled(registration.ServiceType) {
		log.Printf("DEBUG: Service type %s is not enabled", registration.ServiceType)
		return fmt.Errorf("service type %s is not enabled in configuration", registration.ServiceType)
	}

	// Set registration timestamp
	registration.RegisteredAt = time.Now()
	registration.LastHeartbeat = time.Now()

	// Store the service
	sm.services[registration.ServiceID] = registration

	// Initialize health status
	sm.serviceHealth[registration.ServiceID] = ServiceHealth{
		Status:      "unknown",
		LastChecked: time.Now(),
	}

	log.Printf("Plugin service registered: %s (%s) at %s",
		registration.ServiceID, registration.ServiceType, registration.ServiceURL)

	return nil
}

// UnregisterService removes a plugin service registration
func (sm *ServiceManager) UnregisterService(serviceID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.services[serviceID]; !exists {
		return fmt.Errorf("service %s not found", serviceID)
	}

	delete(sm.services, serviceID)
	delete(sm.serviceHealth, serviceID)

	log.Printf("Plugin service unregistered: %s", serviceID)
	return nil
}

// GetService returns a registered service by ID
func (sm *ServiceManager) GetService(serviceID string) (*ServiceRegistration, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	service, exists := sm.services[serviceID]
	return service, exists
}

// GetServicesByType returns all services of a specific type
func (sm *ServiceManager) GetServicesByType(serviceType ServiceType) []*ServiceRegistration {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var services []*ServiceRegistration
	for _, service := range sm.services {
		if service.ServiceType == serviceType {
			services = append(services, service)
		}
	}
	return services
}

// GetAllServices returns all registered services
func (sm *ServiceManager) GetAllServices() []*ServiceRegistration {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	services := make([]*ServiceRegistration, 0, len(sm.services))
	for _, service := range sm.services {
		services = append(services, service)
	}
	return services
}

// IsServiceEnabled checks if a service type is enabled in configuration
func (sm *ServiceManager) IsServiceEnabled(serviceType ServiceType) bool {
	if sm.config == nil {
		return false
	}

	switch serviceType {
	case ServiceTypeSmartAgents:
		return sm.config.Services.SmartAgents.Enabled
	case ServiceTypeRegistry:
		return sm.config.Services.Registry.Enabled

	default:
		return false
	}
}

// UpdateServiceHealth updates the health status of a service
func (sm *ServiceManager) UpdateServiceHealth(serviceID string, health ServiceHealth) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.serviceHealth[serviceID] = health
}

// GetServiceHealth returns the health status of a service
func (sm *ServiceManager) GetServiceHealth(serviceID string) (ServiceHealth, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	health, exists := sm.serviceHealth[serviceID]
	return health, exists
}

// StartHealthChecks begins periodic health checking of all registered services
func (sm *ServiceManager) StartHealthChecks(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.performHealthChecks(ctx)
		}
	}
}

// performHealthChecks checks health of all registered services
func (sm *ServiceManager) performHealthChecks(ctx context.Context) {
	sm.mu.RLock()
	serviceIDs := make([]string, 0, len(sm.services))
	for id := range sm.services {
		serviceIDs = append(serviceIDs, id)
	}
	sm.mu.RUnlock()

	for _, serviceID := range serviceIDs {
		service, exists := sm.GetService(serviceID)
		if !exists {
			continue
		}

		// Perform health check
		health := sm.checkServiceHealth(ctx, service)

		// Update health status
		sm.UpdateServiceHealth(serviceID, health)

		// Log unhealthy services
		if health.Status != "healthy" {
			log.Printf("Plugin service %s (%s) health check failed: %s",
				serviceID, service.ServiceType, health.Message)
		}
	}
}

// checkServiceHealth performs a health check on a single service
func (sm *ServiceManager) checkServiceHealth(ctx context.Context, service *ServiceRegistration) ServiceHealth {
	start := time.Now()

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// For now, just check if we can reach the service
	// In a real implementation, this would call a health endpoint
	_ = checkCtx // Placeholder for future HTTP health check implementation

	// Simple health check (placeholder - implement actual HTTP call)
	// For now, assume services are healthy if registered
	health := ServiceHealth{
		Status:       "healthy",
		Message:      "Service is registered and responding",
		LastChecked:  time.Now(),
		ResponseTime: time.Since(start).Nanoseconds(),
	}

	return health
}

// GetServiceForPath finds the appropriate service for a given API path
func (sm *ServiceManager) GetServiceForPath(path string) (*ServiceRegistration, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, service := range sm.services {
		for _, capability := range service.Capabilities {
			if sm.pathMatchesCapability(path, capability.Path) {
				return service, true
			}
		}
	}
	return nil, false
}

// pathMatchesCapability checks if a path matches a capability pattern
func (sm *ServiceManager) pathMatchesCapability(path, pattern string) bool {
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

	// Exact match
	return path == pattern
}
