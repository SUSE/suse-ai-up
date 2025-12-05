package services

import (
	"context"
	"fmt"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
)

// AdapterService manages adapters for remote MCP servers
type AdapterService struct {
	store               clients.AdapterResourceStore
	registryStore       clients.MCPServerStore
	capabilityDiscovery *mcp.CapabilityDiscoveryService
}

// NewAdapterService creates a new adapter service
func NewAdapterService(store clients.AdapterResourceStore, registryStore clients.MCPServerStore) *AdapterService {
	return &AdapterService{
		store:               store,
		registryStore:       registryStore,
		capabilityDiscovery: mcp.NewCapabilityDiscoveryService(),
	}
}

// CreateAdapter creates a new adapter from a registry server
func (as *AdapterService) CreateAdapter(ctx context.Context, userID, mcpServerID, name string, envVars map[string]string, auth *models.AdapterAuthConfig) (*models.AdapterResource, error) {
	// Get the MCP server from registry
	server, err := as.registryStore.GetMCPServer(mcpServerID)
	if err != nil {
		return nil, fmt.Errorf("MCP server not found: %w", err)
	}

	// Validate required environment variables
	if server.Meta != nil {
		if userAuthRequired, ok := server.Meta["userAuthRequired"].(bool); ok && userAuthRequired {
			// Check if required env vars are provided
			// For now, we'll be lenient and just log warnings
		}
	}

	// Create adapter data
	adapterData := &models.AdapterData{
		Name:                 name,
		Description:          fmt.Sprintf("Adapter for %s", server.Name),
		Protocol:             models.ServerProtocolMCP,
		ConnectionType:       models.ConnectionTypeStreamableHttp,
		EnvironmentVariables: envVars,
		RemoteUrl:            server.URL, // Use the OAuth URL as remote URL
		Authentication:       auth,
	}

	// Discover capabilities
	if err := as.discoverCapabilities(ctx, adapterData); err != nil {
		return nil, fmt.Errorf("failed to discover capabilities: %w", err)
	}

	// Create adapter resource
	adapter := &models.AdapterResource{}
	adapter.Create(*adapterData, userID, time.Now())

	// Store adapter
	if err := as.store.Create(ctx, *adapter); err != nil {
		return nil, fmt.Errorf("failed to store adapter: %w", err)
	}

	return adapter, nil
}

// GetAdapter gets an adapter by ID for a specific user
func (as *AdapterService) GetAdapter(ctx context.Context, userID, adapterID string) (*models.AdapterResource, error) {
	adapter, err := as.store.Get(ctx, adapterID)
	if err != nil {
		return nil, err
	}

	// Check if adapter belongs to user
	if adapter.CreatedBy != userID {
		return nil, fmt.Errorf("adapter not found")
	}

	return adapter, nil
}

// ListAdapters lists all adapters for a user
func (as *AdapterService) ListAdapters(ctx context.Context, userID string) ([]models.AdapterResource, error) {
	return as.store.List(ctx, userID)
}

// UpdateAdapter updates an adapter
func (as *AdapterService) UpdateAdapter(ctx context.Context, userID string, adapter models.AdapterResource) error {
	// Check if adapter belongs to user
	existing, err := as.store.Get(ctx, adapter.ID)
	if err != nil {
		return err
	}

	if existing.CreatedBy != userID {
		return fmt.Errorf("adapter not found")
	}

	// Update the adapter
	adapter.CreatedBy = userID // Ensure user ownership
	return as.store.Update(ctx, adapter)
}

// DeleteAdapter deletes an adapter
func (as *AdapterService) DeleteAdapter(ctx context.Context, userID, adapterID string) error {
	// Check if adapter belongs to user
	existing, err := as.store.Get(ctx, adapterID)
	if err != nil {
		return err
	}

	if existing.CreatedBy != userID {
		return fmt.Errorf("adapter not found")
	}

	return as.store.Delete(ctx, adapterID)
}

// SyncAdapterCapabilities syncs capabilities for an adapter
func (as *AdapterService) SyncAdapterCapabilities(ctx context.Context, userID, adapterID string) error {
	// Get adapter
	adapter, err := as.GetAdapter(ctx, userID, adapterID)
	if err != nil {
		return err
	}

	// Re-discover capabilities
	if err := as.discoverCapabilities(ctx, &adapter.AdapterData); err != nil {
		return fmt.Errorf("failed to sync capabilities: %w", err)
	}

	// Update adapter
	return as.store.Update(ctx, *adapter)
}

// discoverCapabilities discovers MCP capabilities for an adapter
func (as *AdapterService) discoverCapabilities(ctx context.Context, adapterData *models.AdapterData) error {
	// For now, create a basic capability set
	// In a real implementation, this would connect to the remote server
	adapterData.MCPFunctionality = &models.MCPFunctionality{
		ServerInfo: models.MCPServerInfo{
			Name:    adapterData.Name,
			Version: "1.0.0",
		},
		Tools: []models.MCPTool{
			{
				Name:        "example_tool",
				Description: "Example tool from remote server",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"input": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
		Resources:     []models.MCPResource{},
		Prompts:       []models.MCPPrompt{},
		LastRefreshed: time.Now(),
	}

	return nil
}
