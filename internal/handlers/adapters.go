package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
	adaptersvc "suse-ai-up/pkg/services/adapters"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// AdapterHandler handles adapter management requests
type AdapterHandler struct {
	adapterService *adaptersvc.AdapterService
}

// NewAdapterHandler creates a new adapter handler
func NewAdapterHandler(adapterService *adaptersvc.AdapterService) *AdapterHandler {
	return &AdapterHandler{
		adapterService: adapterService,
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateAdapterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

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

	// Create MCP client configuration
	mcpClientConfig := map[string]interface{}{
		"mcpServers": []map[string]interface{}{
			{
				"url": fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", adapter.ID),
				"auth": map[string]interface{}{
					"type":  "bearer",
					"token": "adapter-session-token", // Would be generated properly
				},
			},
		},
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	adapters, err := h.adapterService.ListAdapters(r.Context(), userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list adapters: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adapters)
}

// GetAdapter gets a specific adapter by ID
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

	adapter, err := h.adapterService.GetAdapter(r.Context(), userID, adapterID)
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

// UpdateAdapter updates an existing adapter
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

	var adapter models.AdapterResource
	if err := json.NewDecoder(r.Body).Decode(&adapter); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Ensure the ID matches the path parameter
	adapter.ID = adapterID

	if err := h.adapterService.UpdateAdapter(r.Context(), userID, adapter); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "adapter not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update adapter: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adapter)
}

// DeleteAdapter deletes an adapter and its associated sidecar resources
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
	adapter, err := h.adapterService.GetAdapter(r.Context(), userID, adapterID)
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
		sidecarURL := fmt.Sprintf("http://mcp-sidecar-%s.suse-ai-up-mcp.svc.cluster.local:%d/mcp", adapterID, port)
		fmt.Printf("DEBUG: Proxying MCP request to sidecar URL: %s\n", sidecarURL)
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

	// For other connection types, return not implemented
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(ErrorResponse{Error: "MCP protocol not supported for this adapter type"})
}

// proxyToSidecar proxies requests to the sidecar container
func (h *AdapterHandler) proxyToSidecar(w http.ResponseWriter, r *http.Request, sidecarURL string) {
	fmt.Printf("DEBUG: Attempting to proxy to sidecar URL: %s\n", sidecarURL)
	fmt.Printf("DEBUG: Original request method: %s, path: %s\n", r.Method, r.URL.Path)

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

	// Set Content-Type if not already set
	if sidecarReq.Header.Get("Content-Type") == "" {
		sidecarReq.Header.Set("Content-Type", "application/json")
	}

	fmt.Printf("DEBUG: Making request to sidecar: %s %s\n", sidecarReq.Method, sidecarReq.URL.String())

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
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to connect to sidecar: " + err.Error()})
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

	if err := h.adapterService.SyncAdapterCapabilities(r.Context(), userID, adapterID); err != nil {
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
