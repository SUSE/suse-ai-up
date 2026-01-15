package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"suse-ai-up/pkg/models"
)

// UserStore defines the interface for user storage
type UserStore interface {
	Create(ctx context.Context, user models.User) error
	Get(ctx context.Context, id string) (*models.User, error)
	List(ctx context.Context) ([]models.User, error)
	Update(ctx context.Context, user models.User) error
	Delete(ctx context.Context, user models.User) error
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByExternalID(ctx context.Context, provider, externalID string) (*models.User, error)
	Authenticate(ctx context.Context, id, password string) (*models.User, error)
}

// GroupStore defines the interface for group storage
type GroupStore interface {
	Create(ctx context.Context, group models.Group) error
	Get(ctx context.Context, id string) (*models.Group, error)
	List(ctx context.Context) ([]models.Group, error)
	Update(ctx context.Context, group models.Group) error
	Delete(ctx context.Context, id string) error
	AddMember(ctx context.Context, groupID, userID string) error
	RemoveMember(ctx context.Context, groupID, userID string) error
}

// FileUserStore implements UserStore using file-based storage
type FileUserStore struct {
	filePath string
	users    map[string]models.User
	mu       sync.RWMutex
}

// NewFileUserStore creates a new file-based user store
func NewFileUserStore(filePath string) *FileUserStore {
	store := &FileUserStore{
		filePath: filePath,
		users:    make(map[string]models.User),
	}

	// Load existing users from file
	if err := store.loadFromFile(); err != nil {
		fmt.Printf("Warning: Failed to load users from file: %v\n", err)
	}

	return store
}

// Create stores a new user
func (s *FileUserStore) Create(ctx context.Context, user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; exists {
		return fmt.Errorf("user with ID %s already exists", user.ID)
	}

	user.CreatedAt = time.Now().UTC()
	user.UpdatedAt = time.Now().UTC()
	s.users[user.ID] = user
	return s.saveToFile()
}

// Get retrieves a user by ID
func (s *FileUserStore) Get(ctx context.Context, id string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return nil, fmt.Errorf("user with ID %s not found", id)
	}

	userCopy := user
	return &userCopy, nil
}

// List retrieves all users
func (s *FileUserStore) List(ctx context.Context) ([]models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]models.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	return users, nil
}

// Update modifies an existing user
func (s *FileUserStore) Update(ctx context.Context, user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return fmt.Errorf("user with ID %s not found", user.ID)
	}

	user.UpdatedAt = time.Now().UTC()
	s.users[user.ID] = user
	return s.saveToFile()
}

// Delete removes a user
func (s *FileUserStore) Delete(ctx context.Context, user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return fmt.Errorf("user with ID %s not found", user.ID)
	}

	delete(s.users, user.ID)
	return s.saveToFile()
}

// GetByEmail retrieves a user by email
func (s *FileUserStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.Email == email {
			userCopy := user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user with email %s not found", email)
}

// GetByExternalID retrieves a user by external provider ID
func (s *FileUserStore) GetByExternalID(ctx context.Context, provider, externalID string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.AuthProvider == provider && user.ExternalID == externalID {
			userCopy := user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user with external ID %s not found for provider %s", externalID, provider)
}

// Authenticate verifies user credentials
func (s *FileUserStore) Authenticate(ctx context.Context, id, password string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	if user.AuthProvider != string(models.UserAuthProviderLocal) {
		return nil, fmt.Errorf("user uses external authentication")
	}

	if user.PasswordHash == "" {
		return nil, fmt.Errorf("password not set")
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	userCopy := user
	return &userCopy, nil
}

// loadFromFile loads users from the JSON file
func (s *FileUserStore) loadFromFile() error {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil // File doesn't exist, start with empty store
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read user file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file, start with empty store
	}

	var users []models.User
	if err := json.Unmarshal(data, &users); err != nil {
		return fmt.Errorf("failed to parse user file: %w", err)
	}

	// Convert to map
	for _, user := range users {
		s.users[user.ID] = user
	}

	return nil
}

// saveToFile saves users to the JSON file
func (s *FileUserStore) saveToFile() error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create user directory: %w", err)
	}

	// Convert map to slice for JSON serialization
	users := make([]models.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal users: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary user file: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("failed to rename user file: %w", err)
	}

	return nil
}

// FileGroupStore implements GroupStore using file-based storage
type FileGroupStore struct {
	filePath string
	groups   map[string]models.Group
	mu       sync.RWMutex
}

// NewFileGroupStore creates a new file-based group store
func NewFileGroupStore(filePath string) *FileGroupStore {
	store := &FileGroupStore{
		filePath: filePath,
		groups:   make(map[string]models.Group),
	}

	// Load existing groups from file
	if err := store.loadFromFile(); err != nil {
		fmt.Printf("Warning: Failed to load groups from file: %v\n", err)
	}

	return store
}

// Create stores a new group
func (s *FileGroupStore) Create(ctx context.Context, group models.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[group.ID]; exists {
		return fmt.Errorf("group with ID %s already exists", group.ID)
	}

	group.CreatedAt = time.Now().UTC()
	group.UpdatedAt = time.Now().UTC()
	s.groups[group.ID] = group
	return s.saveToFile()
}

// Get retrieves a group by ID
func (s *FileGroupStore) Get(ctx context.Context, id string) (*models.Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, exists := s.groups[id]
	if !exists {
		return nil, fmt.Errorf("group with ID %s not found", id)
	}

	groupCopy := group
	return &groupCopy, nil
}

// List retrieves all groups
func (s *FileGroupStore) List(ctx context.Context) ([]models.Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groups := make([]models.Group, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}

	return groups, nil
}

// Update modifies an existing group
func (s *FileGroupStore) Update(ctx context.Context, group models.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[group.ID]; !exists {
		return fmt.Errorf("group with ID %s not found", group.ID)
	}

	group.UpdatedAt = time.Now().UTC()
	s.groups[group.ID] = group
	return s.saveToFile()
}

// Delete removes a group
func (s *FileGroupStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[id]; !exists {
		return fmt.Errorf("group with ID %s not found", id)
	}

	delete(s.groups, id)
	return s.saveToFile()
}

// AddMember adds a user to a group
func (s *FileGroupStore) AddMember(ctx context.Context, groupID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return fmt.Errorf("group with ID %s not found", groupID)
	}

	// Check if user is already a member
	for _, member := range group.Members {
		if member == userID {
			return fmt.Errorf("user %s is already a member of group %s", userID, groupID)
		}
	}

	group.Members = append(group.Members, userID)
	group.UpdatedAt = time.Now().UTC()
	s.groups[groupID] = group
	return s.saveToFile()
}

// RemoveMember removes a user from a group
func (s *FileGroupStore) RemoveMember(ctx context.Context, groupID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return fmt.Errorf("group with ID %s not found", groupID)
	}

	// Find and remove the user
	for i, member := range group.Members {
		if member == userID {
			group.Members = append(group.Members[:i], group.Members[i+1:]...)
			group.UpdatedAt = time.Now().UTC()
			s.groups[groupID] = group
			return s.saveToFile()
		}
	}

	return fmt.Errorf("user %s is not a member of group %s", userID, groupID)
}

// loadFromFile loads groups from the JSON file
func (s *FileGroupStore) loadFromFile() error {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil // File doesn't exist, start with empty store
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read group file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file, start with empty store
	}

	var groups []models.Group
	if err := json.Unmarshal(data, &groups); err != nil {
		return fmt.Errorf("failed to parse group file: %w", err)
	}

	// Convert to map
	for _, group := range groups {
		s.groups[group.ID] = group
	}

	return nil
}

// saveToFile saves groups to the JSON file
func (s *FileGroupStore) saveToFile() error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create group directory: %w", err)
	}

	// Convert map to slice for JSON serialization
	groups := make([]models.Group, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}

	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal groups: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary group file: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("failed to rename group file: %w", err)
	}

	return nil
}

// InMemoryUserStore provides an in-memory implementation for testing
type InMemoryUserStore struct {
	users map[string]models.User
	mu    sync.RWMutex
}

// NewInMemoryUserStore creates a new in-memory user store
func NewInMemoryUserStore() *InMemoryUserStore {
	return &InMemoryUserStore{
		users: make(map[string]models.User),
	}
}

// Create stores a new user in memory
func (s *InMemoryUserStore) Create(ctx context.Context, user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; exists {
		return fmt.Errorf("user with ID %s already exists", user.ID)
	}

	user.CreatedAt = time.Now().UTC()
	user.UpdatedAt = time.Now().UTC()
	s.users[user.ID] = user
	return nil
}

// Get retrieves a user by ID from memory
func (s *InMemoryUserStore) Get(ctx context.Context, id string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return nil, fmt.Errorf("user with ID %s not found", id)
	}

	userCopy := user
	return &userCopy, nil
}

// List retrieves all users from memory
func (s *InMemoryUserStore) List(ctx context.Context) ([]models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]models.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	return users, nil
}

// Update modifies an existing user in memory
func (s *InMemoryUserStore) Update(ctx context.Context, user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return fmt.Errorf("user with ID %s not found", user.ID)
	}

	user.UpdatedAt = time.Now().UTC()
	s.users[user.ID] = user
	return nil
}

// Delete removes a user from memory
func (s *InMemoryUserStore) Delete(ctx context.Context, user models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return fmt.Errorf("user with ID %s not found", user.ID)
	}

	delete(s.users, user.ID)
	return nil
}

// GetByEmail retrieves a user by email from memory
func (s *InMemoryUserStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.Email == email {
			userCopy := user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user with email %s not found", email)
}

// GetByExternalID retrieves a user by external provider ID from memory
func (s *InMemoryUserStore) GetByExternalID(ctx context.Context, provider, externalID string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.AuthProvider == provider && user.ExternalID == externalID {
			userCopy := user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user with external ID %s not found for provider %s", externalID, provider)
}

// Authenticate verifies user credentials in memory
func (s *InMemoryUserStore) Authenticate(ctx context.Context, id, password string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[id]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	if user.AuthProvider != string(models.UserAuthProviderLocal) {
		return nil, fmt.Errorf("user uses external authentication")
	}

	if user.PasswordHash == "" {
		return nil, fmt.Errorf("password not set")
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	userCopy := user
	return &userCopy, nil
}

// InMemoryGroupStore provides an in-memory implementation for testing
type InMemoryGroupStore struct {
	groups map[string]models.Group
	mu     sync.RWMutex
}

// NewInMemoryGroupStore creates a new in-memory group store
func NewInMemoryGroupStore() *InMemoryGroupStore {
	return &InMemoryGroupStore{
		groups: make(map[string]models.Group),
	}
}

// Create stores a new group in memory
func (s *InMemoryGroupStore) Create(ctx context.Context, group models.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[group.ID]; exists {
		return fmt.Errorf("group with ID %s already exists", group.ID)
	}

	group.CreatedAt = time.Now().UTC()
	group.UpdatedAt = time.Now().UTC()
	s.groups[group.ID] = group
	return nil
}

// Get retrieves a group by ID from memory
func (s *InMemoryGroupStore) Get(ctx context.Context, id string) (*models.Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	group, exists := s.groups[id]
	if !exists {
		return nil, fmt.Errorf("group with ID %s not found", id)
	}

	groupCopy := group
	return &groupCopy, nil
}

// List retrieves all groups from memory
func (s *InMemoryGroupStore) List(ctx context.Context) ([]models.Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groups := make([]models.Group, 0, len(s.groups))
	for _, group := range s.groups {
		groups = append(groups, group)
	}

	return groups, nil
}

// Update modifies an existing group in memory
func (s *InMemoryGroupStore) Update(ctx context.Context, group models.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[group.ID]; !exists {
		return fmt.Errorf("group with ID %s not found", group.ID)
	}

	group.UpdatedAt = time.Now().UTC()
	s.groups[group.ID] = group
	return nil
}

// Delete removes a group from memory
func (s *InMemoryGroupStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[id]; !exists {
		return fmt.Errorf("group with ID %s not found", id)
	}

	delete(s.groups, id)
	return nil
}

// AddMember adds a user to a group in memory
func (s *InMemoryGroupStore) AddMember(ctx context.Context, groupID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return fmt.Errorf("group with ID %s not found", groupID)
	}

	// Check if user is already a member
	for _, member := range group.Members {
		if member == userID {
			return fmt.Errorf("user %s is already a member of group %s", userID, groupID)
		}
	}

	group.Members = append(group.Members, userID)
	group.UpdatedAt = time.Now().UTC()
	s.groups[groupID] = group
	return nil
}

// RemoveMember removes a user from a group in memory
func (s *InMemoryGroupStore) RemoveMember(ctx context.Context, groupID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return fmt.Errorf("group with ID %s not found", groupID)
	}

	// Find and remove the user
	for i, member := range group.Members {
		if member == userID {
			group.Members = append(group.Members[:i], group.Members[i+1:]...)
			group.UpdatedAt = time.Now().UTC()
			s.groups[groupID] = group
			return nil
		}
	}

	return fmt.Errorf("user %s is not a member of group %s", userID, groupID)
}
