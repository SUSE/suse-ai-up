package clients

import (
	"context"
	"suse-ai-up/pkg/models"
	"sync"
)

// InMemoryAdapterStore implements in-memory adapter storage
type InMemoryAdapterStore struct {
	store map[string]models.AdapterResource
	mutex sync.RWMutex
}

// NewInMemoryAdapterStore creates a new in-memory store
func NewInMemoryAdapterStore() *InMemoryAdapterStore {
	return &InMemoryAdapterStore{
		store: make(map[string]models.AdapterResource),
	}
}

// InitializeAsync does nothing for in-memory store
func (s *InMemoryAdapterStore) InitializeAsync(ctx context.Context) error {
	return nil
}

// TryGetAsync gets an adapter by name
func (s *InMemoryAdapterStore) TryGetAsync(name string, ctx context.Context) (*models.AdapterResource, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	adapter, exists := s.store[name]
	if !exists {
		return nil, nil
	}
	return &adapter, nil
}

// UpsertAsync upserts an adapter
func (s *InMemoryAdapterStore) UpsertAsync(adapter models.AdapterResource, ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.store[adapter.Name] = adapter
	return nil
}

// DeleteAsync deletes an adapter
func (s *InMemoryAdapterStore) DeleteAsync(name string, ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.store, name)
	return nil
}

// ListAsync lists all adapters
func (s *InMemoryAdapterStore) ListAsync(ctx context.Context) ([]models.AdapterResource, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	adapters := make([]models.AdapterResource, 0, len(s.store))
	for _, adapter := range s.store {
		adapters = append(adapters, adapter)
	}
	return adapters, nil
}
