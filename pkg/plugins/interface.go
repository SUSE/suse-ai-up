package plugins

import (
	"context"
	"time"
)

// ServiceType represents the type of plugin service
type ServiceType string

const (
	ServiceTypeSmartAgents ServiceType = "smartagents"
	ServiceTypeRegistry    ServiceType = "registry"
	ServiceTypeVirtualMCP  ServiceType = "virtualmcp"
)

// ServiceCapability represents a capability provided by a plugin service
type ServiceCapability struct {
	Path        string   `json:"path"`    // API path (e.g., "/v1/*", "/agents/*")
	Methods     []string `json:"methods"` // HTTP methods (GET, POST, etc.)
	Description string   `json:"description,omitempty"`
}

// ServiceRegistration contains information about a registered plugin service
type ServiceRegistration struct {
	ServiceID     string              `json:"service_id"`
	ServiceType   ServiceType         `json:"service_type"`
	ServiceURL    string              `json:"service_url"`
	Capabilities  []ServiceCapability `json:"capabilities"`
	Version       string              `json:"version"`
	RegisteredAt  time.Time           `json:"registered_at"`
	LastHeartbeat time.Time           `json:"last_heartbeat"`
}

// ServiceHealth represents the health status of a plugin service
type ServiceHealth struct {
	Status       string    `json:"status"` // "healthy", "unhealthy", "unknown"
	Message      string    `json:"message,omitempty"`
	LastChecked  time.Time `json:"last_checked"`
	ResponseTime int64     `json:"response_time,omitempty"` // Response time in nanoseconds
}

// PluginService defines the interface that all plugin services must implement
type PluginService interface {
	// Register registers the service with the proxy
	Register(ctx context.Context, proxyURL string) (*ServiceRegistration, error)

	// GetCapabilities returns the capabilities provided by this service
	GetCapabilities() []ServiceCapability

	// HealthCheck performs a health check on the service
	HealthCheck(ctx context.Context) ServiceHealth

	// Unregister removes the service registration from the proxy
	Unregister(ctx context.Context) error
}

// PluginServiceManager manages plugin service registrations and routing
type PluginServiceManager interface {
	// RegisterService registers a new plugin service
	RegisterService(registration *ServiceRegistration) error

	// UnregisterService removes a plugin service registration
	UnregisterService(serviceID string) error

	// GetService returns a registered service by ID
	GetService(serviceID string) (*ServiceRegistration, bool)

	// GetServicesByType returns all services of a specific type
	GetServicesByType(serviceType ServiceType) []*ServiceRegistration

	// GetAllServices returns all registered services
	GetAllServices() []*ServiceRegistration

	// IsServiceEnabled checks if a service type is enabled in configuration
	IsServiceEnabled(serviceType ServiceType) bool

	// UpdateServiceHealth updates the health status of a service
	UpdateServiceHealth(serviceID string, health ServiceHealth)

	// GetServiceHealth returns the health status of a service
	GetServiceHealth(serviceID string) (ServiceHealth, bool)

	// GetServiceForPath finds the appropriate service for a given API path
	GetServiceForPath(path string) (*ServiceRegistration, bool)
}
