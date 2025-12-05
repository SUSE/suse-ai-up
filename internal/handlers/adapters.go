package handlers

import (
	"encoding/json"
	"fmt"
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

// DeleteAdapter deletes an adapter
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
