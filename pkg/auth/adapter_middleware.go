package auth

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/clients"
)

// AdapterAuthMiddleware provides authentication for adapter endpoints
type AdapterAuthMiddleware struct {
	store        clients.AdapterResourceStore
	tokenManager *TokenManager
}

// NewAdapterAuthMiddleware creates a new adapter authentication middleware
func NewAdapterAuthMiddleware(store clients.AdapterResourceStore, tokenManager *TokenManager) *AdapterAuthMiddleware {
	return &AdapterAuthMiddleware{
		store:        store,
		tokenManager: tokenManager,
	}
}

// AuthError represents structured authentication error responses
type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Middleware returns the Gin middleware function for adapter authentication
func (aam *AdapterAuthMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		adapterName := c.Param("name")
		clientIP := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")

		// Get adapter configuration
		adapter, err := aam.store.Get(c.Request.Context(), adapterName)
		if err != nil {
			// Log the error but let the main handler deal with it
			fmt.Printf("AUTH: Failed to retrieve adapter %s: %v\n", adapterName, err)
			c.Next()
			return
		}
		if adapter == nil {
			// Adapter doesn't exist - this will be handled by the main route
			c.Next()
			return
		}

		// Check if adapter requires authentication
		if adapter.Authentication == nil || !adapter.Authentication.Required {
			// No authentication required
			c.Set("user", "anonymous")
			c.Set("auth_type", "none")
			c.Next()
			return
		}

		// Check for Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			fmt.Printf("AUTH: Missing authorization header for adapter %s from %s (%s)\n",
				adapterName, clientIP, userAgent)
			c.JSON(http.StatusUnauthorized, AuthError{
				Code:    "MISSING_AUTH_HEADER",
				Message: "Authentication required",
				Details: fmt.Sprintf("Adapter '%s' requires authentication. Please provide a valid Bearer token.", adapterName),
			})
			c.Abort()
			return
		}

		// Validate based on auth type
		switch adapter.Authentication.Type {
		case "bearer":
			// Extract token from header
			token, err := ExtractTokenFromHeader(authHeader)
			if err != nil {
				fmt.Printf("AUTH: Invalid auth header for adapter %s from %s: %v\n", adapterName, clientIP, err)
				c.JSON(http.StatusUnauthorized, AuthError{
					Code:    ErrCodeInvalidFormat,
					Message: "Invalid authorization header",
					Details: err.Error(),
				})
				c.Abort()
				return
			}

			// Try to validate as JWT token first (new format)
			if aam.tokenManager != nil {
				adapterURL := c.Request.URL.String()
				tokenInfo, err := aam.tokenManager.ValidateToken(token, adapterURL)
				if err == nil {
					// JWT token validation successful
					fmt.Printf("AUTH: Successful JWT authentication for adapter %s from %s (token: %s)\n",
						adapterName, clientIP, tokenInfo.TokenID)
					c.Set("user", tokenInfo.Subject)
					c.Set("auth_type", "bearer_jwt")
					c.Set("adapter_name", adapterName)
					c.Set("token_info", tokenInfo)
					c.Next()
					return
				}

				// Log JWT validation failure but try legacy validation
				fmt.Printf("AUTH: JWT validation failed for adapter %s, trying legacy: %v\n", adapterName, err)
			}

			// Fallback to legacy token validation (string comparison)
			if adapter.Authentication.BearerToken == nil || token != adapter.Authentication.BearerToken.Token {
				fmt.Printf("AUTH: Invalid legacy token for adapter %s from %s\n", adapterName, clientIP)
				c.JSON(http.StatusUnauthorized, AuthError{
					Code:    ErrCodeInvalidToken,
					Message: "Authentication failed",
					Details: "The provided Bearer token is invalid or expired",
				})
				c.Abort()
				return
			}

			// Legacy authentication successful
			fmt.Printf("AUTH: Successful legacy authentication for adapter %s from %s\n", adapterName, clientIP)
			c.Set("user", "authenticated-user")
			c.Set("auth_type", "bearer_legacy")
			c.Set("adapter_name", adapterName)
			c.Next()

		case "oauth":
			// For now, delegate to existing OAuth middleware
			// This could be enhanced to support adapter-specific OAuth configs
			fmt.Printf("AUTH: OAuth authentication requested for adapter %s from %s\n", adapterName, clientIP)
			oauthMiddleware := NewOAuthMiddleware(&ExternalOAuthConfig{
				Required: true,
			})
			oauthMiddleware.Middleware()(c)

		default:
			fmt.Printf("AUTH: Unsupported auth type '%s' for adapter %s from %s\n",
				adapter.Authentication.Type, adapterName, clientIP)
			c.JSON(http.StatusUnauthorized, AuthError{
				Code:    "UNSUPPORTED_AUTH_TYPE",
				Message: "Unsupported authentication method",
				Details: fmt.Sprintf("Adapter '%s' uses authentication type '%s' which is not supported",
					adapterName, adapter.Authentication.Type),
			})
			c.Abort()
			return
		}
	}
}
