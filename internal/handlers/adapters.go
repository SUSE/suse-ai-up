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

// HandleAdapters handles both listing and creating adapters
// @Summary List adapters
// @Description List all adapters for the current user
// @Tags adapters
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Success 200 {array} models.AdapterResource "List of adapters"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/adapters [get]
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
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create adapter: " + err.Error()})
		return
	}

	// Generate MCP client configurations for different client types
	var mcpClientConfig map[string]interface{}
	if adapter.ConnectionType == models.ConnectionTypeRemoteHttp {
		// For remote HTTP servers, provide direct connection config
		mcpClientConfig = map[string]interface{}{
			"gemini": map[string]interface{}{
				"mcpServers": map[string]interface{}{
					adapter.ID: map[string]interface{}{
						"url": adapter.RemoteUrl,
						"headers": map[string]string{
							"Authorization": "Bearer adapter-session-token",
						},
					},
				},
			},
			"vscode": map[string]interface{}{
				"inputs": []interface{}{},
				"servers": map[string]interface{}{
					adapter.ID: map[string]interface{}{
						"type": "http",
						"url":  adapter.RemoteUrl,
						"headers": map[string]string{
							"Authorization": "Bearer adapter-session-token",
						},
					},
				},
			},
		}
	} else if adapter.ConnectionType == models.ConnectionTypeStreamableHttp {
		mcpClientConfig = map[string]interface{}{
			"gemini": map[string]interface{}{
				"mcpServers": map[string]interface{}{
					adapter.ID: map[string]interface{}{
						"httpUrl": fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", adapter.ID),
						"headers": map[string]string{
							"Authorization": "Bearer adapter-session-token",
						},
					},
				},
			},
			"vscode": map[string]interface{}{
				"servers": map[string]interface{}{
					adapter.ID: map[string]interface{}{
						"url": fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", adapter.ID),
						"headers": map[string]string{
							"Authorization": "Bearer adapter-session-token",
						},
						"type": "http",
					},
				},
				"inputs": []interface{}{},
			},
		}
	} else {
		// For other connection types (stdio, etc.), use stdio format
		mcpClientConfig = map[string]interface{}{"stdio": "format"}
	}

	response := CreateAdapterResponse{
		ID:              adapter.ID,
		MCPServerID:     req.MCPServerID,
		MCPClientConfig: mcpClientConfig,
		Capabilities:    adapter.MCPFunctionality,
		Status:          "ready",
		CreatedAt:       adapter.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListAdapters lists all adapters for the current user
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
		mcpClientConfig := map[string]interface{}{
			"gemini": map[string]interface{}{
				"mcpServers": map[string]interface{}{
					adapter.ID: map[string]interface{}{
						"httpUrl": adapter.URL,
						"headers": map[string]string{
							"Authorization": "Bearer adapter-session-token",
						},
					},
				},
			},
			"vscode": map[string]interface{}{
				"servers": map[string]interface{}{
					adapter.ID: map[string]interface{}{
						"url": adapter.URL,
						"headers": map[string]string{
							"Authorization": "Bearer adapter-session-token",
						},
						"type": "http",
					},
				},
				"inputs": []interface{}{},
			},
		}

		adapterMap := map[string]interface{}{
			"id":              adapter.ID,
			"name":            adapter.Name,
			"description":     adapter.Description,
			"url":             adapter.URL,
			"mcpClientConfig": mcpClientConfig,
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
	if err := h.adapterService.UpdateAdapter(r.Context(), userID, updateAdapter); err != nil {
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
	if err := h.adapterService.DeleteAdapter(r.Context(), userID, adapterID); err != nil {
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
