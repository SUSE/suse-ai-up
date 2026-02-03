package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"suse-ai-up/pkg/models"
)

// UserAdapterTokenStore defines the interface for user-adapter token storage
type UserAdapterTokenStore interface {
	// CreateToken creates a new token for a user-adapter pair
	CreateToken(ctx context.Context, token models.UserAdapterToken) error

	// GetToken retrieves a token by userID and adapterID
	GetToken(ctx context.Context, userID, adapterID string) (*models.UserAdapterToken, error)

	// GetTokenByValue retrieves a token by its token value
	GetTokenByValue(ctx context.Context, token string) (*models.UserAdapterToken, error)

	// DeleteToken deletes a token for a user-adapter pair
	DeleteToken(ctx context.Context, userID, adapterID string) error

	// ListUserTokens lists all tokens for a specific user
	ListUserTokens(ctx context.Context, userID string) ([]models.UserAdapterToken, error)

	// ListAdapterTokens lists all tokens for a specific adapter
	ListAdapterTokens(ctx context.Context, adapterID string) ([]models.UserAdapterToken, error)

	// UpdateToken updates an existing token
	UpdateToken(ctx context.Context, token models.UserAdapterToken) error
}

// FileUserAdapterTokenStore implements UserAdapterTokenStore using file-based storage
type FileUserAdapterTokenStore struct {
	filePath string
	tokens   map[string]models.UserAdapterToken // key: userID+adapterID
	mu       sync.RWMutex
}

// NewFileUserAdapterTokenStore creates a new file-based token store
func NewFileUserAdapterTokenStore(filePath string) *FileUserAdapterTokenStore {
	store := &FileUserAdapterTokenStore{
		filePath: filePath,
		tokens:   make(map[string]models.UserAdapterToken),
	}

	// Load existing tokens from file
	if err := store.loadFromFile(); err != nil {
		fmt.Printf("Warning: Failed to load tokens from file: %v\n", err)
	}

	return store
}

// makeKey creates a unique key for user-adapter pair
func (s *FileUserAdapterTokenStore) makeKey(userID, adapterID string) string {
	return fmt.Sprintf("%s:%s", userID, adapterID)
}

// CreateToken creates a new token
func (s *FileUserAdapterTokenStore) CreateToken(ctx context.Context, token models.UserAdapterToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(token.UserID, token.AdapterID)
	if _, exists := s.tokens[key]; exists {
		return fmt.Errorf("token already exists for user %s and adapter %s", token.UserID, token.AdapterID)
	}

	token.CreatedAt = time.Now().UTC()
	s.tokens[key] = token
	return s.saveToFile()
}

// GetToken retrieves a token by userID and adapterID
func (s *FileUserAdapterTokenStore) GetToken(ctx context.Context, userID, adapterID string) (*models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeKey(userID, adapterID)
	token, exists := s.tokens[key]
	if !exists {
		return nil, fmt.Errorf("token not found for user %s and adapter %s", userID, adapterID)
	}

	// Check expiration
	if token.IsExpired() {
		return nil, fmt.Errorf("token has expired for user %s and adapter %s", userID, adapterID)
	}

	return &token, nil
}

// GetTokenByValue retrieves a token by its token value (linear search)
func (s *FileUserAdapterTokenStore) GetTokenByValue(ctx context.Context, tokenValue string) (*models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, token := range s.tokens {
		if token.Token == tokenValue {
			// Check expiration
			if token.IsExpired() {
				return nil, fmt.Errorf("token has expired")
			}
			return &token, nil
		}
	}

	return nil, fmt.Errorf("token not found")
}

// DeleteToken deletes a token
func (s *FileUserAdapterTokenStore) DeleteToken(ctx context.Context, userID, adapterID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(userID, adapterID)
	if _, exists := s.tokens[key]; !exists {
		return fmt.Errorf("token not found for user %s and adapter %s", userID, adapterID)
	}

	delete(s.tokens, key)
	return s.saveToFile()
}

// ListUserTokens lists all tokens for a specific user
func (s *FileUserAdapterTokenStore) ListUserTokens(ctx context.Context, userID string) ([]models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tokens []models.UserAdapterToken
	for _, token := range s.tokens {
		if token.UserID == userID {
			tokens = append(tokens, token)
		}
	}

	return tokens, nil
}

// ListAdapterTokens lists all tokens for a specific adapter
func (s *FileUserAdapterTokenStore) ListAdapterTokens(ctx context.Context, adapterID string) ([]models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tokens []models.UserAdapterToken
	for _, token := range s.tokens {
		if token.AdapterID == adapterID {
			tokens = append(tokens, token)
		}
	}

	return tokens, nil
}

// UpdateToken updates an existing token
func (s *FileUserAdapterTokenStore) UpdateToken(ctx context.Context, token models.UserAdapterToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(token.UserID, token.AdapterID)
	if _, exists := s.tokens[key]; !exists {
		return fmt.Errorf("token not found for user %s and adapter %s", token.UserID, token.AdapterID)
	}

	s.tokens[key] = token
	return s.saveToFile()
}

// loadFromFile loads tokens from the JSON file
func (s *FileUserAdapterTokenStore) loadFromFile() error {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil // File doesn't exist, start with empty store
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read token file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file, start with empty store
	}

	var tokens []models.UserAdapterToken
	if err := json.Unmarshal(data, &tokens); err != nil {
		return fmt.Errorf("failed to parse token file: %w", err)
	}

	// Convert to map
	for _, token := range tokens {
		key := s.makeKey(token.UserID, token.AdapterID)
		s.tokens[key] = token
	}

	return nil
}

// saveToFile saves tokens to the JSON file
func (s *FileUserAdapterTokenStore) saveToFile() error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}

	// Convert map to slice for JSON serialization
	tokens := make([]models.UserAdapterToken, 0, len(s.tokens))
	for _, token := range s.tokens {
		tokens = append(tokens, token)
	}

	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary token file: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("failed to rename token file: %w", err)
	}

	return nil
}

// InMemoryUserAdapterTokenStore provides an in-memory implementation for testing
type InMemoryUserAdapterTokenStore struct {
	tokens map[string]models.UserAdapterToken // key: userID:adapterID
	mu     sync.RWMutex
}

// NewInMemoryUserAdapterTokenStore creates a new in-memory token store
func NewInMemoryUserAdapterTokenStore() *InMemoryUserAdapterTokenStore {
	return &InMemoryUserAdapterTokenStore{
		tokens: make(map[string]models.UserAdapterToken),
	}
}

// makeKey creates a unique key for user-adapter pair
func (s *InMemoryUserAdapterTokenStore) makeKey(userID, adapterID string) string {
	return fmt.Sprintf("%s:%s", userID, adapterID)
}

// CreateToken creates a new token in memory
func (s *InMemoryUserAdapterTokenStore) CreateToken(ctx context.Context, token models.UserAdapterToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(token.UserID, token.AdapterID)
	if _, exists := s.tokens[key]; exists {
		return fmt.Errorf("token already exists for user %s and adapter %s", token.UserID, token.AdapterID)
	}

	token.CreatedAt = time.Now().UTC()
	s.tokens[key] = token
	return nil
}

// GetToken retrieves a token by userID and adapterID from memory
func (s *InMemoryUserAdapterTokenStore) GetToken(ctx context.Context, userID, adapterID string) (*models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeKey(userID, adapterID)
	token, exists := s.tokens[key]
	if !exists {
		return nil, fmt.Errorf("token not found for user %s and adapter %s", userID, adapterID)
	}

	if token.IsExpired() {
		return nil, fmt.Errorf("token has expired for user %s and adapter %s", userID, adapterID)
	}

	return &token, nil
}

// GetTokenByValue retrieves a token by its token value from memory
func (s *InMemoryUserAdapterTokenStore) GetTokenByValue(ctx context.Context, tokenValue string) (*models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, token := range s.tokens {
		if token.Token == tokenValue {
			if token.IsExpired() {
				return nil, fmt.Errorf("token has expired")
			}
			return &token, nil
		}
	}

	return nil, fmt.Errorf("token not found")
}

// DeleteToken deletes a token from memory
func (s *InMemoryUserAdapterTokenStore) DeleteToken(ctx context.Context, userID, adapterID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(userID, adapterID)
	if _, exists := s.tokens[key]; !exists {
		return fmt.Errorf("token not found for user %s and adapter %s", userID, adapterID)
	}

	delete(s.tokens, key)
	return nil
}

// ListUserTokens lists all tokens for a specific user from memory
func (s *InMemoryUserAdapterTokenStore) ListUserTokens(ctx context.Context, userID string) ([]models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tokens []models.UserAdapterToken
	for _, token := range s.tokens {
		if token.UserID == userID {
			tokens = append(tokens, token)
		}
	}

	return tokens, nil
}

// ListAdapterTokens lists all tokens for a specific adapter from memory
func (s *InMemoryUserAdapterTokenStore) ListAdapterTokens(ctx context.Context, adapterID string) ([]models.UserAdapterToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tokens []models.UserAdapterToken
	for _, token := range s.tokens {
		if token.AdapterID == adapterID {
			tokens = append(tokens, token)
		}
	}

	return tokens, nil
}

// UpdateToken updates an existing token in memory
func (s *InMemoryUserAdapterTokenStore) UpdateToken(ctx context.Context, token models.UserAdapterToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(token.UserID, token.AdapterID)
	if _, exists := s.tokens[key]; !exists {
		return fmt.Errorf("token not found for user %s and adapter %s", token.UserID, token.AdapterID)
	}

	s.tokens[key] = token
	return nil
}
