package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/session"
)

// MockSessionStore implements session.SessionStore for testing
type MockSessionStore struct{}

func (m *MockSessionStore) Set(sessionID, targetAddress string) error { return nil }
func (m *MockSessionStore) Get(sessionID string) (string, bool)       { return "", true }
func (m *MockSessionStore) SetWithDetails(sessionID, adapterName, targetAddress, connectionType string) error {
	return nil
}
func (m *MockSessionStore) ListByAdapter(adapterName string) ([]session.SessionDetails, error) {
	return nil, nil
}
func (m *MockSessionStore) GetDetails(sessionID string) (*session.SessionDetails, error) {
	return nil, nil
}
func (m *MockSessionStore) Delete(sessionID string) error             { return nil }
func (m *MockSessionStore) DeleteByAdapter(adapterName string) error  { return nil }
func (m *MockSessionStore) UpdateActivity(sessionID string) error     { return nil }
func (m *MockSessionStore) CleanupExpired(maxAge time.Duration) error { return nil }
func (m *MockSessionStore) SetTokenInfo(sessionID string, tokenInfo *session.TokenInfo) error {
	return nil
}
func (m *MockSessionStore) GetTokenInfo(sessionID string) (*session.TokenInfo, error) {
	return nil, nil
}
func (m *MockSessionStore) SetAuthorizationInfo(sessionID string, authInfo *session.AuthorizationInfo) error {
	return nil
}
func (m *MockSessionStore) GetAuthorizationInfo(sessionID string) (*session.AuthorizationInfo, error) {
	return nil, nil
}
func (m *MockSessionStore) IsTokenValid(sessionID string) bool { return true }
func (m *MockSessionStore) RefreshToken(sessionID, newAccessToken string, expiresAt time.Time) error {
	return nil
}

func TestGeneratePKCE(t *testing.T) {
	as := &AuthorizationService{}

	verifier, challenge, err := as.generatePKCE()
	if err != nil {
		t.Fatalf("Failed to generate PKCE: %v", err)
	}

	// Verify verifier is base64url encoded
	_, err = base64.RawURLEncoding.DecodeString(verifier)
	if err != nil {
		t.Errorf("Verifier is not valid base64url: %v", err)
	}

	// Verify challenge is SHA256 hash of verifier
	hash := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	if challenge != expectedChallenge {
		t.Errorf("Challenge does not match expected SHA256 hash. Got %s, expected %s", challenge, expectedChallenge)
	}

	// Verify lengths are reasonable
	if len(verifier) < 40 || len(verifier) > 128 {
		t.Errorf("Verifier length %d is outside expected range [40, 128]", len(verifier))
	}

	if len(challenge) != 43 { // SHA256 base64url is always 43 chars
		t.Errorf("Challenge length %d is not 43 as expected for SHA256 base64url", len(challenge))
	}
}

func TestNewAuthorizationService(t *testing.T) {
	mockStore := &MockSessionStore{}
	baseURL := "http://localhost:8080"

	as := NewAuthorizationService(mockStore, baseURL)

	if as.sessionStore == nil {
		t.Error("Session store not set")
	}

	if as.baseURL != baseURL {
		t.Errorf("Base URL not set correctly. Got %s, expected %s", as.baseURL, baseURL)
	}

	if as.httpClient == nil {
		t.Error("HTTP client not initialized")
	}

	if as.httpClient.Timeout == 0 {
		t.Error("HTTP client timeout not set")
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	as := NewAuthorizationService(nil, "http://localhost:8080")

	authServer := &OAuthServerMetadata{
		AuthorizationEndpoint: "https://auth.example.com/oauth/authorize",
	}

	client := &OAuthClient{
		ClientID:     "test-client",
		RedirectURIs: []string{"http://localhost:8080/callback"},
	}

	authURL, err := as.buildAuthorizationURL(authServer, client, "test-resource")
	if err != nil {
		t.Fatalf("Failed to build authorization URL: %v", err)
	}

	// Verify URL starts with correct endpoint
	if !strings.HasPrefix(authURL, "https://auth.example.com/oauth/authorize?") {
		t.Errorf("Authorization URL does not start with expected endpoint. Got: %s", authURL)
	}

	// Parse the URL to check query parameters
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Failed to parse generated URL: %v", err)
	}

	params := parsedURL.Query()

	// Check required parameters
	expectedParams := map[string]string{
		"client_id":             "test-client",
		"response_type":         "code",
		"redirect_uri":          "http://localhost:8080/callback",
		"scope":                 "openid profile",
		"resource":              "test-resource",
		"code_challenge_method": "S256",
	}

	for key, expectedValue := range expectedParams {
		if actualValue := params.Get(key); actualValue != expectedValue {
			t.Errorf("Parameter %s: expected %s, got %s", key, expectedValue, actualValue)
		}
	}

	// Check that PKCE challenge is present
	if challenge := params.Get("code_challenge"); challenge == "" {
		t.Error("PKCE code challenge is missing")
	}

	if state := params.Get("state"); state == "" {
		t.Error("State parameter is missing")
	}
}

func TestDeleteSession(t *testing.T) {
	// Create test session store
	sessionStore := session.NewInMemorySessionStore()

	// Create test session
	sessionID := "test-session-123"
	adapterName := "test-adapter"

	sessionDetails := session.SessionDetails{
		SessionID:      sessionID,
		AdapterName:    adapterName,
		TargetAddress:  "http://localhost:8000",
		ConnectionType: "StreamableHttp",
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		Status:         "active",
	}

	err := sessionStore.SetWithDetails(sessionID, adapterName, sessionDetails.TargetAddress, sessionDetails.ConnectionType)
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create authorization service
	authService := NewAuthorizationService(sessionStore, "http://localhost:8911")

	// Test DELETE /adapters/{name}/sessions/{sessionId}
	router := gin.New()
	router.DELETE("/adapters/:name/sessions/:sessionId", authService.DeleteSession)

	req, _ := http.NewRequest("DELETE", "/adapters/test-adapter/sessions/test-session-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["message"] != "Session deleted successfully" {
		t.Errorf("Expected message 'Session deleted successfully', got %v", response["message"])
	}

	if response["adapterName"] != "test-adapter" {
		t.Errorf("Expected adapterName 'test-adapter', got %v", response["adapterName"])
	}

	if response["sessionId"] != "test-session-123" {
		t.Errorf("Expected sessionId 'test-session-123', got %v", response["sessionId"])
	}

	// Verify session is deleted
	_, exists := sessionStore.Get(sessionID)
	if exists {
		t.Error("Session should have been deleted but still exists")
	}
}

func TestDeleteSessionNotFound(t *testing.T) {
	// Create test session store
	sessionStore := session.NewInMemorySessionStore()

	// Create authorization service
	authService := NewAuthorizationService(sessionStore, "http://localhost:8911")

	// Test DELETE /adapters/{name}/sessions/{sessionId} with non-existent session
	router := gin.New()
	router.DELETE("/adapters/:name/sessions/:sessionId", authService.DeleteSession)

	req, _ := http.NewRequest("DELETE", "/adapters/test-adapter/sessions/non-existent-session", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["error"] != "Session not found" {
		t.Errorf("Expected error 'Session not found', got %v", response["error"])
	}
}

func TestDeleteAllSessions(t *testing.T) {
	// Create test session store
	sessionStore := session.NewInMemorySessionStore()

	// Create test sessions
	adapterName := "test-adapter"
	sessions := []string{"session-1", "session-2", "session-3"}

	for _, sessionID := range sessions {
		sessionDetails := session.SessionDetails{
			SessionID:      sessionID,
			AdapterName:    adapterName,
			TargetAddress:  "http://localhost:8000",
			ConnectionType: "StreamableHttp",
			CreatedAt:      time.Now(),
			LastActivity:   time.Now(),
			Status:         "active",
		}
		err := sessionStore.SetWithDetails(sessionID, adapterName, sessionDetails.TargetAddress, sessionDetails.ConnectionType)
		if err != nil {
			t.Fatalf("Failed to create test session %s: %v", sessionID, err)
		}
	}

	// Verify sessions exist
	for _, sessionID := range sessions {
		_, exists := sessionStore.Get(sessionID)
		if !exists {
			t.Errorf("Session %s should exist but doesn't", sessionID)
		}
	}

	// Create authorization service
	authService := NewAuthorizationService(sessionStore, "http://localhost:8911")

	// Test DELETE /adapters/{name}/sessions
	router := gin.New()
	router.DELETE("/adapters/:name/sessions", authService.DeleteAllSessions)

	req, _ := http.NewRequest("DELETE", "/adapters/test-adapter/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["message"] != "All sessions deleted successfully" {
		t.Errorf("Expected message 'All sessions deleted successfully', got %v", response["message"])
	}

	if response["adapterName"] != "test-adapter" {
		t.Errorf("Expected adapterName 'test-adapter', got %v", response["adapterName"])
	}

	if response["deletedCount"] != float64(3) { // JSON numbers are float64
		t.Errorf("Expected deletedCount 3, got %v", response["deletedCount"])
	}

	// Verify all sessions are deleted
	for _, sessionID := range sessions {
		_, exists := sessionStore.Get(sessionID)
		if exists {
			t.Errorf("Session %s should have been deleted but still exists", sessionID)
		}
	}
}
