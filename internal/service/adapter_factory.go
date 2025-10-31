package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
)

// RegistryClientInterface defines the interface for communicating with the registry service
type RegistryClientInterface interface {
	GetServer(serverID string) (*RegistryServer, error)
	SearchServers(query string, filters map[string]interface{}) ([]*RegistryServer, error)
}

// RegistryServer represents a server from the registry (simplified for proxy use)
type RegistryServer struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Packages    []RegistryPackage      `json:"packages"`
	Meta        map[string]interface{} `json:"_meta,omitempty"` // Registry metadata
}

// RegistryPackage represents a package from the registry
type RegistryPackage struct {
	RegistryType         string                        `json:"registryType"`
	Identifier           string                        `json:"identifier"`
	Transport            RegistryTransport             `json:"transport"`
	EnvironmentVariables []RegistryEnvironmentVariable `json:"environmentVariables,omitempty"`
}

// RegistryTransport represents transport information
type RegistryTransport struct {
	Type string `json:"type"`
}

// RegistryEnvironmentVariable represents an environment variable
type RegistryEnvironmentVariable struct {
	Name     string `json:"name"`
	IsSecret bool   `json:"isSecret,omitempty"`
}

// AdapterFactory creates proxy adapters from registry entries
type AdapterFactory struct {
	adapterStore   clients.AdapterResourceStore
	registryClient RegistryClientInterface
}

// NewAdapterFactory creates a new adapter factory
func NewAdapterFactory(adapterStore clients.AdapterResourceStore, registryClient RegistryClientInterface) *AdapterFactory {
	return &AdapterFactory{
		adapterStore:   adapterStore,
		registryClient: registryClient,
	}
}

// CreateAdapterFromRegistry creates an adapter from a registry server entry
func (af *AdapterFactory) CreateAdapterFromRegistry(serverID string, adapterName string) (*models.AdapterResource, error) {
	log.Printf("AdapterFactory: Creating adapter %s from registry server %s", adapterName, serverID)

	// Get server from registry
	server, err := af.registryClient.GetServer(serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server from registry: %w", err)
	}

	// Convert to adapter data
	adapterData, err := af.convertRegistryToAdapter(server, adapterName)
	if err != nil {
		return nil, fmt.Errorf("failed to convert registry server to adapter: %w", err)
	}

	// Create adapter resource
	adapter := &models.AdapterResource{}
	adapter.Create(*adapterData, "registry-import", adapter.CreatedAt)

	return adapter, nil
}

// convertRegistryToAdapter converts a registry server to adapter data
func (af *AdapterFactory) convertRegistryToAdapter(server *RegistryServer, adapterName string) (*models.AdapterData, error) {
	adapterData := &models.AdapterData{
		Name:                 adapterName,
		Description:          server.Description,
		Protocol:             models.ServerProtocolMCP,
		EnvironmentVariables: make(map[string]string),
		ReplicaCount:         1,
		UseWorkloadIdentity:  false,
	}

	// Check if this is a local MCP server
	if server.Meta != nil {
		if isLocalMCP, ok := server.Meta["isLocalMCP"].(bool); ok && isLocalMCP {
			return af.convertLocalMCPToAdapter(server, adapterData)
		}
	}

	// Handle standard registry servers
	if len(server.Packages) == 0 {
		return nil, fmt.Errorf("server has no packages")
	}

	// Use the first package (in a full implementation, user might choose)
	pkg := server.Packages[0]

	// Convert transport type
	switch pkg.Transport.Type {
	case "stdio":
		adapterData.ConnectionType = models.ConnectionTypeLocalStdio
		if err := af.configureStdioAdapter(adapterData, &pkg); err != nil {
			return nil, err
		}
	case "sse":
		adapterData.ConnectionType = models.ConnectionTypeSSE
		if err := af.configureSSEAdapter(adapterData, &pkg); err != nil {
			return nil, err
		}
	case "websocket":
		adapterData.ConnectionType = models.ConnectionTypeRemoteHttp
		if err := af.configureWebSocketAdapter(adapterData, &pkg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", pkg.Transport.Type)
	}

	// Convert environment variables
	for _, env := range pkg.EnvironmentVariables {
		if env.IsSecret {
			// For secrets, we'll use a placeholder that needs to be configured
			adapterData.EnvironmentVariables[env.Name] = fmt.Sprintf("***SECRET:%s***", env.Name)
		} else {
			// For non-secrets, we might have defaults or leave empty
			adapterData.EnvironmentVariables[env.Name] = ""
		}
	}

	return adapterData, nil
}

// convertLocalMCPToAdapter converts a local MCP server to adapter data
func (af *AdapterFactory) convertLocalMCPToAdapter(server *RegistryServer, adapterData *models.AdapterData) (*models.AdapterData, error) {
	adapterData.ConnectionType = models.ConnectionTypeLocalStdio

	// Extract MCP client config from metadata
	if mcpClientConfigRaw, ok := server.Meta["mcpClientConfig"]; ok {
		if mcpClientConfigMap, ok := mcpClientConfigRaw.(map[string]interface{}); ok {
			// Convert to MCPClientConfig struct
			configJSON, err := json.Marshal(mcpClientConfigMap)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal MCP client config: %w", err)
			}

			var mcpClientConfig models.MCPClientConfig
			if err := json.Unmarshal(configJSON, &mcpClientConfig); err != nil {
				return nil, fmt.Errorf("failed to unmarshal MCP client config: %w", err)
			}

			adapterData.MCPClientConfig = mcpClientConfig
		}
	}

	return adapterData, nil
}

// configureStdioAdapter configures an adapter for stdio transport
func (af *AdapterFactory) configureStdioAdapter(adapterData *models.AdapterData, pkg *RegistryPackage) error {
	switch pkg.RegistryType {
	case "oci":
		// For OCI images, extract image name from identifier
		// Format: docker.io/user/image:tag or docker.io/user/image
		parts := strings.Split(pkg.Identifier, "/")
		if len(parts) >= 2 {
			imageName := strings.Join(parts[1:], "/")
			// Remove tag if present
			if colonIndex := strings.LastIndex(imageName, ":"); colonIndex != -1 {
				imageName = imageName[:colonIndex]
			}
			adapterData.ImageName = imageName
			adapterData.ImageVersion = "latest" // Default, user can change
		} else {
			return fmt.Errorf("invalid OCI identifier format: %s", pkg.Identifier)
		}
	case "npm":
		// For npm packages, we might need to run them via npx or similar
		// This is more complex and would require additional configuration
		return fmt.Errorf("npm registry type not yet supported for stdio")
	default:
		return fmt.Errorf("unsupported registry type for stdio: %s", pkg.RegistryType)
	}

	return nil
}

// configureSSEAdapter configures an adapter for SSE transport
func (af *AdapterFactory) configureSSEAdapter(adapterData *models.AdapterData, pkg *RegistryPackage) error {
	// For SSE, the identifier might be a URL or we need to extract endpoint info
	// This is simplified - in practice, we'd need more metadata
	if strings.HasPrefix(pkg.Identifier, "http") {
		adapterData.RemoteUrl = pkg.Identifier
	} else {
		// Assume it's an image that serves SSE
		adapterData.ImageName = pkg.Identifier
		adapterData.ImageVersion = "latest"
	}

	return nil
}

// configureWebSocketAdapter configures an adapter for WebSocket transport
func (af *AdapterFactory) configureWebSocketAdapter(adapterData *models.AdapterData, pkg *RegistryPackage) error {
	// Similar to SSE but for WebSocket connections
	if strings.HasPrefix(pkg.Identifier, "ws") {
		adapterData.RemoteUrl = pkg.Identifier
	} else {
		// Assume it's an image that serves WebSocket
		adapterData.ImageName = pkg.Identifier
		adapterData.ImageVersion = "latest"
	}

	return nil
}

// InstallServerFromRegistry creates and deploys an adapter from a registry server
func (af *AdapterFactory) InstallServerFromRegistry(serverID, adapterName string) error {
	log.Printf("AdapterFactory: Installing server %s as adapter %s", serverID, adapterName)

	// Create adapter
	adapter, err := af.CreateAdapterFromRegistry(serverID, adapterName)
	if err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}

	// Store adapter (this would trigger deployment in a full implementation)
	if err := af.adapterStore.UpsertAsync(*adapter, context.Background()); err != nil {
		return fmt.Errorf("failed to store adapter: %w", err)
	}

	log.Printf("AdapterFactory: Successfully installed adapter %s from registry server %s", adapterName, serverID)
	return nil
}

// ListAvailableServers returns available servers from the registry
func (af *AdapterFactory) ListAvailableServers(query string, filters map[string]interface{}) ([]*RegistryServer, error) {
	return af.registryClient.SearchServers(query, filters)
}
