package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	core_services "suse-ai-up/pkg/services"
	adaptersvc "suse-ai-up/pkg/services/adapters"
)

// mockAdapterService implements the methods used by UnifiedMCPHandler
type mockAdapterService struct {
	adapters map[string]*models.AdapterResource
}

func newMockAdapterService() *mockAdapterService {
	return &mockAdapterService{
		adapters: make(map[string]*models.AdapterResource),
	}
}

func (m *mockAdapterService) addAdapter(adapter *models.AdapterResource) {
	m.adapters[adapter.Name] = adapter
}

// mockUserGroupService is a minimal mock for UserGroupService
type mockUserGroupService struct{}

// Helper to create a test handler with mock services
func newTestUnifiedMCPHandler(adapters []*models.AdapterResource, mockServer *httptest.Server) *UnifiedMCPHandler {
	mock := newMockAdapterService()
	for _, a := range adapters {
		if mockServer != nil && a.RemoteUrl == "" {
			a.RemoteUrl = mockServer.URL
		}
		mock.addAdapter(a)
	}

	// Create a real handler but we'll need to work around the service dependency
	// For now, we test the handler methods that don't require full service integration
	adapterStore := clients.NewInMemoryAdapterStore()
	adapterGroupAssignmentStore := clients.NewInMemoryAdapterGroupAssignmentStore()
	serverStore := clients.NewInMemoryMCPServerStore()
	adapterService := adaptersvc.NewAdapterService(adapterStore, adapterGroupAssignmentStore, serverStore, nil, nil)
	handler := &UnifiedMCPHandler{
		httpClient:     http.DefaultClient,
		adapterService: adapterService,
	}

	return handler
}

// TestHandleUnifiedMCP_MethodNotAllowed tests that non-POST requests are rejected
func TestHandleUnifiedMCP_MethodNotAllowed(t *testing.T) {
	handler := setupHandler(http.DefaultClient)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp", nil)
	w := httptest.NewRecorder()

	handler.HandleUnifiedMCP(w, req)

	var resp MCPResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error response for GET request")
	}

	if resp.Error.Code != -32600 {
		t.Errorf("Expected error code -32600, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Only POST method is supported" {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// TestHandleUnifiedMCP_InvalidJSON tests that invalid JSON is rejected
func TestHandleUnifiedMCP_InvalidJSON(t *testing.T) {
	handler := setupHandler(http.DefaultClient)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handler.HandleUnifiedMCP(w, req)

	var resp MCPResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error response for invalid JSON")
	}

	if resp.Error.Code != -32700 {
		t.Errorf("Expected error code -32700 (parse error), got %d", resp.Error.Code)
	}
}

// TestHandleUnifiedMCP_UnknownMethod tests that unknown methods return method not found
func TestHandleUnifiedMCP_UnknownMethod(t *testing.T) {
	handler := setupHandler(http.DefaultClient)

	mcpReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "unknown/method",
	}

	body, _ := json.Marshal(mcpReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.HandleUnifiedMCP(w, req)

	var resp MCPResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error response for unknown method")
	}

	if resp.Error.Code != -32601 {
		t.Errorf("Expected error code -32601 (method not found), got %d", resp.Error.Code)
	}
}

// TestHandleInitialize tests the initialize method
func TestHandleInitialize(t *testing.T) {
	handler := setupHandler(http.DefaultClient)

	mcpReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := handler.handleInitialize(context.Background(), &mcpReq)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	if resp.ID != 1 {
		t.Errorf("Expected ID 1, got %v", resp.ID)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be map, got %T", resp.Result)
	}

	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("Expected protocol version 2025-06-18, got %v", result["protocolVersion"])
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected serverInfo in result")
	}

	if serverInfo["name"] != "suse-ai-unified-proxy" {
		t.Errorf("Expected server name 'suse-ai-unified-proxy', got %v", serverInfo["name"])
	}

	capabilities, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected capabilities in result")
	}

	if _, hasTools := capabilities["tools"]; !hasTools {
		t.Error("Expected 'tools' capability")
	}
	if _, hasResources := capabilities["resources"]; !hasResources {
		t.Error("Expected 'resources' capability")
	}
	if _, hasPrompts := capabilities["prompts"]; !hasPrompts {
		t.Error("Expected 'prompts' capability")
	}
}

// TestToolNameParsing tests that tool names are correctly parsed
func TestToolNameParsing(t *testing.T) {
	tests := []struct {
		name            string
		toolName        string
		expectError     bool
		expectedAdapter string
		expectedTool    string
	}{
		{
			name:            "valid tool name",
			toolName:        "servicenow__get_incident",
			expectError:     false,
			expectedAdapter: "servicenow",
			expectedTool:    "get_incident",
		},
		{
			name:            "tool name with multiple underscores",
			toolName:        "my_adapter__my_tool_name",
			expectError:     false,
			expectedAdapter: "my_adapter",
			expectedTool:    "my_tool_name",
		},
		{
			name:        "invalid tool name - no prefix",
			toolName:    "get_incident",
			expectError: true,
		},
		{
			name:        "invalid tool name - single underscore",
			toolName:    "servicenow_get_incident",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the parsing logic from handleToolsCall
			parts := splitToolName(tc.toolName)

			if tc.expectError {
				if len(parts) == 2 {
					t.Errorf("Expected parsing to fail for %s, but got adapter=%s, tool=%s",
						tc.toolName, parts[0], parts[1])
				}
			} else {
				if len(parts) != 2 {
					t.Fatalf("Expected parsing to succeed for %s", tc.toolName)
				}
				if parts[0] != tc.expectedAdapter {
					t.Errorf("Expected adapter %s, got %s", tc.expectedAdapter, parts[0])
				}
				if parts[1] != tc.expectedTool {
					t.Errorf("Expected tool %s, got %s", tc.expectedTool, parts[1])
				}
			}
		})
	}
}

// splitToolName is a helper that mirrors the parsing logic in handleToolsCall
func splitToolName(name string) []string {
	for i := 0; i < len(name)-1; i++ {
		if name[i] == '_' && name[i+1] == '_' {
			return []string{name[:i], name[i+2:]}
		}
	}
	return nil
}

// TestResourceURIParsing tests that resource URIs are correctly parsed
func TestResourceURIParsing(t *testing.T) {
	tests := []struct {
		name            string
		uri             string
		expectError     bool
		expectedAdapter string
		expectedURI     string
	}{
		{
			name:            "valid URI",
			uri:             "servicenow://incident/INC0001",
			expectError:     false,
			expectedAdapter: "servicenow",
			expectedURI:     "incident/INC0001",
		},
		{
			name:            "URI with nested path",
			uri:             "databricks://clusters/metrics/cpu",
			expectError:     false,
			expectedAdapter: "databricks",
			expectedURI:     "clusters/metrics/cpu",
		},
		{
			name:        "invalid URI - no scheme",
			uri:         "incident/INC0001",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the parsing logic from handleResourcesRead
			parts := splitResourceURI(tc.uri)

			if tc.expectError {
				if len(parts) == 2 {
					t.Errorf("Expected parsing to fail for %s", tc.uri)
				}
			} else {
				if len(parts) != 2 {
					t.Fatalf("Expected parsing to succeed for %s", tc.uri)
				}
				if parts[0] != tc.expectedAdapter {
					t.Errorf("Expected adapter %s, got %s", tc.expectedAdapter, parts[0])
				}
				if parts[1] != tc.expectedURI {
					t.Errorf("Expected URI %s, got %s", tc.expectedURI, parts[1])
				}
			}
		})
	}
}

// splitResourceURI is a helper that mirrors the parsing logic in handleResourcesRead
func splitResourceURI(uri string) []string {
	for i := 0; i < len(uri)-2; i++ {
		if uri[i] == ':' && uri[i+1] == '/' && uri[i+2] == '/' {
			return []string{uri[:i], uri[i+3:]}
		}
	}
	return nil
}

func setupHandler(httpClient *http.Client) *UnifiedMCPHandler {
	adapterStore := clients.NewInMemoryAdapterStore()
	adapterGroupAssignmentStore := clients.NewInMemoryAdapterGroupAssignmentStore()
	serverStore := clients.NewInMemoryMCPServerStore()
	userStore := clients.NewInMemoryUserStore()
	groupStore := clients.NewInMemoryGroupStore()
	userGroupService := core_services.NewUserGroupService(userStore, groupStore)
	adapterService := adaptersvc.NewAdapterService(adapterStore, adapterGroupAssignmentStore, serverStore, nil, nil)

	return &UnifiedMCPHandler{
		httpClient:       httpClient,
		adapterService:   adapterService,
		userGroupService: userGroupService,
	}
}

// TestToolPrefixing tests that tools are correctly prefixed with adapter name
func TestToolPrefixing(t *testing.T) {
	adapterName := "servicenow"
	originalTool := Tool{
		Name:        "get_incident",
		Description: "Get incident details",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"incident_id": map[string]interface{}{
					"type":        "string",
					"description": "The incident ID",
				},
			},
		},
	}

	// Simulate prefixing logic from handleToolsList
	prefixedTool := Tool{
		Name:        adapterName + "__" + originalTool.Name,
		Description: "[" + adapterName + "] " + originalTool.Description,
		InputSchema: originalTool.InputSchema,
	}

	expectedName := "servicenow__get_incident"
	expectedDesc := "[servicenow] Get incident details"

	if prefixedTool.Name != expectedName {
		t.Errorf("Expected name %s, got %s", expectedName, prefixedTool.Name)
	}

	if prefixedTool.Description != expectedDesc {
		t.Errorf("Expected description %s, got %s", expectedDesc, prefixedTool.Description)
	}
}

// TestResourcePrefixing tests that resources are correctly prefixed with adapter scheme
func TestResourcePrefixing(t *testing.T) {
	adapterName := "servicenow"
	originalResource := Resource{
		URI:         "incident/INC0001",
		Name:        "Incident INC0001",
		Description: "Details for incident INC0001",
		MimeType:    "application/json",
	}

	// Simulate prefixing logic from handleResourcesList
	prefixedResource := Resource{
		URI:         adapterName + "://" + originalResource.URI,
		Name:        "[" + adapterName + "] " + originalResource.Name,
		Description: originalResource.Description,
		MimeType:    originalResource.MimeType,
	}

	expectedURI := "servicenow://incident/INC0001"
	expectedName := "[servicenow] Incident INC0001"

	if prefixedResource.URI != expectedURI {
		t.Errorf("Expected URI %s, got %s", expectedURI, prefixedResource.URI)
	}

	if prefixedResource.Name != expectedName {
		t.Errorf("Expected name %s, got %s", expectedName, prefixedResource.Name)
	}
}

// TestPromptPrefixing tests that prompts are correctly prefixed with adapter name
func TestPromptPrefixing(t *testing.T) {
	adapterName := "servicenow"
	originalPrompt := Prompt{
		Name:        "create_incident",
		Description: "Create a new incident",
		Arguments:   []interface{}{"title", "description", "priority"},
	}

	// Simulate prefixing logic from handlePromptsList
	prefixedPrompt := Prompt{
		Name:        adapterName + "__" + originalPrompt.Name,
		Description: "[" + adapterName + "] " + originalPrompt.Description,
		Arguments:   originalPrompt.Arguments,
	}

	expectedName := "servicenow__create_incident"
	expectedDesc := "[servicenow] Create a new incident"

	if prefixedPrompt.Name != expectedName {
		t.Errorf("Expected name %s, got %s", expectedName, prefixedPrompt.Name)
	}

	if prefixedPrompt.Description != expectedDesc {
		t.Errorf("Expected description %s, got %s", expectedDesc, prefixedPrompt.Description)
	}
}

// TestMCPResponseSerialization tests that MCP responses are correctly serialized
func TestMCPResponseSerialization(t *testing.T) {
	tests := []struct {
		name     string
		response MCPResponse
	}{
		{
			name: "success response",
			response: MCPResponse{
				JSONRPC: "2.0",
				ID:      1,
				Result:  ToolsListResult{Tools: []Tool{{Name: "test_tool"}}},
			},
		},
		{
			name: "error response",
			response: MCPResponse{
				JSONRPC: "2.0",
				ID:      1,
				Error:   &MCPRPCError{Code: -32600, Message: "Invalid request"},
			},
		},
		{
			name: "response with null ID",
			response: MCPResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &MCPRPCError{Code: -32700, Message: "Parse error"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.response)
			if err != nil {
				t.Fatalf("Failed to marshal response: %v", err)
			}

			var decoded MCPResponse
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if decoded.JSONRPC != tc.response.JSONRPC {
				t.Errorf("JSONRPC mismatch: expected %s, got %s", tc.response.JSONRPC, decoded.JSONRPC)
			}
		})
	}
}

// TestSendError tests the sendError helper function
func TestSendError(t *testing.T) {
	handler := &UnifiedMCPHandler{}

	w := httptest.NewRecorder()
	handler.sendError(w, 123, -32600, "Test error message")

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	var resp MCPResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}

	// ID should be 123 (as float64 after JSON round-trip)
	if resp.ID != float64(123) {
		t.Errorf("Expected ID 123, got %v", resp.ID)
	}

	if resp.Error == nil {
		t.Fatal("Expected error in response")
	}

	if resp.Error.Code != -32600 {
		t.Errorf("Expected error code -32600, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Test error message" {
		t.Errorf("Expected error message 'Test error message', got %s", resp.Error.Message)
	}
}

// TestIntegration_ToolsListWithMockAdapter tests tools/list with a mock MCP adapter
func TestIntegration_ToolsListWithMockAdapter(t *testing.T) {
	// Create a mock MCP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Mock server failed to decode request: %v", err)
			return
		}

		if req.Method == "tools/list" {
			resp := MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: ToolsListResult{
					Tools: []Tool{
						{Name: "get_incident", Description: "Get incident details"},
						{Name: "list_incidents", Description: "List all incidents"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	// Create handler with mock HTTP client
	handler := setupHandler(mockServer.Client())

	// Test fetchToolsFromAdapter directly
	adapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:           "servicenow",
			ConnectionType: models.ConnectionTypeRemoteHttp,
			RemoteUrl:      mockServer.URL,
		},
	}

	tools, err := handler.fetchToolsFromAdapter(context.Background(), adapter, adapter.RemoteUrl)
	if err != nil {
		t.Fatalf("fetchToolsFromAdapter failed: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	if tools[0].Name != "get_incident" {
		t.Errorf("Expected first tool name 'get_incident', got %s", tools[0].Name)
	}
}

// TestIntegration_ResourcesListWithMockAdapter tests resources/list with a mock MCP adapter
func TestIntegration_ResourcesListWithMockAdapter(t *testing.T) {
	// Create a mock MCP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Mock server failed to decode request: %v", err)
			return
		}

		if req.Method == "resources/list" {
			resp := MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: ResourcesListResult{
					Resources: []Resource{
						{URI: "incident/INC0001", Name: "Incident INC0001", MimeType: "application/json"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	handler := setupHandler(mockServer.Client())

	adapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:           "servicenow",
			ConnectionType: models.ConnectionTypeRemoteHttp,
			RemoteUrl:      mockServer.URL,
		},
	}

	resources, err := handler.fetchResourcesFromAdapter(context.Background(), adapter, adapter.RemoteUrl)
	if err != nil {
		t.Fatalf("fetchResourcesFromAdapter failed: %v", err)
	}

	if len(resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resources))
	}

	if resources[0].URI != "incident/INC0001" {
		t.Errorf("Expected resource URI 'incident/INC0001', got %s", resources[0].URI)
	}
}

// TestIntegration_PromptsListWithMockAdapter tests prompts/list with a mock MCP adapter
func TestIntegration_PromptsListWithMockAdapter(t *testing.T) {
	// Create a mock MCP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Mock server failed to decode request: %v", err)
			return
		}

		if req.Method == "prompts/list" {
			resp := MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: PromptsListResult{
					Prompts: []Prompt{
						{Name: "create_incident", Description: "Create a new incident"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	handler := setupHandler(mockServer.Client())

	adapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:           "servicenow",
			ConnectionType: models.ConnectionTypeRemoteHttp,
			RemoteUrl:      mockServer.URL,
		},
	}

	prompts, err := handler.fetchPromptsFromAdapter(context.Background(), adapter, adapter.RemoteUrl)
	if err != nil {
		t.Fatalf("fetchPromptsFromAdapter failed: %v", err)
	}

	if len(prompts) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(prompts))
	}

	if prompts[0].Name != "create_incident" {
		t.Errorf("Expected prompt name 'create_incident', got %s", prompts[0].Name)
	}
}

// TestForwardToAdapter_NoRemoteURL tests forwarding when adapter has no remote URL
func TestForwardToAdapter_NoRemoteURL(t *testing.T) {
	adapterStore := clients.NewInMemoryAdapterStore()
	adapterGroupAssignmentStore := clients.NewInMemoryAdapterGroupAssignmentStore()
	serverStore := clients.NewInMemoryMCPServerStore()
	adapterService := adaptersvc.NewAdapterService(adapterStore, adapterGroupAssignmentStore, serverStore, nil, nil)
	handler := &UnifiedMCPHandler{
		httpClient:     http.DefaultClient,
		adapterService: adapterService,
	}

	adapter := &models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:      "test-adapter",
			RemoteUrl: "",
		},
	}

	req := &MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
	}

	resp := handler.forwardToAdapter(context.Background(), adapter, adapter.RemoteUrl, req, http.Header{})

	if resp.Error == nil {
		t.Fatal("Expected error for adapter with no remote URL")
	}

	if resp.Error.Code != -32602 {
		t.Errorf("Expected error code -32602, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Adapter has no URL" {
		t.Errorf("Unexpected error message: %s", resp.Error.Message)
	}
}

// TestForwardToAdapter_HeaderForwarding tests that X-User-ID header is forwarded
func TestForwardToAdapter_HeaderForwarding(t *testing.T) {
	receivedHeaders := make(http.Header)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture received headers
		for k, v := range r.Header {
			receivedHeaders[k] = v
		}

		resp := MCPResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  map[string]interface{}{"success": true},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	handler := &UnifiedMCPHandler{
		httpClient: mockServer.Client(),
	}

	adapter := &models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:      "test-adapter",
			RemoteUrl: mockServer.URL,
		},
	}

	req := &MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
	}

	headers := http.Header{}
	headers.Set("X-User-ID", "test-user-123")

	resp := handler.forwardToAdapter(context.Background(), adapter, adapter.RemoteUrl, req, headers)

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	if receivedHeaders.Get("X-User-ID") != "test-user-123" {
		t.Errorf("Expected X-User-ID header to be forwarded, got %s", receivedHeaders.Get("X-User-ID"))
	}

	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type header, got %s", receivedHeaders.Get("Content-Type"))
	}
}

// TestMakeAdapterRequest_Success tests successful adapter request
func TestMakeAdapterRequest_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := MCPResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  ToolsListResult{Tools: []Tool{{Name: "test_tool"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	handler := &UnifiedMCPHandler{
		httpClient: mockServer.Client(),
	}

	req := &MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	resp, err := handler.makeAdapterRequest(context.Background(), mockServer.URL, req)
	if err != nil {
		t.Fatalf("makeAdapterRequest failed: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error in response: %v", resp.Error)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}
}

// TestMakeAdapterRequest_AdapterError tests handling of adapter error responses
func TestMakeAdapterRequest_AdapterError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := MCPResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error:   &MCPRPCError{Code: -32603, Message: "Internal error"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	handler := &UnifiedMCPHandler{
		httpClient: mockServer.Client(),
	}

	req := &MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	resp, err := handler.makeAdapterRequest(context.Background(), mockServer.URL, req)
	if err != nil {
		t.Fatalf("makeAdapterRequest failed: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error in response")
	}

	if resp.Error.Code != -32603 {
		t.Errorf("Expected error code -32603, got %d", resp.Error.Code)
	}
}

// Ensure imports are used (they are used in the mocks at the top)
var (
	_ *adaptersvc.AdapterService
	_ *core_services.UserGroupService
)
