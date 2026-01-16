package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
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

		// Extract sidecar configuration from server metadata
		extractedConfig := as.getSidecarConfig(server)
		if extractedConfig != nil {
			sidecarConfig = extractedConfig
			// Process template variables in the command
			processedConfig := as.processCommandTemplates(sidecarConfig, server)
			sidecarConfig = processedConfig

			// Check if this is an HTTP remote server (no sidecar needed)
			if sidecarConfig.CommandType == "http" {
				connectionType = models.ConnectionTypeRemoteHttp
				logging.AdapterLogger.Success("Created HTTP remote config for server %s", server.Name)
			} else {
				connectionType = models.ConnectionTypeStreamableHttp
				logging.AdapterLogger.Success("Created sidecar config with commandType: %s", sidecarConfig.CommandType)
			}
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
		// For remote servers, check for HTTP sidecar config first
		extractedConfig := as.getSidecarConfig(server)
		if extractedConfig != nil && extractedConfig.CommandType == "http" {
			// Process template variables in the command (URL)
			processedConfig := as.processCommandTemplates(extractedConfig, server)
			sidecarConfig = processedConfig
			connectionType = models.ConnectionTypeRemoteHttp
			logging.AdapterLogger.Success("Created HTTP remote config for server %s", server.Name)
		} else {
			// For other remote servers, use RemoteHttp if they have a URL
			if server.URL != "" {
				connectionType = models.ConnectionTypeRemoteHttp
				logging.AdapterLogger.Success("Created remote HTTP config for server %s", server.Name)
			} else {
				fmt.Printf("ADAPTER_SERVICE_DEBUG: Will NOT create adapter for server %s (no URL)\n", server.Name)
				return nil, fmt.Errorf("server %s has no URL for remote connection", server.Name)
			}
		}
	}

	// Generate a secure token for the adapter
	token := as.generateSecureToken()

	// Determine remote URL based on connection type
	remoteUrl := server.URL // Default to server URL
	if connectionType == models.ConnectionTypeRemoteHttp && sidecarConfig != nil {
		// For HTTP remote connections, use the command as the remote URL
		remoteUrl = sidecarConfig.Command
	}

	// Determine initial status
	initialStatus := models.AdapterLifecycleStatusNotReady
	if connectionType == models.ConnectionTypeRemoteHttp {
		initialStatus = models.AdapterLifecycleStatusReady // HTTP remotes are ready immediately
	}

	// Create adapter data
	adapterData := &models.AdapterData{
		Name:                 name,
		ConnectionType:       connectionType,
		Status:               initialStatus,
		EnvironmentVariables: envVars,   // Use the provided environment variables
		RemoteUrl:            remoteUrl, // Use appropriate remote URL
		URL:                  fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", name),
		SidecarConfig:        sidecarConfig,
	}

	// Create MCP client configuration based on connection type
	if connectionType == models.ConnectionTypeRemoteHttp {
		// For RemoteHttp adapters, set direct URL-based configuration
		adapterData.MCPClientConfig = models.MCPClientConfig{
			MCPServers: map[string]models.MCPServerConfig{
				name: {
					URL: remoteUrl, // Direct connection to remote MCP server
					Headers: map[string]string{
						"Authorization": fmt.Sprintf("Bearer %s", token),
					},
				},
			},
		}
	} else if connectionType == models.ConnectionTypeStreamableHttp {
		// For StreamableHttp adapters (proxy-based), set URL-based configuration
		// We store the standard MCP client config, but the handlers will provide multiple formats
		adapterData.MCPClientConfig = models.MCPClientConfig{
			MCPServers: map[string]models.MCPServerConfig{
				name: {
					URL: fmt.Sprintf("http://localhost:8911/api/v1/adapters/%s/mcp", name),
					Headers: map[string]string{
						"Authorization": fmt.Sprintf("Bearer %s", token),
					},
				},
			},
		}
	} else {
		// For stdio-based adapters, set command-based configuration
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
				// Set status to error before cleanup
				adapter.Status = models.AdapterLifecycleStatusError
				as.store.Update(ctx, *adapter) // Update status before deletion
				// If sidecar deployment fails, we should clean up the adapter
				as.store.Delete(ctx, adapter.ID)
				return nil, fmt.Errorf("failed to deploy sidecar: %w", err)
			}
			logging.AdapterLogger.Success("Sidecar deployment successful for adapter %s", adapter.ID)
			logging.AdapterLogger.Info("Waiting for sidecar to be ready before capability discovery...")

			// Wait longer for the sidecar to be fully ready (MCP servers need time to start)
			time.Sleep(10 * time.Second)

			// Discover actual capabilities from the deployed sidecar
			logging.AdapterLogger.Info("Starting capability discovery for adapter %s", adapter.ID)
			if err := as.discoverCapabilitiesFromSidecar(ctx, adapter); err != nil {
				logging.AdapterLogger.Warn("Failed to discover capabilities from sidecar for adapter %s: %v", adapter.ID, err)
				// Set status to error if capability discovery fails
				adapter.Status = models.AdapterLifecycleStatusError
				// Don't fail the entire creation - just log the warning
				// The adapter will still work with basic capabilities
			} else {
				logging.AdapterLogger.Success("Successfully discovered capabilities from sidecar for adapter %s", adapter.ID)
				// Set status to ready if capability discovery succeeds
				adapter.Status = models.AdapterLifecycleStatusReady
			}

			// Check sidecar health and update status accordingly
			if err := as.checkAndUpdateSidecarHealth(ctx, adapter); err != nil {
				logging.AdapterLogger.Warn("Failed to check sidecar health for adapter %s: %v", adapter.ID, err)
				// Continue anyway - health check failure doesn't prevent adapter creation
			}

			// Update the stored adapter with the allocated port, discovered capabilities, and status
			if err := as.store.Update(ctx, *adapter); err != nil {
				logging.AdapterLogger.Error("Failed to update adapter in store: %v", err)
				return nil, fmt.Errorf("failed to update adapter: %w", err)
			}
		}
	} else {
		logging.AdapterLogger.Info("ADAPTER_SERVICE: Sidecar deployment NOT needed - SidecarConfig nil: %v, ConnectionType: %v", adapter.SidecarConfig == nil, adapter.ConnectionType)
		// For non-sidecar adapters, set status to ready immediately
		adapter.Status = models.AdapterLifecycleStatusReady

		// Update the stored adapter with the ready status
		if err := as.store.Update(ctx, *adapter); err != nil {
			logging.AdapterLogger.Error("Failed to update adapter status in store: %v", err)
			return nil, fmt.Errorf("failed to update adapter: %w", err)
		}
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

// processCommandTemplates processes template variables in the sidecar command
func (as *AdapterService) processCommandTemplates(sidecarConfig *models.SidecarConfig, server *models.MCPServer) *models.SidecarConfig {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: processCommandTemplates called with command: %s\n", sidecarConfig.Command)

	if sidecarConfig == nil || sidecarConfig.Command == "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Returning early - sidecarConfig nil or empty command\n")
		return sidecarConfig
	}

	// Check if the command contains template variables
	if !strings.Contains(sidecarConfig.Command, "{{") {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: No template variables found in command\n")
		return sidecarConfig
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: Processing templates in command: %s\n", sidecarConfig.Command)
	fmt.Printf("ADAPTER_SERVICE_DEBUG: Server meta keys: %+v\n", getMapKeys(server.Meta))
	fmt.Printf("ADAPTER_SERVICE_DEBUG: Server meta: %+v\n", server.Meta)

	// Create a copy of the config to modify
	processedConfig := *sidecarConfig

	// Process template variables based on command type
	fmt.Printf("ADAPTER_SERVICE_DEBUG: CommandType: %s\n", sidecarConfig.CommandType)
	switch sidecarConfig.CommandType {
	case "docker":
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Processing docker templates\n")
		processedConfig.Command = as.processDockerTemplates(sidecarConfig.Command, server)
	case "python", "npx", "go":
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Processing python/npx/go templates\n")
		// For python/npx/go, templates are processed but the command structure may remain similar
		processedConfig.Command = as.processGenericTemplates(sidecarConfig.Command, server)
	default:
		// For unknown types, leave as-is
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Unknown command type %s, skipping template processing\n", sidecarConfig.CommandType)
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: Processed command: %s\n", processedConfig.Command)
	return &processedConfig
}

// processDockerTemplates processes templates for docker commands
func (as *AdapterService) processDockerTemplates(command string, server *models.MCPServer) string {
	return as.processTemplates(command, server, func(varName, envName string) string {
		return fmt.Sprintf("-e %s=$%s", envName, envName)
	})
}

// processGenericTemplates processes templates for python/npx commands
func (as *AdapterService) processGenericTemplates(command string, server *models.MCPServer) string {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: processGenericTemplates called with command: %s\n", command)
	// For generic commands, substitute template variables with environment variable references
	return as.processTemplatesGeneric(command, server)
}

// processTemplatesGeneric processes templates for generic commands, substituting all found templates
func (as *AdapterService) processTemplatesGeneric(command string, server *models.MCPServer) string {
	// Find all template variables in the command
	templateRegex := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := templateRegex.FindAllStringSubmatch(command, -1)

	result := command
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		varName := strings.TrimSpace(match[1])

		// Look up the variable in config.secrets
		envName := as.lookupTemplatedVariableGeneric(varName, server)
		if envName == "" {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Variable %s not found, skipping\n", varName)
			continue
		}

		// For generic commands, substitute with environment variable reference
		substitution := fmt.Sprintf("$%s", envName)
		templatePattern := fmt.Sprintf("{{%s}}", varName)
		result = strings.ReplaceAll(result, templatePattern, substitution)

		fmt.Printf("ADAPTER_SERVICE_DEBUG: Replaced %s with %s\n", templatePattern, substitution)
	}

	return result
}

// lookupTemplatedVariableGeneric looks up template variables for generic processing (always substitutes)
func (as *AdapterService) lookupTemplatedVariableGeneric(varName string, server *models.MCPServer) string {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: lookupTemplatedVariableGeneric called for varName: %s\n", varName)

	if server.Meta == nil {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: server.Meta is nil\n")
		return ""
	}

	// First try direct secrets (new format)
	secretsRaw, ok := server.Meta["secrets"]
	if !ok {
		// Fall back to config.secrets (old format)
		fmt.Printf("ADAPTER_SERVICE_DEBUG: secrets not found directly, trying config.secrets\n")
		configRaw, ok := server.Meta["config"]
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: config not found in server.Meta, available keys: %+v\n", getMapKeys(server.Meta))
			return ""
		}

		configMap, ok := configRaw.(map[string]interface{})
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: config is not a map, type: %T\n", configRaw)
			return ""
		}

		secretsRaw, ok = configMap["secrets"]
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: secrets not found in config, available config keys: %+v\n", getMapKeys(configMap))
			return ""
		}
	}

	secretsSlice, ok := secretsRaw.([]interface{})
	if !ok {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: secrets is not a slice, type: %T, value: %+v\n", secretsRaw, secretsRaw)
		return ""
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: Found %d secrets\n", len(secretsSlice))

	for _, secretRaw := range secretsSlice {
		secretMap, ok := secretRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this secret matches the variable name
		name, ok := secretMap["name"].(string)
		if !ok || name != varName {
			continue
		}

		// Get the environment variable name
		envName, ok := secretMap["env"].(string)
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Variable %s missing env field\n", varName)
			return ""
		}

		fmt.Printf("ADAPTER_SERVICE_DEBUG: Found variable %s -> %s\n", varName, envName)
		return envName
	}

	return ""
}

// processTemplates processes template variables using a custom substitution function
func (as *AdapterService) processTemplates(command string, server *models.MCPServer, substituteFunc func(varName, envName string) string) string {
	// Find all template variables in the command
	templateRegex := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := templateRegex.FindAllStringSubmatch(command, -1)

	result := command
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		varName := strings.TrimSpace(match[1])
		fmt.Printf("ADAPTER_SERVICE_DEBUG: Found template variable: %s\n", varName)

		// Look up the variable in config.secrets
		envName := as.lookupTemplatedVariable(varName, server)
		if envName == "" {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Variable %s not found or not templated, skipping\n", varName)
			continue
		}

		// Apply the substitution function
		substitution := substituteFunc(varName, envName)
		templatePattern := fmt.Sprintf("{{%s}}", varName)
		result = strings.ReplaceAll(result, templatePattern, substitution)

		fmt.Printf("ADAPTER_SERVICE_DEBUG: Replaced %s with %s\n", templatePattern, substitution)
	}

	return result
}

// lookupTemplatedVariable looks up a variable name in the server's secrets
func (as *AdapterService) lookupTemplatedVariable(varName string, server *models.MCPServer) string {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: lookupTemplatedVariable called for varName: %s\n", varName)

	if server.Meta == nil {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: server.Meta is nil\n")
		return ""
	}

	// First try direct secrets (new format)
	secretsRaw, ok := server.Meta["secrets"]
	if !ok {
		// Fall back to config.secrets (old format)
		fmt.Printf("ADAPTER_SERVICE_DEBUG: secrets not found directly, trying config.secrets\n")
		configRaw, ok := server.Meta["config"]
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: config not found in server.Meta, available keys: %+v\n", getMapKeys(server.Meta))
			return ""
		}

		configMap, ok := configRaw.(map[string]interface{})
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: config is not a map, type: %T\n", configRaw)
			return ""
		}

		secretsRaw, ok = configMap["secrets"]
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: secrets not found in config, available config keys: %+v\n", getMapKeys(configMap))
			return ""
		}
	}

	secretsSlice, ok := secretsRaw.([]interface{})
	if !ok {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: secrets is not a slice, type: %T, value: %+v\n", secretsRaw, secretsRaw)
		return ""
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: Found %d secrets\n", len(secretsSlice))

	for _, secretRaw := range secretsSlice {
		secretMap, ok := secretRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this secret matches the variable name
		name, ok := secretMap["name"].(string)
		if !ok || name != varName {
			continue
		}

		// Note: We now process all variables, not just templated ones

		// Get the environment variable name
		envName, ok := secretMap["env"].(string)
		if !ok {
			fmt.Printf("ADAPTER_SERVICE_DEBUG: Variable %s missing env field\n", varName)
			return ""
		}

		fmt.Printf("ADAPTER_SERVICE_DEBUG: Found templated variable %s -> %s\n", varName, envName)
		return envName
	}

	return ""
}

// getSidecarConfig extracts the complete sidecar configuration from server metadata
func (as *AdapterService) getSidecarConfig(server *models.MCPServer) *models.SidecarConfig {
	fmt.Printf("ADAPTER_SERVICE_DEBUG: getSidecarConfig called for server %s\n", server.Name)

	if server.Meta == nil {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: server.Meta is nil\n")
		return nil
	}

	sidecarConfigRaw, ok := server.Meta["sidecarConfig"]
	if !ok {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig not found in meta\n")
		return nil
	}

	configMap, ok := sidecarConfigRaw.(map[string]interface{})
	if !ok {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig is not a map, type: %T, value: %v\n", sidecarConfigRaw, sidecarConfigRaw)
		return nil
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: sidecarConfig keys: %v\n", getMapKeys(configMap))

	commandType, ok := configMap["commandType"].(string)
	if !ok || commandType == "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: commandType not found or empty\n")
		return nil
	}

	command, ok := configMap["command"].(string)
	if !ok || command == "" {
		fmt.Printf("ADAPTER_SERVICE_DEBUG: command not found or empty\n")
		return nil
	}

	port := 8000 // Default port
	if portRaw, ok := configMap["port"]; ok {
		if portFloat, ok := portRaw.(float64); ok {
			port = int(portFloat)
		}
	}

	sidecarConfig := &models.SidecarConfig{
		CommandType: commandType,
		Command:     command,
		Port:        port,
	}

	// Extract source and lastUpdated if available
	if source, ok := configMap["source"].(string); ok {
		sidecarConfig.Source = source
	}
	if lastUpdated, ok := configMap["lastUpdated"].(string); ok {
		sidecarConfig.LastUpdated = lastUpdated
	}

	// Extract project URL and release URL from server source_info metadata
	// The source information is stored in Meta["source_info"] during YAML parsing
	if sourceInfo, ok := server.Meta["source_info"]; ok {
		if sourceMap, ok := sourceInfo.(map[string]interface{}); ok {
			if project, ok := sourceMap["project"].(string); ok && project != "" {
				sidecarConfig.ProjectURL = project
				fmt.Printf("ADAPTER_SERVICE_DEBUG: Found project URL: %s\n", project)
			}
			if release, ok := sourceMap["release"].(string); ok && release != "" {
				sidecarConfig.ReleaseURL = release
				fmt.Printf("ADAPTER_SERVICE_DEBUG: Found release URL: %s\n", release)
			}
		}
	}

	fmt.Printf("ADAPTER_SERVICE_DEBUG: Created sidecar config: %+v\n", sidecarConfig)
	return sidecarConfig
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

// CheckAdapterHealth checks and updates the health status of an adapter
func (as *AdapterService) CheckAdapterHealth(ctx context.Context, userID, adapterID string, userGroupService *services.UserGroupService) error {
	// Get adapter
	adapter, err := as.GetAdapter(ctx, userID, adapterID, userGroupService)
	if err != nil {
		return err
	}

	// Check and update sidecar health
	if err := as.checkAndUpdateSidecarHealth(ctx, adapter); err != nil {
		return fmt.Errorf("failed to check adapter health: %w", err)
	}

	return nil
}

// discoverCapabilities discovers MCP capabilities for an adapter
// discoverCapabilities sets basic capabilities for adapters without sidecars
func (as *AdapterService) discoverCapabilities(ctx context.Context, adapterData *models.AdapterData) error {
	// For adapters without sidecars (remote connections), set basic capabilities
	adapterData.MCPFunctionality = &models.MCPFunctionality{
		ServerInfo: models.MCPServerInfo{
			Name:    adapterData.Name,
			Version: "1.0.0",
		},
		Tools:         []models.MCPTool{},
		Resources:     []models.MCPResource{},
		Prompts:       []models.MCPPrompt{},
		LastRefreshed: time.Now(),
	}

	return nil
}

// discoverCapabilitiesFromSidecar discovers capabilities from a deployed sidecar
func (as *AdapterService) discoverCapabilitiesFromSidecar(ctx context.Context, adapter *models.AdapterResource) error {
	logging.AdapterLogger.Info("Starting capability discovery for sidecar adapter %s", adapter.ID)

	// Use the internal sidecar service URL instead of the external proxy URL
	sidecarServiceURL := fmt.Sprintf("http://mcp-sidecar-%s.suseai.svc.cluster.local:8000", adapter.ID)
	logging.AdapterLogger.Info("Using sidecar service URL: %s", sidecarServiceURL)

	// For sidecar communication, we don't need authentication since it's internal cluster communication
	auth := (*models.AdapterAuthConfig)(nil) // No auth needed for internal service calls

	// First, try a health check to see if the sidecar is responding
	if err := as.healthCheckSidecar(ctx, sidecarServiceURL); err != nil {
		logging.AdapterLogger.Warn("Sidecar health check failed for adapter %s: %v", adapter.ID, err)
		// Continue with discovery attempt anyway
	} else {
		logging.AdapterLogger.Info("Sidecar health check passed for adapter %s", adapter.ID)
	}

	// Use the capability discovery service to get real tools with retry logic
	logging.AdapterLogger.Info("Calling MCP capability discovery service for adapter %s", adapter.ID)
	capabilities, err := as.discoverCapabilitiesWithRetry(ctx, sidecarServiceURL, auth)
	if err != nil {
		logging.AdapterLogger.Warn("MCP capability discovery failed for adapter %s: %v", adapter.ID, err)

		// Try to get basic server info as fallback
		logging.AdapterLogger.Info("Attempting to get basic server info as fallback for adapter %s", adapter.ID)
		serverInfo, infoErr := as.getBasicServerInfo(ctx, sidecarServiceURL, auth)
		if infoErr != nil {
			logging.AdapterLogger.Warn("Basic server info discovery also failed for adapter %s: %v", adapter.ID, infoErr)
			// Set minimal capabilities
			adapter.MCPFunctionality = &models.MCPFunctionality{
				ServerInfo: models.MCPServerInfo{
					Name:    adapter.Name,
					Version: "1.0.0",
				},
				Tools:         []models.MCPTool{},
				Resources:     []models.MCPResource{},
				Prompts:       []models.MCPPrompt{},
				LastRefreshed: time.Now(),
			}
			logging.AdapterLogger.Info("Set minimal capabilities for adapter %s due to discovery failures", adapter.ID)
			return nil
		}

		// Set capabilities with discovered server info but no tools
		adapter.MCPFunctionality = &models.MCPFunctionality{
			ServerInfo:    *serverInfo,
			Tools:         []models.MCPTool{},
			Resources:     []models.MCPResource{},
			Prompts:       []models.MCPPrompt{},
			LastRefreshed: time.Now(),
		}
		logging.AdapterLogger.Info("Set server info capabilities for adapter %s (%d tools found via basic discovery)", adapter.ID, len(capabilities.Tools))
		return nil
	}

	// Update adapter with discovered capabilities
	adapter.MCPFunctionality = capabilities
	logging.AdapterLogger.Success("Successfully discovered capabilities for adapter %s: %d tools, %d resources, %d prompts",
		adapter.ID, len(capabilities.Tools), len(capabilities.Resources), len(capabilities.Prompts))

	return nil
}

// discoverCapabilitiesWithRetry attempts capability discovery with exponential backoff retry
func (as *AdapterService) discoverCapabilitiesWithRetry(ctx context.Context, serverURL string, auth *models.AdapterAuthConfig) (*models.MCPFunctionality, error) {
	maxRetries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * baseDelay
			logging.AdapterLogger.Info("Retrying capability discovery in %v (attempt %d/%d)", delay, attempt+1, maxRetries)
			time.Sleep(delay)
		}

		capabilities, err := mcp.NewCapabilityDiscoveryService().DiscoverCapabilities(ctx, serverURL, auth)
		if err == nil {
			return capabilities, nil
		}

		logging.AdapterLogger.Warn("Capability discovery attempt %d/%d failed: %v", attempt+1, maxRetries, err)

		// If this is the last attempt, return the error
		if attempt == maxRetries-1 {
			return nil, err
		}
	}

	// This should never be reached, but just in case
	return nil, fmt.Errorf("capability discovery failed after %d attempts", maxRetries)
}

// healthCheckSidecar performs a basic health check on the sidecar
func (as *AdapterService) healthCheckSidecar(ctx context.Context, serverURL string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", serverURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// getBasicServerInfo attempts to get basic server information
func (as *AdapterService) getBasicServerInfo(ctx context.Context, serverURL string, auth *models.AdapterAuthConfig) (*models.MCPServerInfo, error) {
	// For internal cluster communication, create client without auth
	client := mcp.NewMCPClient(serverURL, nil)

	if err := client.Initialize(ctx); err != nil {
		return nil, err
	}
	defer client.Close()

	return client.GetServerInfo(ctx)
}

// checkAndUpdateSidecarHealth checks the health of sidecar deployments and updates adapter status
func (as *AdapterService) checkAndUpdateSidecarHealth(ctx context.Context, adapter *models.AdapterResource) error {
	if adapter.SidecarConfig == nil || adapter.ConnectionType != models.ConnectionTypeStreamableHttp {
		// Non-sidecar adapters are always ready
		return nil
	}

	if as.sidecarManager == nil {
		logging.AdapterLogger.Warn("SidecarManager not available for health check of adapter %s", adapter.ID)
		return nil
	}

	// Check if sidecar is healthy (this would need to be implemented in SidecarManager)
	// For now, we'll check if we can reach the sidecar service
	sidecarServiceURL := fmt.Sprintf("http://mcp-sidecar-%s.suseai.svc.cluster.local:8000", adapter.ID)

	healthy, err := as.isSidecarHealthy(ctx, sidecarServiceURL)
	if err != nil {
		logging.AdapterLogger.Warn("Failed to check sidecar health for adapter %s: %v", adapter.ID, err)
		// Don't change status on check failure - might be temporary network issue
		return nil
	}

	// Update status based on health
	oldStatus := adapter.Status
	if healthy {
		if adapter.Status == models.AdapterLifecycleStatusError {
			logging.AdapterLogger.Info("Sidecar for adapter %s is now healthy, changing status from error to ready", adapter.ID)
			adapter.Status = models.AdapterLifecycleStatusReady
		}
	} else {
		if adapter.Status == models.AdapterLifecycleStatusReady {
			logging.AdapterLogger.Warn("Sidecar for adapter %s is unhealthy, changing status from ready to error", adapter.ID)
			adapter.Status = models.AdapterLifecycleStatusError
		}
	}

	// Update in store if status changed
	if oldStatus != adapter.Status {
		if err := as.store.Update(ctx, *adapter); err != nil {
			logging.AdapterLogger.Error("Failed to update adapter status for %s: %v", adapter.ID, err)
			return err
		}
		logging.AdapterLogger.Info("Updated adapter %s status from %s to %s", adapter.ID, oldStatus, adapter.Status)
	}

	return nil
}

// isSidecarHealthy checks if the sidecar service is responding
func (as *AdapterService) isSidecarHealthy(ctx context.Context, serviceURL string) (bool, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL+"/health", nil)
	if err != nil {
		return false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
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
