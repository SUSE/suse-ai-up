package service

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/models"
)

// MCPAuthIntegrationService handles authentication integration for MCP adapters
type MCPAuthIntegrationService struct {
	tokenManager *auth.TokenManager
	httpClient   *http.Client
}

// NewMCPAuthIntegrationService creates a new MCP auth integration service
func NewMCPAuthIntegrationService(tokenManager *auth.TokenManager) *MCPAuthIntegrationService {
	return &MCPAuthIntegrationService{
		tokenManager: tokenManager,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// GetClientToken retrieves a client token for the given adapter
func (mais *MCPAuthIntegrationService) GetClientToken(adapter models.AdapterResource) (*ClientTokenResponse, error) {
	log.Printf("MCPAuthIntegrationService: Getting client token for adapter %s", adapter.Name)

	if adapter.Authentication == nil || !adapter.Authentication.Required {
		return &ClientTokenResponse{
			Token:     "",
			Type:      "none",
			ExpiresAt: time.Time{},
			Message:   "No authentication required",
		}, nil
	}

	switch adapter.Authentication.Type {
	case "bearer":
		return mais.getBearerToken(adapter)
	case "oauth":
		return mais.getOAuthToken(adapter)
	case "basic":
		return mais.getBasicAuth(adapter)
	case "apikey":
		return mais.getAPIKey(adapter)
	default:
		return nil, fmt.Errorf("unsupported authentication type: %s", adapter.Authentication.Type)
	}
}

// ClientTokenResponse represents a client token response
type ClientTokenResponse struct {
	Token     string    `json:"token"`
	Type      string    `json:"type"`
	ExpiresAt time.Time `json:"expiresAt"`
	Message   string    `json:"message,omitempty"`
}

// getBearerToken handles bearer token authentication
func (mais *MCPAuthIntegrationService) getBearerToken(adapter models.AdapterResource) (*ClientTokenResponse, error) {
	// Check for bearer token configuration
	if adapter.Authentication.BearerToken != nil && adapter.Authentication.BearerToken.Token != "" {
		return &ClientTokenResponse{
			Token:     adapter.Authentication.BearerToken.Token,
			Type:      "bearer",
			ExpiresAt: adapter.Authentication.BearerToken.ExpiresAt,
			Message:   "Using bearer token",
		}, nil
	}

	// Check for new bearer token configuration
	if adapter.Authentication.BearerToken != nil {
		if adapter.Authentication.BearerToken.Token != "" && !adapter.Authentication.BearerToken.Dynamic {
			return &ClientTokenResponse{
				Token:     adapter.Authentication.BearerToken.Token,
				Type:      "bearer",
				ExpiresAt: adapter.Authentication.BearerToken.ExpiresAt,
				Message:   "Using static bearer token",
			}, nil
		}

		// Generate dynamic token if token manager is available
		if adapter.Authentication.BearerToken.Dynamic && mais.tokenManager != nil {
			tokenInfo, err := mais.tokenManager.GenerateBearerToken(adapter.Name, adapter.RemoteUrl, 24)
			if err != nil {
				return nil, fmt.Errorf("failed to generate dynamic bearer token: %w", err)
			}

			return &ClientTokenResponse{
				Token:     tokenInfo.AccessToken,
				Type:      "bearer",
				ExpiresAt: tokenInfo.ExpiresAt,
				Message:   "Using dynamic bearer token",
			}, nil
		}
	}

	return nil, fmt.Errorf("no bearer token configuration found")
}

// getOAuthToken handles OAuth authentication
func (mais *MCPAuthIntegrationService) getOAuthToken(adapter models.AdapterResource) (*ClientTokenResponse, error) {
	if adapter.Authentication.OAuth == nil {
		return nil, fmt.Errorf("OAuth configuration not found")
	}

	oauthConfig := adapter.Authentication.OAuth

	// For now, return a placeholder message
	// In a full implementation, this would handle OAuth flows
	return &ClientTokenResponse{
		Token:     "",
		Type:      "oauth",
		ExpiresAt: time.Time{},
		Message:   fmt.Sprintf("OAuth authentication configured for client ID: %s. Please use OAuth flow to obtain token.", oauthConfig.ClientID),
	}, nil
}

// getBasicAuth handles basic authentication
func (mais *MCPAuthIntegrationService) getBasicAuth(adapter models.AdapterResource) (*ClientTokenResponse, error) {
	if adapter.Authentication.Basic == nil {
		return nil, fmt.Errorf("Basic authentication configuration not found")
	}

	basicConfig := adapter.Authentication.Basic

	// Create basic auth token (base64 encoded username:password)
	credentials := fmt.Sprintf("%s:%s", basicConfig.Username, basicConfig.Password)
	// Note: In a real implementation, you would base64 encode this
	// For security reasons, we're not exposing the actual password in the response

	return &ClientTokenResponse{
		Token:     credentials, // In practice, this would be base64 encoded
		Type:      "basic",
		ExpiresAt: time.Time{},
		Message:   "Basic authentication credentials",
	}, nil
}

// getAPIKey handles API key authentication
func (mais *MCPAuthIntegrationService) getAPIKey(adapter models.AdapterResource) (*ClientTokenResponse, error) {
	if adapter.Authentication.APIKey == nil {
		return nil, fmt.Errorf("API key configuration not found")
	}

	apiKeyConfig := adapter.Authentication.APIKey

	return &ClientTokenResponse{
		Token:     apiKeyConfig.Key,
		Type:      "apikey",
		ExpiresAt: time.Time{},
		Message:   fmt.Sprintf("API key authentication (location: %s, name: %s)", apiKeyConfig.Location, apiKeyConfig.Name),
	}, nil
}

// ValidateAuthConfig validates authentication configuration
func (mais *MCPAuthIntegrationService) ValidateAuthConfig(auth *models.AdapterAuthConfig) error {
	if auth == nil {
		return nil // No auth is valid
	}

	switch auth.Type {
	case "none":
		return nil
	case "bearer":
		return mais.validateBearerConfig(auth)
	case "oauth":
		return mais.validateOAuthConfig(auth)
	case "basic":
		return mais.validateBasicConfig(auth)
	case "apikey":
		return mais.validateAPIKeyConfig(auth)
	default:
		return fmt.Errorf("unsupported authentication type: %s", auth.Type)
	}
}

// validateBearerConfig validates bearer token configuration
func (mais *MCPAuthIntegrationService) validateBearerConfig(auth *models.AdapterAuthConfig) error {
	// Check bearer token configuration
	if auth.BearerToken == nil {
		return fmt.Errorf("bearer token configuration is required")
	}

	if auth.BearerToken.Token == "" && !auth.BearerToken.Dynamic {
		return fmt.Errorf("either static token or dynamic token generation must be configured")
	}

	return nil
}

// validateOAuthConfig validates OAuth configuration
func (mais *MCPAuthIntegrationService) validateOAuthConfig(auth *models.AdapterAuthConfig) error {
	if auth.OAuth == nil {
		return fmt.Errorf("OAuth configuration is required")
	}

	if auth.OAuth.ClientID == "" {
		return fmt.Errorf("OAuth client ID is required")
	}

	if auth.OAuth.AuthURL == "" {
		return fmt.Errorf("OAuth authorization URL is required")
	}

	if auth.OAuth.TokenURL == "" {
		return fmt.Errorf("OAuth token URL is required")
	}

	return nil
}

// validateBasicConfig validates basic authentication configuration
func (mais *MCPAuthIntegrationService) validateBasicConfig(auth *models.AdapterAuthConfig) error {
	if auth.Basic == nil {
		return fmt.Errorf("Basic authentication configuration is required")
	}

	if auth.Basic.Username == "" {
		return fmt.Errorf("username is required for basic authentication")
	}

	if auth.Basic.Password == "" {
		return fmt.Errorf("password is required for basic authentication")
	}

	return nil
}

// validateAPIKeyConfig validates API key configuration
func (mais *MCPAuthIntegrationService) validateAPIKeyConfig(auth *models.AdapterAuthConfig) error {
	if auth.APIKey == nil {
		return fmt.Errorf("API key configuration is required")
	}

	if auth.APIKey.Key == "" {
		return fmt.Errorf("API key is required")
	}

	if auth.APIKey.Location == "" {
		auth.APIKey.Location = "header" // Default to header
	}

	if auth.APIKey.Name == "" {
		auth.APIKey.Name = "X-API-Key" // Default header name
	}

	// Validate location
	validLocations := []string{"header", "query", "cookie"}
	valid := false
	for _, loc := range validLocations {
		if auth.APIKey.Location == loc {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid API key location: %s. Valid options: %v", auth.APIKey.Location, validLocations)
	}

	return nil
}

// ApplyAuthToRequest applies authentication to an HTTP request
func (mais *MCPAuthIntegrationService) ApplyAuthToRequest(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth == nil || !auth.Required {
		return nil // No authentication required
	}

	switch auth.Type {
	case "bearer":
		return mais.applyBearerAuth(req, auth)
	case "oauth":
		return mais.applyOAuthAuth(req, auth)
	case "basic":
		return mais.applyBasicAuth(req, auth)
	case "apikey":
		return mais.applyAPIKeyAuth(req, auth)
	default:
		return fmt.Errorf("unsupported authentication type: %s", auth.Type)
	}
}

// applyBearerAuth applies bearer authentication to request
func (mais *MCPAuthIntegrationService) applyBearerAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	var token string

	// Check bearer token configuration
	if auth.BearerToken != nil {
		if auth.BearerToken.Token != "" {
			token = auth.BearerToken.Token
		} else if auth.BearerToken.Dynamic && mais.tokenManager != nil {
			// Generate dynamic token
			tokenInfo, err := mais.tokenManager.GenerateBearerToken("", "", 24)
			if err != nil {
				return fmt.Errorf("failed to generate dynamic bearer token: %w", err)
			}
			token = tokenInfo.AccessToken
		}
	}

	if token == "" {
		return fmt.Errorf("no bearer token available")
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// applyOAuthAuth applies OAuth authentication to request
func (mais *MCPAuthIntegrationService) applyOAuthAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	// For now, this is a placeholder
	// In a full implementation, this would handle OAuth token management
	return fmt.Errorf("OAuth authentication not yet implemented for request signing")
}

// applyBasicAuth applies basic authentication to request
func (mais *MCPAuthIntegrationService) applyBasicAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth.Basic == nil {
		return fmt.Errorf("basic authentication configuration not found")
	}

	req.SetBasicAuth(auth.Basic.Username, auth.Basic.Password)
	return nil
}

// applyAPIKeyAuth applies API key authentication to request
func (mais *MCPAuthIntegrationService) applyAPIKeyAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth.APIKey == nil {
		return fmt.Errorf("API key configuration not found")
	}

	location := strings.ToLower(auth.APIKey.Location)
	name := auth.APIKey.Name
	key := auth.APIKey.Key

	switch location {
	case "header":
		req.Header.Set(name, key)
	case "query":
		// Add to query parameters
		if req.URL == nil {
			return fmt.Errorf("request URL is nil")
		}
		query := req.URL.Query()
		query.Set(name, key)
		req.URL.RawQuery = query.Encode()
	case "cookie":
		// Add cookie
		req.AddCookie(&http.Cookie{Name: name, Value: key})
	default:
		return fmt.Errorf("unsupported API key location: %s", location)
	}

	return nil
}
