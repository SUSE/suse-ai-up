package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"suse-ai-up/internal/config"
)

func TestServiceManager(t *testing.T) {
	// Create a test config
	cfg := &config.Config{
		Services: config.PluginServicesConfig{
			Services: map[string]config.ServiceConfig{
				"smartagents": {
					Enabled: true,
					URL:     "http://localhost:8910",
					Timeout: "30s",
				},
			},
		},
	}

	// Create service manager
	sm := NewServiceManager(cfg, nil)

	// Test service registration
	registration := &ServiceRegistration{
		ServiceID:   "test-smartagents",
		ServiceType: ServiceTypeSmartAgents,
		ServiceURL:  "http://localhost:8910",
		Version:     "1.0.0",
		Capabilities: []ServiceCapability{
			{
				Path:        "/v1/*",
				Methods:     []string{"POST"},
				Description: "Test capability",
			},
		},
	}

	err := sm.RegisterService(registration)
	if err != nil {
		t.Fatalf("Failed to register service: %v", err)
	}

	// Test service retrieval
	service, exists := sm.GetService("test-smartagents")
	if !exists {
		t.Fatal("Service not found after registration")
	}

	if service.ServiceID != "test-smartagents" {
		t.Errorf("Expected service ID 'test-smartagents', got '%s'", service.ServiceID)
	}

	// Test service lookup by path
	foundService, exists := sm.GetServiceForPath("/v1/chat/completions")
	if !exists {
		t.Fatal("Service not found for path /v1/chat/completions")
	}

	if foundService.ServiceID != "test-smartagents" {
		t.Errorf("Expected service ID 'test-smartagents', got '%s'", foundService.ServiceID)
	}

	// Test service type filtering
	services := sm.GetServicesByType(ServiceTypeSmartAgents)
	if len(services) != 1 {
		t.Errorf("Expected 1 smartagents service, got %d", len(services))
	}

	// Test service unregistration
	err = sm.UnregisterService("test-smartagents")
	if err != nil {
		t.Fatalf("Failed to unregister service: %v", err)
	}

	_, exists = sm.GetService("test-smartagents")
	if exists {
		t.Fatal("Service still exists after unregistration")
	}
}

func TestServiceManagerDisabledService(t *testing.T) {
	// Create a config with smartagents disabled
	cfg := &config.Config{
		Services: config.PluginServicesConfig{
			Services: map[string]config.ServiceConfig{
				"smartagents": {
					Enabled: false, // Disabled
					URL:     "http://localhost:8910",
					Timeout: "30s",
				},
			},
		},
	}

	sm := NewServiceManager(cfg, nil)

	registration := &ServiceRegistration{
		ServiceID:    "test-smartagents",
		ServiceType:  ServiceTypeSmartAgents,
		ServiceURL:   "http://localhost:8910",
		Version:      "1.0.0",
		Capabilities: []ServiceCapability{},
	}

	// With the new flexible system, disabled services in config should still be rejected
	err := sm.RegisterService(registration)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")

	// But custom service types should be allowed
	customRegistration := &ServiceRegistration{
		ServiceID:    "test-custom",
		ServiceType:  ServiceType("custom-service"),
		ServiceURL:   "http://localhost:9000",
		Version:      "1.0.0",
		Capabilities: []ServiceCapability{},
	}

	err = sm.RegisterService(customRegistration)
	assert.NoError(t, err)
}

func TestServiceManagerCustomServiceType(t *testing.T) {
	// Test that custom service types can register themselves
	cfg := &config.Config{
		Services: config.PluginServicesConfig{
			Services: map[string]config.ServiceConfig{}, // Empty map
		},
	}

	sm := NewServiceManager(cfg, nil)

	// Test custom service type
	customServiceType := ServiceType("my-custom-service")
	registration := &ServiceRegistration{
		ServiceID:   "test-custom-service",
		ServiceType: customServiceType,
		ServiceURL:  "http://localhost:9000",
		Version:     "1.0.0",
		Capabilities: []ServiceCapability{
			{
				Path:        "/api/v1/custom",
				Methods:     []string{"GET", "POST"},
				Description: "Custom service endpoints",
			},
		},
	}

	err := sm.RegisterService(registration)
	assert.NoError(t, err)

	// Verify service was registered
	service, exists := sm.GetService("test-custom-service")
	assert.True(t, exists)
	assert.Equal(t, customServiceType, service.ServiceType)

	// Verify the service is enabled (should be true for custom types)
	assert.True(t, sm.IsServiceEnabled(customServiceType))
}
