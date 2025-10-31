package plugins

import (
	"testing"

	"suse-ai-up/internal/config"
)

func TestServiceManager(t *testing.T) {
	// Create a test config
	cfg := &config.Config{
		Services: config.PluginServicesConfig{
			SmartAgents: config.ServiceConfig{
				Enabled: true,
				URL:     "http://localhost:8910",
				Timeout: "30s",
			},
		},
	}

	// Create service manager
	sm := NewServiceManager(cfg)

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
			SmartAgents: config.ServiceConfig{
				Enabled: false, // Disabled
				URL:     "http://localhost:8910",
				Timeout: "30s",
			},
		},
	}

	sm := NewServiceManager(cfg)

	registration := &ServiceRegistration{
		ServiceID:   "test-smartagents",
		ServiceType: ServiceTypeSmartAgents,
		ServiceURL:  "http://localhost:8910",
		Version:     "1.0.0",
	}

	err := sm.RegisterService(registration)
	if err == nil {
		t.Fatal("Expected error when registering disabled service type")
	}

	expectedError := "service type smartagents is not enabled in configuration"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}
