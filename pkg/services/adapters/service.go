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
	}
	if server == nil {
		return nil, fmt.Errorf("MCP server not found: %s", mcpServerID)
	}
	fmt.Printf("DEBUG: Retrieved server %s with Meta: %+v\n", server.Name, server.Meta)

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
	fmt.Printf("ADAPTER_SERVICE_DEBUG: Checking server %s for sidecar creation\n", server.Name)
	fmt.Printf("ADAPTER_SERVICE_DEBUG: hasStdioPackage: %v\n", as.hasStdioPackage(server))
	fmt.Printf("ADAPTER_SERVICE_DEBUG: contains uyuni: %v\n", strings.Contains(server.Name, "uyuni"))
	fmt.Printf("ADAPTER_SERVICE_DEBUG: contains bugzilla: %v\n", strings.Contains(server.Name, "bugzilla"))

	if as.hasStdioPackage(server) || strings.Contains(server.Name, "uyuni") || strings.Contains(server.Name, "bugzilla") {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Will create sidecar for server %s\n", server.Name)
		// Get the docker command from the server metadata
		dockerCommand := as.getDockerCommand(server)
		fmt.Printf("ADAPTER_SERVICE_DEBUG: dockerCommand returned: '%s'\n", dockerCommand)
		if dockerCommand != "" {
			sidecarConfig = &models.SidecarConfig{
				CommandType: "docker",
				Command:     dockerCommand,
				Port:        8000, // Fixed port
			}
			connectionType = models.ConnectionTypeStreamableHttp
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Created docker sidecar config with command: %s\n", dockerCommand)
		} else {
			// Fallback: try to create a generic sidecar configuration
			sidecarConfig = &models.SidecarConfig{
				CommandType: "npx",
				Command:     "npx",
				Args:        []string{"-y", "@modelcontextprotocol/server-everything"},
				Port:        0, // Will be allocated dynamically
			}
			connectionType = models.ConnectionTypeStreamableHttp
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Created fallback sidecar config\n")
		}
	} else {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Will NOT create sidecar for server %s\n", server.Name)
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

// getDockerCommand extracts the docker command from server metadata
func (as *AdapterService) getDockerCommand(server *models.MCPServer) string {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: getDockerCommand called for server %s\n", server.Name)
	if server.Meta == nil {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: server.Meta is nil\n")
		return ""
	}

	sidecarConfig, ok := server.Meta["sidecarConfig"]
	if !ok {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig not found in meta\n")
		return ""
	}

	configMap, ok := sidecarConfig.(map[string]interface{})
	if !ok {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig is not a map\n")
		return ""
	}

	commandType, ok := configMap["commandType"].(string)
	if !ok || commandType != "docker" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: commandType is not docker or not found: %v\n", configMap["commandType"])
		return ""
	}

	// First try to get the full command
	command, ok := configMap["command"].(string)
	if ok && command != "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Found full docker command: %s\n", command)
		return command
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: Full command not found, trying to reconstruct\n")

	// If no full command, try to reconstruct from dockerCommand and dockerImage
	dockerCommand, ok := configMap["dockerCommand"].(string)
	if !ok || dockerCommand == "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: dockerCommand not found or empty: %v\n", configMap["dockerCommand"])
		return ""
	}

	dockerImage, ok := configMap["dockerImage"].(string)
	if !ok || dockerImage == "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: dockerImage not found or empty: %v\n", configMap["dockerImage"])
		return ""
	}

	// Reconstruct the docker command with just the image (env vars come from adapter)
	fullCommand := fmt.Sprintf("docker run -it --rm %s %s", dockerImage, dockerCommand)
	fmt.Printf("ADAPTER_SERVICE_DEBUG: Reconstructed docker command: %s\n", fullCommand)
	return fullCommand
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
func (as *AdapterService) getSidecarMeta(server *models.MCPServer, envVars map[string]string) *sidecarMeta {
	fmt.Printf("DEBUG: getSidecarMeta called for server %s, Meta: %+v\n", server.Name, server.Meta)
	if server.Meta == nil {
		fmt.Printf("DEBUG: server.Meta is nil\n")
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
				// Perform template substitution for placeholders like {{uyuni.server}}
				substitutedArg := as.substituteTemplates(argStr, envVars)
				meta.Args = append(meta.Args, substitutedArg)
			}
		}
	}

	if port, ok := configMap["port"].(float64); ok {
		meta.Port = int(port)
	}

	// Extract environment variables from env section
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

	// Parse -e flags from args (for docker run style commands)
	if len(meta.Args) > 0 {
		fmt.Printf("DEBUG: Parsing docker args: %+v\n", meta.Args)
		parsedArgs := []string{}
		i := 0
		for i < len(meta.Args) {
			arg := meta.Args[i]
			if arg == "-e" && i+1 < len(meta.Args) {
				// Parse -e KEY=VALUE
				envPair := meta.Args[i+1]
				if eqIndex := strings.Index(envPair, "="); eqIndex > 0 {
					key := envPair[:eqIndex]
					value := envPair[eqIndex+1:]
					fmt.Printf("DEBUG: Parsed env var: %s=%s\n", key, value)
					envVar := map[string]string{
						"name":  key,
						"value": value,
					}
					meta.Env = append(meta.Env, envVar)
				}
				i += 2 // Skip -e and the env var
			} else {
				// Keep all other args
				parsedArgs = append(parsedArgs, arg)
				i++
			}
		}
		meta.Args = parsedArgs
		fmt.Printf("DEBUG: Final args: %+v, env: %+v\n", meta.Args, meta.Env)
	}
	if port, ok := configMap["port"].(float64); ok {
		meta.Port = int(port)
	}

	// Return nil if required fields are missing
	if meta.CommandType == "" || meta.Command == "" {
		return nil
	}

	return meta
}

// substituteTemplates replaces template placeholders like {{uyuni.server}} with actual values
func (as *AdapterService) substituteTemplates(template string, envVars map[string]string) string {
	result := template

	// Replace {{variable}} patterns with values from envVars
	for key, value := range envVars {
		// Convert env var names to template format (e.g., UYUNI_SERVER -> uyuni.server)
		templateKey := strings.ToLower(strings.ReplaceAll(key, "_", "."))
		placeholder := "{{" + templateKey + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
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
