package services

import (
	"context"
	"testing"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	core_services "suse-ai-up/pkg/services"
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
	adapterGroupAssignmentStore := clients.NewInMemoryAdapterGroupAssignmentStore()
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
	service := NewAdapterService(adapterStore, adapterGroupAssignmentStore, serverStore, nil)

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
	_, err = service.CreateAdapter(context.Background(), "test-user", testServer.ID, "test-adapter", map[string]string{}, nil, nil)
	if err == nil {
		t.Fatal("Expected adapter creation to fail without sidecar manager")
	}

	// Verify the error message
	expectedError := "sidecar manager not available for adapter deployment"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestAdapterService_PermissionChecks(t *testing.T) {
	// Setup In-Memory Stores
	adapterStore := clients.NewInMemoryAdapterStore()
	adapterGroupAssignmentStore := clients.NewInMemoryAdapterGroupAssignmentStore()
	serverStore := clients.NewInMemoryMCPServerStore()
	userStore := clients.NewInMemoryUserStore()
	groupStore := clients.NewInMemoryGroupStore()

	// Setup Services
	userGroupService := core_services.NewUserGroupService(userStore, groupStore)
	service := NewAdapterService(adapterStore, adapterGroupAssignmentStore, serverStore, nil)

	// Setup Context
	ctx := context.Background()

	// 1. Create Users
	adminUser := models.User{ID: "admin", Groups: []string{"admin-group"}}
	regularUser := models.User{ID: "user", Groups: []string{"assignable-group", "write-only-group", "read-only-group"}}
	userStore.Create(ctx, adminUser)
	userStore.Create(ctx, regularUser)

	// 2. Create Groups
	// Admin group can do everything
	adminGroup := models.Group{
		ID:          "admin-group",
		Permissions: []string{"adapter:assign", "adapter:read", "adapter:create", "group:manage", "adapter:*"},
		Members:     []string{"admin"},
	}

	// Group valid for assignment (has adapter:assign and adapter:read)
	assignableGroup := models.Group{
		ID:          "assignable-group",
		Permissions: []string{"adapter:assign", "adapter:read"},
		Members:     []string{"user"},
	}

	// Group that can be assigned to but cannot read adapters (missing adapter:read)
	writeOnlyGroup := models.Group{
		ID:          "write-only-group",
		Permissions: []string{"adapter:assign"},
		Members:     []string{"user"},
	}

	// Group that cannot be assigned to (missing adapter:assign)
	readOnlyGroup := models.Group{
		ID:          "read-only-group",
		Permissions: []string{"adapter:read"},
		Members:     []string{"user"},
	}

	groupStore.Create(ctx, adminGroup)
	groupStore.Create(ctx, assignableGroup)
	groupStore.Create(ctx, writeOnlyGroup)
	groupStore.Create(ctx, readOnlyGroup)

	// 3. Create Adapter (owned by admin)
	adapter := models.AdapterResource{
		AdapterData: models.AdapterData{Name: "test-adapter"},
		ID:          "test-adapter",
		CreatedBy:   "admin",
		CreatedAt:   time.Now(),
	}
	adapterStore.Create(ctx, adapter)

	// Test Case 1: Assign Adapter to Group WITHOUT adapter:assign permission
	// Admin tries to assign to read-only-group
	err := service.AssignAdapterToGroup(ctx, "admin", "test-adapter", "read-only-group", "read", userGroupService)
	if err == nil {
		t.Error("Expected error when assigning to group without adapter:assign permission")
	} else if err.Error() != "insufficient permissions: group read-only-group does not have adapter:assign permission" {
		t.Errorf("Unexpected error message: %v", err)
	}

	// Test Case 2: Assign Adapter to Group WITH adapter:assign permission
	err = service.AssignAdapterToGroup(ctx, "admin", "test-adapter", "assignable-group", "read", userGroupService)
	if err != nil {
		t.Errorf("Expected success when assigning to valid group, got: %v", err)
	}

	// Assign to write-only group (valid for assignment)
	err = service.AssignAdapterToGroup(ctx, "admin", "test-adapter", "write-only-group", "read", userGroupService)
	if err != nil {
		t.Errorf("Expected success when assigning to write-only group, got: %v", err)
	}

	// Test Case 3: List Adapters - Verify filtering based on adapter:read permission

	// "user" is member of:
	// - assignable-group (has adapter:read, has assignment) -> Should show adapter
	// - write-only-group (NO adapter:read, has assignment) -> Should NOT show adapter via this group
	// - read-only-group (has adapter:read, NO assignment) -> Should NOT show adapter (no assignment)

	adapters, err := service.ListAdapters(ctx, "user", userGroupService)
	if err != nil {
		t.Fatalf("ListAdapters failed: %v", err)
	}

	// We expect to see "test-adapter" because it is assigned to "assignable-group" which has "adapter:read"
	found := false
	for _, a := range adapters {
		if a.ID == "test-adapter" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find test-adapter for user (via assignable-group)")
	}

	// Now, let's remove assignment from "assignable-group" and keep it on "write-only-group"
	// Since "write-only-group" does not have "adapter:read", the user should NOT see the adapter even if assigned.
	service.RemoveAdapterFromGroup(ctx, "admin", "test-adapter", "assignable-group", userGroupService)

	adapters, err = service.ListAdapters(ctx, "user", userGroupService)
	if err != nil {
		t.Fatalf("ListAdapters failed: %v", err)
	}

	found = false
	for _, a := range adapters {
		if a.ID == "test-adapter" {
			found = true
			break
		}
	}
	if found {
		t.Error("Expected NOT to find test-adapter because write-only-group lacks adapter:read permission")
	}
}
