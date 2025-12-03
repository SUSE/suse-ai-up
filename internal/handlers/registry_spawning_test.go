package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/internal/config"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

func TestRegistrySpawningWorkflow(t *testing.T) {
	// Setup test environment
	gin.SetMode(gin.TestMode)

	// Create test configuration
	cfg := &config.SpawningConfig{
		RetryAttempts:  1, // Reduce for testing
		RetryBackoffMs: 100,
		DefaultCpu:     "100m",
		DefaultMemory:  "64Mi",
		LogLevel:       "debug",
		IncludeContext: false,
	}

	// Create test store and handlers
	store := clients.NewInMemoryMCPServerStore()
	registryManager := NewDefaultRegistryManager(store)
	adapterStore := clients.NewInMemoryAdapterStore()

	// Create a mock deployment handler that doesn't actually spawn processes
	deploymentHandler := &mockDeploymentHandler{}

	// Create registry handler
	registryHandler := NewRegistryHandler(store, registryManager, deploymentHandler, adapterStore, cfg)

	// Create test MCP server
	testServer := &models.MCPServer{
		ID:          "test-filesystem",
		Name:        "Test Filesystem",
		Description: "Test filesystem server",
		Version:     "1.0.0",
		Packages: []models.Package{
			{
				RegistryType: "npm",
				Identifier:   "@modelcontextprotocol/server-filesystem",
				Transport: models.Transport{
					Type: "stdio",
				},
			},
		},
		Meta: map[string]interface{}{
			"source": "official",
		},
	}

	// Generate config template
	testServer.ConfigTemplate = registryHandler.generateConfigTemplate(testServer, "official")

	// Store the server
	err := store.CreateMCPServer(testServer)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}

	// Test spawning via adapter creation
	t.Run("CreateAdapterFromRegistry", func(t *testing.T) {
		// Create request payload
		reqBody := CreateAdapterFromRegistryRequest{
			ReplicaCount: 1,
			EnvironmentVariables: map[string]string{
				"ALLOWED_DIRS": "/tmp",
			},
		}

		reqBytes, _ := json.Marshal(reqBody)

		// Create HTTP request
		req, _ := http.NewRequest("POST", "/api/v1/registry/test-filesystem/create-adapter", bytes.NewBuffer(reqBytes))
		req.Header.Set("Content-Type", "application/json")

		// Create response recorder
		w := httptest.NewRecorder()

		// Create Gin context
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Params = []gin.Param{{Key: "id", Value: "test-filesystem"}}

		// Call the handler
		registryHandler.CreateAdapterFromRegistry(c)

		// Check response
		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", w.Code)
		}

		// Parse response
		var response CreateAdapterFromRegistryResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		// Verify response structure
		if response.Message == "" {
			t.Error("Expected non-empty message")
		}

		if response.Adapter == nil {
			t.Error("Expected adapter in response")
		}

		if response.McpEndpoint == "" {
			t.Error("Expected MCP endpoint in response")
		}
	})

	// Test config template generation
	t.Run("ConfigTemplateGeneration", func(t *testing.T) {
		template := registryHandler.generateConfigTemplate(testServer, "official")

		if template == nil {
			t.Fatal("Expected non-nil config template")
		}

		if template.Command != "npx" {
			t.Errorf("Expected command 'npx', got '%s'", template.Command)
		}

		if len(template.Args) == 0 || template.Args[0] != "-y" {
			t.Error("Expected npx args to start with '-y'")
		}

		if template.Transport != "stdio" {
			t.Errorf("Expected transport 'stdio', got '%s'", template.Transport)
		}

		if template.ResourceLimits == nil {
			t.Error("Expected resource limits to be set")
		}

		if template.ResourceLimits.CPU != cfg.DefaultCpu {
			t.Errorf("Expected CPU limit '%s', got '%s'", cfg.DefaultCpu, template.ResourceLimits.CPU)
		}
	})

	// Test pre-loaded servers initialization
	t.Run("PreloadedServers", func(t *testing.T) {
		// Check if filesystem server was pre-loaded
		server, err := store.GetMCPServer("filesystem")
		if err != nil {
			t.Errorf("Expected filesystem server to be pre-loaded: %v", err)
		}

		if server == nil {
			t.Error("Expected filesystem server to exist")
		}

		if server.ConfigTemplate == nil {
			t.Error("Expected config template for pre-loaded server")
		}
	})
}

// Mock deployment handler for testing
type mockDeploymentHandler struct{}

func (m *mockDeploymentHandler) DeployMCPDirect(serverID string, envVars map[string]string, replicas int) error {
	// Simulate successful deployment
	time.Sleep(10 * time.Millisecond) // Small delay to simulate work
	return nil
}

func (m *mockDeploymentHandler) GetProcessInfo(serverID string) (*ProcessInfo, error) {
	return &ProcessInfo{
		Cmd:      nil,
		Port:     8080,
		ServerID: serverID,
		Started:  time.Now(),
		Pid:      12345,
	}, nil
}

func (m *mockDeploymentHandler) Shutdown() {
	// No-op for testing
}
