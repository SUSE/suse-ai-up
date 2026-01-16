package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// MockSessionStore for testing
type MockSessionStore struct {
	mock.Mock
}

func (m *MockSessionStore) Set(sessionID, targetAddress string) error {
	args := m.Called(sessionID, targetAddress)
	return args.Error(0)
}

func (m *MockSessionStore) SetWithDetails(sessionID, adapterName, clientType, connectionType string) error {
	args := m.Called(sessionID, adapterName, clientType, connectionType)
	return args.Error(0)
}

func (m *MockSessionStore) Get(sessionID string) (string, bool) {
	args := m.Called(sessionID)
	return args.String(0), args.Bool(1)
}

func (m *MockSessionStore) Delete(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

func (m *MockSessionStore) List() map[string]session.SessionDetails {
	args := m.Called()
	return args.Get(0).(map[string]session.SessionDetails)
}

func (m *MockSessionStore) SetMCPSessionID(sessionID, mcpSessionID string) error {
	args := m.Called(sessionID, mcpSessionID)
	return args.Error(0)
}

func (m *MockSessionStore) GetMCPSessionID(sessionID string) (string, error) {
	args := m.Called(sessionID)
	return args.String(0), args.Error(1)
}

func (m *MockSessionStore) CleanupExpired(timeout time.Duration) error {
	args := m.Called(timeout)
	return args.Error(0)
}

func (m *MockSessionStore) DeleteByAdapter(adapterName string) error {
	args := m.Called(adapterName)
	return args.Error(0)
}

func (m *MockSessionStore) ListByAdapter(adapterName string) ([]session.SessionDetails, error) {
	args := m.Called(adapterName)
	return args.Get(0).([]session.SessionDetails), args.Error(1)
}

func (m *MockSessionStore) GetDetails(sessionID string) (*session.SessionDetails, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*session.SessionDetails), args.Error(1)
}

func (m *MockSessionStore) UpdateActivity(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

func (m *MockSessionStore) SetTokenInfo(sessionID string, tokenInfo *session.TokenInfo) error {
	args := m.Called(sessionID, tokenInfo)
	return args.Error(0)
}

func (m *MockSessionStore) GetTokenInfo(sessionID string) (*session.TokenInfo, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*session.TokenInfo), args.Error(1)
}

func (m *MockSessionStore) SetAuthorizationInfo(sessionID string, authInfo *session.AuthorizationInfo) error {
	args := m.Called(sessionID, authInfo)
	return args.Error(0)
}

func (m *MockSessionStore) GetAuthorizationInfo(sessionID string) (*session.AuthorizationInfo, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*session.AuthorizationInfo), args.Error(1)
}

func (m *MockSessionStore) IsTokenValid(sessionID string) bool {
	args := m.Called(sessionID)
	return args.Bool(0)
}

func (m *MockSessionStore) RefreshToken(sessionID, newAccessToken string, expiresAt time.Time) error {
	args := m.Called(sessionID, newAccessToken, expiresAt)
	return args.Error(0)
}

func (m *MockSessionStore) SetMCPCapabilities(sessionID string, capabilities map[string]interface{}) error {
	args := m.Called(sessionID, capabilities)
	return args.Error(0)
}

func (m *MockSessionStore) GetMCPCapabilities(sessionID string) (map[string]interface{}, error) {
	args := m.Called(sessionID)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockSessionStore) SetMCPClientInfo(sessionID string, clientInfo *session.MCPClientInfo) error {
	args := m.Called(sessionID, clientInfo)
	return args.Error(0)
}

func (m *MockSessionStore) GetMCPClientInfo(sessionID string) (*session.MCPClientInfo, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*session.MCPClientInfo), args.Error(1)
}

func (m *MockSessionStore) SetMCPServerInfo(sessionID string, serverInfo *session.MCPServerInfo) error {
	args := m.Called(sessionID, serverInfo)
	return args.Error(0)
}

func (m *MockSessionStore) GetMCPServerInfo(sessionID string) (*session.MCPServerInfo, error) {
	args := m.Called(sessionID)
	return args.Get(0).(*session.MCPServerInfo), args.Error(1)
}

func (m *MockSessionStore) FindByMCPSessionID(mcpSessionID string) (*session.SessionDetails, error) {
	args := m.Called(mcpSessionID)
	return args.Get(0).(*session.SessionDetails), args.Error(1)
}

func (m *MockSessionStore) GetActiveMCPSessions() ([]session.SessionDetails, error) {
	args := m.Called()
	return args.Get(0).([]session.SessionDetails), args.Error(1)
}

// MockMessageRouter for testing
type MockMessageRouter struct {
	mock.Mock
}

func (m *MockMessageRouter) RouteMessage(ctx context.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*mcp.JSONRPCMessage, error) {
	args := m.Called(ctx, message, adapter, sessionID)
	return args.Get(0).(*mcp.JSONRPCMessage), args.Error(1)
}

// MockProtocolHandler for testing
type MockProtocolHandler struct {
	mock.Mock
}

func (m *MockProtocolHandler) HandleMessage(ctx context.Context, data []byte, adapter models.AdapterResource, sessionID string) (*mcp.JSONRPCMessage, error) {
	args := m.Called(ctx, data, adapter, sessionID)
	return args.Get(0).(*mcp.JSONRPCMessage), args.Error(1)
}

func TestNewRemoteHTTPProxyAdapter(t *testing.T) {
	mockSessionStore := &MockSessionStore{}
	capabilityCache := mcp.NewCapabilityCache()

	adapter := NewRemoteHTTPProxyAdapter(mockSessionStore, nil, nil, capabilityCache)

	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.httpClient)
	assert.Equal(t, mockSessionStore, adapter.sessionStore)
	assert.Equal(t, capabilityCache, adapter.capabilityCache)
	assert.NotNil(t, adapter.sessions)
	assert.NotNil(t, adapter.pendingRequests)
}

func TestRemoteHTTPProxyAdapter_CanHandle(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	assert.True(t, adapter.CanHandle(models.ConnectionTypeRemoteHttp))
	assert.False(t, adapter.CanHandle(models.ConnectionTypeLocalStdio))
	assert.False(t, adapter.CanHandle(models.ConnectionTypeStreamableHttp))
}

func TestRemoteHTTPProxyAdapter_HandleRequest_Initialize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockSessionStore := &MockSessionStore{}
	capabilityCache := mcp.NewCapabilityCache()

	adapter := NewRemoteHTTPProxyAdapter(mockSessionStore, nil, nil, capabilityCache)

	// Create test adapter
	testAdapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:           "test-adapter",
			ConnectionType: models.ConnectionTypeRemoteHttp,
			RemoteUrl:      "http://localhost:8080/mcp",
		},
		ID: "test-adapter",
	}

	// Create initialize request
	initMessage := mcp.JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		},
	}

	body, _ := json.Marshal(initMessage)

	// Create test request
	req := httptest.NewRequest("POST", "/test-adapter/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Mock session store
	mockSessionStore.On("SetWithDetails", mock.AnythingOfType("string"), testAdapter.Name, "", "remote-http").Return(nil)

	// Handle request
	err := adapter.HandleRequest(c, testAdapter)

	// Should return error because messageRouter is nil
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "messageRouter is nil")
}

func TestRemoteHTTPProxyAdapter_HandleRequest_SSE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockSessionStore := &MockSessionStore{}
	capabilityCache := mcp.NewCapabilityCache()

	adapter := NewRemoteHTTPProxyAdapter(mockSessionStore, nil, nil, capabilityCache)

	// Create test adapter
	testAdapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:           "test-adapter",
			ConnectionType: models.ConnectionTypeRemoteHttp,
			RemoteUrl:      "http://localhost:8080/mcp",
		},
		ID: "test-adapter",
	}

	// Create SSE request
	req := httptest.NewRequest("GET", "/test-adapter/mcp", nil)
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Mock session store
	mockSessionStore.On("SetWithDetails", mock.AnythingOfType("string"), testAdapter.Name, "", "remote-http").Return(nil)

	// Handle request
	err := adapter.HandleRequest(c, testAdapter)

	// Should return error because messageRouter is nil
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "messageRouter is nil")
}

func TestRemoteHTTPProxyAdapter_HandleRequest_Notification(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockSessionStore := &MockSessionStore{}
	capabilityCache := mcp.NewCapabilityCache()

	adapter := NewRemoteHTTPProxyAdapter(mockSessionStore, nil, nil, capabilityCache)

	// Create test adapter
	testAdapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name:           "test-adapter",
			ConnectionType: models.ConnectionTypeRemoteHttp,
			RemoteUrl:      "http://localhost:8080/mcp",
		},
		ID: "test-adapter",
	}

	// Create notification request
	notifMessage := mcp.JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/tools/list_changed",
	}

	body, _ := json.Marshal(notifMessage)

	// Create test request with session ID
	req := httptest.NewRequest("POST", "/test-adapter/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", "test-session-123")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Create session manually
	session := &RemoteHTTPSession{
		ID:           "test-session-123",
		AdapterName:  testAdapter.Name,
		RemoteURL:    testAdapter.RemoteUrl,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}
	adapter.sessions["test-session-123"] = session

	// Handle request
	err := adapter.HandleRequest(c, testAdapter)

	// Should return error because messageRouter is nil
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "messageRouter is nil")
}

func TestRemoteHTTPProxyAdapter_GetStatus(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	testAdapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name: "test-adapter",
		},
		ID: "test-adapter",
	}

	status, err := adapter.GetStatus(testAdapter)

	assert.NoError(t, err)
	assert.Equal(t, "Idle", status.ReplicaStatus) // No active sessions
}

func TestRemoteHTTPProxyAdapter_GetLogs(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	testAdapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name: "test-adapter",
		},
		ID: "test-adapter",
	}

	logs, err := adapter.GetLogs(testAdapter)

	assert.NoError(t, err)
	assert.Contains(t, logs, "Remote HTTP Proxy Adapter for test-adapter")
	assert.Contains(t, logs, "Active Sessions: 0")
}

func TestRemoteHTTPProxyAdapter_Cleanup(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	// Create a test session
	session := &RemoteHTTPSession{
		ID:           "test-session",
		AdapterName:  "test-adapter",
		RemoteURL:    "http://localhost:8080/mcp",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}
	adapter.sessions["test-session"] = session

	// Cleanup
	err := adapter.Cleanup("test-session")

	assert.NoError(t, err)
	assert.Empty(t, adapter.sessions)
}

func TestRemoteHTTPProxyAdapter_extractSessionID(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	tests := []struct {
		name     string
		setup    func(*gin.Context)
		expected string
	}{
		{
			name: "Mcp-Session-Id header",
			setup: func(c *gin.Context) {
				c.Request.Header.Set("Mcp-Session-Id", "session-123")
			},
			expected: "session-123",
		},
		{
			name: "mcp-session-id header (lowercase)",
			setup: func(c *gin.Context) {
				c.Request.Header.Set("mcp-session-id", "session-456")
			},
			expected: "session-456",
		},
		{
			name: "sessionId query parameter",
			setup: func(c *gin.Context) {
				c.Request, _ = http.NewRequest("GET", "/test?sessionId=session-789", nil)
			},
			expected: "session-789",
		},
		{
			name:     "no session ID",
			setup:    func(c *gin.Context) {},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			tt.setup(c)

			result := adapter.extractSessionID(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoteHTTPProxyAdapter_generateSessionID(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	sessionID1 := adapter.generateSessionID()
	sessionID2 := adapter.generateSessionID()

	assert.NotEmpty(t, sessionID1)
	assert.NotEmpty(t, sessionID2)
	assert.NotEqual(t, sessionID1, sessionID2)
	assert.Contains(t, sessionID1, "remote-http-")
	assert.Contains(t, sessionID2, "remote-http-")
}

func TestRemoteHTTPProxyAdapter_getRequestID(t *testing.T) {
	adapter := NewRemoteHTTPProxyAdapter(nil, nil, nil, nil)

	tests := []struct {
		name     string
		message  *mcp.JSONRPCMessage
		expected string
	}{
		{
			name: "string ID",
			message: &mcp.JSONRPCMessage{
				ID: "test-id",
			},
			expected: "test-id",
		},
		{
			name: "float64 ID",
			message: &mcp.JSONRPCMessage{
				ID: 123.0,
			},
			expected: "123",
		},
		{
			name: "int ID",
			message: &mcp.JSONRPCMessage{
				ID: 456,
			},
			expected: "456",
		},
		{
			name: "nil ID",
			message: &mcp.JSONRPCMessage{
				ID: nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.getRequestID(tt.message)
			assert.Equal(t, tt.expected, result)
		})
	}
}
