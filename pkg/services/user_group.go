package services

import (
	"context"
	"fmt"
	"strings"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// UserGroupService manages users, groups, and permissions
type UserGroupService struct {
	userStore  clients.UserStore
	groupStore clients.GroupStore
}

// NewUserGroupService creates a new user/group service
func NewUserGroupService(userStore clients.UserStore, groupStore clients.GroupStore) *UserGroupService {
	return &UserGroupService{
		userStore:  userStore,
		groupStore: groupStore,
	}
}

// CreateUser creates a new user
func (ugs *UserGroupService) CreateUser(ctx context.Context, user models.User) error {
	return ugs.userStore.Create(ctx, user)
}

// GetUser gets a user by ID
func (ugs *UserGroupService) GetUser(ctx context.Context, id string) (*models.User, error) {
	return ugs.userStore.Get(ctx, id)
}

// ListUsers lists all users
func (ugs *UserGroupService) ListUsers(ctx context.Context) ([]models.User, error) {
	return ugs.userStore.List(ctx)
}

// UpdateUser updates a user
func (ugs *UserGroupService) UpdateUser(ctx context.Context, user models.User) error {
	return ugs.userStore.Update(ctx, user)
}

// DeleteUser deletes a user
func (ugs *UserGroupService) DeleteUser(ctx context.Context, id string) error {
	user, err := ugs.userStore.Get(ctx, id)
	if err != nil {
		return err
	}
	return ugs.userStore.Delete(ctx, *user)
}

// GetUserByEmail gets a user by email
func (ugs *UserGroupService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return ugs.userStore.GetByEmail(ctx, email)
}

// CreateGroup creates a new group
func (ugs *UserGroupService) CreateGroup(ctx context.Context, group models.Group) error {
	return ugs.groupStore.Create(ctx, group)
}

// GetGroup gets a group by ID
func (ugs *UserGroupService) GetGroup(ctx context.Context, id string) (*models.Group, error) {
	return ugs.groupStore.Get(ctx, id)
}

// ListGroups lists all groups
func (ugs *UserGroupService) ListGroups(ctx context.Context) ([]models.Group, error) {
	return ugs.groupStore.List(ctx)
}

// UpdateGroup updates a group
func (ugs *UserGroupService) UpdateGroup(ctx context.Context, group models.Group) error {
	return ugs.groupStore.Update(ctx, group)
}

// DeleteGroup deletes a group
func (ugs *UserGroupService) DeleteGroup(ctx context.Context, id string) error {
	return ugs.groupStore.Delete(ctx, id)
}

// AddUserToGroup adds a user to a group
func (ugs *UserGroupService) AddUserToGroup(ctx context.Context, groupID, userID string) error {
	return ugs.groupStore.AddMember(ctx, groupID, userID)
}

// RemoveUserFromGroup removes a user from a group
func (ugs *UserGroupService) RemoveUserFromGroup(ctx context.Context, groupID, userID string) error {
	return ugs.groupStore.RemoveMember(ctx, groupID, userID)
}

// CanAccessServer checks if a user can access a server based on route assignments
func (ugs *UserGroupService) CanAccessServer(ctx context.Context, userID, serverID string) (bool, error) {
	// Get the user
	user, err := ugs.userStore.Get(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user has admin permissions (can access all servers)
	if ugs.hasPermission(user.Groups, "server:*") {
		return true, nil
	}

	// Check specific server access permissions
	if ugs.hasPermission(user.Groups, fmt.Sprintf("server:%s:*", serverID)) {
		return true, nil
	}

	// Check read access to the server
	if ugs.hasPermission(user.Groups, fmt.Sprintf("server:%s:read", serverID)) {
		return true, nil
	}

	return false, nil
}

// CanManageUsers checks if a user can manage users
func (ugs *UserGroupService) CanManageUsers(ctx context.Context, userID string) (bool, error) {
	// Allow dev-admin special permissions for development
	if userID == "dev-admin" {
		return true, nil
	}

	user, err := ugs.userStore.Get(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	return ugs.hasPermission(user.Groups, "user:manage"), nil
}

// CanManageGroups checks if a user can manage groups
func (ugs *UserGroupService) CanManageGroups(ctx context.Context, userID string) (bool, error) {
	// Allow dev-admin special permissions for development
	if userID == "dev-admin" {
		return true, nil
	}

	user, err := ugs.userStore.Get(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	return ugs.hasPermission(user.Groups, "group:manage"), nil
}

// CanCreateAdapters checks if a user can create adapters
func (ugs *UserGroupService) CanCreateAdapters(ctx context.Context, userID string) (bool, error) {
	user, err := ugs.userStore.Get(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	return ugs.hasPermission(user.Groups, "adapter:create"), nil
}

// GetUserGroups gets all groups for a user
func (ugs *UserGroupService) GetUserGroups(ctx context.Context, userID string) ([]models.Group, error) {
	user, err := ugs.userStore.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	var userGroups []models.Group
	for _, groupID := range user.Groups {
		group, err := ugs.groupStore.Get(ctx, groupID)
		if err != nil {
			continue // Skip groups that can't be found
		}
		userGroups = append(userGroups, *group)
	}

	return userGroups, nil
}

// GetGroupMembers gets all members of a group
func (ugs *UserGroupService) GetGroupMembers(ctx context.Context, groupID string) ([]models.User, error) {
	group, err := ugs.groupStore.Get(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group: %w", err)
	}

	var members []models.User
	for _, userID := range group.Members {
		user, err := ugs.userStore.Get(ctx, userID)
		if err != nil {
			continue // Skip users that can't be found
		}
		members = append(members, *user)
	}

	return members, nil
}

// InitializeDefaultGroups creates default groups if they don't exist
func (ugs *UserGroupService) InitializeDefaultGroups(ctx context.Context) error {
	defaultGroups := []models.Group{
		{
			ID:          "mcp-users",
			Name:        "MCP Users",
			Description: "Users with access to MCP servers",
			Permissions: []string{"server:read", "adapter:create"},
		},
		{
			ID:          "mcp-admins",
			Name:        "MCP Admins",
			Description: "Administrators with full MCP access",
			Permissions: []string{"server:*", "user:manage", "group:manage", "adapter:*"},
		},
		{
			ID:          "weather-users",
			Name:        "Weather API Users",
			Description: "Users with access to weather MCP servers",
			Permissions: []string{"server:weather-*"},
		},
	}

	for _, group := range defaultGroups {
		// Check if group already exists
		_, err := ugs.groupStore.Get(ctx, group.ID)
		if err != nil {
			// Group doesn't exist, create it
			if createErr := ugs.groupStore.Create(ctx, group); createErr != nil {
				return fmt.Errorf("failed to create default group %s: %w", group.ID, createErr)
			}
		}
	}

	return nil
}

// hasPermission checks if any of the user's groups have the specified permission
func (ugs *UserGroupService) hasPermission(userGroups []string, permission string) bool {
	for _, groupID := range userGroups {
		group, err := ugs.groupStore.Get(context.Background(), groupID)
		if err != nil {
			continue
		}

		for _, groupPerm := range group.Permissions {
			if ugs.permissionMatches(groupPerm, permission) {
				return true
			}
		}
	}
	return false
}

// permissionMatches checks if a permission pattern matches a required permission
func (ugs *UserGroupService) permissionMatches(pattern, required string) bool {
	// Exact match
	if pattern == required {
		return true
	}

	// Wildcard matching
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return strings.HasPrefix(required, prefix+":")
	}

	// Pattern matching with wildcards
	if strings.Contains(pattern, "*") {
		// Simple wildcard matching
		patternParts := strings.Split(pattern, "*")
		if len(patternParts) == 2 {
			return strings.HasPrefix(required, patternParts[0]) &&
				strings.HasSuffix(required, patternParts[1])
		}
	}

	return false
}

// ValidateUserID validates that a user ID exists
func (ugs *UserGroupService) ValidateUserID(ctx context.Context, userID string) error {
	_, err := ugs.userStore.Get(ctx, userID)
	return err
}

// ValidateGroupID validates that a group ID exists
func (ugs *UserGroupService) ValidateGroupID(ctx context.Context, groupID string) error {
	_, err := ugs.groupStore.Get(ctx, groupID)
	return err
}
