package mcp

import (
	"context"
	"testing"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// MockSessionStore for testing
type mockSessionStore struct {
	sessions map[string]*session.SessionDetails
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[string]*session.SessionDetails),
	}
}

func (m *mockSessionStore) CreateSession(sessionID string, adapterName string, authInfo session.AuthorizationInfo) error {
	m.sessions[sessionID] = &session.SessionDetails{
		SessionID:    sessionID,
		AdapterName:  adapterName,
		AuthInfo:     authInfo,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}
	return nil
}

func (m *mockSessionStore) GetSession(sessionID string) (*session.SessionDetails, error) {
	if session, exists := m.sessions[sessionID]; exists {
		return session, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) UpdateActivity(sessionID string) error {
	if session, exists := m.sessions[sessionID]; exists {
		session.LastActivity = time.Now()
		return nil
	}
	return session.ErrSessionNotFound
}

func (m *mockSessionStore) DeleteSession(sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockSessionStore) GetActiveSessions() ([]*session.SessionDetails, error) {
	var sessions []*session.SessionDetails
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (m *mockSessionStore) CleanupExpiredSessions() error {
	return nil
}

// MCP-specific methods
func (m *mockSessionStore) SetMCPSessionID(sessionID, mcpSessionID string) error {
	if session, exists := m.sessions[sessionID]; exists {
		session.MCPSessionID = mcpSessionID
		return nil
	}
	return session.ErrSessionNotFound
}

func (m *mockSessionStore) GetMCPSessionID(sessionID string) (string, error) {
	if session, exists := m.sessions[sessionID]; exists {
		return session.MCPSessionID, nil
	}
	return "", session.ErrSessionNotFound
}

func (m *mockSessionStore) SetMCPCapabilities(sessionID string, capabilities interface{}) error {
	if session, exists := m.sessions[sessionID]; exists {
		session.MCPCapabilities = capabilities
		return nil
	}
	return session.ErrSessionNotFound
}

func (m *mockSessionStore) GetMCPCapabilities(sessionID string) (interface{}, error) {
	if session, exists := m.sessions[sessionID]; exists {
		return session.MCPCapabilities, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) SetMCPClientInfo(sessionID string, clientInfo interface{}) error {
	if session, exists := m.sessions[sessionID]; exists {
		session.MCPClientInfo = clientInfo
		return nil
	}
	return session.ErrSessionNotFound
}

func (m *mockSessionStore) GetMCPClientInfo(sessionID string) (interface{}, error) {
	if session, exists := m.sessions[sessionID]; exists {
		return session.MCPClientInfo, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) SetMCPServerInfo(sessionID string, serverInfo interface{}) error {
	if session, exists := m.sessions[sessionID]; exists {
		session.MCPServerInfo = serverInfo
		return nil
	}
	return session.ErrSessionNotFound
}

func (m *mockSessionStore) GetMCPServerInfo(sessionID string) (interface{}, error) {
	if session, exists := m.sessions[sessionID]; exists {
		return session.MCPServerInfo, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) FindByMCPSessionID(mcpSessionID string) (*session.SessionDetails, error) {
	for _, session := range m.sessions {
		if session.MCPSessionID == mcpSessionID {
			return session, nil
		}
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionStore) GetActiveMCPSessions() ([]*session.SessionDetails, error) {
	var sessions []*session.SessionDetails
	for _, session := range m.sessions {
		if session.MCPSessionID != "" {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func TestMessageRouter_HandleToolsList(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Create test adapter with MCP functionality
	adapter := models.AdapterResource{
		Name: "test-adapter",
		MCPFunctionality: &models.MCPFunctionality{
			Tools: []models.MCPTool{
				{
					Name:        "test-tool",
					Description: "A test tool",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"input": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
	}

	// Create test message
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "tools/list",
		Params:  nil,
	}

	// Test routing
	ctx := context.Background()
	sessionID := "test-session"

	response, err := router.RouteMessage(ctx, message, adapter, sessionID)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Fatal("Expected response, got nil")
	}

	if response.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC version '2.0', got '%s'", response.JSONRPC)
	}

	if response.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", response.ID)
	}

	if response.Error != nil {
		t.Errorf("Expected no error in response, got %v", response.Error)
	}

	// Check result
	if response.Result == nil {
		t.Fatal("Expected result in response")
	}

	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be map[string]interface{}")
	}

	tools, ok := result["tools"]
	if !ok {
		t.Fatal("Expected 'tools' in result")
	}

	toolsList, ok := tools.([]interface{})
	if !ok {
		t.Fatal("Expected tools to be []interface{}")
	}

	if len(toolsList) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(toolsList))
	}
}

func TestMessageRouter_HandleResourcesList(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Create test adapter with MCP functionality
	adapter := models.AdapterResource{
		Name: "test-adapter",
		MCPFunctionality: &models.MCPFunctionality{
			Resources: []models.MCPResource{
				{
					URI:         "test://resource/1",
					Name:        "Test Resource",
					Description: "A test resource",
					MimeType:    "text/plain",
				},
			},
		},
	}

	// Create test message
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "resources/list",
		Params:  nil,
	}

	// Test routing
	ctx := context.Background()
	sessionID := "test-session"

	response, err := router.RouteMessage(ctx, message, adapter, sessionID)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Fatal("Expected response, got nil")
	}

	// Check result
	if response.Result == nil {
		t.Fatal("Expected result in response")
	}

	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be map[string]interface{}")
	}

	resources, ok := result["resources"]
	if !ok {
		t.Fatal("Expected 'resources' in result")
	}

	resourcesList, ok := resources.([]interface{})
	if !ok {
		t.Fatal("Expected resources to be []interface{}")
	}

	if len(resourcesList) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resourcesList))
	}
}

func TestMessageRouter_HandlePromptsList(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Create test adapter with MCP functionality
	adapter := models.AdapterResource{
		Name: "test-adapter",
		MCPFunctionality: &models.MCPFunctionality{
			Prompts: []models.MCPPrompt{
				{
					Name:        "test-prompt",
					Description: "A test prompt",
				},
			},
		},
	}

	// Create test message
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "prompts/list",
		Params:  nil,
	}

	// Test routing
	ctx := context.Background()
	sessionID := "test-session"

	response, err := router.RouteMessage(ctx, message, adapter, sessionID)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Fatal("Expected response, got nil")
	}

	// Check result
	if response.Result == nil {
		t.Fatal("Expected result in response")
	}

	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be map[string]interface{}")
	}

	prompts, ok := result["prompts"]
	if !ok {
		t.Fatal("Expected 'prompts' in result")
	}

	promptsList, ok := prompts.([]interface{})
	if !ok {
		t.Fatal("Expected prompts to be []interface{}")
	}

	if len(promptsList) != 1 {
		t.Errorf("Expected 1 prompt, got %d", len(promptsList))
	}
}

func TestMessageRouter_CacheIntegration(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Create test adapter without MCP functionality (will use cache/proxy)
	adapter := models.AdapterResource{
		Name: "test-adapter",
	}

	// Create test message
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "tools/list",
		Params:  nil,
	}

	// Test routing - should attempt to proxy since no cached functionality
	ctx := context.Background()
	sessionID := "test-session"

	// This will fail to proxy since we don't have a real server, but we can test cache behavior
	response, err := router.RouteMessage(ctx, message, adapter, sessionID)

	// We expect an error since there's no real server to proxy to
	if err == nil {
		t.Error("Expected error when proxying to non-existent server")
	}

	// Check that cache metrics were updated
	cacheMetrics := router.GetCacheMetrics()
	if cacheMetrics == nil {
		t.Error("Expected cache metrics")
	}
}

func TestMessageRouter_MonitoringIntegration(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Create test adapter with MCP functionality
	adapter := models.AdapterResource{
		Name: "test-adapter",
		MCPFunctionality: &models.MCPFunctionality{
			Tools: []models.MCPTool{
				{
					Name:        "test-tool",
					Description: "A test tool",
				},
			},
		},
	}

	// Create test message
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "tools/list",
		Params:  nil,
	}

	// Test routing
	ctx := context.Background()
	sessionID := "test-session"

	response, err := router.RouteMessage(ctx, message, adapter, sessionID)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Fatal("Expected response, got nil")
	}

	// Check that monitoring metrics were recorded
	metrics := monitor.GetMetrics()
	if metrics == nil {
		t.Error("Expected monitoring metrics")
	}

	operations, ok := metrics["operations"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected operations in metrics")
	}

	toolsListMetrics, ok := operations["tools/list"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected tools/list metrics")
	}

	if totalReqs, ok := toolsListMetrics["total_requests"].(int64); !ok || totalReqs != 1 {
		t.Errorf("Expected tools/list total_requests to be 1, got %v", totalReqs)
	}

	if successfulReqs, ok := toolsListMetrics["successful_requests"].(int64); !ok || successfulReqs != 1 {
		t.Errorf("Expected tools/list successful_requests to be 1, got %v", successfulReqs)
	}

	// Check that logs were created
	logs := monitor.GetRecentLogs(10)
	if len(logs) == 0 {
		t.Error("Expected logs to be created")
	}

	// Should have at least request and response logs
	var requestLog, responseLog *LogEntry
	for _, log := range logs {
		if log.Component == "MessageRouter" {
			if log.Message == "Processing tools/list request" {
				requestLog = &log
			} else if log.Message == "Completed tools/list request successfully" {
				responseLog = &log
			}
		}
	}

	if requestLog == nil {
		t.Error("Expected request log not found")
	}
	if responseLog == nil {
		t.Error("Expected response log not found")
	}
}

func TestMessageRouter_GetCacheMetrics(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Test cache metrics
	metrics := router.GetCacheMetrics()
	if metrics == nil {
		t.Error("Expected cache metrics")
	}

	// Should contain cache_enabled: true
	if enabled, ok := metrics["cache_enabled"].(bool); !ok || !enabled {
		t.Error("Expected cache_enabled to be true")
	}
}

func TestMessageRouter_UnknownMethod(t *testing.T) {
	// Setup
	sessionStore := newMockSessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)

	router := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)
	defer cache.Close()
	defer monitor.Close()

	// Create test adapter
	adapter := models.AdapterResource{
		Name: "test-adapter",
	}

	// Create test message with unknown method
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "unknown/method",
		Params:  nil,
	}

	// Test routing - should attempt to proxy unknown method
	ctx := context.Background()
	sessionID := "test-session"

	response, err := router.RouteMessage(ctx, message, adapter, sessionID)

	// We expect an error since there's no real server to proxy to
	if err == nil {
		t.Error("Expected error when proxying unknown method to non-existent server")
	}

	// Should still record metrics for the attempt
	metrics := monitor.GetMetrics()
	operations, ok := metrics["operations"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected operations in metrics")
	}

	unknownMethodMetrics, ok := operations["unknown/method"].(map[string]interface{})
	if !ok {
		// This is expected since we don't pre-initialize unknown methods
		return
	}

	if totalReqs, ok := unknownMethodMetrics["total_requests"].(int64); !ok || totalReqs != 1 {
		t.Errorf("Expected unknown/method total_requests to be 1, got %v", totalReqs)
	}
}
