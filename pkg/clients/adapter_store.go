package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"suse-ai-up/pkg/models"
)

// AdapterResourceStore defines the interface for adapter storage
type AdapterResourceStore interface {
	Create(ctx context.Context, adapter models.AdapterResource) error
	Get(ctx context.Context, id string) (*models.AdapterResource, error)
	List(ctx context.Context, userID string) ([]models.AdapterResource, error)
	ListAll(ctx context.Context) ([]models.AdapterResource, error)
	Update(ctx context.Context, adapter models.AdapterResource) error
	Delete(ctx context.Context, id string) error
	UpsertAsync(adapter models.AdapterResource, ctx context.Context) error
}

// FileAdapterStore implements AdapterResourceStore using file-based storage
type FileAdapterStore struct {
	filePath string
	adapters map[string]models.AdapterResource
	mu       sync.RWMutex
}

// NewFileAdapterStore creates a new file-based adapter store
func NewFileAdapterStore(filePath string) *FileAdapterStore {
	store := &FileAdapterStore{
		filePath: filePath,
		adapters: make(map[string]models.AdapterResource),
	}

	// Load existing adapters from file
	if err := store.loadFromFile(); err != nil {
		// Log error but continue with empty store
		fmt.Printf("Warning: Failed to load adapters from file: %v\n", err)
	}

	return store
}

// Create stores a new adapter
func (s *FileAdapterStore) Create(ctx context.Context, adapter models.AdapterResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.adapters[adapter.ID]; exists {
		return fmt.Errorf("adapter with ID %s already exists", adapter.ID)
	}

	s.adapters[adapter.ID] = adapter
	return s.saveToFile()
}

// Get retrieves an adapter by ID
func (s *FileAdapterStore) Get(ctx context.Context, id string) (*models.AdapterResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	adapter, exists := s.adapters[id]
	if !exists {
		return nil, fmt.Errorf("adapter with ID %s not found", id)
	}

	// Return a copy to prevent external modifications
	adapterCopy := adapter
	return &adapterCopy, nil
}

// List retrieves all adapters for a specific user
func (s *FileAdapterStore) List(ctx context.Context, userID string) ([]models.AdapterResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userAdapters := make([]models.AdapterResource, 0)
	for _, adapter := range s.adapters {
		if adapter.CreatedBy == userID {
			userAdapters = append(userAdapters, adapter)
		}
	}

	return userAdapters, nil
}

// ListAll retrieves all adapters regardless of user ownership
func (s *FileAdapterStore) ListAll(ctx context.Context) ([]models.AdapterResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allAdapters := make([]models.AdapterResource, 0, len(s.adapters))
	for _, adapter := range s.adapters {
		allAdapters = append(allAdapters, adapter)
	}

	return allAdapters, nil
}

// Update modifies an existing adapter
func (s *FileAdapterStore) Update(ctx context.Context, adapter models.AdapterResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.adapters[adapter.ID]; !exists {
		return fmt.Errorf("adapter with ID %s not found", adapter.ID)
	}

	s.adapters[adapter.ID] = adapter
	return s.saveToFile()
}

// Delete removes an adapter
func (s *FileAdapterStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.adapters[id]; !exists {
		return fmt.Errorf("adapter with ID %s not found", id)
	}

	delete(s.adapters, id)
	return s.saveToFile()
}

// UpsertAsync stores or updates an adapter asynchronously
func (s *FileAdapterStore) UpsertAsync(adapter models.AdapterResource, ctx context.Context) error {
	return s.Create(ctx, adapter)
}

// loadFromFile loads adapters from the JSON file
func (s *FileAdapterStore) loadFromFile() error {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		// File doesn't exist, start with empty store
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read adapter file: %w", err)
	}

	if len(data) == 0 {
		// Empty file, start with empty store
		return nil
	}

	var adapters []models.AdapterResource
	if err := json.Unmarshal(data, &adapters); err != nil {
		return fmt.Errorf("failed to parse adapter file: %w", err)
	}

	// Convert to map
	for _, adapter := range adapters {
		s.adapters[adapter.ID] = adapter
	}

	return nil
}

// saveToFile saves adapters to the JSON file
func (s *FileAdapterStore) saveToFile() error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create adapter directory: %w", err)
	}

	// Convert map to slice for JSON serialization
	adapters := make([]models.AdapterResource, 0, len(s.adapters))
	for _, adapter := range s.adapters {
		adapters = append(adapters, adapter)
	}

	data, err := json.MarshalIndent(adapters, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal adapters: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary adapter file: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("failed to rename adapter file: %w", err)
	}

	return nil
}

// InMemoryAdapterStore provides an in-memory implementation for testing
type InMemoryAdapterStore struct {
	adapters map[string]models.AdapterResource
	mu       sync.RWMutex
}

// NewInMemoryAdapterStore creates a new in-memory adapter store
func NewInMemoryAdapterStore() *InMemoryAdapterStore {
	return &InMemoryAdapterStore{
		adapters: make(map[string]models.AdapterResource),
	}
}

// Create stores a new adapter in memory
func (s *InMemoryAdapterStore) Create(ctx context.Context, adapter models.AdapterResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.adapters[adapter.ID]; exists {
		return fmt.Errorf("adapter with ID %s already exists", adapter.ID)
	}

	s.adapters[adapter.ID] = adapter
	return nil
}

// Get retrieves an adapter by ID from memory
func (s *InMemoryAdapterStore) Get(ctx context.Context, id string) (*models.AdapterResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	adapter, exists := s.adapters[id]
	if !exists {
		return nil, fmt.Errorf("adapter with ID %s not found", id)
	}

	adapterCopy := adapter
	return &adapterCopy, nil
}

// List retrieves all adapters for a specific user from memory
func (s *InMemoryAdapterStore) List(ctx context.Context, userID string) ([]models.AdapterResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userAdapters := make([]models.AdapterResource, 0)
	for _, adapter := range s.adapters {
		if adapter.CreatedBy == userID {
			userAdapters = append(userAdapters, adapter)
		}
	}

	return userAdapters, nil
}

// ListAll retrieves all adapters regardless of user ownership
func (s *InMemoryAdapterStore) ListAll(ctx context.Context) ([]models.AdapterResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allAdapters := make([]models.AdapterResource, 0, len(s.adapters))
	for _, adapter := range s.adapters {
		allAdapters = append(allAdapters, adapter)
	}

	return allAdapters, nil
}

// Update modifies an existing adapter in memory
func (s *InMemoryAdapterStore) Update(ctx context.Context, adapter models.AdapterResource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.adapters[adapter.ID]; !exists {
		return fmt.Errorf("adapter with ID %s not found", adapter.ID)
	}

	s.adapters[adapter.ID] = adapter
	return nil
}

// Delete removes an adapter from memory
func (s *InMemoryAdapterStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.adapters[id]; !exists {
		return fmt.Errorf("adapter with ID %s not found", id)
	}

	delete(s.adapters, id)
	return nil
}

// UpsertAsync stores or updates an adapter in memory
func (s *InMemoryAdapterStore) UpsertAsync(adapter models.AdapterResource, ctx context.Context) error {
	return s.Create(ctx, adapter)
}
