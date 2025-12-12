package services

import (
	"context"
	"fmt"
	"strings"
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
	// Get the MCP server from registry - first try by ID, then by name
	server, err := as.registryStore.GetMCPServer(mcpServerID)
	if err != nil {
		// If not found by ID, try to find by name
		servers := as.registryStore.ListMCPServers()
		for _, s := range servers {
			if s.Name == mcpServerID {
				server = s
				break
			}
		}
		if server == nil {
			return nil, fmt.Errorf("MCP server not found: %w", err)
		}
	}

	fmt.Printf("DEBUG: CreateAdapter called for server %s\n", server.Name)
	if len(server.Packages) == 0 {
		fmt.Printf("DEBUG: Server %s has no packages defined\n", server.Name)
	} else {
		fmt.Printf("DEBUG: Server %s transport: %s\n", server.Name, server.Packages[0].Transport.Type)
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

	// For non-remote servers (those with stdio packages), always create sidecars
	// The MCP inside the sidecar will use HTTP streamable-HTTP transport
	fmt.Printf("DEBUG: Checking server %s for sidecar creation\n", server.Name)
	if as.hasStdioPackage(server) || strings.Contains(server.Name, "uyuni") || strings.Contains(server.Name, "bugzilla") {
		fmt.Printf("DEBUG: Creating sidecar for server %s (hasStdio: %v)\n", server.Name, as.hasStdioPackage(server))
		// Create sidecar configuration for stdio-based MCP servers
		sidecarMeta := as.getSidecarMeta(server)
		if sidecarMeta != nil {
			// For HTTP transport, modify the dockerCommand to remove socat forwarding
			dockerCommand := sidecarMeta.DockerCommand
			if strings.Contains(dockerCommand, "socat") {
				// For HTTP transport, replace socat forwarding with direct MCP server execution
				// Extract the MCP server command from the complex command
				parts := strings.Split(dockerCommand, "&&")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if strings.Contains(part, "mcp-server") && !strings.Contains(part, "socat") {
						// Found the MCP server command
						if strings.HasPrefix(part, "(") && strings.Contains(part, "&") {
							// Extract command from "(mcp-server-uyuni & ..."
							start := strings.Index(part, "(")
							end := strings.Index(part, "&")
							if start >= 0 && end > start {
								dockerCommand = strings.TrimSpace(part[start+1 : end])
								break
							}
						}
					}
				}
			}

			// Prepare args, appending image for Docker commands
			args := sidecarMeta.Args
			if sidecarMeta.CommandType == "docker" && server.Image != "" {
				args = append(args, server.Image)
			}

			sidecarConfig = &models.SidecarConfig{
				CommandType: sidecarMeta.CommandType,
				Command:     sidecarMeta.Command,
				Args:        args,
				Env:         sidecarMeta.Env,
				Port:        0, // Will be allocated dynamically to prevent conflicts
			}
			connectionType = models.ConnectionTypeStreamableHttp // Sidecar will provide HTTP interface
			fmt.Printf("DEBUG: Creating sidecar for stdio-based MCP server %s\n", server.Name)
		} else {
			// Fallback: try to create a generic sidecar configuration
			sidecarConfig = &models.SidecarConfig{
				CommandType: "npx",
				Command:     "npx",
				Args:        []string{"-y", "@modelcontextprotocol/server-everything"},
				Port:        0, // Will be allocated dynamically
			}
			connectionType = models.ConnectionTypeStreamableHttp
			fmt.Printf("DEBUG: Creating generic sidecar for stdio-based MCP server %s\n", server.Name)
		}

		// Configure environment variables for HTTP transport
		fmt.Printf("DEBUG: Using provided environment variables: %+v\n", envVars)
	}

	// Create adapter data
	adapterData := &models.AdapterData{
		Name:                 name,
		ConnectionType:       connectionType,
		EnvironmentVariables: envVars,    // Use the provided environment variables
		RemoteUrl:            server.URL, // Use the OAuth URL as remote URL
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
	if adapter.SidecarConfig != nil && adapter.ConnectionType == models.ConnectionTypeStreamableHttp {
		if as.sidecarManager == nil {
			fmt.Printf("DEBUG: SidecarManager is nil, cannot deploy sidecar for adapter %s\n", adapter.ID)
			// Clean up the adapter since sidecar deployment is required
			as.store.Delete(ctx, adapter.ID)
			return nil, fmt.Errorf("sidecar manager not available for adapter deployment")
		} else {
			fmt.Printf("DEBUG: Attempting to deploy sidecar for adapter %s\n", adapter.ID)
			if err := as.sidecarManager.DeploySidecar(ctx, *adapter); err != nil {
				fmt.Printf("DEBUG: Sidecar deployment failed: %v\n", err)
				// If sidecar deployment fails, we should clean up the adapter
				as.store.Delete(ctx, adapter.ID)
				return nil, fmt.Errorf("failed to deploy sidecar: %w", err)
			}
			fmt.Printf("DEBUG: Sidecar deployment successful for adapter %s\n", adapter.ID)
			// Update the stored adapter with the allocated port
			as.store.Update(ctx, *adapter)
		}
	}

	return adapter, nil
}

// hasStdioPackage checks if the server has stdio packages
func (as *AdapterService) hasStdioPackage(server *models.MCPServer) bool {
	for _, pkg := range server.Packages {
		if pkg.RegistryType == "stdio" || pkg.Transport.Type == "stdio" {
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
	Port             int
	Env              []map[string]string
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
	}

	// Extract command and args
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

	if port, ok := configMap["port"].(float64); ok {
		meta.Port = int(port)
	}

	// Extract environment variables
	if envInterface, ok := configMap["env"].([]interface{}); ok {
		for _, envItem := range envInterface {
			if envMap, ok := envItem.(map[string]interface{}); ok {
				envVar := make(map[string]string)
				if name, ok := envMap["name"].(string); ok {
					envVar["name"] = name
				}
				if value, ok := envMap["value"].(string); ok {
					envVar["value"] = value
				}
				if len(envVar) == 2 {
					meta.Env = append(meta.Env, envVar)
				}
			}
		}
	}
	if port, ok := configMap["port"].(float64); ok {
		meta.Port = int(port)
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

// DeleteAdapter deletes an adapter and its associated resources
func (as *AdapterService) DeleteAdapter(ctx context.Context, userID, adapterID string) error {
	fmt.Printf("DEBUG: DeleteAdapter called for adapter %s by user %s\n", adapterID, userID)

	// Get adapter before deletion to check if it has sidecar resources
	adapter, err := as.store.Get(ctx, adapterID)
	if err != nil {
		fmt.Printf("DEBUG: Failed to get adapter %s: %v\n", adapterID, err)
	} else if adapter != nil {
		fmt.Printf("DEBUG: Found adapter %s with connection type: %s\n", adapterID, adapter.ConnectionType)

		// If this is a sidecar adapter (StreamableHttp with sidecar config), clean up the sidecar resources
		if adapter.ConnectionType == models.ConnectionTypeStreamableHttp && adapter.SidecarConfig != nil {
			if as.sidecarManager == nil {
				fmt.Printf("DEBUG: SidecarManager is nil, cannot cleanup sidecar for adapter %s\n", adapterID)
			} else {
				fmt.Printf("DEBUG: Attempting to cleanup sidecar for adapter %s\n", adapterID)
				if cleanupErr := as.sidecarManager.CleanupSidecar(ctx, adapterID); cleanupErr != nil {
					// Log the error but don't fail the adapter deletion
					fmt.Printf("Warning: Failed to cleanup sidecar for adapter %s: %v\n", adapterID, cleanupErr)
				} else {
					fmt.Printf("DEBUG: Successfully initiated sidecar cleanup for adapter %s\n", adapterID)
				}
			}
		} else {
			fmt.Printf("DEBUG: Adapter %s is not a sidecar adapter (type: %s), skipping sidecar cleanup\n", adapterID, adapter.ConnectionType)
		}
	} else {
		fmt.Printf("DEBUG: Adapter %s not found in store\n", adapterID)
	}

	// Delete the adapter from store
	if err := as.store.Delete(ctx, adapterID); err != nil {
		fmt.Printf("DEBUG: Failed to delete adapter %s from store: %v\n", adapterID, err)
		return fmt.Errorf("failed to delete adapter from store: %w", err)
	}

	fmt.Printf("DEBUG: Successfully deleted adapter %s from store\n", adapterID)
	return nil
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
