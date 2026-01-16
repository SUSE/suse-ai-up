package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/internal/config"
	"suse-ai-up/pkg/auth"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/scanner"
)

// RegistrationHandler handles registration of discovered MCP servers as adapters
type RegistrationHandler struct {
	scanner       *scanner.NetworkScanner
	adapterStore  clients.AdapterResourceStore
	tokenManager  *auth.TokenManager
	config        *config.Config
	toolDiscovery *mcp.MCPToolDiscoveryService
}

// NewRegistrationHandler creates a new registration handler
func NewRegistrationHandler(scanner *scanner.NetworkScanner, adapterStore clients.AdapterResourceStore, tokenManager *auth.TokenManager, config *config.Config) *RegistrationHandler {
	return &RegistrationHandler{
		scanner:       scanner,
		adapterStore:  adapterStore,
		tokenManager:  tokenManager,
		config:        config,
		toolDiscovery: mcp.NewMCPToolDiscoveryService(),
	}
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	DiscoveredServerID string `json:"discoveredServerId" binding:"required"`
}

// RegisterResponse represents a registration response
type RegisterResponse struct {
	Message      string                  `json:"message"`
	Adapter      *models.AdapterResource `json:"adapter"`
	McpEndpoint  string                  `json:"mcp_endpoint"`
	SecurityNote string                  `json:"security_note,omitempty"`
	TokenInfo    *auth.TokenInfo         `json:"token_info,omitempty"`
}

// RegisterDiscoveredServer handles POST /register
// @Summary Register a discovered MCP server as an adapter
// @Description Creates an adapter from a discovered MCP server with automatic security configuration
// @Tags registration, adapters
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Registration request"
// @Success 200 {object} RegisterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/discovery/register [post]
func (h *RegistrationHandler) RegisterDiscoveredServer(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Registration: Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, RegistrationErrorResponse{"Invalid request body", err.Error()})
		return
	}

	log.Printf("Registration: Request to register discovered server: %s", req.DiscoveredServerID)

	// Retrieve discovered server data
	discoveredServer := h.scanner.GetDiscoveredServer(req.DiscoveredServerID)
	if discoveredServer == nil {
		log.Printf("Registration: Discovered server not found: %s", req.DiscoveredServerID)
		c.JSON(http.StatusNotFound, RegistrationErrorResponse{"Discovered server not found", fmt.Sprintf("Server ID '%s' not found in discovery results", req.DiscoveredServerID)})
		return
	}

	log.Printf("Registration: Found server %s at %s with vulnerability score: %s",
		discoveredServer.Name, discoveredServer.Address, discoveredServer.VulnerabilityScore)

	// Create adapter configuration from discovered server
	adapterData := h.createAdapterDataFromDiscovered(discoveredServer)

	// Configure authentication based on vulnerability assessment
	tokenInfo, err := h.configureAuthentication(discoveredServer, adapterData)
	if err != nil {
		log.Printf("Registration: Failed to configure authentication: %v", err)
		c.JSON(http.StatusInternalServerError, RegistrationErrorResponse{"Failed to configure authentication", err.Error()})
		return
	}

	// Try to discover tools from the discovered server
	var discoveredTools []models.MCPTool
	if adapterData.ConnectionType == models.ConnectionTypeStreamableHttp {
		log.Printf("Registration: Attempting to discover tools from discovered server at %s", discoveredServer.Address)
		discoveredTools, err = h.toolDiscovery.DiscoverTools(c.Request.Context(), discoveredServer.Address, adapterData.Authentication)
		if err != nil {
			log.Printf("Registration: Failed to discover tools from discovered server, adapter will be created without tool information: %v", err)
			discoveredTools = []models.MCPTool{}
		} else {
			log.Printf("Registration: Successfully discovered %d tools from discovered server", len(discoveredTools))
			// Store discovered functionality in adapter data
			adapterData.MCPFunctionality = &models.MCPFunctionality{
				Tools:         discoveredTools,
				LastRefreshed: time.Now(),
			}
		}
	}

	// Create adapter resource
	adapter := &models.AdapterResource{}
	adapter.Create(*adapterData, "system", time.Now())

	// Store adapter
	if err := h.adapterStore.UpsertAsync(*adapter, c.Request.Context()); err != nil {
		log.Printf("Registration: Failed to store adapter: %v", err)
		c.JSON(http.StatusInternalServerError, RegistrationErrorResponse{"Failed to create adapter", err.Error()})
		return
	}

	log.Printf("Registration: Successfully created adapter '%s' for discovered server '%s'",
		adapter.ID, discoveredServer.ID)

	// Prepare response
	response := RegisterResponse{
		Message:      "Adapter created successfully",
		Adapter:      adapter,
		McpEndpoint:  fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", adapter.ID),
		SecurityNote: h.getSecurityNote(discoveredServer),
		TokenInfo:    tokenInfo,
	}

	c.JSON(http.StatusOK, response)
}

// createAdapterDataFromDiscovered creates adapter configuration from discovered server
func (h *RegistrationHandler) createAdapterDataFromDiscovered(server *models.DiscoveredServer) *models.AdapterData {
	// Generate adapter name from discovered server
	adapterName := h.generateAdapterName(server)

	// Check if this is a VirtualMCP server
	isVirtualMCP := false
	if server.Metadata != nil {
		if source, exists := server.Metadata["source"]; exists && source == "virtualmcp" {
			isVirtualMCP = true
		}
	}

	var adapterData *models.AdapterData

	if isVirtualMCP {
		// VirtualMCP servers should be deployed remotely and connected via HTTP
		adapterData = &models.AdapterData{
			Name:                 adapterName,
			Protocol:             models.ServerProtocolMCP,
			ConnectionType:       models.ConnectionTypeStreamableHttp,
			EnvironmentVariables: make(map[string]string),
			ReplicaCount:         1,
			Description:          fmt.Sprintf("VirtualMCP server: %s", server.Name),
			UseWorkloadIdentity:  false,
			RemoteUrl:            server.Address, // Will be updated when server is deployed
		}

		// VirtualMCP servers don't need MCPClientConfig - they connect remotely
		log.Printf("Registration: Created VirtualMCP adapter data for '%s'", adapterName)
	} else {
		// Extract connection type from discovered server
		connectionType := models.ConnectionTypeStreamableHttp // Default
		if server.Connection != "" {
			connectionType = models.ConnectionType(server.Connection)
		}

		// Create base adapter configuration for regular MCP servers
		adapterData = &models.AdapterData{
			Name:                 adapterName,
			ImageName:            "mcp-proxy", // Use proxy image for all discovered servers
			ImageVersion:         "1.0.0",
			Protocol:             models.ServerProtocolMCP,
			ConnectionType:       connectionType,
			EnvironmentVariables: make(map[string]string),
			ReplicaCount:         1,
			Description:          fmt.Sprintf("Auto-discovered MCP server at %s", server.Address),
			UseWorkloadIdentity:  false,
			RemoteUrl:            server.Address,
			Command:              "",
			Args:                 []string{},
		}

		// Set environment variables for proxy
		adapterData.EnvironmentVariables["MCP_PROXY_URL"] = server.Address
		if server.Metadata != nil {
			if authTypeStr, exists := server.Metadata["auth_type"]; exists {
				adapterData.EnvironmentVariables["MCP_SERVER_AUTH_TYPE"] = authTypeStr
			}
		}

		log.Printf("Registration: Created adapter data for '%s' with connection type: %s",
			adapterName, connectionType)
	}

	return adapterData
}

// configureAuthentication configures authentication based on vulnerability assessment
func (h *RegistrationHandler) configureAuthentication(server *models.DiscoveredServer, adapterData *models.AdapterData) (*auth.TokenInfo, error) {
	var tokenInfo *auth.TokenInfo
	var err error

	// ALWAYS require client authentication for security - proxy acts as gatekeeper
	log.Printf("Registration: Configuring client authentication for server %s (vulnerability: %s)",
		server.ID, server.VulnerabilityScore)

	if h.tokenManager != nil {
		// Generate secure token for adapter
		audience := fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s", adapterData.Name)
		tokenInfo, err = h.tokenManager.GenerateBearerToken(adapterData.Name, audience, 24)
		if err != nil {
			return nil, fmt.Errorf("failed to generate secure token: %w", err)
		}
	} else {
		// Fallback to simple token generation
		token := h.generateSecureToken()
		tokenInfo = &auth.TokenInfo{
			TokenID:     fmt.Sprintf("token-%d", time.Now().Unix()),
			AccessToken: token,
			TokenType:   "Bearer",
			Subject:     "adapter-" + adapterData.Name,
			IssuedAt:    time.Now(),
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			Audience:    fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s", adapterData.Name),
			Issuer:      "suse-ai-up",
		}
	}

	// Always require authentication from clients
	adapterData.Authentication = &models.AdapterAuthConfig{
		Required: true,
		Type:     "bearer",
		BearerToken: &models.BearerTokenConfig{
			Token:     tokenInfo.AccessToken,
			Dynamic:   h.tokenManager != nil,
			ExpiresAt: tokenInfo.ExpiresAt,
		},
	}

	// Set backend authentication flag based on server vulnerability
	switch server.VulnerabilityScore {
	case "high":
		// Backend has no auth - proxy handles all auth
		adapterData.EnvironmentVariables["MCP_BACKEND_AUTH_REQUIRED"] = "false"
		adapterData.Description += " [PROXY AUTH - BACKEND NO AUTH]"
	case "low":
		// Backend has auth - proxy may need to add backend auth
		adapterData.EnvironmentVariables["MCP_BACKEND_AUTH_REQUIRED"] = "true"
		adapterData.Description += " [PROXY AUTH - BACKEND HAS AUTH]"
	default:
		// Conservative approach
		adapterData.EnvironmentVariables["MCP_BACKEND_AUTH_REQUIRED"] = "true"
		adapterData.Description += " [PROXY AUTH - BACKEND UNKNOWN]"
	}

	return tokenInfo, nil
}

// generateAdapterName generates a unique adapter name from discovered server
func (h *RegistrationHandler) generateAdapterName(server *models.DiscoveredServer) string {
	// Use server name directly if available
	if server.Name != "" && server.Name != "Unknown MCP Server" {
		// Clean the name for use as an identifier
		return strings.ToLower(strings.ReplaceAll(server.Name, " ", "-"))
	}

	// Fallback to server ID or address-based name
	parts := strings.Split(server.Address, ":")
	if len(parts) >= 3 {
		// Format: http://host:port -> host-port
		host := strings.ReplaceAll(parts[1], ".", "-")
		port := parts[2]
		return fmt.Sprintf("%s-%s", host, port)
	}

	return server.ID
}

// generateSecureToken generates a cryptographically secure random token
func (h *RegistrationHandler) generateSecureToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		log.Printf("Registration: Failed to generate secure token, falling back to timestamp: %v", err)
		// Fallback to timestamp-based token
		return fmt.Sprintf("token-%d-%s", time.Now().Unix(), "fallback")
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

// getSecurityNote generates a security note based on vulnerability assessment
func (h *RegistrationHandler) getSecurityNote(server *models.DiscoveredServer) string {
	switch server.VulnerabilityScore {
	case "high":
		return "Adapter requires client authentication. Backend MCP server has no authentication - all security handled by proxy."
	case "medium":
		return "Adapter requires client authentication. Backend MCP server has optional authentication."
	case "low":
		return "Adapter requires client authentication. Backend MCP server has authentication - proxy provides additional security layer."
	default:
		return "Adapter requires client authentication. Backend MCP server security status unknown - conservative security applied."
	}
}

// RegistrationErrorResponse represents a registration error response
type RegistrationErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}
