package services

import (
	"context"
	"testing"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

func TestAdapterService_hasStdioPackage(t *testing.T) {
	service := &AdapterService{}

	// Test server with stdio package
	serverWithStdio := &models.MCPServer{
		Packages: []models.Package{
			{RegistryType: "remote-http"},
			{RegistryType: "stdio"},
		},
	}

	// Test server without stdio package
	serverWithoutStdio := &models.MCPServer{
		Packages: []models.Package{
			{RegistryType: "remote-http"},
			{RegistryType: "docker"},
		},
	}

	if !service.hasStdioPackage(serverWithStdio) {
		t.Error("Expected server with stdio package to return true")
	}

	if service.hasStdioPackage(serverWithoutStdio) {
		t.Error("Expected server without stdio package to return false")
	}
}

func TestAdapterService_getSidecarMeta(t *testing.T) {
	service := &AdapterService{}

	// Test server with sidecar config
	serverWithSidecar := &models.MCPServer{
		Meta: map[string]interface{}{
			"sidecarConfig": map[string]interface{}{
				"dockerImage":   "kskarthik/mcp-bugzilla:latest",
				"dockerCommand": "--bugzilla-server https://bugzilla.example.com --host 0.0.0.0 --port 8000",
			},
		},
	}

	// Test server without sidecar config
	serverWithoutSidecar := &models.MCPServer{
		Meta: map[string]interface{}{},
	}

	meta := service.getSidecarMeta(serverWithSidecar)
	if meta == nil {
		t.Error("Expected to get sidecar meta")
	}
	if meta.DockerImage != "kskarthik/mcp-bugzilla:latest" {
		t.Errorf("Expected docker image to be 'kskarthik/mcp-bugzilla:latest', got '%s'", meta.DockerImage)
	}
	if meta.DockerCommand != "--bugzilla-server https://bugzilla.example.com --host 0.0.0.0 --port 8000" {
		t.Errorf("Expected docker command to be correct, got '%s'", meta.DockerCommand)
	}

	meta2 := service.getSidecarMeta(serverWithoutSidecar)
	if meta2 != nil {
		t.Error("Expected to get nil for server without sidecar config")
	}
}

func TestAdapterService_CreateAdapter_SidecarStdio(t *testing.T) {
	// Create mock stores
	adapterStore := clients.NewInMemoryAdapterStore()
	serverStore := clients.NewInMemoryMCPServerStore()

	// Create test server with stdio package and sidecar config
	testServer := &models.MCPServer{
		ID:   "test-server",
		Name: "Test Server",
		Packages: []models.Package{
			{RegistryType: "stdio"},
		},
		Meta: map[string]interface{}{
			"sidecarConfig": map[string]interface{}{
				"dockerImage":   "kskarthik/mcp-bugzilla:latest",
				"dockerCommand": "--bugzilla-server https://bugzilla.example.com --host 0.0.0.0 --port 8000",
			},
		},
	}

	// Add server to store
	err := serverStore.CreateMCPServer(testServer)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}

	// Create adapter service (without sidecar manager for now)
	service := NewAdapterService(adapterStore, serverStore, nil)

	// Verify server was stored
	storedServer, err := serverStore.GetMCPServer(testServer.ID)
	if err != nil {
		t.Fatalf("Failed to get stored server: %v", err)
	}
	if storedServer == nil {
		t.Fatal("Server was not stored")
	}

	// Check if server has stdio package
	if !service.hasStdioPackage(storedServer) {
		t.Error("Server should have stdio package")
	}

	// Check sidecar meta
	meta := service.getSidecarMeta(storedServer)
	if meta == nil {
		t.Error("Server should have sidecar meta")
	} else {
		t.Logf("Sidecar meta: %+v", meta)
	}

	// Create adapter - this should fail because sidecar manager is required for stdio-based servers
	_, err = service.CreateAdapter(context.Background(), "test-user", testServer.ID, "test-adapter", map[string]string{}, nil)
	if err == nil {
		t.Fatal("Expected adapter creation to fail without sidecar manager")
	}

	// Verify the error message
	expectedError := "sidecar manager not available for adapter deployment"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}
