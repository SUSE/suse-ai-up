package services

import (
	"context"
	"fmt"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/proxy"
)

// AdapterService manages adapters for remote MCP servers
type AdapterService struct {
	store               clients.AdapterResourceStore
	registryStore       clients.MCPServerStore
	capabilityDiscovery *mcp.CapabilityDiscoveryService
	sidecarManager      *proxy.SidecarManager
}

// NewAdapterService creates a new adapter service
func NewAdapterService(store clients.AdapterResourceStore, registryStore clients.MCPServerStore, sidecarManager *proxy.SidecarManager) *AdapterService {
	return &AdapterService{
		store:               store,
		registryStore:       registryStore,
		capabilityDiscovery: mcp.NewCapabilityDiscoveryService(),
		sidecarManager:      sidecarManager,
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

	// Determine connection type and sidecar configuration
	connectionType := models.ConnectionTypeStreamableHttp
	var sidecarConfig *models.SidecarConfig

	// Check if server has stdio packages and sidecar configuration
	if as.hasStdioPackage(server) {
		if sidecarMeta := as.getSidecarMeta(server); sidecarMeta != nil {
			connectionType = models.ConnectionTypeSidecarStdio
			sidecarConfig = &models.SidecarConfig{
				CommandType:      sidecarMeta.CommandType,
				BaseImage:        sidecarMeta.BaseImage,
				Command:          sidecarMeta.Command,
				Args:             sidecarMeta.Args,
				DockerImage:      sidecarMeta.DockerImage,
				DockerCommand:    sidecarMeta.DockerCommand,
				DockerEntrypoint: sidecarMeta.DockerEntrypoint,
			}

			// Set default base images based on command type
			if sidecarConfig.BaseImage == "" {
				switch sidecarConfig.CommandType {
				case "npx":
					sidecarConfig.BaseImage = "registry.suse.com/bci/nodejs:22"
				case "python", "uv":
					sidecarConfig.BaseImage = "registry.suse.com/bci/python:3.12"
				}
			}
			// For Uyuni, add HTTP transport configuration
			if mcpServerID == "suse-uyuni" {
				if envVars == nil {
					envVars = make(map[string]string)
				}
				envVars["UYUNI_MCP_TRANSPORT"] = "http"
				envVars["MCP_HOST"] = "0.0.0.0"
			}
		} else {
			// Fall back to local stdio if no sidecar config
			connectionType = models.ConnectionTypeLocalStdio
		}
	}

	// Create adapter data
	adapterData := &models.AdapterData{
		Name:                 name,
		Description:          fmt.Sprintf("Adapter for %s", server.Name),
		Protocol:             models.ServerProtocolMCP,
		ConnectionType:       connectionType,
		EnvironmentVariables: envVars,
		RemoteUrl:            server.URL, // Use the OAuth URL as remote URL
		Authentication:       auth,
		SidecarConfig:        sidecarConfig,
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

	// Deploy sidecar if needed
	if adapter.ConnectionType == models.ConnectionTypeSidecarStdio {
		if as.sidecarManager == nil {
			// Fall back to local stdio if no sidecar manager is available
			adapter.ConnectionType = models.ConnectionTypeLocalStdio
			adapter.SidecarConfig = nil
			// Update the stored adapter
			as.store.Update(ctx, *adapter)
		} else {
			if err := as.sidecarManager.DeploySidecar(ctx, *adapter); err != nil {
				// If sidecar deployment fails, we should clean up the adapter
				as.store.Delete(ctx, adapter.ID)
				return nil, fmt.Errorf("failed to deploy sidecar: %w", err)
			}
		}
	}

	return adapter, nil
}

// hasStdioPackage checks if the server has stdio packages
func (as *AdapterService) hasStdioPackage(server *models.MCPServer) bool {
	for _, pkg := range server.Packages {
		if pkg.RegistryType == "stdio" {
			return true
		}
	}
	return false
}

// sidecarMeta represents sidecar configuration from server metadata
type sidecarMeta struct {
	CommandType      string
	BaseImage        string
	Command          string
	Args             []string
	DockerImage      string
	DockerCommand    string
	DockerEntrypoint string
}

// getSidecarMeta extracts sidecar configuration from server metadata
func (as *AdapterService) getSidecarMeta(server *models.MCPServer) *sidecarMeta {
	if server.Meta == nil {
		return nil
	}

	sidecarConfig, ok := server.Meta["sidecarConfig"]
	if !ok {
		return nil
	}

	configMap, ok := sidecarConfig.(map[string]interface{})
	if !ok {
		return nil
	}

	meta := &sidecarMeta{}

	// Extract command type
	if commandType, ok := configMap["commandType"].(string); ok {
		meta.CommandType = commandType
	} else {
		// Default to docker for backward compatibility
		meta.CommandType = "docker"
	}

	// Extract new fields
	if baseImage, ok := configMap["baseImage"].(string); ok {
		meta.BaseImage = baseImage
	}
	if command, ok := configMap["command"].(string); ok {
		meta.Command = command
	}
	if argsInterface, ok := configMap["args"].([]interface{}); ok {
		for _, arg := range argsInterface {
			if argStr, ok := arg.(string); ok {
				meta.Args = append(meta.Args, argStr)
			}
		}
	}

	// Extract legacy Docker fields for backward compatibility
	if dockerImage, ok := configMap["dockerImage"].(string); ok {
		meta.DockerImage = dockerImage
	}
	if dockerCommand, ok := configMap["dockerCommand"].(string); ok {
		meta.DockerCommand = dockerCommand
	}
	if dockerEntrypoint, ok := configMap["dockerEntrypoint"].(string); ok {
		meta.DockerEntrypoint = dockerEntrypoint
	}

	// Return nil if required fields are missing
	if meta.DockerImage == "" {
		return nil
	}

	return meta
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
