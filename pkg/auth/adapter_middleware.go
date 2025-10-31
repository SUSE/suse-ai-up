package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/clients"
)

// AdapterAuthMiddleware provides authentication for adapter endpoints
type AdapterAuthMiddleware struct {
	store clients.AdapterResourceStore
}

// NewAdapterAuthMiddleware creates a new adapter authentication middleware
func NewAdapterAuthMiddleware(store clients.AdapterResourceStore) *AdapterAuthMiddleware {
	return &AdapterAuthMiddleware{store: store}
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
		adapter, err := aam.store.TryGetAsync(adapterName, nil)
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
			if !strings.HasPrefix(authHeader, "Bearer ") {
				fmt.Printf("AUTH: Invalid auth type for adapter %s from %s: expected Bearer, got %s\n",
					adapterName, clientIP, strings.Split(authHeader, " ")[0])
				c.JSON(http.StatusUnauthorized, AuthError{
					Code:    "INVALID_AUTH_TYPE",
					Message: "Bearer token required",
					Details: "Authorization header must start with 'Bearer '",
				})
				c.Abort()
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				fmt.Printf("AUTH: Empty bearer token for adapter %s from %s\n", adapterName, clientIP)
				c.JSON(http.StatusUnauthorized, AuthError{
					Code:    "EMPTY_TOKEN",
					Message: "Bearer token required",
					Details: "Bearer token cannot be empty",
				})
				c.Abort()
				return
			}

			if token != adapter.Authentication.Token {
				fmt.Printf("AUTH: Invalid token for adapter %s from %s\n", adapterName, clientIP)
				c.JSON(http.StatusUnauthorized, AuthError{
					Code:    "INVALID_TOKEN",
					Message: "Authentication failed",
					Details: "The provided Bearer token is invalid or expired",
				})
				c.Abort()
				return
			}

			// Authentication successful
			fmt.Printf("AUTH: Successful authentication for adapter %s from %s\n", adapterName, clientIP)
			c.Set("user", "authenticated-user")
			c.Set("auth_type", "bearer")
			c.Set("adapter_name", adapterName)
			c.Next()

		case "oauth":
			// For now, delegate to existing OAuth middleware
			// This could be enhanced to support adapter-specific OAuth configs
			fmt.Printf("AUTH: OAuth authentication requested for adapter %s from %s\n", adapterName, clientIP)
			oauthMiddleware := NewOAuthMiddleware(&OAuthConfig{
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
