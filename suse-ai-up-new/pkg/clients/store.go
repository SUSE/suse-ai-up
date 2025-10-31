package clients

import (
	"context"
	"suse-ai-up/pkg/models"
)

// AdapterResourceStore interface for storing and retrieving adapter resources
type AdapterResourceStore interface {
	InitializeAsync(ctx context.Context) error
	TryGetAsync(name string, ctx context.Context) (*models.AdapterResource, error)
	UpsertAsync(adapter models.AdapterResource, ctx context.Context) error
	DeleteAsync(name string, ctx context.Context) error
	ListAsync(ctx context.Context) ([]models.AdapterResource, error)
}

// NewAdapterResourceStore creates a new in-memory store
func NewAdapterResourceStore() AdapterResourceStore {
	return NewInMemoryAdapterStore()
}
