package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DevelopmentAuthMiddleware is a simple middleware for development
func DevelopmentAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// In development, set a fake user
		c.Set("user", "dev")
		c.Next()
	}
}

// ExternalOAuthConfig holds external OAuth provider configuration
type ExternalOAuthConfig struct {
	Provider string
	ClientID string
	TenantID string
	JWKSURL  string
	Issuer   string
	Audience string
	Required bool
}

// OAuthMiddleware provides OAuth authentication
type OAuthMiddleware struct {
	config *ExternalOAuthConfig
}

// NewOAuthMiddleware creates a new OAuth middleware
func NewOAuthMiddleware(config *ExternalOAuthConfig) *OAuthMiddleware {
	return &OAuthMiddleware{config: config}
}

// Middleware returns the Gin middleware function
func (om *OAuthMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For minimal version, check for Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			if om.config.Required {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
				c.Abort()
				return
			}
			// Not required, set default user
			c.Set("user", "anonymous")
			c.Next()
			return
		}

		// Basic Bearer token validation (placeholder)
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if userID, err := om.validateToken(token); err == nil {
				c.Set("user", userID)
				c.Next()
				return
			}
		}

		if om.config.Required {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Fallback to anonymous
		c.Set("user", "anonymous")
		c.Next()
	}
}

// validateToken performs basic token validation (placeholder for full JWT validation)
func (om *OAuthMiddleware) validateToken(tokenString string) (string, error) {
	// For minimal version, accept any non-empty token as valid
	// In production, this would validate JWT signature, issuer, audience, etc.
	if tokenString == "" {
		return "", fmt.Errorf("empty token")
	}

	// For minimal version, just return a user ID based on token presence
	// This is NOT secure and should be replaced with proper JWT validation
	return "authenticated-user", nil
}
