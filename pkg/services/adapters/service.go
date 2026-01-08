package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/logging"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/proxy"
	"suse-ai-up/pkg/services"
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
	logging.AdapterLogger.Info("ADAPTER_SERVICE: CreateAdapter started for server ID %s (user: %s)", mcpServerID, userID)

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
		logging.AdapterLogger.Error("MCP server not found: %s", mcpServerID)
		return nil, fmt.Errorf("MCP server not found: %s", mcpServerID)
	}

	logging.AdapterLogger.Info("Retrieved server %s with %d packages", server.Name, len(server.Packages))
	if len(server.Packages) > 0 {
		logging.AdapterLogger.Info("Server transport: %s", server.Packages[0].Transport.Type)
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
	logging.AdapterLogger.Info("Checking server %s for sidecar creation (hasStdio: %v, uyuni: %v, bugzilla: %v)",
		server.Name, as.hasStdioPackage(server), strings.Contains(server.Name, "uyuni"), strings.Contains(server.Name, "bugzilla"))

	if as.hasStdioPackage(server) || strings.Contains(server.Name, "uyuni") || strings.Contains(server.Name, "bugzilla") {
		logging.AdapterLogger.Info("Will create sidecar for server %s", server.Name)
		// Get the docker command from the server metadata
		dockerCommand := as.getDockerCommand(server)
		logging.AdapterLogger.Info("Docker command: '%s'", dockerCommand)

		if dockerCommand != "" {
			sidecarConfig = &models.SidecarConfig{
				CommandType: "docker",
				Command:     dockerCommand,
				Port:        8000, // Fixed port
			}
			connectionType = models.ConnectionTypeStreamableHttp
			logging.AdapterLogger.Success("Created docker sidecar config")
		} else {
			// Fallback: try to create a generic sidecar configuration
			sidecarConfig = &models.SidecarConfig{
				CommandType: "npx",
				Command:     "npx",
				Args:        []string{"-y", "@modelcontextprotocol/server-everything"},
				Port:        0, // Will be allocated dynamically
			}
			connectionType = models.ConnectionTypeStreamableHttp
			logging.AdapterLogger.Info("Created fallback sidecar config")
		}
	} else {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Will NOT create sidecar for server %s\n", server.Name)
	}

	// Generate a secure token for the adapter
	token := as.generateSecureToken()

	// Create adapter data
	adapterData := &models.AdapterData{
		Name:                 name,
		ConnectionType:       connectionType,
		EnvironmentVariables: envVars,    // Use the provided environment variables
		RemoteUrl:            server.URL, // Use the OAuth URL as remote URL
		URL:                  fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", name),
		SidecarConfig:        sidecarConfig,
	}

	// Create MCP client configuration
	adapterData.MCPClientConfig = models.MCPClientConfig{
		MCPServers: map[string]models.MCPServerConfig{
			name: {
				Command: "remote",
				Args: []string{
					name,
					fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", name),
					"--header",
					fmt.Sprintf("Authorization: Bearer %s", token),
				},
				Env: map[string]string{
					"AUTH_TOKEN": token,
				},
			},
		},
	}

	// Set up authentication configuration
	adapterData.Authentication = &models.AdapterAuthConfig{
		Required: true,
		Type:     "bearer",
		BearerToken: &models.BearerTokenConfig{
			Token:   token,
			Dynamic: false,
		},
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
	logging.AdapterLogger.Info("ADAPTER_SERVICE: Checking sidecar deployment - SidecarConfig: %v, ConnectionType: %v", adapter.SidecarConfig != nil, adapter.ConnectionType)
	if adapter.SidecarConfig != nil && adapter.ConnectionType == models.ConnectionTypeStreamableHttp {
		logging.AdapterLogger.Info("Sidecar deployment needed for adapter %s (SidecarConfig: %+v)", adapter.ID, adapter.SidecarConfig)
		if as.sidecarManager == nil {
			logging.AdapterLogger.Error("SidecarManager is nil, cannot deploy sidecar for adapter %s", adapter.ID)
			// Clean up the adapter since sidecar deployment is required
			as.store.Delete(ctx, adapter.ID)
			return nil, fmt.Errorf("sidecar manager not available for adapter deployment")
		} else {
			logging.AdapterLogger.Info("Deploying sidecar for adapter %s", adapter.ID)
			if err := as.sidecarManager.DeploySidecar(ctx, *adapter); err != nil {
				logging.AdapterLogger.Error("Sidecar deployment failed for adapter %s: %v", adapter.ID, err)
				// If sidecar deployment fails, we should clean up the adapter
				as.store.Delete(ctx, adapter.ID)
				return nil, fmt.Errorf("failed to deploy sidecar: %w", err)
			}
			logging.AdapterLogger.Success("Sidecar deployment successful for adapter %s", adapter.ID)
			// Update the stored adapter with the allocated port
			as.store.Update(ctx, *adapter)
		}
	} else {
		logging.AdapterLogger.Info("ADAPTER_SERVICE: Sidecar deployment NOT needed - SidecarConfig nil: %v, ConnectionType: %v", adapter.SidecarConfig == nil, adapter.ConnectionType)
	}

	logging.AdapterLogger.Success("CreateAdapter completed successfully for adapter %s", adapter.ID)
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

// getMapKeys returns the keys of a map[string]interface{}
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getDockerCommand extracts the docker command from server metadata
func (as *AdapterService) getDockerCommand(server *models.MCPServer) string {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: getDockerCommand called for server %s\n", server.Name)
	fmt.Printf("ADAPTER_SERVICE_DEBUG: server.Image: %s\n", server.Image)
	fmt.Printf("ADAPTER_SERVICE_DEBUG: server.Meta keys: %v\n", getMapKeys(server.Meta))
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
		fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig is not a map, type: %T, value: %v\n", sidecarConfig, sidecarConfig)
		return ""
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig keys: %v\n", getMapKeys(configMap))
	commandType, ok := configMap["commandType"].(string)
	if !ok || commandType != "docker" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: commandType is not docker or not found: %v (type: %T)\n", configMap["commandType"], configMap["commandType"])
		return ""
	}

	// First try to get the full command
	command, ok := configMap["command"].(string)
	if ok && command != "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Found full docker command: %s\n", command)

		// Check if the command already contains an image (non-flag argument)
		parts := strings.Fields(command)
		hasImage := false
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Checking command parts: %v\n", parts)
		for i := 2; i < len(parts); i++ {
			arg := parts[i]
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Checking arg %d: %s\n", i, arg)
			if !strings.HasPrefix(arg, "-") && arg != "" {
				hasImage = true
				fmt.Printf("ADAPTER_SERVICE_DEBUG: Found image in command: %s\n", arg)
				break
			}
			// Skip -e flag arguments
			if arg == "-e" && i+1 < len(parts) {
				i++
			}
		}
		fmt.Printf("ADAPTER_SERVICE_DEBUG: hasImage=%v, server.Image='%s'\n", hasImage, server.Image)

		// If no image in command, append the server's image
		if !hasImage {
			var imageToAppend string
			if server.Image != "" {
				imageToAppend = server.Image
			} else if len(server.Packages) > 0 && server.Packages[0].Identifier != "" {
				imageToAppend = server.Packages[0].Identifier
			}

			if imageToAppend != "" {
				// Remove trailing space and append image
				command = strings.TrimSpace(command) + " " + imageToAppend
				fmt.Printf("ADAPTER_SERVICE_DEBUG: Appended image to command: %s\n", command)
			}
		}

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

// GetAdapter gets an adapter by ID with permission checking
func (as *AdapterService) GetAdapter(ctx context.Context, userID, adapterID string, userGroupService *services.UserGroupService) (*models.AdapterResource, error) {
	adapter, err := as.store.Get(ctx, adapterID)
	if err != nil {
		return nil, err
	}

	// Check if user can access this adapter
	if adapter.CreatedBy != userID {
		// Check admin permissions
		if userGroupService != nil {
			if canManage, err := userGroupService.CanManageGroups(ctx, userID); err == nil && canManage {
				// Admin can access any adapter
			} else {
				return nil, fmt.Errorf("adapter not found")
			}
		} else {
			return nil, fmt.Errorf("adapter not found")
		}
	}

	return adapter, nil
}

// ListAdapters lists adapters with permission-based filtering
func (as *AdapterService) ListAdapters(ctx context.Context, userID string, userGroupService *services.UserGroupService) ([]models.AdapterResource, error) {
	// Check if user is admin (can see all adapters)
	if userGroupService != nil {
		if canManage, err := userGroupService.CanManageGroups(ctx, userID); err == nil && canManage {
			return as.store.ListAll(ctx)
		}
	}

	// Regular users only see their own adapters
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
	logging.AdapterLogger.Info("DeleteAdapter called for adapter %s by user %s", adapterID, userID)

	// Get adapter before deletion to check if it has sidecar resources
	adapter, err := as.store.Get(ctx, adapterID)
	if err != nil {
		logging.AdapterLogger.Error("Failed to get adapter %s: %v", adapterID, err)
	} else if adapter != nil {
		logging.AdapterLogger.Info("Found adapter %s with connection type: %s", adapterID, adapter.ConnectionType)

		// If this is a sidecar adapter (StreamableHttp with sidecar config), clean up the sidecar resources
		if adapter.ConnectionType == models.ConnectionTypeStreamableHttp && adapter.SidecarConfig != nil {
			if as.sidecarManager == nil {
				logging.AdapterLogger.Warn("SidecarManager is nil, cannot cleanup sidecar for adapter %s", adapterID)
			} else {
				logging.AdapterLogger.Info("Cleaning up sidecar for adapter %s", adapterID)
				if cleanupErr := as.sidecarManager.CleanupSidecar(ctx, adapterID); cleanupErr != nil {
					// Log the error but don't fail the adapter deletion
					logging.AdapterLogger.Warn("Failed to cleanup sidecar for adapter %s: %v", adapterID, cleanupErr)
				} else {
					logging.AdapterLogger.Success("Successfully initiated sidecar cleanup for adapter %s", adapterID)
				}
			}
		} else {
			logging.AdapterLogger.Info("Adapter %s is not a sidecar adapter (type: %s), skipping sidecar cleanup", adapterID, adapter.ConnectionType)
		}
	} else {
		logging.AdapterLogger.Warn("Adapter %s not found in store", adapterID)
	}

	// Delete the adapter from store
	if err := as.store.Delete(ctx, adapterID); err != nil {
		logging.AdapterLogger.Error("Failed to delete adapter %s from store: %v", adapterID, err)
		return fmt.Errorf("failed to delete adapter from store: %w", err)
	}

	logging.AdapterLogger.Success("Successfully deleted adapter %s", adapterID)
	return nil
}

// SyncAdapterCapabilities syncs capabilities for an adapter
func (as *AdapterService) SyncAdapterCapabilities(ctx context.Context, userID, adapterID string, userGroupService *services.UserGroupService) error {
	// Get adapter
	adapter, err := as.GetAdapter(ctx, userID, adapterID, userGroupService)
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

// generateSecureToken generates a cryptographically secure random token
func (as *AdapterService) generateSecureToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		logging.AdapterLogger.Warn("Failed to generate secure token, falling back to timestamp: %v", err)
		// Fallback to timestamp-based token
		return fmt.Sprintf("token-%d-%s", time.Now().Unix(), "fallback")
	}
	return base64.URLEncoding.EncodeToString(bytes)
}
