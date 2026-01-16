package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// TokenHandler handles token-related operations
type TokenHandler struct {
	store        clients.AdapterResourceStore
	tokenManager *auth.TokenManager
}

// NewTokenHandler creates a new token handler
func NewTokenHandler(store clients.AdapterResourceStore, tokenManager *auth.TokenManager) *TokenHandler {
	return &TokenHandler{
		store:        store,
		tokenManager: tokenManager,
	}
}

// GetAdapterToken retrieves or generates a token for an adapter
// @Summary Get adapter token
// @Description Retrieves the current token for an adapter or generates a new one if none exists
// @Tags tokens
// @Param name path string true "Adapter name"
// @Param generate query bool false "Generate new token if none exists" default(true)
// @Param expiresIn query int false "Token expiration time in hours" default(24)
// @Success 200 {object} auth.TokenInfo
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 500 {object} handlers.ErrorResponse
// @Router /api/v1/adapters/{name}/token [get]
func (th *TokenHandler) GetAdapterToken(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Adapter name is required"})
		return
	}

	// Get adapter configuration
	adapter, err := th.store.Get(c.Request.Context(), adapterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}
	if adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Check if adapter requires authentication
	if adapter.Authentication == nil || !adapter.Authentication.Required {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Adapter does not require authentication",
			"adapter": gin.H{
				"name":          adapterName,
				"auth_required": false,
			},
		})
		return
	}

	// Parse query parameters
	generateNew := c.DefaultQuery("generate", "true") == "true"
	expiresInStr := c.DefaultQuery("expiresIn", "24")
	expiresIn, err := strconv.Atoi(expiresInStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiresIn parameter"})
		return
	}

	var tokenInfo *auth.TokenInfo

	// Check if we have a token manager for JWT tokens
	if th.tokenManager != nil {
		// Try to validate existing token as JWT first
		if adapter.Authentication.BearerToken != nil && adapter.Authentication.BearerToken.Token != "" {
			if existingTokenInfo, err := th.tokenManager.ValidateToken(adapter.Authentication.BearerToken.Token, adapter.RemoteUrl); err == nil {
				// Existing token is valid JWT
				tokenInfo = existingTokenInfo
				c.JSON(http.StatusOK, gin.H{
					"adapter": adapterName,
					"token": gin.H{
						"token_id":     tokenInfo.TokenID,
						"access_token": tokenInfo.AccessToken,
						"token_type":   tokenInfo.TokenType,
						"expires_at":   tokenInfo.ExpiresAt.Format(time.RFC3339),
						"issued_at":    tokenInfo.IssuedAt.Format(time.RFC3339),
						"scope":        tokenInfo.Scope,
						"audience":     tokenInfo.Audience,
						"issuer":       tokenInfo.Issuer,
						"subject":      tokenInfo.Subject,
						"format":       "jwt",
					},
					"message": "Existing valid JWT token retrieved",
				})
				return
			}
		}

		// Generate new JWT token if requested or no valid token exists
		if generateNew {
			newTokenInfo, err := th.tokenManager.GenerateBearerToken(adapterName, adapter.RemoteUrl, expiresIn)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
				return
			}

			// Update adapter with new token
			if adapter.Authentication.BearerToken == nil {
				adapter.Authentication.BearerToken = &models.BearerTokenConfig{}
			}
			adapter.Authentication.BearerToken.Token = newTokenInfo.AccessToken
			if err := th.store.UpsertAsync(*adapter, nil); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update adapter with new token"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"adapter": adapterName,
				"token": gin.H{
					"token_id":     newTokenInfo.TokenID,
					"access_token": newTokenInfo.AccessToken,
					"token_type":   newTokenInfo.TokenType,
					"expires_at":   newTokenInfo.ExpiresAt.Format(time.RFC3339),
					"issued_at":    newTokenInfo.IssuedAt.Format(time.RFC3339),
					"scope":        newTokenInfo.Scope,
					"audience":     newTokenInfo.Audience,
					"issuer":       newTokenInfo.Issuer,
					"subject":      newTokenInfo.Subject,
					"format":       "jwt",
				},
				"message": "New JWT token generated and saved to adapter",
			})
			return
		}
	}

	// Fallback to legacy token handling
	if adapter.Authentication.BearerToken != nil && adapter.Authentication.BearerToken.Token != "" {
		c.JSON(http.StatusOK, gin.H{
			"adapter": adapterName,
			"token": gin.H{
				"access_token": adapter.Authentication.BearerToken.Token,
				"token_type":   "Bearer",
				"format":       "legacy",
				"note":         "Legacy token format - consider upgrading to JWT",
			},
			"message": "Legacy token retrieved",
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"error":      "No token available for adapter",
		"adapter":    adapterName,
		"suggestion": "Use ?generate=true to create a new token",
	})
}

// ValidateToken validates a token and returns its information
// @Summary Validate token
// @Description Validates a token and returns its claims and validity
// @Tags tokens
// @Param name path string true "Adapter name"
// @Param token query string true "Token to validate"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /api/v1/adapters/{name}/token/validate [post]
func (th *TokenHandler) ValidateToken(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Adapter name is required"})
		return
	}

	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token parameter is required"})
		return
	}

	// Get adapter configuration for audience validation
	adapter, err := th.store.Get(c.Request.Context(), adapterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}
	if adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Try JWT validation first
	if th.tokenManager != nil {
		tokenInfo, err := th.tokenManager.ValidateToken(token, adapter.RemoteUrl)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{
				"valid":   true,
				"adapter": adapterName,
				"token": gin.H{
					"token_id":   tokenInfo.TokenID,
					"token_type": tokenInfo.TokenType,
					"expires_at": tokenInfo.ExpiresAt.Format(time.RFC3339),
					"issued_at":  tokenInfo.IssuedAt.Format(time.RFC3339),
					"scope":      tokenInfo.Scope,
					"audience":   tokenInfo.Audience,
					"issuer":     tokenInfo.Issuer,
					"subject":    tokenInfo.Subject,
					"format":     "jwt",
				},
				"message": "Token is valid",
			})
			return
		}
	}

	// Fallback to legacy validation
	if adapter.Authentication != nil && adapter.Authentication.BearerToken != nil && adapter.Authentication.BearerToken.Token == token {
		c.JSON(http.StatusOK, gin.H{
			"valid":   true,
			"adapter": adapterName,
			"token": gin.H{
				"format": "legacy",
				"note":   "Legacy token validation - exact match only",
			},
			"message": "Legacy token is valid",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   false,
		"adapter": adapterName,
		"error":   "Invalid token",
	})
}

// RefreshToken generates a new token for an adapter
// @Summary Refresh adapter token
// @Description Generates a new token for an adapter, invalidating the old one
// @Tags tokens
// @Param name path string true "Adapter name"
// @Param expiresIn query int false "Token expiration time in hours" default(24)
// @Success 200 {object} auth.TokenInfo
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Failure 500 {object} handlers.ErrorResponse
// @Router /api/v1/adapters/{name}/token/refresh [post]
func (th *TokenHandler) RefreshToken(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Adapter name is required"})
		return
	}

	// Get adapter configuration
	adapter, err := th.store.Get(c.Request.Context(), adapterName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}
	if adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Check if adapter requires authentication
	if adapter.Authentication == nil || !adapter.Authentication.Required {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Adapter does not require authentication",
			"adapter": gin.H{
				"name":          adapterName,
				"auth_required": false,
			},
		})
		return
	}

	// Parse expiration parameter
	expiresInStr := c.DefaultQuery("expiresIn", "24")
	expiresIn, err := strconv.Atoi(expiresInStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiresIn parameter"})
		return
	}

	// Generate new token
	if th.tokenManager != nil {
		// Generate JWT token
		newTokenInfo, err := th.tokenManager.GenerateBearerToken(adapterName, adapter.RemoteUrl, expiresIn)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new token"})
			return
		}

		// Update adapter with new token
		if adapter.Authentication.BearerToken == nil {
			adapter.Authentication.BearerToken = &models.BearerTokenConfig{}
		}
		adapter.Authentication.BearerToken.Token = newTokenInfo.AccessToken
		if err := th.store.UpsertAsync(*adapter, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update adapter with new token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"adapter": adapterName,
			"token": gin.H{
				"token_id":     newTokenInfo.TokenID,
				"access_token": newTokenInfo.AccessToken,
				"token_type":   newTokenInfo.TokenType,
				"expires_at":   newTokenInfo.ExpiresAt.Format(time.RFC3339),
				"issued_at":    newTokenInfo.IssuedAt.Format(time.RFC3339),
				"scope":        newTokenInfo.Scope,
				"audience":     newTokenInfo.Audience,
				"issuer":       newTokenInfo.Issuer,
				"subject":      newTokenInfo.Subject,
				"format":       "jwt",
			},
			"message": "Token refreshed successfully",
		})
	} else {
		// Fallback to legacy token generation
		newToken := generateLegacyToken()
		if adapter.Authentication.BearerToken == nil {
			adapter.Authentication.BearerToken = &models.BearerTokenConfig{}
		}
		adapter.Authentication.BearerToken.Token = newToken
		if err := th.store.UpsertAsync(*adapter, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update adapter with new token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"adapter": adapterName,
			"token": gin.H{
				"access_token": newToken,
				"token_type":   "Bearer",
				"format":       "legacy",
				"note":         "Legacy token format - consider upgrading to JWT",
			},
			"message": "Legacy token refreshed successfully",
		})
	}
}

// Helper function for legacy token generation
func generateLegacyToken() string {
	// This should match the legacy token generation in discovery service
	// For now, return a simple timestamp-based token
	return "legacy-token-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
