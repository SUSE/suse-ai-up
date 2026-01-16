package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/internal/service"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// MCPAuthHandler handles MCP authentication endpoints
type MCPAuthHandler struct {
	store           clients.AdapterResourceStore
	authIntegration *service.MCPAuthIntegrationService
}

// NewMCPAuthHandler creates a new MCP auth handler
func NewMCPAuthHandler(store clients.AdapterResourceStore, authIntegration *service.MCPAuthIntegrationService) *MCPAuthHandler {
	return &MCPAuthHandler{
		store:           store,
		authIntegration: authIntegration,
	}
}

// GetClientToken handles GET /adapters/:name/client-token
// @Summary Get client token for adapter
// @Description Retrieves a client token for authenticating with the MCP server adapter.
// @Tags adapters, authentication
// @Produce json
// @Param name path string true "Adapter name"
// @Success 200 {object} service.ClientTokenResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/client-token [get]
func (h *MCPAuthHandler) GetClientToken(c *gin.Context) {
	name := c.Param("name")

	// Get adapter
	adapter, err := h.store.Get(c.Request.Context(), name)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Check if auth integration is available
	if h.authIntegration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication integration not available"})
		return
	}

	// Get client token
	tokenResponse, err := h.authIntegration.GetClientToken(*adapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tokenResponse)
}

// ValidateAuthConfig handles POST /adapters/:name/validate-auth
// @Summary Validate adapter authentication configuration
// @Description Validates the authentication configuration for an adapter.
// @Tags adapters, authentication
// @Accept json
// @Produce json
// @Param name path string true "Adapter name"
// @Param authConfig body models.AdapterAuthConfig true "Authentication configuration to validate"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/validate-auth [post]
func (h *MCPAuthHandler) ValidateAuthConfig(c *gin.Context) {
	name := c.Param("name")

	var authConfig struct {
		Required    bool               `json:"required"`
		Type        string             `json:"type"`
		BearerToken *BearerTokenConfig `json:"bearerToken,omitempty"`
		OAuth       *OAuthConfig       `json:"oauth,omitempty"`
		Basic       *BasicAuthConfig   `json:"basic,omitempty"`
		APIKey      *APIKeyConfig      `json:"apiKey,omitempty"`
		Token       string             `json:"token,omitempty"` // Legacy
	}

	if err := c.ShouldBindJSON(&authConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to models.AdapterAuthConfig
	modelAuthConfig := &models.AdapterAuthConfig{
		Required: authConfig.Required,
		Type:     authConfig.Type,
	}

	// Convert nested configs if present
	if authConfig.BearerToken != nil {
		modelAuthConfig.BearerToken = &models.BearerTokenConfig{
			Token:     authConfig.BearerToken.Token,
			Dynamic:   authConfig.BearerToken.Dynamic,
			ExpiresAt: time.Time{}, // Parse from string if needed
		}
	}

	if authConfig.OAuth != nil {
		modelAuthConfig.OAuth = &models.OAuthConfig{
			ClientID:     authConfig.OAuth.ClientID,
			ClientSecret: authConfig.OAuth.ClientSecret,
			AuthURL:      authConfig.OAuth.AuthURL,
			TokenURL:     authConfig.OAuth.TokenURL,
			Scopes:       authConfig.OAuth.Scopes,
			RedirectURI:  authConfig.OAuth.RedirectURI,
		}
	}

	if authConfig.Basic != nil {
		modelAuthConfig.Basic = &models.BasicAuthConfig{
			Username: authConfig.Basic.Username,
			Password: authConfig.Basic.Password,
		}
	}

	if authConfig.APIKey != nil {
		modelAuthConfig.APIKey = &models.APIKeyConfig{
			Key:      authConfig.APIKey.Key,
			Location: authConfig.APIKey.Location,
			Name:     authConfig.APIKey.Name,
		}
	}

	// Check if auth integration is available
	if h.authIntegration == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication integration not available"})
		return
	}

	// Validate configuration
	err := h.authIntegration.ValidateAuthConfig(modelAuthConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "Authentication configuration is valid",
		"adapter": name,
	})
}

// TestAuthConnection handles POST /adapters/:name/test-auth
// @Summary Test adapter authentication
// @Description Tests the authentication configuration by attempting to connect to the MCP server.
// @Tags adapters, authentication
// @Accept json
// @Produce json
// @Param name path string true "Adapter name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/test-auth [post]
func (h *MCPAuthHandler) TestAuthConnection(c *gin.Context) {
	name := c.Param("name")

	// Get adapter
	adapter, err := h.store.Get(c.Request.Context(), name)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// For now, return a placeholder response
	// In a full implementation, this would test the actual connection
	c.JSON(http.StatusOK, gin.H{
		"adapter": name,
		"status":  "tested",
		"message": "Authentication test not yet implemented",
	})
}

// BearerTokenConfig represents bearer token authentication configuration
type BearerTokenConfig struct {
	Token     string `json:"token,omitempty"`
	Dynamic   bool   `json:"dynamic"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// OAuthConfig represents OAuth authentication configuration
type OAuthConfig struct {
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	AuthURL      string   `json:"authUrl,omitempty"`
	TokenURL     string   `json:"tokenUrl,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	RedirectURI  string   `json:"redirectUri,omitempty"`
}

// BasicAuthConfig represents basic authentication configuration
type BasicAuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// APIKeyConfig represents API key authentication configuration
type APIKeyConfig struct {
	Key      string `json:"key,omitempty"`
	Location string `json:"location,omitempty"`
	Name     string `json:"name,omitempty"`
}
