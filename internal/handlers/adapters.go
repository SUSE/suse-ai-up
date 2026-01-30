package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/logging"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/services"
	adaptersvc "suse-ai-up/pkg/services/adapters"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// AdapterHandler handles adapter management requests
type AdapterHandler struct {
	adapterService   *adaptersvc.AdapterService
	userGroupService *services.UserGroupService
}

// NewAdapterHandler creates a new adapter handler
func NewAdapterHandler(adapterService *adaptersvc.AdapterService, userGroupService *services.UserGroupService) *AdapterHandler {
	return &AdapterHandler{
		adapterService:   adapterService,
		userGroupService: userGroupService,
	}
}

// CreateAdapterRequest represents a request to create an adapter
type CreateAdapterRequest struct {
	MCPServerID          string                    `json:"mcpServerId"`
	Name                 string                    `json:"name"`
	Description          string                    `json:"description"`
	EnvironmentVariables map[string]string         `json:"environmentVariables"`
	Authentication       *models.AdapterAuthConfig `json:"authentication"`
	DeploymentMethod     string                    `json:"deploymentMethod,omitempty"` // "helm", "docker", "systemd", "local"
}

// AddAdapterToGroupRequest represents a request to add an adapter to a group
type AddAdapterToGroupRequest struct {
	GroupID    string `json:"groupId" example:"mcp-users"`
	Permission string `json:"permission" example:"read"` // "read"
}

// CreateAdapterResponse represents the response for adapter creation
type CreateAdapterResponse struct {
	ID              string                   `json:"id"`
	MCPServerID     string                   `json:"mcpServerId"`
	MCPClientConfig map[string]interface{}   `json:"mcpClientConfig"`
	Capabilities    *models.MCPFunctionality `json:"capabilities"`
	Status          string                   `json:"status"`
	CreatedAt       time.Time                `json:"createdAt"`
}

// ListAdapterResponse represents an adapter in the list response
type ListAdapterResponse struct {
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Description     string                   `json:"description,omitempty"`
	URL             string                   `json:"url"`
	MCPClientConfig map[string]interface{}   `json:"mcpClientConfig"`
	Capabilities    *models.MCPFunctionality `json:"capabilities,omitempty"`
	Status          string                   `json:"status"`
	CreatedAt       time.Time                `json:"createdAt"`
	LastUpdatedAt   time.Time                `json:"lastUpdatedAt"`
	CreatedBy       string                   `json:"createdBy"`
	ConnectionType  models.ConnectionType    `json:"connectionType"`
}

// parseTrentoConfig parses TRENTO_CONFIG format: "TRENTO_URL={url},TOKEN={pat}"
func parseTrentoConfig(config string) (trentoURL, token string, err error) {
	if config == "" {
		return "", "", fmt.Errorf("TRENTO_CONFIG cannot be empty")
	}

	// Parse format: TRENTO_URL={url},TOKEN={pat}
	parts := strings.Split(config, ",")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid TRENTO_CONFIG format, expected 'TRENTO_URL={url},TOKEN={pat}'")
	}

	var urlPart, tokenPart string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "TRENTO_URL=") {
			urlPart = strings.TrimPrefix(part, "TRENTO_URL=")
		} else if strings.HasPrefix(part, "TOKEN=") {
			tokenPart = strings.TrimPrefix(part, "TOKEN=")
		}
	}

	if urlPart == "" {
		return "", "", fmt.Errorf("TRENTO_URL not found in TRENTO_CONFIG")
	}
	if tokenPart == "" {
		return "", "", fmt.Errorf("TOKEN not found in TRENTO_CONFIG")
	}

	return urlPart, tokenPart, nil
}

// generateClientConfig creates the client-specific configuration for a given adapter
func (h *AdapterHandler) generateClientConfig(adapter *models.AdapterResource, includeSecrets bool) map[string]interface{} {
	clientConfig := make(map[string]interface{})

	// Retrieve the stored MCPClientConfig from the adapter resource
	// This contains the internal representation with real URLs and tokens
	internalMCPClientConfig := adapter.MCPClientConfig

	logging.AdapterLogger.Info("generateClientConfig: adapter=%s, includeSecrets=%v, servers=%d", adapter.Name, includeSecrets, len(internalMCPClientConfig.MCPServers))

	// Assuming a single server entry per adapter for simplicity in current structure
	var serverConfig models.MCPServerConfig
	foundConfig := false
	if config, ok := internalMCPClientConfig.MCPServers[adapter.Name]; ok {
		serverConfig = config
		foundConfig = true
	} else {
		// Fallback: if adapter.Name doesn't match a key, try to find any config
		for _, cfg := range internalMCPClientConfig.MCPServers {
			serverConfig = cfg
			foundConfig = true
			break
		}
	}

	if !foundConfig || (serverConfig.URL == "" && serverConfig.Command == "") {
		logging.AdapterLogger.Info("generateClientConfig: no valid config found for adapter %s", adapter.Name)
		return clientConfig // Return empty if no valid server config found
	}

	// Prepare headers
	headers := make(map[string]string)
	if includeSecrets {
		// Use real headers from stored config
		for k, v := range serverConfig.Headers {
			headers[k] = v
		}
	} else {
		// Use placeholder for security
		headers["Authorization"] = "Bearer adapter-session-token"
	}

	// Gemini Configuration
	if serverConfig.URL != "" {
		geminiServerConfig := map[string]interface{}{
			"httpUrl": serverConfig.URL, // Use the URL from the stored config
			"headers": headers,
		}
		clientConfig["gemini"] = map[string]interface{}{
			"mcpServers": map[string]interface{}{
				adapter.Name: geminiServerConfig,
			},
		}
	}

	// VSCode Configuration
	// VSCode client often uses "url" field instead of "httpUrl"
	if serverConfig.URL != "" {
		vscodeServerConfig := map[string]interface{}{
			"url":     serverConfig.URL, // Use the URL from the stored config
			"headers": headers,
			"type":    "http", // Assuming HTTP connection type for VSCode client
		}
		clientConfig["vscode"] = map[string]interface{}{
			"servers": map[string]interface{}{
				adapter.Name: vscodeServerConfig,
			},
			"inputs": []interface{}{}, // VSCode clients might have inputs, but not part of adapter config directly
		}
	}

	return clientConfig
}

// HandleAdapters handles both listing and creating adapters
func (h *AdapterHandler) HandleAdapters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListAdapters(w, r)
	case http.MethodPost:
		h.CreateAdapter(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// CreateAdapter creates a new adapter from a registry server
// @Summary Create adapter
// @Description Create a new MCP adapter
// @Tags adapters
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param request body CreateAdapterRequest true "Adapter creation request"
// @Success 201 {object} CreateAdapterResponse "Created adapter"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters [post]
func (h *AdapterHandler) CreateAdapter(w http.ResponseWriter, r *http.Request) {
	logging.AdapterLogger.Info("CreateAdapter handler invoked")

	var req CreateAdapterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.AdapterLogger.Error("Failed to decode JSON: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	logging.AdapterLogger.Info("Decoded request: mcpServerId=%s, name=%s", req.MCPServerID, req.Name)

	// Basic validation
	if req.MCPServerID == "" || req.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "mcpServerId and name are required"})
		return
	}

	// Get user ID from header (would be set by auth middleware)
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user" // For development
	}

	// Handle Trento-specific configuration
	if req.MCPServerID == "suse-trento" {
		if trentoConfig, exists := req.EnvironmentVariables["TRENTO_CONFIG"]; exists && trentoConfig != "" {
			// Parse Trento configuration
			trentoURL, token, err := parseTrentoConfig(trentoConfig)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid TRENTO_CONFIG format: " + err.Error()})
				return
			}

			// Set up proper environment variables for Trento
			req.EnvironmentVariables["TRENTO_URL"] = trentoURL
			delete(req.EnvironmentVariables, "TRENTO_CONFIG") // Remove the combined config

			// Set up authentication with Trento PAT
			if req.Authentication == nil {
				req.Authentication = &models.AdapterAuthConfig{}
			}
			req.Authentication.Type = "bearer"
			req.Authentication.BearerToken = &models.BearerTokenConfig{
				Token:   token,
				Dynamic: false, // Static token for Trento PAT
			}
		}
	}

	// Create the adapter
	adapter, err := h.adapterService.CreateAdapter(
		r.Context(),
		userID,
		req.MCPServerID,
		req.Name,
		req.EnvironmentVariables,
		req.Authentication,
		h.userGroupService,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create adapter: " + err.Error()})
		return
	}

	// Generate MCP client configurations for different client types
	// Use includeSecrets=false to return placeholder tokens for creation response
	adapterClientConfig := h.generateClientConfig(adapter, false)

	response := CreateAdapterResponse{
		ID:              adapter.ID,
		MCPServerID:     req.MCPServerID,
		MCPClientConfig: adapterClientConfig, // Use the generated config
		Capabilities:    adapter.MCPFunctionality,
		Status:          "ready",
		CreatedAt:       adapter.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListAdapters lists all adapters for the current user
// @Summary List adapters
// @Description List all adapters for the current user
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Success 200 {array} models.AdapterResource "List of adapters"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters [get]
func (h *AdapterHandler) ListAdapters(w http.ResponseWriter, r *http.Request) {
	logging.AdapterLogger.Info("ListAdapters handler invoked")
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	adapters, err := h.adapterService.ListAdapters(r.Context(), userID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list adapters: " + err.Error()})
		return
	}

	// Transform adapters to include multi-format MCP client configurations
	listAdapters := make([]map[string]interface{}, len(adapters))
	for i, adapter := range adapters {
		// Generate MCP client configurations for different client types
		// Use includeSecrets=false to return placeholder tokens for list view
		adapterClientConfig := h.generateClientConfig(&adapter, false)

		adapterMap := map[string]interface{}{
			"id":              adapter.ID,
			"name":            adapter.Name,
			"description":     adapter.Description,
			"url":             adapter.URL,         // This is the proxy URL
			"mcpClientConfig": adapterClientConfig, // Use the generated config
			"capabilities":    adapter.MCPFunctionality,
			"status":          adapter.Status,
			"createdAt":       adapter.CreatedAt,
			"lastUpdatedAt":   adapter.LastUpdatedAt,
			"createdBy":       adapter.CreatedBy,
			"connectionType":  adapter.ConnectionType,
		}
		listAdapters[i] = adapterMap
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listAdapters)
}

// GetAdapter gets a specific adapter by ID
// @Summary Get adapter details
// @Description Retrieve details of a specific adapter
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 200 {object} models.AdapterResource "Adapter details"
// @Failure 404 {object} ErrorResponse "Adapter not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name} [get]
func (h *AdapterHandler) GetAdapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	adapterID := strings.Split(path, "/")[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	adapter, err := h.adapterService.GetAdapter(r.Context(), userID, adapterID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "adapter not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to get adapter: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adapter)
}

// CheckAdapterHealth checks and updates the health status of an adapter
// @Summary Check adapter health
// @Description Check the health of an adapter's sidecar and update its status
// @Tags adapters
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter name"
// @Success 200 {object} map[string]string
// UpdateAdapter updates an existing adapter
// @Summary Update adapter
// @Description Update an existing adapter's configuration
// @Tags adapters
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter name"
// @Param adapter body models.AdapterData true "Updated adapter data"
// @Success 200 {object} models.AdapterResource
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name} [put]
func (h *AdapterHandler) UpdateAdapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	adapterID := strings.Split(path, "/")[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	// Get current adapter
	currentAdapter, err := h.adapterService.GetAdapter(r.Context(), userID, adapterID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "adapter not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to get adapter: " + err.Error()})
		}
		return
	}

	// Parse request body
	var updateAdapter models.AdapterResource
	if err := json.NewDecoder(r.Body).Decode(&updateAdapter); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Preserve system fields
	updateAdapter.ID = currentAdapter.ID
	updateAdapter.CreatedBy = currentAdapter.CreatedBy
	updateAdapter.CreatedAt = currentAdapter.CreatedAt
	updateAdapter.LastUpdatedAt = time.Now().UTC()

	// Update adapter
	if err := h.adapterService.UpdateAdapter(r.Context(), userID, updateAdapter, h.userGroupService); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update adapter: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updateAdapter)
}

// CheckAdapterHealth checks and updates the health status of an adapter
// @Summary Check adapter health
// @Description Check the health of an adapter's sidecar and update its status
// @Tags adapters
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter name"
// @Success 200 {object} map[string]string
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/health [post]
func (h *AdapterHandler) CheckAdapterHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	pathParts := strings.Split(path, "/")
	adapterID := pathParts[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	// Check adapter health
	if err := h.adapterService.CheckAdapterHealth(r.Context(), userID, adapterID, h.userGroupService); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to check adapter health: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message":   "Adapter health check completed",
		"adapterId": adapterID,
	})
}

// DeleteAdapter deletes an adapter and its associated sidecar resources
// @Summary Delete adapter
// @Description Delete an adapter and clean up its associated resources
// @Tags adapters
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse "Adapter not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name} [delete]
func (h *AdapterHandler) DeleteAdapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	adapterID := strings.Split(path, "/")[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	// Note: Sidecar cleanup is handled automatically by the adapter service

	// Delete the adapter
	if err := h.adapterService.DeleteAdapter(r.Context(), userID, adapterID, h.userGroupService); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "adapter not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to delete adapter: " + err.Error()})
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleMCPProtocol proxies MCP protocol requests to the sidecar
// @Summary Proxy MCP protocol requests
// @Description Proxy MCP protocol requests (tools, resources, prompts) to the adapter
// @Tags adapters,mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 200 {object} map[string]interface{} "MCP response"
// @Failure 404 {object} ErrorResponse "Adapter not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/mcp [post]
func (h *AdapterHandler) HandleMCPProtocol(w http.ResponseWriter, r *http.Request) {
	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "mcp" {
		http.NotFound(w, r)
		return
	}

	adapterID := parts[0]

	// Get user ID from header (would be set by auth middleware)
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user" // For development
	}

	// Get adapter information
	adapter, err := h.adapterService.GetAdapter(r.Context(), userID, adapterID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		return
	}

	// For sidecar adapters (StreamableHttp with sidecar config), proxy to the sidecar
	if adapter.ConnectionType == models.ConnectionTypeStreamableHttp && adapter.SidecarConfig != nil {
		// Construct sidecar URL dynamically using the port from sidecar config
		// Sidecar runs in suse-ai-up-mcp namespace with name mcp-sidecar-{adapterID}
		port := 8000 // default
		if adapter.SidecarConfig != nil {
			port = adapter.SidecarConfig.Port
		}
		// For HTTP transport MCP servers, use internal DNS
		sidecarURL := fmt.Sprintf("http://mcp-sidecar-%s.suse-ai-up-mcp.svc.cluster.local:%d/mcp", adapterID, port)
		h.proxyToSidecar(w, r, sidecarURL)
		return
	}

	// For LocalStdio adapters OR StreamableHttp adapters without sidecar config, return a proper MCP response
	fmt.Printf("DEBUG: Adapter %s - ConnectionType: %s, SidecarConfig: %v\n", adapterID, adapter.ConnectionType, adapter.SidecarConfig)
	if adapter.ConnectionType == models.ConnectionTypeLocalStdio ||
		(adapter.ConnectionType == models.ConnectionTypeStreamableHttp && adapter.SidecarConfig == nil) {
		fmt.Printf("DEBUG: Returning MCP response for LocalStdio adapter %s\n", adapterID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"serverInfo": map[string]interface{}{
					"name":    adapter.Name,
					"version": "1.0.0",
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						"listChanged": true,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// For RemoteHttp adapters, proxy to the remote MCP server
	if adapter.ConnectionType == models.ConnectionTypeRemoteHttp {
		if adapter.RemoteUrl == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Remote URL not configured for adapter"})
			return
		}
		h.proxyToRemoteMCP(w, r, adapter.RemoteUrl)
		return
	}

	// For other connection types, return not implemented
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(ErrorResponse{Error: "MCP protocol not supported for this adapter type"})
}

// proxyToRemoteMCP proxies requests to a remote MCP server
func (h *AdapterHandler) proxyToRemoteMCP(w http.ResponseWriter, r *http.Request, remoteURL string) {
	fmt.Printf("DEBUG: Proxying MCP request to remote server: %s\n", remoteURL)

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid adapter path"})
		return
	}
	adapterID := parts[0]

	// Get user ID from header
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user" // For development
	}

	// Get adapter information to access environment variables
	adapter, err := h.adapterService.GetAdapter(r.Context(), userID, adapterID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		return
	}

	// Create a new request to the remote MCP server
	remoteReq, err := http.NewRequestWithContext(r.Context(), r.Method, remoteURL, r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create remote request"})
		return
	}

	// Copy headers from the original request, but replace authorization
	for key, values := range r.Header {
		if strings.ToLower(key) == "authorization" {
			// For GitHub, use the personal access token from environment variables
			if token := adapter.EnvironmentVariables["GITHUB_PERSONAL_ACCESS_TOKEN"]; token != "" {
				remoteReq.Header.Set("Authorization", "Bearer "+token)
			} else if token := adapter.EnvironmentVariables["GITHUB_ACCESS_TOKEN"]; token != "" {
				remoteReq.Header.Set("Authorization", "Bearer "+token)
			}
			// Skip the original authorization header
		} else {
			for _, value := range values {
				remoteReq.Header.Add(key, value)
			}
		}
	}

	// Ensure we have the proper content type for MCP
	if remoteReq.Header.Get("Content-Type") == "" {
		remoteReq.Header.Set("Content-Type", "application/json")
	}

	// Make the request to the remote MCP server
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(remoteReq)
	if err != nil {
		fmt.Printf("DEBUG: Failed to connect to remote MCP server %s: %v\n", remoteURL, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to connect to remote MCP server"})
		return
	}
	defer resp.Body.Close()

	fmt.Printf("DEBUG: Remote MCP server responded with status: %d\n", resp.StatusCode)

	// Copy the response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set the status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body
	io.Copy(w, resp.Body)
}

// proxyToSidecar proxies requests to the sidecar container
func (h *AdapterHandler) proxyToSidecar(w http.ResponseWriter, r *http.Request, sidecarURL string) {

	fmt.Printf("DEBUG: Request headers: %+v\n", r.Header)

	// Extract adapter ID from the request path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/"), "/")
	adapterID := pathParts[0]

	// Create a new request to the sidecar
	sidecarReq, err := http.NewRequestWithContext(r.Context(), r.Method, sidecarURL, r.Body)
	if err != nil {
		fmt.Printf("DEBUG: Failed to create sidecar request: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create sidecar request"})
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			sidecarReq.Header.Add(key, value)
		}
	}

	// Ensure Accept header includes required types for MCP HTTP transport
	if sidecarReq.Header.Get("Accept") == "" {
		sidecarReq.Header.Set("Accept", "application/json, text/event-stream")
	}

	// Set Host header to localhost for MCP servers that may check host
	sidecarReq.Host = "localhost"

	// Set Content-Type if not already set
	if sidecarReq.Header.Get("Content-Type") == "" {
		sidecarReq.Header.Set("Content-Type", "application/json")
	}

	// Make the request to the sidecar
	client := &http.Client{
		Timeout: 30 * time.Second,
		// Don't follow redirects to avoid exposing internal URLs
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(sidecarReq)
	if err != nil {
		fmt.Printf("DEBUG: Failed to connect to sidecar: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "UNIQUE_ERROR: Failed to connect to sidecar: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	fmt.Printf("DEBUG: Sidecar response status: %d, location: %s\n", resp.StatusCode, resp.Header.Get("Location"))

	// If it's a redirect, don't pass it through to avoid exposing internal URLs
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		fmt.Printf("DEBUG: Blocking redirect response to avoid exposing internal URLs\n")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Sidecar returned redirect - internal routing issue"})
		return
	}

	// Copy response headers (but filter out location headers for redirects)
	for key, values := range resp.Header {
		if strings.ToLower(key) != "location" { // Don't pass through redirect locations
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Read and potentially rewrite the response body
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		// For JSON responses, rewrite any sidecar URLs to proxy URLs
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("DEBUG: Failed to read response body: %v\n", err)
			return
		}

		// Rewrite URLs in the response
		rewrittenBody := h.rewriteSidecarURLs(string(bodyBytes), adapterID)
		w.Write([]byte(rewrittenBody))
	} else {
		// For non-JSON responses, copy directly
		io.Copy(w, resp.Body)
	}
}

// rewriteSidecarURLs rewrites any sidecar URLs in the response to proxy URLs
func (h *AdapterHandler) rewriteSidecarURLs(responseBody, adapterID string) string {
	// Construct the sidecar base URL pattern
	sidecarBaseURL := fmt.Sprintf("http://mcp-sidecar-%s.suse-ai-up-mcp.svc.cluster.local", adapterID)

	// Replace sidecar URLs with proxy URLs
	proxyBaseURL := fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s", adapterID)

	// Replace any occurrences of sidecar URLs with proxy URLs
	rewritten := strings.ReplaceAll(responseBody, sidecarBaseURL, proxyBaseURL)

	if rewritten != responseBody {
		fmt.Printf("DEBUG: Rewrote sidecar URLs in response\n")
	}

	return rewritten
}

// SyncAdapterCapabilities syncs capabilities for an adapter
// @Summary Sync adapter capabilities
// @Description Synchronize and refresh the capabilities of an adapter
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter ID"
// @Success 200 {object} map[string]string "Sync result"
// @Failure 404 {object} ErrorResponse "Adapter not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters/{name}/sync [post]
func (h *AdapterHandler) SyncAdapterCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "sync" {
		http.NotFound(w, r)
		return
	}
	adapterID := parts[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	if err := h.adapterService.SyncAdapterCapabilities(r.Context(), userID, adapterID, h.userGroupService); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "adapter not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to sync capabilities: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "capabilities_synced",
		"message": "Adapter capabilities have been synchronized",
	})
}

// AssignAdapterToGroup assigns an adapter to a group
// @Summary Assign adapter to group
// @Description Assign an adapter to a specific group with permissions
// @Tags adapters
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter Name"
// @Param request body AddAdapterToGroupRequest true "Assignment details"
// @Success 201 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/groups [post]
func (h *AdapterHandler) AssignAdapterToGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "groups" {
		http.NotFound(w, r)
		return
	}
	adapterID := parts[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	var req AddAdapterToGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.GroupID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "groupId is required"})
		return
	}

	permission := req.Permission
	if permission == "" {
		permission = "read"
	}

	if err := h.adapterService.AssignAdapterToGroup(r.Context(), userID, adapterID, req.GroupID, permission, h.userGroupService); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Resource not found: " + err.Error()})
		} else if strings.Contains(err.Error(), "permissions") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		} else if strings.Contains(err.Error(), "already exists") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to assign adapter: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "assigned",
		"message": fmt.Sprintf("Adapter %s assigned to group %s", adapterID, req.GroupID),
	})
}

// RemoveAdapterFromGroup removes an adapter from a group
// @Summary Remove adapter from group
// @Description Remove an adapter assignment from a specific group
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter Name"
// @Param groupId path string true "Group ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/groups/{groupId} [delete]
func (h *AdapterHandler) RemoveAdapterFromGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID and group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "groups" {
		http.NotFound(w, r)
		return
	}
	adapterID := parts[0]
	groupID := parts[2]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	if err := h.adapterService.RemoveAdapterFromGroup(r.Context(), userID, adapterID, groupID, h.userGroupService); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Resource not found: " + err.Error()})
		} else if strings.Contains(err.Error(), "permissions") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to remove assignment: " + err.Error()})
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListAdapterGroupAssignments lists all group assignments for an adapter
// @Summary List adapter group assignments
// @Description List all groups that have access to this adapter
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Adapter Name"
// @Success 200 {array} models.AdapterGroupAssignment
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/groups [get]
func (h *AdapterHandler) ListAdapterGroupAssignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract adapter ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/adapters/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "groups" {
		http.NotFound(w, r)
		return
	}
	adapterID := parts[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	assignments, err := h.adapterService.ListAdapterAssignments(r.Context(), userID, adapterID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else if strings.Contains(err.Error(), "denied") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Access denied"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list assignments: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignments)
}

// GetClientConfig returns the client configuration for all adapters the user has access to
// @Summary Get client configuration
// @Description Get the aggregated client configuration for all adapters the user has access to
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/user/config [get]
func (h *AdapterHandler) GetClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	adapters, err := h.adapterService.ListAdapters(r.Context(), userID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list adapters: " + err.Error()})
		return
	}

	// Initialize response structure
	finalGeminiServers := make(map[string]interface{})
	finalVscodeServers := make(map[string]interface{})

	logging.AdapterLogger.Info("GetClientConfig: processing %d adapters for user %s", len(adapters), userID)

	for _, adapter := range adapters {
		// Generate client config for this adapter, including real tokens
		adapterClientConfig := h.generateClientConfig(&adapter, true)

		logging.AdapterLogger.Info("GetClientConfig: generated config for adapter %s: %+v", adapter.Name, adapterClientConfig)

		if geminiConfig, ok := adapterClientConfig["gemini"].(map[string]interface{}); ok {
			if mcpServers, ok := geminiConfig["mcpServers"].(map[string]interface{}); ok {
				for k, v := range mcpServers {
					finalGeminiServers[k] = v
				}
			}
		}

		if vscodeConfig, ok := adapterClientConfig["vscode"].(map[string]interface{}); ok {
			if servers, ok := vscodeConfig["servers"].(map[string]interface{}); ok {
				for k, v := range servers {
					finalVscodeServers[k] = v
				}
			}
		}
	}

	response := map[string]interface{}{
		"mcpClientConfig": map[string]interface{}{
			"gemini": map[string]interface{}{
				"mcpServers": finalGeminiServers,
			},
			"vscode": map[string]interface{}{
				"servers": finalVscodeServers,
				"inputs":  []interface{}{}, // Inputs are not aggregated from individual adapters
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
