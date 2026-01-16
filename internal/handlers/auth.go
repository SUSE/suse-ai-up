package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/models"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *auth.UserAuthService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authService *auth.UserAuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	UserID   string `json:"user_id"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token *models.AuthToken `json:"token"`
	User  *models.User      `json:"user"`
}

// OAuthLoginRequest represents an OAuth login initiation request
type OAuthLoginRequest struct {
	Provider string `json:"provider"` // "github" or "rancher"
}

// OAuthCallbackRequest represents an OAuth callback request
type OAuthCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Login handles POST /api/v1/auth/login
// @Summary User login
// @Description Authenticate a user with username/password
// @Tags auth
// @Accept json
// @Produce json
// @Param login body LoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.UserID == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user_id and password are required"})
		return
	}

	user, err := h.authService.AuthenticateUser(c.Request.Context(), req.UserID, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
		return
	}

	// Check if password change is required
	if h.authService.Config.Local != nil && h.authService.Config.Local.ForcePasswordChange {
		if user.PasswordChangedAt == nil || user.AuthProvider != string(models.UserAuthProviderLocal) {
			// Force password change for local users who haven't changed it
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Password change required"})
			return
		}
	}

	token, err := h.authService.GenerateJWT(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate token"})
		return
	}

	response := LoginResponse{
		Token: token,
		User:  user,
	}

	c.JSON(http.StatusOK, response)
}

// OAuthLogin initiates OAuth login flow
// @Summary Initiate OAuth login
// @Description Start OAuth authentication flow
// @Tags auth
// @Accept json
// @Produce json
// @Param oauth body OAuthLoginRequest true "OAuth provider"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/auth/oauth/login [post]
func (h *AuthHandler) OAuthLogin(c *gin.Context) {
	var req OAuthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	var authURL string
	var err error

	switch req.Provider {
	case "github":
		if h.authService.Config.GitHub == nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "GitHub auth not configured"})
			return
		}
		authURL, err = h.buildGitHubAuthURL()
	case "rancher":
		if h.authService.Config.Rancher == nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Rancher auth not configured"})
			return
		}
		authURL, err = h.buildRancherAuthURL()
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Unsupported provider"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to build auth URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"auth_url": authURL})
}

// OAuthCallback handles OAuth callback
// @Summary OAuth callback
// @Description Handle OAuth provider callback
// @Tags auth
// @Accept json
// @Produce json
// @Param callback body OAuthCallbackRequest true "OAuth callback data"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/auth/oauth/callback [post]
func (h *AuthHandler) OAuthCallback(c *gin.Context) {
	var req OAuthCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	provider := c.Query("provider")
	if provider == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provider not specified"})
		return
	}

	// Exchange code for token and get user info
	// This is a simplified version - in production, you'd validate state, exchange code, etc.
	var user *models.User
	var err error

	switch provider {
	case "github":
		user, err = h.handleGitHubCallback(req.Code)
	case "rancher":
		user, err = h.handleRancherCallback(req.Code)
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Unsupported provider"})
		return
	}

	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Authentication failed: " + err.Error()})
		return
	}

	token, err := h.authService.GenerateJWT(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate token"})
		return
	}

	response := LoginResponse{
		Token: token,
		User:  user,
	}

	c.JSON(http.StatusOK, response)
}

// ChangePassword handles password change
// @Summary Change password
// @Description Change the current user's password
// @Tags auth
// @Accept json
// @Produce json
// @Param password body ChangePasswordRequest true "Password change data"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/auth/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	user, ok := userInterface.(*models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Invalid user context"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "New password is required"})
		return
	}

	// For local users, verify current password
	if user.AuthProvider == string(models.UserAuthProviderLocal) {
		if !h.authService.ValidatePassword(req.CurrentPassword, user.PasswordHash) {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Current password is incorrect"})
			return
		}
	}

	// Hash new password
	hash, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to hash password"})
		return
	}

	// Update user
	now := time.Now().UTC()
	user.PasswordHash = hash
	user.PasswordChangedAt = &now
	user.UpdatedAt = now

	err = h.authService.UserStore.Update(c.Request.Context(), *user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

// Logout handles logout (client-side token removal)
// @Summary Logout
// @Description Logout the current user
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	// In a stateless JWT system, logout is handled client-side by removing the token
	// In the future, you could implement token blacklisting here
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// GetAuthMode returns the current authentication mode and configuration
// @Summary Get authentication mode
// @Description Returns the current authentication configuration (unauthenticated endpoint)
// @Tags auth
// @Produce json
// @Success 200 {object} AuthModeResponse
// @Router /auth/mode [get]
func (h *AuthHandler) GetAuthMode(c *gin.Context) {
	config := h.authService.Config

	response := AuthModeResponse{
		Mode:    config.Mode,
		DevMode: config.DevMode,
	}

	// Only include sensitive information if not in production mode
	// In production, you might want to limit what information is exposed
	if config.Local != nil {
		response.Local = &struct {
			DefaultAdminPassword string `json:"default_admin_password,omitempty"`
			ForcePasswordChange  bool   `json:"force_password_change"`
			PasswordMinLength    int    `json:"password_min_length"`
		}{
			DefaultAdminPassword: config.Local.DefaultAdminPassword,
			ForcePasswordChange:  config.Local.ForcePasswordChange,
			PasswordMinLength:    config.Local.PasswordMinLength,
		}
	}

	if config.GitHub != nil {
		response.GitHub = &struct {
			ClientID    string   `json:"client_id,omitempty"`
			RedirectURI string   `json:"redirect_uri,omitempty"`
			AllowedOrgs []string `json:"allowed_orgs,omitempty"`
			AdminTeams  []string `json:"admin_teams,omitempty"`
		}{
			ClientID:    config.GitHub.ClientID,
			RedirectURI: config.GitHub.RedirectURI,
			AllowedOrgs: config.GitHub.AllowedOrgs,
			AdminTeams:  config.GitHub.AdminTeams,
		}
	}

	if config.Rancher != nil {
		response.Rancher = &struct {
			IssuerURL     string   `json:"issuer_url,omitempty"`
			ClientID      string   `json:"client_id,omitempty"`
			RedirectURI   string   `json:"redirect_uri,omitempty"`
			AdminGroups   []string `json:"admin_groups,omitempty"`
			FallbackLocal bool     `json:"fallback_local"`
		}{
			IssuerURL:     config.Rancher.IssuerURL,
			ClientID:      config.Rancher.ClientID,
			RedirectURI:   config.Rancher.RedirectURI,
			AdminGroups:   config.Rancher.AdminGroups,
			FallbackLocal: config.Rancher.FallbackLocal,
		}
	}

	c.JSON(http.StatusOK, response)
}

// buildGitHubAuthURL builds GitHub OAuth authorization URL
func (h *AuthHandler) buildGitHubAuthURL() (string, error) {
	config := h.authService.Config.GitHub
	if config == nil {
		return "", fmt.Errorf("GitHub config not found")
	}

	baseURL := "https://github.com/login/oauth/authorize"
	params := fmt.Sprintf("?client_id=%s&redirect_uri=%s&scope=user:email,read:org",
		config.ClientID, config.RedirectURI)

	return baseURL + params, nil
}

// buildRancherAuthURL builds Rancher OIDC authorization URL
func (h *AuthHandler) buildRancherAuthURL() (string, error) {
	config := h.authService.Config.Rancher
	if config == nil {
		return "", fmt.Errorf("Rancher config not found")
	}

	baseURL := fmt.Sprintf("%s/oauth2/authorize", config.IssuerURL)
	params := fmt.Sprintf("?client_id=%s&redirect_uri=%s&response_type=code&scope=openid profile email groups",
		config.ClientID, config.RedirectURI)

	return baseURL + params, nil
}

// handleGitHubCallback processes GitHub OAuth callback
func (h *AuthHandler) handleGitHubCallback(code string) (*models.User, error) {
	config := h.authService.Config.GitHub
	if config == nil {
		return nil, fmt.Errorf("GitHub auth not configured")
	}

	// Exchange code for access token
	tokenURL := "https://github.com/login/oauth/access_token"
	data := fmt.Sprintf("client_id=%s&client_secret=%s&code=%s",
		config.ClientID, config.ClientSecret, code)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("failed to obtain access token")
	}

	// Fetch user info from GitHub API
	userInfo, groups, err := h.fetchGitHubUserInfo(tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	user, err := h.authService.AuthenticateExternalUser(
		context.Background(),
		models.UserAuthProviderGitHub,
		userInfo.ID,
		userInfo.Email,
		userInfo.Name,
		groups,
	)

	return user, err
}

// fetchGitHubUserInfo fetches user information from GitHub API
func (h *AuthHandler) fetchGitHubUserInfo(accessToken string) (*GitHubUserInfo, []string, error) {
	// Fetch user profile
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "SUSE-AI-Uniproxy")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var userInfo GitHubUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, nil, err
	}

	// Fetch user organizations
	orgsReq, err := http.NewRequest("GET", "https://api.github.com/user/orgs", nil)
	if err != nil {
		return nil, nil, err
	}
	orgsReq.Header.Set("Authorization", "Bearer "+accessToken)
	orgsReq.Header.Set("User-Agent", "SUSE-AI-Uniproxy")

	orgsResp, err := client.Do(orgsReq)
	if err != nil {
		return nil, nil, err
	}
	defer orgsResp.Body.Close()

	var orgs []GitHubOrg
	if err := json.NewDecoder(orgsResp.Body).Decode(&orgs); err != nil {
		return nil, nil, err
	}

	groups := make([]string, len(orgs))
	for i, org := range orgs {
		groups[i] = org.Login
	}

	return &userInfo, groups, nil
}

// GitHubUserInfo represents GitHub user information
type GitHubUserInfo struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GitHubOrg represents a GitHub organization
type GitHubOrg struct {
	Login string `json:"login"`
}

// handleRancherCallback processes Rancher OIDC callback
func (h *AuthHandler) handleRancherCallback(code string) (*models.User, error) {
	config := h.authService.Config.Rancher
	if config == nil {
		return nil, fmt.Errorf("Rancher auth not configured")
	}

	// Exchange code for tokens using Rancher OIDC
	tokenURL := fmt.Sprintf("%s/oauth2/token", config.IssuerURL)
	data := fmt.Sprintf("grant_type=authorization_code&client_id=%s&client_secret=%s&code=%s&redirect_uri=%s",
		config.ClientID, config.ClientSecret, code, config.RedirectURI)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("failed to obtain access token")
	}

	// Decode ID token to get user info (simplified - in production validate JWT)
	// For now, we'll assume the ID token contains the user info
	userInfo, groups, err := h.parseRancherIDToken(tokenResp.IDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}

	user, err := h.authService.AuthenticateExternalUser(
		context.Background(),
		models.UserAuthProviderRancher,
		userInfo.ID,
		userInfo.Email,
		userInfo.Name,
		groups,
	)

	return user, err
}

// parseRancherIDToken parses Rancher OIDC ID token (simplified implementation)
func (h *AuthHandler) parseRancherIDToken(idToken string) (*RancherUserInfo, []string, error) {
	// In production, properly validate and decode the JWT
	// For now, return mock data based on token
	userID := "rancher_" + strings.ReplaceAll(idToken[:8], "-", "_")

	userInfo := &RancherUserInfo{
		ID:    userID,
		Name:  fmt.Sprintf("Rancher User %s", userID),
		Email: fmt.Sprintf("user+%s@rancher.local", userID),
	}

	// Use configured admin groups
	groups := h.authService.Config.Rancher.AdminGroups

	return userInfo, groups, nil
}

// RancherUserInfo represents Rancher user information
type RancherUserInfo struct {
	ID    string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// AuthModeResponse represents the authentication mode response
type AuthModeResponse struct {
	Mode    string `json:"mode"`
	DevMode bool   `json:"dev_mode"`
	Local   *struct {
		DefaultAdminPassword string `json:"default_admin_password,omitempty"`
		ForcePasswordChange  bool   `json:"force_password_change"`
		PasswordMinLength    int    `json:"password_min_length"`
	} `json:"local,omitempty"`
	GitHub *struct {
		ClientID    string   `json:"client_id,omitempty"`
		RedirectURI string   `json:"redirect_uri,omitempty"`
		AllowedOrgs []string `json:"allowed_orgs,omitempty"`
		AdminTeams  []string `json:"admin_teams,omitempty"`
	} `json:"github,omitempty"`
	Rancher *struct {
		IssuerURL     string   `json:"issuer_url,omitempty"`
		ClientID      string   `json:"client_id,omitempty"`
		RedirectURI   string   `json:"redirect_uri,omitempty"`
		AdminGroups   []string `json:"admin_groups,omitempty"`
		FallbackLocal bool     `json:"fallback_local"`
	} `json:"rancher,omitempty"`
}
