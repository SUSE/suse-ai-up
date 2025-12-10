package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/services"
)

// RegistryStore defines the interface for registry operations needed by route assignments
type RegistryStore interface {
	GetMCPServer(id string) (*models.MCPServer, error)
	UpdateMCPServer(id string, updated *models.MCPServer) error
}

// RouteAssignmentHandler handles route assignment management
type RouteAssignmentHandler struct {
	userGroupService *services.UserGroupService
	registryStore    RegistryStore
}

// NewRouteAssignmentHandler creates a new route assignment handler
func NewRouteAssignmentHandler(userGroupService *services.UserGroupService, registryStore RegistryStore) *RouteAssignmentHandler {
	return &RouteAssignmentHandler{
		userGroupService: userGroupService,
		registryStore:    registryStore,
	}
}

// CreateRouteAssignment creates a route assignment for a server
func (h *RouteAssignmentHandler) CreateRouteAssignment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract server ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "routes" {
		http.NotFound(w, r)
		return
	}
	serverID := parts[0]

	var req CreateRouteAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Validate permissions
	if req.Permissions != "" && req.Permissions != "read" && req.Permissions != "write" && req.Permissions != "admin" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "permissions must be 'read', 'write', or 'admin'"})
		return
	}

	// Check user permissions
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageGroups(r.Context(), userID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage route assignments"})
		return
	}

	// Validate user and group IDs
	for _, uid := range req.UserIDs {
		if err := h.userGroupService.ValidateUserID(r.Context(), uid); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid user ID: " + uid})
			return
		}
	}

	for _, gid := range req.GroupIDs {
		if err := h.userGroupService.ValidateGroupID(r.Context(), gid); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid group ID: " + gid})
			return
		}
	}

	// Get the server
	server, err := h.registryStore.GetMCPServer(serverID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Server not found"})
		return
	}

	// Create route assignment
	assignment := models.RouteAssignment{
		ID:          generateRouteAssignmentID(serverID),
		ServerID:    serverID,
		UserIDs:     req.UserIDs,
		GroupIDs:    req.GroupIDs,
		AutoSpawn:   req.AutoSpawn,
		Permissions: req.Permissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Add to server's route assignments
	server.RouteAssignments = append(server.RouteAssignments, assignment)

	// Update server in registry
	if err := h.registryStore.UpdateMCPServer(serverID, server); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update server route assignments: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(assignment)
}

// ListRouteAssignments lists route assignments for a server
func (h *RouteAssignmentHandler) ListRouteAssignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract server ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "routes" {
		http.NotFound(w, r)
		return
	}
	serverID := parts[0]

	// Get the server
	server, err := h.registryStore.GetMCPServer(serverID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Server not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server.RouteAssignments)
}

// UpdateRouteAssignment updates a route assignment
func (h *RouteAssignmentHandler) UpdateRouteAssignment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract server ID and assignment ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "routes" {
		http.NotFound(w, r)
		return
	}
	serverID := parts[0]
	assignmentID := parts[2]

	var req CreateRouteAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Check permissions
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageGroups(r.Context(), userID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage route assignments"})
		return
	}

	// Get the server
	server, err := h.registryStore.GetMCPServer(serverID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Server not found"})
		return
	}

	// Find and update the assignment
	found := false
	for i, assignment := range server.RouteAssignments {
		if assignment.ID == assignmentID {
			server.RouteAssignments[i].UserIDs = req.UserIDs
			server.RouteAssignments[i].GroupIDs = req.GroupIDs
			server.RouteAssignments[i].AutoSpawn = req.AutoSpawn
			server.RouteAssignments[i].Permissions = req.Permissions
			server.RouteAssignments[i].UpdatedAt = time.Now().UTC()
			found = true
			break
		}
	}

	if !found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Route assignment not found"})
		return
	}

	// Update server in registry
	if err := h.registryStore.UpdateMCPServer(serverID, server); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update server route assignments: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server.RouteAssignments)
}

// DeleteRouteAssignment deletes a route assignment
func (h *RouteAssignmentHandler) DeleteRouteAssignment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract server ID and assignment ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/registry/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "routes" {
		http.NotFound(w, r)
		return
	}
	serverID := parts[0]
	assignmentID := parts[2]

	// Check permissions
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageGroups(r.Context(), userID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage route assignments"})
		return
	}

	// Get the server
	server, err := h.registryStore.GetMCPServer(serverID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Server not found"})
		return
	}

	// Find and remove the assignment
	found := false
	for i, assignment := range server.RouteAssignments {
		if assignment.ID == assignmentID {
			server.RouteAssignments = append(server.RouteAssignments[:i], server.RouteAssignments[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Route assignment not found"})
		return
	}

	// Update server in registry
	if err := h.registryStore.UpdateMCPServer(serverID, server); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update server route assignments: " + err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// generateRouteAssignmentID generates a unique ID for a route assignment
func generateRouteAssignmentID(serverID string) string {
	return fmt.Sprintf("assignment-%s-%d", serverID, time.Now().UnixNano())
}
