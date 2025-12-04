package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"suse-ai-up/internal/config"
	"suse-ai-up/pkg/models"
)

// RegistryManagerInterface defines the interface for registry management
type RegistryManagerInterface interface {
	UploadRegistryEntries(entries []*models.MCPServer) error
}

// ServiceManager implements PluginServiceManager
type ServiceManager struct {
	mu              sync.RWMutex
	services        map[string]*ServiceRegistration
	serviceHealth   map[string]ServiceHealth
	config          *config.Config
	httpClient      *http.Client
	registryManager RegistryManagerInterface
}

// NewServiceManager creates a new plugin service manager
func NewServiceManager(cfg *config.Config, registryManager RegistryManagerInterface) *ServiceManager {
	return &ServiceManager{
		services:        make(map[string]*ServiceRegistration),
		serviceHealth:   make(map[string]ServiceHealth),
		config:          cfg,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		registryManager: registryManager,
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

	// If this is a VirtualMCP service, discover its MCP implementations
	if registration.ServiceType == ServiceTypeVirtualMCP {
		go sm.discoverMCPImplementations(registration)
	}

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
// By default, allows all service types to register themselves
func (sm *ServiceManager) IsServiceEnabled(serviceType ServiceType) bool {
	if sm.config == nil {
		return true // Allow all services if no config
	}

	// Check if the service type is explicitly configured
	if serviceConfig, exists := sm.config.Services.Services[string(serviceType)]; exists {
		return serviceConfig.Enabled
	}

	// Allow any service type that registers itself
	// This enables extensibility for new plugin types
	return true
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
			sm.performMCPSync(ctx) // Also sync MCP implementations
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

// performMCPSync performs periodic sync of MCP implementations from VirtualMCP services
func (sm *ServiceManager) performMCPSync(ctx context.Context) {
	sm.mu.RLock()
	serviceIDs := make([]string, 0, len(sm.services))
	for id, service := range sm.services {
		if service.ServiceType == ServiceTypeVirtualMCP {
			serviceIDs = append(serviceIDs, id)
		}
	}
	sm.mu.RUnlock()

	for _, serviceID := range serviceIDs {
		service, exists := sm.GetService(serviceID)
		if !exists {
			continue
		}

		// Perform MCP sync
		select {
		case <-ctx.Done():
			return
		default:
			sm.discoverMCPImplementations(service)
		}
	}
}

// checkServiceHealth performs a health check on a single service
func (sm *ServiceManager) checkServiceHealth(ctx context.Context, service *ServiceRegistration) ServiceHealth {
	start := time.Now()

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var health ServiceHealth
	health.LastChecked = time.Now()

	// Perform service-specific health checks
	switch service.ServiceType {
	case ServiceTypeVirtualMCP:
		health = sm.checkVirtualMCPHealth(checkCtx, service)
	default:
		// For other services, perform basic connectivity check
		health = sm.checkBasicServiceHealth(checkCtx, service)
	}

	health.ResponseTime = time.Since(start).Nanoseconds()
	return health
}

// checkBasicServiceHealth performs a basic health check for non-VirtualMCP services
func (sm *ServiceManager) checkBasicServiceHealth(ctx context.Context, service *ServiceRegistration) ServiceHealth {
	// For now, assume services are healthy if registered
	// In a real implementation, this would call a health endpoint
	return ServiceHealth{
		Status:  "healthy",
		Message: "Service is registered",
	}
}

// checkVirtualMCPHealth performs health check specific to VirtualMCP services
func (sm *ServiceManager) checkVirtualMCPHealth(ctx context.Context, service *ServiceRegistration) ServiceHealth {
	// Check the basic service health endpoint
	healthURL := fmt.Sprintf("%s/health", service.ServiceURL)

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return ServiceHealth{
			Status:  "unhealthy",
			Message: fmt.Sprintf("Failed to create health check request: %v", err),
		}
	}

	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return ServiceHealth{
			Status:  "unhealthy",
			Message: fmt.Sprintf("Health check failed: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ServiceHealth{
			Status:  "unhealthy",
			Message: fmt.Sprintf("Health check returned status %d", resp.StatusCode),
		}
	}

	// Also check the MCP discovery endpoint
	mcpURL := fmt.Sprintf("%s/api/v1/mcps", service.ServiceURL)
	req2, err := http.NewRequestWithContext(ctx, "GET", mcpURL, nil)
	if err != nil {
		return ServiceHealth{
			Status:  "degraded",
			Message: "Basic health OK but MCP discovery check failed",
		}
	}

	resp2, err := sm.httpClient.Do(req2)
	if err != nil {
		return ServiceHealth{
			Status:  "degraded",
			Message: fmt.Sprintf("Basic health OK but MCP discovery failed: %v", err),
		}
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return ServiceHealth{
			Status:  "degraded",
			Message: fmt.Sprintf("Basic health OK but MCP discovery returned status %d", resp2.StatusCode),
		}
	}

	// Parse MCP discovery response to get implementation count
	var discoveryResponse struct {
		Count int `json:"count"`
	}
	body, err := io.ReadAll(resp2.Body)
	if err == nil {
		json.Unmarshal(body, &discoveryResponse)
	}

	return ServiceHealth{
		Status:  "healthy",
		Message: fmt.Sprintf("VirtualMCP service healthy with %d MCP implementations", discoveryResponse.Count),
	}
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

// discoverMCPImplementations discovers MCP implementations from a VirtualMCP service
func (sm *ServiceManager) discoverMCPImplementations(service *ServiceRegistration) {
	log.Printf("Discovering MCP implementations from VirtualMCP service: %s", service.ServiceID)

	// Construct the MCP discovery URL
	discoveryURL := fmt.Sprintf("%s/api/v1/mcps", service.ServiceURL)

	// Make HTTP request to discover MCP implementations
	resp, err := sm.httpClient.Get(discoveryURL)
	if err != nil {
		log.Printf("Failed to discover MCP implementations from %s: %v", service.ServiceID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("MCP discovery failed for %s: status %d", service.ServiceID, resp.StatusCode)
		return
	}

	// Parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read MCP discovery response from %s: %v", service.ServiceID, err)
		return
	}

	var discoveryResponse struct {
		Implementations []map[string]interface{} `json:"implementations"`
		Count           int                      `json:"count"`
		Service         string                   `json:"service"`
	}

	if err := json.Unmarshal(body, &discoveryResponse); err != nil {
		log.Printf("Failed to parse MCP discovery response from %s: %v", service.ServiceID, err)
		return
	}

	log.Printf("Discovered %d MCP implementations from %s", len(discoveryResponse.Implementations), service.ServiceID)

	// Convert to MCPServer entries and register them
	var mcpServers []*models.MCPServer
	for _, impl := range discoveryResponse.Implementations {
		mcpServer := sm.convertMCPImplementationToMCPServer(impl, service)
		if mcpServer != nil {
			mcpServers = append(mcpServers, mcpServer)
		}
	}

	// Register the MCP servers in the registry
	if len(mcpServers) > 0 && sm.registryManager != nil {
		if err := sm.registryManager.UploadRegistryEntries(mcpServers); err != nil {
			log.Printf("Failed to register MCP implementations from %s: %v", service.ServiceID, err)
		} else {
			log.Printf("Successfully registered %d MCP implementations from %s", len(mcpServers), service.ServiceID)
		}
	}
}

// convertMCPImplementationToMCPServer converts an MCP implementation to an MCPServer
func (sm *ServiceManager) convertMCPImplementationToMCPServer(impl map[string]interface{}, service *ServiceRegistration) *models.MCPServer {
	server := &models.MCPServer{
		ValidationStatus: "new",
		DiscoveredAt:     time.Now(),
		Meta: map[string]interface{}{
			"source":         "virtualmcp",
			"service_id":     service.ServiceID,
			"service_url":    service.ServiceURL,
			"discovery_time": time.Now().Format(time.RFC3339),
		},
	}

	// Extract basic fields
	if id, ok := impl["id"].(string); ok {
		server.ID = fmt.Sprintf("virtualmcp-%s-%s", service.ServiceID, id)
	} else {
		return nil // ID is required
	}

	if name, ok := impl["name"].(string); ok {
		server.Name = name
	}

	if description, ok := impl["description"].(string); ok {
		server.Description = description
	}

	server.Version = "1.0.0" // Default version
	if version, ok := impl["version"].(string); ok {
		server.Version = version
	}

	// Handle packages - create a package entry for this implementation
	server.Packages = []models.Package{
		{
			RegistryType: "virtualmcp",
			Identifier:   server.ID,
			Transport: models.Transport{
				Type: "http", // Use HTTP transport for virtualMCP
			},
		},
	}

	// Handle tools
	if tools, ok := impl["tools"].([]interface{}); ok {
		server.Tools = make([]models.MCPTool, 0, len(tools))
		for _, toolData := range tools {
			if toolMap, ok := toolData.(map[string]interface{}); ok {
				tool := models.MCPTool{}
				if name, ok := toolMap["name"].(string); ok {
					tool.Name = name
				}
				if description, ok := toolMap["description"].(string); ok {
					tool.Description = description
				}
				if inputSchema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
					tool.InputSchema = inputSchema
				}
				// Handle VirtualMCP specific fields
				if sourceType, ok := toolMap["source_type"].(string); ok {
					tool.SourceType = sourceType
				}
				if config, ok := toolMap["config"].(map[string]interface{}); ok {
					tool.Config = config
				}
				server.Tools = append(server.Tools, tool)
			}
		}
	}

	return server
}
