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
		Image: "kskarthik/mcp-bugzilla:latest",
		Meta: map[string]interface{}{
			"sidecarConfig": map[string]interface{}{
				"commandType": "docker",
				"command":     "docker",
				"args":        []interface{}{"run", "-e", "BUGZILLA_SERVER=https://bugzilla.example.com", "--host", "0.0.0.0", "--port", "8000"},
				"port":        8000.0,
			},
		},
	}

	// Test server without sidecar config
	serverWithoutSidecar := &models.MCPServer{
		Meta: map[string]interface{}{},
	}

	meta := service.getSidecarMeta(serverWithSidecar, map[string]string{})
	if meta == nil {
		t.Error("Expected to get sidecar meta")
	}
	if meta.CommandType != "docker" {
		t.Errorf("Expected command type to be 'docker', got '%s'", meta.CommandType)
	}
	if meta.Command != "docker" {
		t.Errorf("Expected command to be 'docker', got '%s'", meta.Command)
	}

	if len(meta.Args) != 5 {
		t.Errorf("Expected 5 args after env var parsing, got %d", len(meta.Args))
	}

	meta2 := service.getSidecarMeta(serverWithoutSidecar, map[string]string{})
	if meta2 != nil {
		t.Error("Expected to get nil for server without sidecar config")
	}
}

func TestAdapterService_UyuniSidecarExtraction(t *testing.T) {
	service := &AdapterService{}

	// Test uyuni server with sidecar config (similar to YAML)
	serverUyuni := &models.MCPServer{
		Name:  "uyuni",
		Image: "ghcr.io/uyuni-project/mcp-server-uyuni:latest",
		Meta: map[string]interface{}{
			"sidecarConfig": map[string]interface{}{
				"commandType": "docker",
				"command":     "docker",
				"args": []interface{}{
					"run", "-i", "--rm",
					"-e", "UYUNI_SERVER={{uyuni.server}}",
					"-e", "UYUNI_USER={{uyuni.user}}",
					"-e", "UYUNI_PASS={{uyuni.pass}}",
					"-e", "UYUNI_MCP_TRANSPORT=http",
					"-e", "UYUNI_MCP_HOST=0.0.0.0",
					"ghcr.io/uyuni-project/mcp-server-uyuni:latest",
				},
				"port": 8000.0,
			},
		},
	}

	// Test with some environment variables
	envVars := map[string]string{
		"uyuni.server": "http://uyuni.example.com",
		"uyuni.user":   "admin",
		"uyuni.pass":   "secret",
	}

	meta := service.getSidecarMeta(serverUyuni, envVars)
	if meta == nil {
		t.Error("Expected to get sidecar meta for uyuni")
		return
	}

	t.Logf("Uyuni Sidecar Meta:")
	t.Logf("  CommandType: %s", meta.CommandType)
	t.Logf("  Command: %s", meta.Command)
	t.Logf("  Args: %+v", meta.Args)
	t.Logf("  Env: %+v", meta.Env)
	t.Logf("  Port: %d", meta.Port)

	// Verify expected results
	if meta.CommandType != "docker" {
		t.Errorf("Expected CommandType 'docker', got '%s'", meta.CommandType)
	}
	if meta.Command != "docker" {
		t.Errorf("Expected Command 'docker', got '%s'", meta.Command)
	}
	if len(meta.Args) != 4 { // run, -i, --rm, image
		t.Errorf("Expected 4 args, got %d: %+v", len(meta.Args), meta.Args)
	}
	if len(meta.Env) != 5 { // 5 environment variables
		t.Errorf("Expected 5 env vars, got %d: %+v", len(meta.Env), meta.Env)
	}
}

func TestAdapterService_BugzillaSidecarExtraction(t *testing.T) {
	service := &AdapterService{}

	// Test bugzilla server with sidecar config (similar to YAML)
	serverBugzilla := &models.MCPServer{
		Name:  "bugzilla",
		Image: "kskarthik/mcp-bugzilla:latest",
		Meta: map[string]interface{}{
			"sidecarConfig": map[string]interface{}{
				"commandType": "docker",
				"command":     "docker",
				"args": []interface{}{
					"run", "-e", "BUGZILLA_SERVER={{bugzilla.server}}",
					"--host", "0.0.0.0", "--port", "8000",
				},
				"port": 8000.0,
			},
		},
	}

	// Test with environment variables
	envVars := map[string]string{
		"bugzilla.server": "https://bugzilla.suse.com",
	}

	meta := service.getSidecarMeta(serverBugzilla, envVars)
	if meta == nil {
		t.Error("Expected to get sidecar meta for bugzilla")
		return
	}

	t.Logf("Bugzilla Sidecar Meta:")
	t.Logf("  CommandType: %s", meta.CommandType)
	t.Logf("  Command: %s", meta.Command)
	t.Logf("  Args: %+v", meta.Args)
	t.Logf("  Env: %+v", meta.Env)
	t.Logf("  Port: %d", meta.Port)

	// Verify expected results
	if meta.CommandType != "docker" {
		t.Errorf("Expected CommandType 'docker', got '%s'", meta.CommandType)
	}
	if len(meta.Args) != 5 { // run, --host, 0.0.0.0, --port, 8000 (image appended later)
		t.Errorf("Expected 5 args, got %d: %+v", len(meta.Args), meta.Args)
	}
	if len(meta.Env) != 1 { // 1 environment variable
		t.Errorf("Expected 1 env var, got %d: %+v", len(meta.Env), meta.Env)
	}
}

func TestAdapterService_CreateAdapter_SidecarStdio(t *testing.T) {
	// Create mock stores
	adapterStore := clients.NewInMemoryAdapterStore()
	serverStore := clients.NewInMemoryMCPServerStore()

	// Create test server with stdio package and sidecar config
	testServer := &models.MCPServer{
		ID:    "test-server",
		Name:  "Test Server",
		Image: "kskarthik/mcp-bugzilla:latest",
		Packages: []models.Package{
			{RegistryType: "stdio"},
		},
		Meta: map[string]interface{}{
			"sidecarConfig": map[string]interface{}{
				"commandType": "docker",
				"command":     "docker",
				"args":        []interface{}{"run", "-e", "BUGZILLA_SERVER=https://bugzilla.example.com", "--host", "0.0.0.0", "--port", "8000"},
				"port":        8000.0,
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
	meta := service.getSidecarMeta(storedServer, map[string]string{})
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
