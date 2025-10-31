package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/session"
)

// OAuthClient holds OAuth client information
type OAuthClient struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris,omitempty"`
}

// OAuthServerMetadata holds OAuth server metadata
type OAuthServerMetadata struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	JWKSURI                string   `json:"jwks_uri,omitempty"`
	RegistrationEndpoint   string   `json:"registration_endpoint,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported []string `json:"response_types_supported,omitempty"`
	GrantTypesSupported    []string `json:"grant_types_supported,omitempty"`
}

// ProtectedResourceMetadata holds OAuth protected resource metadata
type ProtectedResourceMetadata struct {
	Resource                    string                 `json:"resource"`
	AuthorizationServers        []string               `json:"authorization_servers"`
	ResourceDocumentation       string                 `json:"resource_documentation,omitempty"`
	Scopes                      []string               `json:"scopes,omitempty"`
	AuthorizationServerMetadata map[string]interface{} `json:"authorization_server_metadata,omitempty"`
}

// AuthorizationService handles MCP server authorization flows
type AuthorizationService struct {
	sessionStore session.SessionStore
	httpClient   *http.Client
	baseURL      string // Proxy base URL for callbacks
}

// NewAuthorizationService creates a new authorization service
func NewAuthorizationService(sessionStore session.SessionStore, baseURL string) *AuthorizationService {
	return &AuthorizationService{
		sessionStore: sessionStore,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// GetAuthorizationStatus handles GET /adapters/{name}/auth/status
func (as *AuthorizationService) GetAuthorizationStatus(c *gin.Context) {
	adapterName := c.Param("name")

	// Get active sessions for this adapter
	sessions, err := as.sessionStore.ListByAdapter(adapterName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve sessions"})
		return
	}

	// Find the most recent authorized session
	var authorizedSession *session.SessionDetails
	for _, session := range sessions {
		if session.AuthorizationInfo != nil && session.AuthorizationInfo.Status == "authorized" {
			if authorizedSession == nil || session.LastActivity.After(authorizedSession.LastActivity) {
				authorizedSession = &session
			}
		}
	}

	if authorizedSession == nil {
		c.JSON(http.StatusOK, gin.H{
			"adapterName": adapterName,
			"status":      "unauthorized",
			"message":     "No authorized sessions found",
		})
		return
	}

	response := gin.H{
		"adapterName":  adapterName,
		"status":       authorizedSession.AuthorizationInfo.Status,
		"sessionId":    authorizedSession.SessionID,
		"authorizedAt": authorizedSession.AuthorizationInfo.AuthorizedAt,
		"lastActivity": authorizedSession.LastActivity,
	}

	if authorizedSession.TokenInfo != nil {
		response["tokenExpiresAt"] = authorizedSession.TokenInfo.ExpiresAt
		response["tokenValid"] = as.sessionStore.IsTokenValid(authorizedSession.SessionID)
	}

	c.JSON(http.StatusOK, response)
}

// StartAuthorization handles POST /adapters/{name}/auth/authorize
func (as *AuthorizationService) StartAuthorization(c *gin.Context) {
	adapterName := c.Param("name")

	// Parse request body for client info
	var req struct {
		ClientInfo map[string]interface{} `json:"clientInfo,omitempty"`
		Resource   string                 `json:"resource,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// For now, return a placeholder response
	// In full implementation, this would:
	// 1. Discover OAuth metadata from the MCP server
	// 2. Register OAuth client if needed
	// 3. Generate PKCE challenge
	// 4. Return authorization URL for browser redirect

	sessionID := fmt.Sprintf("auth-session-%d", time.Now().Unix())

	// Create the session first
	err := as.sessionStore.SetWithDetails(sessionID, adapterName, "", "oauth")
	if err != nil {
		log.Printf("AuthorizationService: Failed to create session for adapter %s: %v", adapterName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create authorization session"})
		return
	}

	// Create authorization info
	authInfo := &session.AuthorizationInfo{
		Status:           "authorizing",
		AuthorizationURL: fmt.Sprintf("%s/oauth/authorize?session=%s", as.baseURL, sessionID),
		AuthorizedAt:     time.Now(),
	}

	// Store authorization state
	err = as.sessionStore.SetAuthorizationInfo(sessionID, authInfo)
	if err != nil {
		log.Printf("AuthorizationService: Failed to store auth info for adapter %s: %v", adapterName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start authorization"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"adapterName":      adapterName,
		"sessionId":        sessionID,
		"status":           "authorizing",
		"authorizationUrl": authInfo.AuthorizationURL,
		"message":          "Redirect user to authorization URL to complete OAuth flow",
	})
}

// RefreshToken handles POST /adapters/{name}/auth/refresh
func (as *AuthorizationService) RefreshToken(c *gin.Context) {
	name := c.Param("name")

	// Find authorized session for this adapter
	sessions, err := as.sessionStore.ListByAdapter(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve sessions"})
		return
	}

	var authorizedSession *session.SessionDetails
	for _, session := range sessions {
		if session.AuthorizationInfo != nil && session.AuthorizationInfo.Status == "authorized" {
			authorizedSession = &session
			break
		}
	}

	if authorizedSession == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No authorized session found"})
		return
	}

	// For now, return placeholder response
	// In full implementation, this would use the refresh token to get a new access token
	c.JSON(http.StatusOK, gin.H{
		"sessionId": authorizedSession.SessionID,
		"status":    "refreshed",
		"message":   "Token refresh completed",
	})
}

// RevokeTokens handles DELETE /adapters/{name}/auth/tokens
func (as *AuthorizationService) RevokeTokens(c *gin.Context) {
	name := c.Param("name")

	// Find all sessions for this adapter and clear token info
	sessions, err := as.sessionStore.ListByAdapter(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve sessions"})
		return
	}

	revokedCount := 0
	for _, session := range sessions {
		if session.TokenInfo != nil {
			// Clear token info
			err := as.sessionStore.SetTokenInfo(session.SessionID, nil)
			if err != nil {
				log.Printf("AuthorizationService: Failed to clear tokens for session %s: %v", session.SessionID, err)
				continue
			}
			revokedCount++
		}

		// Update authorization status
		if session.AuthorizationInfo != nil {
			authInfo := *session.AuthorizationInfo
			authInfo.Status = "unauthorized"
			as.sessionStore.SetAuthorizationInfo(session.SessionID, &authInfo)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"adapterName":   name,
		"tokensRevoked": revokedCount,
		"message":       "Tokens revoked successfully",
	})
}

// AuthorizeAdapter handles POST /adapters/{name}/auth/authorize
func (as *AuthorizationService) AuthorizeAdapter(c *gin.Context) {
	adapterName := c.Param("name")

	var req struct {
		ClientInfo map[string]interface{} `json:"clientInfo"`
		Resource   string                 `json:"resource"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// For now, return a placeholder authorization URL
	// In full implementation, this would:
	// 1. Discover OAuth server metadata
	// 2. Register client if needed
	// 3. Build authorization URL with PKCE

	authURL := fmt.Sprintf("%s/oauth/authorize?client_id=mcp-proxy&response_type=code&scope=read%%20write&state=%s&redirect_uri=%s/oauth/callback",
		as.baseURL, "test-state", as.baseURL)

	c.JSON(http.StatusOK, gin.H{
		"adapterName":      adapterName,
		"sessionId":        "auth-session-" + fmt.Sprintf("%d", time.Now().Unix()),
		"status":           "authorizing",
		"authorizationUrl": authURL,
		"message":          "Redirect user to authorization URL to complete OAuth flow",
	})
}

// ListSessions handles GET /adapters/{name}/sessions
func (as *AuthorizationService) ListSessions(c *gin.Context) {
	adapterName := c.Param("name")

	sessions, err := as.sessionStore.ListByAdapter(adapterName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"adapterName": adapterName,
		"sessions":    sessions,
	})
}

// CreateSession handles POST /adapters/{name}/sessions
func (as *AuthorizationService) CreateSession(c *gin.Context) {
	adapterName := c.Param("name")

	var req struct {
		ForceReinitialize bool                   `json:"forceReinitialize"`
		ClientInfo        map[string]interface{} `json:"clientInfo"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create new session
	sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
	sessionDetails := session.SessionDetails{
		SessionID:      sessionID,
		AdapterName:    adapterName,
		TargetAddress:  fmt.Sprintf("http://localhost:8001"), // placeholder
		ConnectionType: "StreamableHttp",
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
	}

	if err := as.sessionStore.SetWithDetails(sessionID, adapterName, sessionDetails.TargetAddress, string(sessionDetails.ConnectionType)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, sessionDetails)
}

// OAuthCallback handles GET /oauth/callback (for OAuth redirects)
func (as *AuthorizationService) OAuthCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")

	if errorParam != "" {
		errorDesc := c.Query("error_description")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             errorParam,
			"error_description": errorDesc,
		})
		return
	}

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "authorization_code_required"})
		return
	}

	// For now, return success
	// In full implementation, this would:
	// 1. Validate state parameter
	// 2. Exchange code for tokens
	// 3. Store tokens in session

	c.JSON(http.StatusOK, gin.H{
		"message": "Authorization completed",
		"code":    code,
		"state":   state,
	})
}

// Helper methods for OAuth discovery and flows

// discoverProtectedResourceMetadata fetches OAuth protected resource metadata
func (as *AuthorizationService) discoverProtectedResourceMetadata(resourceURL string) (*ProtectedResourceMetadata, error) {
	metadataURL := strings.TrimSuffix(resourceURL, "/") + "/.well-known/oauth-protected-resource"

	resp, err := as.httpClient.Get(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch protected resource metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("protected resource metadata returned status %d", resp.StatusCode)
	}

	var metadata ProtectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode protected resource metadata: %w", err)
	}

	return &metadata, nil
}

// discoverAuthorizationServerMetadata fetches OAuth authorization server metadata
func (as *AuthorizationService) discoverAuthorizationServerMetadata(authServerURL string) (*OAuthServerMetadata, error) {
	metadataURL := strings.TrimSuffix(authServerURL, "/") + "/.well-known/oauth-authorization-server"

	resp, err := as.httpClient.Get(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch authorization server metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authorization server metadata returned status %d", resp.StatusCode)
	}

	var metadata OAuthServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode authorization server metadata: %w", err)
	}

	return &metadata, nil
}

// registerOAuthClient performs dynamic client registration
func (as *AuthorizationService) registerOAuthClient(registrationURL string, clientInfo map[string]interface{}) (*OAuthClient, error) {
	clientData := map[string]interface{}{
		"redirect_uris":              []string{fmt.Sprintf("%s/oauth/callback", as.baseURL)},
		"client_name":                "SUSE AI Universal Proxy",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "client_secret_basic",
	}

	// Merge with provided client info
	for k, v := range clientInfo {
		clientData[k] = v
	}

	jsonData, err := json.Marshal(clientData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal client registration data: %w", err)
	}

	req, err := http.NewRequest("POST", registrationURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create registration request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := as.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to register client: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var client OAuthClient
	if err := json.NewDecoder(resp.Body).Decode(&client); err != nil {
		return nil, fmt.Errorf("failed to decode client registration response: %w", err)
	}

	return &client, nil
}

// generatePKCE generates PKCE challenge and verifier
func (as *AuthorizationService) generatePKCE() (string, string, error) {
	// Generate random verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate PKCE verifier: %w", err)
	}

	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Create challenge from verifier
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return verifier, challenge, nil
}

// buildAuthorizationURL builds the OAuth authorization URL
func (as *AuthorizationService) buildAuthorizationURL(authServer *OAuthServerMetadata, client *OAuthClient, resource string) (string, error) {
	authURL, err := url.Parse(authServer.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid authorization endpoint: %w", err)
	}

	params := url.Values{}
	params.Set("client_id", client.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", client.RedirectURIs[0])
	params.Set("scope", "openid profile") // Default scopes
	params.Set("state", fmt.Sprintf("state-%d", time.Now().Unix()))

	if resource != "" {
		params.Set("resource", resource)
	}

	// Add PKCE challenge
	verifier, challenge, err := as.generatePKCE()
	if err != nil {
		return "", err
	}
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")

	// In full implementation, store verifier for code exchange verification
	_ = verifier

	authURL.RawQuery = params.Encode()
	return authURL.String(), nil
}
