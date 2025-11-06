package auth

import (
	"testing"
)

func TestTokenManager(t *testing.T) {
	// Create token manager
	tm, err := NewTokenManager("test-issuer")
	if err != nil {
		t.Fatalf("Failed to create token manager: %v", err)
	}

	// Test token generation
	tokenInfo, err := tm.GenerateBearerToken("test-adapter", "http://localhost:8080", 24)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if tokenInfo.AccessToken == "" {
		t.Error("Access token should not be empty")
	}

	if tokenInfo.TokenType != "Bearer" {
		t.Errorf("Expected token type 'Bearer', got '%s'", tokenInfo.TokenType)
	}

	if tokenInfo.Subject != "test-adapter" {
		t.Errorf("Expected subject 'test-adapter', got '%s'", tokenInfo.Subject)
	}

	// Test token validation
	validatedToken, err := tm.ValidateToken(tokenInfo.AccessToken, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if validatedToken.TokenID != tokenInfo.TokenID {
		t.Errorf("Expected token ID '%s', got '%s'", tokenInfo.TokenID, validatedToken.TokenID)
	}

	// Test invalid audience
	_, err = tm.ValidateToken(tokenInfo.AccessToken, "http://wrong-audience")
	if err == nil {
		t.Error("Expected validation error for wrong audience")
	}

	// Test refresh token generation
	refreshToken, err := tm.GenerateRefreshToken("test-adapter")
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	if refreshToken == "" {
		t.Error("Refresh token should not be empty")
	}

	// Test refresh token validation
	adapterName, err := tm.ValidateRefreshToken(refreshToken)
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}

	if adapterName != "test-adapter" {
		t.Errorf("Expected adapter name 'test-adapter', got '%s'", adapterName)
	}
}

func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		expectError bool
		expected    string
	}{
		{
			name:        "valid bearer token",
			header:      "Bearer abc123",
			expectError: false,
			expected:    "abc123",
		},
		{
			name:        "empty header",
			header:      "",
			expectError: true,
		},
		{
			name:        "invalid format",
			header:      "InvalidToken abc123",
			expectError: true,
		},
		{
			name:        "missing token",
			header:      "Bearer ",
			expectError: true,
		},
		{
			name:        "wrong scheme",
			header:      "Basic abc123",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractTokenFromHeader(tt.header)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if token != tt.expected {
					t.Errorf("Expected token '%s', got '%s'", tt.expected, token)
				}
			}
		})
	}
}
