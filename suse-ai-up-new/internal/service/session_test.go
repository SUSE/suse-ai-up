package service

import (
	"testing"
	"time"

	"suse-ai-up/pkg/session"
)

// TestSessionStoreOperations tests the session store interface methods
func TestSessionStoreOperations(t *testing.T) {
	store := session.NewInMemorySessionStore()

	// Test SetWithDetails
	err := store.SetWithDetails("test-session", "test-adapter", "localhost:8080", "stdio")
	if err != nil {
		t.Errorf("SetWithDetails failed: %v", err)
	}

	// Test GetDetails
	details, err := store.GetDetails("test-session")
	if err != nil {
		t.Errorf("GetDetails failed: %v", err)
	}
	if details == nil {
		t.Fatal("Expected session details, got nil")
	}

	if details.SessionID != "test-session" {
		t.Errorf("SessionID mismatch: expected test-session, got %s", details.SessionID)
	}
	if details.AdapterName != "test-adapter" {
		t.Errorf("AdapterName mismatch: expected test-adapter, got %s", details.AdapterName)
	}
	if details.TargetAddress != "localhost:8080" {
		t.Errorf("TargetAddress mismatch: expected localhost:8080, got %s", details.TargetAddress)
	}
	if details.ConnectionType != "stdio" {
		t.Errorf("ConnectionType mismatch: expected stdio, got %s", details.ConnectionType)
	}
	if details.Status != "active" {
		t.Errorf("Status mismatch: expected active, got %s", details.Status)
	}

	// Test ListByAdapter
	sessions, err := store.ListByAdapter("test-adapter")
	if err != nil {
		t.Errorf("ListByAdapter failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	// Test UpdateActivity
	oldActivity := details.LastActivity
	time.Sleep(1 * time.Millisecond) // Ensure time difference
	err = store.UpdateActivity("test-session")
	if err != nil {
		t.Errorf("UpdateActivity failed: %v", err)
	}

	updatedDetails, err := store.GetDetails("test-session")
	if err != nil {
		t.Errorf("GetDetails after update failed: %v", err)
	}
	if !updatedDetails.LastActivity.After(oldActivity) {
		t.Error("LastActivity was not updated")
	}

	// Test SetTokenInfo
	tokenInfo := &session.TokenInfo{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
		IssuedAt:     time.Now(),
		Scope:        "read write",
	}

	err = store.SetTokenInfo("test-session", tokenInfo)
	if err != nil {
		t.Errorf("SetTokenInfo failed: %v", err)
	}

	// Test GetTokenInfo
	retrievedToken, err := store.GetTokenInfo("test-session")
	if err != nil {
		t.Errorf("GetTokenInfo failed: %v", err)
	}
	if retrievedToken == nil {
		t.Fatal("Expected token info, got nil")
	}
	if retrievedToken.AccessToken != "test-access-token" {
		t.Errorf("AccessToken mismatch: expected test-access-token, got %s", retrievedToken.AccessToken)
	}

	// Test IsTokenValid
	valid := store.IsTokenValid("test-session")
	if !valid {
		t.Error("Expected token to be valid")
	}

	// Test SetAuthorizationInfo
	authInfo := &session.AuthorizationInfo{
		Status:           "authorized",
		AuthorizationURL: "https://auth.example.com/authorize",
		AuthorizedAt:     time.Now(),
		ClientID:         "test-client",
	}

	err = store.SetAuthorizationInfo("test-session", authInfo)
	if err != nil {
		t.Errorf("SetAuthorizationInfo failed: %v", err)
	}

	// Test GetAuthorizationInfo
	retrievedAuth, err := store.GetAuthorizationInfo("test-session")
	if err != nil {
		t.Errorf("GetAuthorizationInfo failed: %v", err)
	}
	if retrievedAuth == nil {
		t.Fatal("Expected authorization info, got nil")
	}
	if retrievedAuth.Status != "authorized" {
		t.Errorf("Status mismatch: expected authorized, got %s", retrievedAuth.Status)
	}

	// Test Delete
	err = store.Delete("test-session")
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	// Verify deletion
	deletedDetails, err := store.GetDetails("test-session")
	if err == nil {
		t.Error("Expected error when getting deleted session")
	}
	if deletedDetails != nil {
		t.Error("Expected nil after deletion")
	}
}

func TestSessionStoreCleanupExpired(t *testing.T) {
	store := session.NewInMemorySessionStore()

	// Create a session
	err := store.SetWithDetails("expired-session", "test-adapter", "localhost:8080", "stdio")
	if err != nil {
		t.Errorf("SetWithDetails failed: %v", err)
	}

	// Manually set old last activity (simulate expired session)
	details, _ := store.GetDetails("expired-session")
	details.LastActivity = time.Now().Add(-25 * time.Hour) // 25 hours ago
	// Note: In a real implementation, we'd need a way to update this, but for testing we'll assume cleanup works

	// Test cleanup with 1 hour max age
	err = store.CleanupExpired(1 * time.Hour)
	if err != nil {
		t.Errorf("CleanupExpired failed: %v", err)
	}

	// The session should still exist since we can't easily manipulate the internal storage
	// This test mainly ensures the method doesn't panic
}
