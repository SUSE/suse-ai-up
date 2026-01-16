package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"suse-ai-up/pkg/models"
)

// AuthMode represents the authentication mode for an adapter
type AuthMode string

const (
	AuthModeBearer AuthMode = "bearer"
	AuthModeOAuth  AuthMode = "oauth"
)

// TokenInfo represents information about a token
type TokenInfo struct {
	TokenID      string    `json:"tokenId"`
	AccessToken  string    `json:"accessToken"`
	TokenType    string    `json:"tokenType"`
	ExpiresAt    time.Time `json:"expiresAt"`
	IssuedAt     time.Time `json:"issuedAt"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	Audience     string    `json:"audience"`
	Issuer       string    `json:"issuer"`
	Subject      string    `json:"subject"`
}

// OAuthConfig represents OAuth 2.1 configuration
type OAuthConfig struct {
	Provider     string            `json:"provider"`
	ClientID     string            `json:"clientId"`
	ClientSecret string            `json:"clientSecret"`
	AuthURL      string            `json:"authUrl"`
	TokenURL     string            `json:"tokenUrl"`
	Scopes       []string          `json:"scopes"`
	RedirectURI  string            `json:"redirectUri"`
	ExtraParams  map[string]string `json:"extraParams,omitempty"`
}

// BearerTokenConfig represents Bearer token configuration
type BearerTokenConfig struct {
	AutoGenerate bool   `json:"autoGenerate"`
	CustomToken  string `json:"customToken,omitempty"`
	ExpiresIn    int    `json:"expiresIn,omitempty"` // in hours, default 24
}

// AdapterAuthConfig represents the complete authentication configuration for an adapter
type AdapterAuthConfig struct {
	Mode         AuthMode           `json:"mode"`
	Required     bool               `json:"required"`
	OAuthConfig  *OAuthConfig       `json:"oauthConfig,omitempty"`
	BearerConfig *BearerTokenConfig `json:"bearerConfig,omitempty"`
}

// TokenManager manages OAuth 2.1 compliant tokens
type TokenManager struct {
	privateKey *rsa.PrivateKey
	issuer     string
}

// NewTokenManager creates a new token manager
func NewTokenManager(issuer string) (*TokenManager, error) {
	// Generate RSA key pair for JWT signing
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key pair: %w", err)
	}

	return &TokenManager{
		privateKey: privateKey,
		issuer:     issuer,
	}, nil
}

// GenerateBearerToken generates a secure Bearer token with JWT format
func (tm *TokenManager) GenerateBearerToken(adapterName, audience string, expiresInHours int) (*TokenInfo, error) {
	if expiresInHours <= 0 {
		expiresInHours = 24 // default to 24 hours
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(expiresInHours) * time.Hour)
	tokenID := tm.generateTokenID()

	// Create JWT claims
	claims := jwt.MapClaims{
		"jti":       tokenID,              // JWT ID
		"sub":       adapterName,          // Subject (adapter name)
		"aud":       audience,             // Audience (adapter URL)
		"iss":       tm.issuer,            // Issuer
		"iat":       now.Unix(),           // Issued at
		"exp":       expiresAt.Unix(),     // Expires at
		"scope":     "mcp:read mcp:write", // Default scopes
		"token_use": "access",             // Token use
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(tm.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign JWT token: %w", err)
	}

	return &TokenInfo{
		TokenID:     tokenID,
		AccessToken: signedToken,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		IssuedAt:    now,
		Audience:    audience,
		Issuer:      tm.issuer,
		Subject:     adapterName,
		Scope:       "mcp:read mcp:write",
	}, nil
}

// CreateTokenForAdapter creates a token specifically for an adapter created from a discovered server
func (tm *TokenManager) CreateTokenForAdapter(adapterName string, server *models.DiscoveredServer) (*TokenInfo, error) {
	audience := fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s", adapterName)

	// Adjust expiration based on vulnerability score
	expiresInHours := 24 // default
	if server.VulnerabilityScore == "high" {
		expiresInHours = 12 // Shorter expiration for high-risk servers
	}

	tokenInfo, err := tm.GenerateBearerToken(adapterName, audience, expiresInHours)
	if err != nil {
		return nil, err
	}

	// Add metadata about the discovered server
	tokenInfo.Scope = fmt.Sprintf("mcp:read mcp:write server:%s risk:%s", server.ID, server.VulnerabilityScore)

	return tokenInfo, nil
}

// ValidateToken validates a JWT token and returns the claims
func (tm *TokenManager) ValidateToken(tokenString, expectedAudience string) (*TokenInfo, error) {
	// Parse and validate token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &tm.privateKey.PublicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate expiration
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token has expired")
		}
	} else {
		return nil, fmt.Errorf("missing expiration claim")
	}

	// Validate audience
	if aud, ok := claims["aud"].(string); ok {
		if aud != expectedAudience {
			return nil, fmt.Errorf("invalid audience: expected %s, got %s", expectedAudience, aud)
		}
	} else {
		return nil, fmt.Errorf("missing audience claim")
	}

	// Extract token info
	tokenInfo := &TokenInfo{
		AccessToken: tokenString,
		TokenType:   "Bearer",
		Issuer:      getStringClaim(claims, "iss"),
		Subject:     getStringClaim(claims, "sub"),
		Audience:    getStringClaim(claims, "aud"),
		Scope:       getStringClaim(claims, "scope"),
	}

	// Parse timestamps
	if iat, ok := claims["iat"].(float64); ok {
		tokenInfo.IssuedAt = time.Unix(int64(iat), 0)
	}
	if exp, ok := claims["exp"].(float64); ok {
		tokenInfo.ExpiresAt = time.Unix(int64(exp), 0)
	}
	if jti, ok := claims["jti"].(string); ok {
		tokenInfo.TokenID = jti
	}

	return tokenInfo, nil
}

// GenerateRefreshToken generates a refresh token
func (tm *TokenManager) GenerateRefreshToken(adapterName string) (string, error) {
	// Generate secure random token
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Add timestamp and adapter name for uniqueness
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%s:%d:%s", adapterName, timestamp, base64.URLEncoding.EncodeToString(bytes))

	return base64.URLEncoding.EncodeToString([]byte(data)), nil
}

// ValidateRefreshToken validates a refresh token
func (tm *TokenManager) ValidateRefreshToken(refreshToken string) (string, error) {
	// Decode base64
	data, err := base64.URLEncoding.DecodeString(refreshToken)
	if err != nil {
		return "", fmt.Errorf("invalid refresh token format")
	}

	// Parse the token data
	parts := strings.Split(string(data), ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid refresh token structure")
	}

	adapterName := parts[0]

	// Validate timestamp (refresh tokens should be valid for 30 days)
	timestampStr := parts[1]
	var timestamp int64
	if _, err := fmt.Sscanf(timestampStr, "%d", &timestamp); err != nil {
		return "", fmt.Errorf("invalid refresh token timestamp")
	}

	expiry := timestamp + (30 * 24 * 60 * 60) // 30 days
	if time.Now().Unix() > expiry {
		return "", fmt.Errorf("refresh token has expired")
	}

	return adapterName, nil
}

// GetPublicKey returns the public key for token validation
func (tm *TokenManager) GetPublicKey() *rsa.PublicKey {
	return &tm.privateKey.PublicKey
}

// GetPrivateKey returns the private key for token signing
func (tm *TokenManager) GetPrivateKey() *rsa.PrivateKey {
	return tm.privateKey
}

// generateTokenID generates a unique token ID
func (tm *TokenManager) generateTokenID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("tok_%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

// getStringClaim safely extracts a string claim from JWT claims
func getStringClaim(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key].(string); ok {
		return val
	}
	return ""
}

// ExtractTokenFromHeader extracts token from Authorization header
func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid authorization header format")
	}

	scheme := strings.ToLower(parts[0])
	if scheme != "bearer" {
		return "", fmt.Errorf("unsupported authorization scheme: %s", scheme)
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", fmt.Errorf("empty token")
	}

	return token, nil
}

// Common authentication error codes
const (
	ErrCodeMissingAuth     = "MISSING_AUTH_HEADER"
	ErrCodeInvalidFormat   = "INVALID_AUTH_FORMAT"
	ErrCodeInvalidScheme   = "INVALID_AUTH_SCHEME"
	ErrCodeEmptyToken      = "EMPTY_TOKEN"
	ErrCodeInvalidToken    = "INVALID_TOKEN"
	ErrCodeExpiredToken    = "EXPIRED_TOKEN"
	ErrCodeInvalidAudience = "INVALID_AUDIENCE"
	ErrCodeUnsupportedAuth = "UNSUPPORTED_AUTH_TYPE"
)
