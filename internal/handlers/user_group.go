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

// UserGroupHandler handles user and group management requests
type UserGroupHandler struct {
	userGroupService *services.UserGroupService
}

// NewUserGroupHandler creates a new user/group handler
func NewUserGroupHandler(userGroupService *services.UserGroupService) *UserGroupHandler {
	return &UserGroupHandler{
		userGroupService: userGroupService,
	}
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	ID     string   `json:"id" example:"user123"`
	Name   string   `json:"name" example:"John Doe"`
	Email  string   `json:"email" example:"john@example.com"`
	Groups []string `json:"groups,omitempty" example:"[\"mcp-users\"]"`
}

// CreateUserResponse represents the response for user creation
type CreateUserResponse struct {
	User      models.User `json:"user"`
	CreatedAt time.Time   `json:"createdAt"`
}

// UpdateUserRequest represents a request to update a user
type UpdateUserRequest struct {
	Name   string   `json:"name,omitempty"`
	Email  string   `json:"email,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// CreateGroupRequest represents a request to create a group
type CreateGroupRequest struct {
	ID          string   `json:"id" example:"weather-team"`
	Name        string   `json:"name" example:"Weather Team"`
	Description string   `json:"description" example:"Team with access to weather APIs"`
	Permissions []string `json:"permissions,omitempty" example:"[\"server:weather-*\"]"`
}

// CreateGroupResponse represents the response for group creation
type CreateGroupResponse struct {
	Group     models.Group `json:"group"`
	CreatedAt time.Time    `json:"createdAt"`
}

// UpdateGroupRequest represents a request to update a group
type UpdateGroupRequest struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// AddUserToGroupRequest represents a request to add a user to a group
type AddUserToGroupRequest struct {
	UserID string `json:"userId" example:"user123"`
}

// CreateRouteAssignmentRequest represents a request to create a route assignment
type CreateRouteAssignmentRequest struct {
	UserIDs     []string `json:"userIds,omitempty" example:"[\"user123\"]"`
	GroupIDs    []string `json:"groupIds,omitempty" example:"[\"weather-team\"]"`
	AutoSpawn   bool     `json:"autoSpawn" example:"true"`
	Permissions string   `json:"permissions" example:"read"` // "read", "write", "admin"
}

// HandleUsers handles both listing and creating users
func (h *UserGroupHandler) HandleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListUsers(w, r)
	case http.MethodPost:
		h.CreateUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// CreateUser creates a new user
// CreateUser handles POST /api/v1/users
// @Summary Create a new user
// @Description Create a new user in the system
// @Tags users
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "User data"
// @Success 201 {object} CreateUserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users [post]
func (h *UserGroupHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Basic validation
	if req.ID == "" || req.Name == "" || req.Email == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "id, name, and email are required"})
		return
	}

	// Check permissions
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageUsers(r.Context(), userID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage users"})
		return
	}

	// Create user
	user := models.User{
		ID:     req.ID,
		Name:   req.Name,
		Email:  req.Email,
		Groups: req.Groups,
	}

	if err := h.userGroupService.CreateUser(r.Context(), user); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create user: " + err.Error()})
		return
	}

	response := CreateUserResponse{
		User:      user,
		CreatedAt: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListUsers handles GET /api/v1/users
// @Summary List all users
// @Description Retrieve a list of all users in the system
// @Tags users
// @Produce json
// @Success 200 {array} models.User
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users [get]
func (h *UserGroupHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users, err := h.userGroupService.ListUsers(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list users: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// GetUser gets a specific user by ID
// GetUser handles GET /api/v1/users/{id}
// @Summary Get user details
// @Description Retrieve details of a specific user
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} models.User
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users/{id} [get]
func (h *UserGroupHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	userID := strings.Split(path, "/")[0]

	user, err := h.userGroupService.GetUser(r.Context(), userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "User not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to get user: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// UpdateUser updates an existing user
// UpdateUser handles PUT /api/v1/users/{id}
// @Summary Update a user
// @Description Update an existing user's information
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param user body UpdateUserRequest true "Updated user data"
// @Success 200 {object} models.User
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users/{id} [put]
func (h *UserGroupHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	userID := strings.Split(path, "/")[0]

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Check permissions
	currentUserID := r.Header.Get("X-User-ID")
	if currentUserID == "" {
		currentUserID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageUsers(r.Context(), currentUserID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage users"})
		return
	}

	// Get existing user
	user, err := h.userGroupService.GetUser(r.Context(), userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "User not found"})
		return
	}

	// Update fields
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Groups != nil {
		user.Groups = req.Groups
	}

	if err := h.userGroupService.UpdateUser(r.Context(), *user); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update user: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// DeleteUser deletes a user
// DeleteUser handles DELETE /api/v1/users/{id}
// @Summary Delete a user
// @Description Delete a user from the system
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users/{id} [delete]
func (h *UserGroupHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	userID := strings.Split(path, "/")[0]

	// Check permissions
	currentUserID := r.Header.Get("X-User-ID")
	if currentUserID == "" {
		currentUserID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageUsers(r.Context(), currentUserID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage users"})
		return
	}

	if err := h.userGroupService.DeleteUser(r.Context(), userID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "User not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to delete user: " + err.Error()})
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGroups handles both listing and creating groups
func (h *UserGroupHandler) HandleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListGroups(w, r)
	case http.MethodPost:
		h.CreateGroup(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// CreateGroup creates a new group
// CreateGroup handles POST /api/v1/groups
// @Summary Create a new group
// @Description Create a new group in the system
// @Tags groups
// @Accept json
// @Produce json
// @Param group body CreateGroupRequest true "Group data"
// @Success 201 {object} models.Group
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups [post]
func (h *UserGroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	// Basic validation
	if req.ID == "" || req.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "id and name are required"})
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
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage groups"})
		return
	}

	// Create group
	group := models.Group{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}

	if err := h.userGroupService.CreateGroup(r.Context(), group); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create group: " + err.Error()})
		return
	}

	response := CreateGroupResponse{
		Group:     group,
		CreatedAt: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListGroups handles GET /api/v1/groups
// @Summary List all groups
// @Description Retrieve a list of all groups in the system
// @Tags groups
// @Produce json
// @Success 200 {array} models.Group
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups [get]
func (h *UserGroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groups, err := h.userGroupService.ListGroups(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list groups: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

// GetGroup gets a specific group by ID
// GetGroup handles GET /api/v1/groups/{id}
// @Summary Get group details
// @Description Retrieve details of a specific group
// @Tags groups
// @Produce json
// @Param id path string true "Group ID"
// @Success 200 {object} models.Group
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id} [get]
func (h *UserGroupHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	groupID := strings.Split(path, "/")[0]

	group, err := h.userGroupService.GetGroup(r.Context(), groupID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Group not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to get group: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

// UpdateGroup updates an existing group
// UpdateGroup handles PUT /api/v1/groups/{id}
// @Summary Update a group
// @Description Update an existing group's information
// @Tags groups
// @Accept json
// @Produce json
// @Param id path string true "Group ID"
// @Param group body UpdateGroupRequest true "Updated group data"
// @Success 200 {object} models.Group
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id} [put]
func (h *UserGroupHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	groupID := strings.Split(path, "/")[0]

	var req UpdateGroupRequest
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
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage groups"})
		return
	}

	// Get existing group
	group, err := h.userGroupService.GetGroup(r.Context(), groupID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Group not found"})
		return
	}

	// Update fields
	if req.Name != "" {
		group.Name = req.Name
	}
	if req.Description != "" {
		group.Description = req.Description
	}
	if req.Permissions != nil {
		group.Permissions = req.Permissions
	}

	if err := h.userGroupService.UpdateGroup(r.Context(), *group); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update group: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

// DeleteGroup deletes a group
// DeleteGroup handles DELETE /api/v1/groups/{id}
// @Summary Delete a group
// @Description Delete a group from the system
// @Tags groups
// @Produce json
// @Param id path string true "Group ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id} [delete]
func (h *UserGroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	groupID := strings.Split(path, "/")[0]

	// Check permissions
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageGroups(r.Context(), userID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage groups"})
		return
	}

	if err := h.userGroupService.DeleteGroup(r.Context(), groupID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Group not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to delete group: " + err.Error()})
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddUserToGroup adds a user to a group
// AddUserToGroup handles POST /api/v1/groups/{id}/members
// @Summary Add user to group
// @Description Add a user to a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path string true "Group ID"
// @Param request body AddUserToGroupRequest true "User to add"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id}/members [post]
func (h *UserGroupHandler) AddUserToGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "members" {
		http.NotFound(w, r)
		return
	}
	groupID := parts[0]

	var req AddUserToGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.UserID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "userId is required"})
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
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage groups"})
		return
	}

	if err := h.userGroupService.AddUserToGroup(r.Context(), groupID, req.UserID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to add user to group: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "user_added",
		"message": fmt.Sprintf("User %s added to group %s", req.UserID, groupID),
	})
}

// RemoveUserFromGroup removes a user from a group
// RemoveUserFromGroup handles DELETE /api/v1/groups/{id}/members/{userId}
// @Summary Remove user from group
// @Description Remove a user from a specific group
// @Tags groups
// @Produce json
// @Param id path string true "Group ID"
// @Param userId path string true "User ID to remove"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id}/members/{userId} [delete]
func (h *UserGroupHandler) RemoveUserFromGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID and user ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "members" {
		http.NotFound(w, r)
		return
	}
	groupID := parts[0]
	userID := parts[2]

	// Check permissions
	currentUserID := r.Header.Get("X-User-ID")
	if currentUserID == "" {
		currentUserID = "default-user"
	}

	canManage, err := h.userGroupService.CanManageGroups(r.Context(), currentUserID)
	if err != nil || !canManage {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Insufficient permissions to manage groups"})
		return
	}

	if err := h.userGroupService.RemoveUserFromGroup(r.Context(), groupID, userID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to remove user from group: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "user_removed",
		"message": fmt.Sprintf("User %s removed from group %s", userID, groupID),
	})
}
