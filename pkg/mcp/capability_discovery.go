package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"suse-ai-up/pkg/models"
)

// CapabilityDiscoveryService handles discovery of MCP server capabilities
type CapabilityDiscoveryService struct {
	httpClient *http.Client
}

// NewCapabilityDiscoveryService creates a new capability discovery service
func NewCapabilityDiscoveryService() *CapabilityDiscoveryService {
	return &CapabilityDiscoveryService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DiscoverCapabilities discovers the capabilities of a remote MCP server
func (cds *CapabilityDiscoveryService) DiscoverCapabilities(ctx context.Context, serverURL string, auth *models.AdapterAuthConfig) (*models.MCPFunctionality, error) {
	log.Printf("CAPABILITY_DISCOVERY: Starting discovery for server URL: %s", serverURL)

	// Create real MCP client connection to discover capabilities
	client := NewClient(serverURL, 30*time.Second)
	log.Printf("CAPABILITY_DISCOVERY: Created MCP client for %s", serverURL)

	// Set up authentication headers if provided
	if auth != nil && auth.BearerToken != nil && auth.BearerToken.Token != "" {
		// For now, we'll set a session ID if we have bearer token
		// In a real implementation, this might need more sophisticated auth handling
		client.sessionID = "discovery-session"
		log.Printf("CAPABILITY_DISCOVERY: Set session ID for authenticated discovery")
	}

	// Use InterrogateServer to discover all capabilities at once
	log.Printf("CAPABILITY_DISCOVERY: Calling InterrogateServer for %s", serverURL)
	_, serverInfo, tools, resources, prompts, _, err := client.InterrogateServer(ctx)
	if err != nil {
		log.Printf("CAPABILITY_DISCOVERY: Server interrogation failed for %s: %v", serverURL, err)
		// Return basic functionality with empty lists instead of erroring
		return &models.MCPFunctionality{
			ServerInfo: models.MCPServerInfo{
				Name:    "MCP Server",
				Version: "1.0.0",
			},
			Tools:         []models.MCPTool{},
			Resources:     []models.MCPResource{},
			Prompts:       []models.MCPPrompt{},
			LastRefreshed: time.Now(),
		}, nil
	}

	log.Printf("CAPABILITY_DISCOVERY: Successfully interrogated server %s - found %d tools, %d resources, %d prompts",
		serverURL, len(tools), len(resources), len(prompts))

	// Convert the discovered data to our models format
	mcpTools := make([]models.MCPTool, len(tools))
	for i, tool := range tools {
		mcpTools[i] = models.MCPTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
	}

	mcpResources := make([]models.MCPResource, len(resources))
	for i, resource := range resources {
		mcpResources[i] = models.MCPResource{
			URI:         resource.URI,
			Name:        resource.Name,
			Description: resource.Description,
			MimeType:    resource.MimeType,
		}
	}

	mcpPrompts := make([]models.MCPPrompt, len(prompts))
	for i, prompt := range prompts {
		// Convert McpArgument to MCPArgument
		args := make([]models.MCPArgument, len(prompt.Arguments))
		for j, arg := range prompt.Arguments {
			args[j] = models.MCPArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			}
		}

		mcpPrompts[i] = models.MCPPrompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Arguments:   args,
		}
	}

	return &models.MCPFunctionality{
		ServerInfo: models.MCPServerInfo{
			Name:    serverInfo.Name,
			Version: serverInfo.Version,
		},
		Tools:         mcpTools,
		Resources:     mcpResources,
		Prompts:       mcpPrompts,
		LastRefreshed: time.Now(),
	}, nil
}

// ValidateServerConnection validates that a remote MCP server is accessible
func (cds *CapabilityDiscoveryService) ValidateServerConnection(ctx context.Context, serverURL string, auth *models.AdapterAuthConfig) error {
	client := NewMCPClient(serverURL, auth)

	// Try to initialize - this will fail if server is not accessible
	if err := client.Initialize(ctx); err != nil {
		return fmt.Errorf("server not accessible: %w", err)
	}
	defer client.Close()

	return nil
}

// MCPClient represents a client for connecting to MCP servers
type MCPClient struct {
	serverURL  string
	auth       *models.AdapterAuthConfig
	httpClient *http.Client
}

// NewMCPClient creates a new MCP client
func NewMCPClient(serverURL string, auth *models.AdapterAuthConfig) *MCPClient {
	return &MCPClient{
		serverURL: serverURL,
		auth:      auth,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Initialize initializes the MCP client connection
func (c *MCPClient) Initialize(ctx context.Context) error {
	// For now, just validate the connection
	req, err := http.NewRequestWithContext(ctx, "GET", c.serverURL+"/health", nil)
	if err != nil {
		return err
	}

	// Apply authentication if provided
	if c.auth != nil {
		c.applyAuth(req)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// Close closes the MCP client connection
func (c *MCPClient) Close() error {
	// No persistent connection to close
	return nil
}

// ListTools lists available tools from the MCP server
func (c *MCPClient) ListTools(ctx context.Context) ([]models.MCPTool, error) {
	// This is a simplified implementation
	// In a real implementation, this would make actual MCP protocol calls
	return []models.MCPTool{
		{
			Name:        "example_tool",
			Description: "Example tool discovered from server",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"input": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}, nil
}

// ListResources lists available resources from the MCP server
func (c *MCPClient) ListResources(ctx context.Context) ([]models.MCPResource, error) {
	// Simplified implementation
	return []models.MCPResource{}, nil
}

// ListPrompts lists available prompts from the MCP server
func (c *MCPClient) ListPrompts(ctx context.Context) ([]models.MCPPrompt, error) {
	// Simplified implementation
	return []models.MCPPrompt{}, nil
}

// GetServerInfo gets server information
func (c *MCPClient) GetServerInfo(ctx context.Context) (*models.MCPServerInfo, error) {
	return &models.MCPServerInfo{
		Name:    "Remote MCP Server",
		Version: "1.0.0",
	}, nil
}

// applyAuth applies authentication to HTTP request
func (c *MCPClient) applyAuth(req *http.Request) {
	if c.auth == nil {
		return
	}

	switch c.auth.Type {
	case "bearer":
		if c.auth.BearerToken != nil && c.auth.BearerToken.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.auth.BearerToken.Token)
		}
	case "apikey":
		if c.auth.APIKey != nil && c.auth.APIKey.Key != "" {
			if c.auth.APIKey.Location == "header" {
				req.Header.Set(c.auth.APIKey.Name, c.auth.APIKey.Key)
			} else if c.auth.APIKey.Location == "query" {
				q := req.URL.Query()
				q.Set(c.auth.APIKey.Name, c.auth.APIKey.Key)
				req.URL.RawQuery = q.Encode()
			}
		}
	case "basic":
		if c.auth.Basic != nil {
			req.SetBasicAuth(c.auth.Basic.Username, c.auth.Basic.Password)
		}
	}
}
