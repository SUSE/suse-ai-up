package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/session"
)

func TestExtractResourceMetadataURL(t *testing.T) {
	ph := &ProxyHandler{}

	tests := []struct {
		name     string
		wwwAuth  string
		expected string
	}{
		{
			name:     "Valid resource metadata URL",
			wwwAuth:  `Bearer error="invalid_token", error_description="The access token expired", resource_metadata="https://example.com/.well-known/oauth-protected-resource"`,
			expected: "https://example.com/.well-known/oauth-protected-resource",
		},
		{
			name:     "No resource metadata",
			wwwAuth:  `Bearer error="invalid_token", error_description="The access token expired"`,
			expected: "",
		},
		{
			name:     "Empty WWW-Authenticate",
			wwwAuth:  "",
			expected: "",
		},
		{
			name:     "Multiple parameters with resource metadata",
			wwwAuth:  `Bearer realm="example", error="invalid_token", resource_metadata="https://api.example.com/oauth/resource"`,
			expected: "https://api.example.com/oauth/resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ph.extractResourceMetadataURL(tt.wwwAuth)
			if result != tt.expected {
				t.Errorf("extractResourceMetadataURL() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestExtractSessionID(t *testing.T) {
	ph := &ProxyHandler{}

	tests := []struct {
		name       string
		headers    map[string]string
		expectedID string
	}{
		{
			name: "Valid MCP session ID header",
			headers: map[string]string{
				"mcp-session-id": "session-12345",
			},
			expectedID: "session-12345",
		},
		{
			name: "No session ID header",
			headers: map[string]string{
				"content-type": "application/json",
			},
			expectedID: "",
		},
		{
			name: "Empty session ID header",
			headers: map[string]string{
				"mcp-session-id": "",
			},
			expectedID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: make(http.Header),
			}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}

			result := ph.extractSessionID(resp)
			if result != tt.expectedID {
				t.Errorf("extractSessionID() = %v, expected %v", result, tt.expectedID)
			}
		})
	}
}

func TestProxyAuthorizationHandling(t *testing.T) {
	// Setup
	store := session.NewInMemorySessionStore()

	// Create a test session with valid token
	sessionID := "test-session-123"
	err := store.SetWithDetails(sessionID, "test-adapter", "http://localhost:8080", "http")
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	tokenInfo := &session.TokenInfo{
		AccessToken: "test-access-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
		IssuedAt:    time.Now(),
	}
	err = store.SetTokenInfo(sessionID, tokenInfo)
	if err != nil {
		t.Fatalf("Failed to set token info: %v", err)
	}

	// Test that token is considered valid
	if !store.IsTokenValid(sessionID) {
		t.Error("Token should be valid")
	}

	// Test token retrieval
	retrievedToken, err := store.GetTokenInfo(sessionID)
	if err != nil {
		t.Errorf("Failed to retrieve token: %v", err)
	}
	if retrievedToken.AccessToken != "test-access-token" {
		t.Errorf("Retrieved wrong token: %s", retrievedToken.AccessToken)
	}
}

func TestProxyHandlerCreation(t *testing.T) {
	store := session.NewInMemorySessionStore()

	// Test with nil kubeClient and adapterStore (should not panic)
	ph := NewProxyHandler(store, nil, nil)

	if ph.sessionStore == nil {
		t.Error("Session store should be set")
	}

	if ph.httpClient == nil {
		t.Error("HTTP client should be initialized")
	}
}

func TestGetSessionID(t *testing.T) {
	ph := &ProxyHandler{}

	tests := []struct {
		name       string
		headers    map[string]string
		expectedID string
	}{
		{
			name: "Valid session ID in header",
			headers: map[string]string{
				"mcp-session-id": "session-abc123",
			},
			expectedID: "session-abc123",
		},
		{
			name: "No session ID header",
			headers: map[string]string{
				"content-type": "application/json",
			},
			expectedID: "",
		},
		{
			name: "Empty session ID",
			headers: map[string]string{
				"mcp-session-id": "",
			},
			expectedID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("GET", "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			c.Request = req

			result := ph.getSessionID(c)
			if result != tt.expectedID {
				t.Errorf("getSessionID() = %v, expected %v", result, tt.expectedID)
			}
		})
	}
}
