package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/session"
)

// MockOAuthServer simulates an OAuth authorization server
type MockOAuthServer struct {
	server *httptest.Server
}

func NewMockOAuthServer() *MockOAuthServer {
	mux := http.NewServeMux()

	// Mock authorization endpoint
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		// Redirect back to callback with authorization code
		callbackURL := r.URL.Query().Get("redirect_uri")
		if callbackURL != "" {
			http.Redirect(w, r, callbackURL+"?code=test-auth-code&state="+r.URL.Query().Get("state"), http.StatusFound)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	// Mock token endpoint
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Return mock token response
		tokenResponse := map[string]interface{}{
			"access_token":  "mock-access-token-123",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "mock-refresh-token-456",
			"scope":         "read write",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	})

	// Mock JWKS endpoint
	mux.HandleFunc("/oauth/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwks := map[string]interface{}{
			"keys": []map[string]interface{}{
				{
					"kty": "RSA",
					"use": "sig",
					"kid": "test-key-id",
					"n":   "mock-modulus",
					"e":   "AQAB",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	})

	server := httptest.NewServer(mux)
	return &MockOAuthServer{server: server}
}

func (m *MockOAuthServer) URL() string {
	return m.server.URL
}

func (m *MockOAuthServer) Close() {
	m.server.Close()
}

// MockMCPServer simulates an MCP server that requires OAuth
type MockMCPServer struct {
	server *httptest.Server
}

func NewMockMCPServer() *MockMCPServer {
	mux := http.NewServeMux()

	// Mock MCP endpoint that requires authorization
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			// Return 401 with WWW-Authenticate header
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="No access token provided", resource_metadata="`+r.Host+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if authHeader != "Bearer mock-access-token-123" {
			// Return 401 for invalid token
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="Invalid access token"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return successful MCP response
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"tools": []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock protected resource metadata endpoint
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		metadata := map[string]interface{}{
			"resource": "mcp-server",
			"authorization_servers": []string{
				"http://auth-server:8080", // This will be replaced with actual mock server URL
			},
			"scopes": []string{"read", "write"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metadata)
	})

	server := httptest.NewServer(mux)
	return &MockMCPServer{server: server}
}

func (m *MockMCPServer) URL() string {
	return m.server.URL
}

func (m *MockMCPServer) Close() {
	m.server.Close()
}

func TestEndToEndOAuthAuthorizationFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup mock servers
	oauthServer := NewMockOAuthServer()
	defer oauthServer.Close()

	mcpServer := NewMockMCPServer()
	defer mcpServer.Close()

	// Setup proxy components
	sessionStore := session.NewInMemorySessionStore()
	authService := NewAuthorizationService(sessionStore, "http://localhost:8080")
	proxyHandler := NewProxyHandler(sessionStore, nil, nil)

	// Step 1: Initial request to MCP server without authorization should fail with 401
	t.Run("InitialUnauthorizedRequest", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// Mock request to /adapters/test-mcp/mcp
		req := httptest.NewRequest("POST", "/adapters/test-mcp/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req
		c.Params = []gin.Param{{Key: "name", Value: "test-mcp"}}

		// Mock the proxy to point to our mock MCP server
		// This is a simplified test - in reality we'd need to mock the session resolution
		proxyHandler.proxyRequest(c, mcpServer.URL(), "")

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 Unauthorized, got %d", w.Code)
		}

		// Check response contains OAuth error information
		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if response["error"] != "oauth_authorization_required" {
			t.Errorf("Expected oauth_authorization_required error, got %v", response["error"])
		}
	})

	// Step 2: Start authorization flow
	t.Run("StartAuthorizationFlow", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// Provide required client info
		clientInfo := map[string]interface{}{
			"client_id": "test-client",
		}
		reqBody := map[string]interface{}{
			"clientInfo": clientInfo,
			"resource":   "test-resource",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/adapters/test-mcp/auth/authorize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req
		c.Params = []gin.Param{{Key: "name", Value: "test-mcp"}}

		authService.StartAuthorization(c)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if response["status"] != "authorizing" {
			t.Errorf("Expected authorizing status, got %v", response["status"])
		}
	})

	// Step 3: Simulate successful authorization (token storage)
	t.Run("SimulateSuccessfulAuthorization", func(t *testing.T) {
		sessionID := "test-session-123"

		// First create the session
		err := sessionStore.SetWithDetails(sessionID, "test-mcp", mcpServer.URL(), "http")
		if err != nil {
			t.Errorf("Failed to create session: %v", err)
		}

		// Store authorization info
		authInfo := &session.AuthorizationInfo{
			Status:       "authorized",
			AuthorizedAt: time.Now(),
			ClientID:     "test-client",
		}
		err = sessionStore.SetAuthorizationInfo(sessionID, authInfo)
		if err != nil {
			t.Errorf("Failed to set authorization info: %v", err)
		}

		// Store token info
		tokenInfo := &session.TokenInfo{
			AccessToken:  "mock-access-token-123",
			RefreshToken: "mock-refresh-token-456",
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(time.Hour),
			IssuedAt:     time.Now(),
			Scope:        "read write",
		}
		err = sessionStore.SetTokenInfo(sessionID, tokenInfo)
		if err != nil {
			t.Errorf("Failed to set token info: %v", err)
		}

		// Verify token is valid
		if !sessionStore.IsTokenValid(sessionID) {
			t.Error("Token should be valid")
		}
	})

	// Step 4: Authorized request should succeed
	t.Run("AuthorizedRequestSucceeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("POST", "/adapters/test-mcp/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("mcp-session-id", "test-session-123") // Include session ID
		c.Request = req
		c.Params = []gin.Param{{Key: "name", Value: "test-mcp"}}

		// This would normally go through the full proxy logic
		// For this test, we'll simulate the authorization header injection
		proxyHandler.proxyRequest(c, mcpServer.URL(), "test-session-123")

		// The mock MCP server should accept the Bearer token and return 200
		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d. Response: %s", w.Code, w.Body.String())
		}
	})

	// Step 5: Check authorization status
	t.Run("CheckAuthorizationStatus", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/adapters/test-mcp/auth/status", nil)
		c.Request = req
		c.Params = []gin.Param{{Key: "name", Value: "test-mcp"}}

		authService.GetAuthorizationStatus(c)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if response["status"] != "authorized" {
			t.Errorf("Expected authorized status, got %v", response["status"])
		}

		if response["tokenValid"] != true {
			t.Error("Token should be reported as valid")
		}
	})
}
