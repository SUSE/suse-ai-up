package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"suse-ai-up/pkg/models"
)

// MCPToolDiscoveryService handles discovery of tools from MCP servers
type MCPToolDiscoveryService struct {
	httpClient *http.Client
}

// NewMCPToolDiscoveryService creates a new tool discovery service
func NewMCPToolDiscoveryService() *MCPToolDiscoveryService {
	return &MCPToolDiscoveryService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DiscoverTools queries an MCP server for its available tools
func (s *MCPToolDiscoveryService) DiscoverTools(ctx context.Context, serverURL string, auth *models.AdapterAuthConfig) ([]models.MCPTool, error) {
	log.Printf("MCPToolDiscovery: Discovering tools from server: %s", serverURL)

	// Validate input parameters
	if serverURL == "" {
		return nil, fmt.Errorf("server URL cannot be empty")
	}

	// Create tools/list request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request with timeout
	req, err := http.NewRequestWithContext(ctx, "POST", serverURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "suse-ai-up-tool-discovery/1.0")

	// Add authentication if required
	if auth != nil && auth.Required && auth.Type == "bearer" && auth.BearerToken != nil && auth.BearerToken.Token != "" {
		req.Header.Set("Authorization", "Bearer "+auth.BearerToken.Token)
	}

	// Make the request with error handling
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server at %s: %w", serverURL, err)
	}
	defer resp.Body.Close()

	// Read response with size limit to prevent memory exhaustion
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to extract error message from response body
		var errorResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errorResp) == nil && errorResp.Error.Message != "" {
			return nil, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, errorResp.Error.Message)
		}
		return nil, fmt.Errorf("MCP server returned HTTP %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Tools []struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				InputSchema map[string]interface{} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w (response: %s)", err, string(body)[:500])
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP server returned JSON-RPC error %d: %s", response.Error.Code, response.Error.Message)
	}

	// Validate response structure
	if response.JSONRPC != "2.0" {
		return nil, fmt.Errorf("invalid JSON-RPC version: expected '2.0', got '%s'", response.JSONRPC)
	}

	// Convert to MCPTool format with validation
	tools := make([]models.MCPTool, 0, len(response.Result.Tools))
	for i, tool := range response.Result.Tools {
		// Validate required fields
		if tool.Name == "" {
			log.Printf("MCPToolDiscovery: Warning: tool at index %d has empty name, skipping", i)
			continue
		}

		// Create validated tool
		mcpTool := models.MCPTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			SourceType:  "api", // Default to API for discovered tools
		}

		// Basic validation of input schema
		if mcpTool.InputSchema == nil {
			mcpTool.InputSchema = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}

		tools = append(tools, mcpTool)
	}

	log.Printf("MCPToolDiscovery: Successfully discovered %d valid tools from server", len(tools))
	return tools, nil
}

// DiscoverServerCapabilities queries an MCP server for its capabilities and tools
func (s *MCPToolDiscoveryService) DiscoverServerCapabilities(ctx context.Context, serverURL string, auth *models.AdapterAuthConfig) (*models.MCPFunctionality, error) {
	log.Printf("MCPToolDiscovery: Discovering capabilities from server: %s", serverURL)

	// Validate input
	if serverURL == "" {
		return nil, fmt.Errorf("server URL cannot be empty")
	}

	// First, initialize the connection to get server info
	initRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "suse-ai-up-discovery",
				"version": "1.0.0",
			},
		},
	}

	initBody, err := json.Marshal(initRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal initialize request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL, bytes.NewReader(initBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create initialize request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "suse-ai-up-tool-discovery/1.0")

	if auth != nil && auth.Required && auth.Type == "bearer" && auth.BearerToken != nil && auth.BearerToken.Token != "" {
		req.Header.Set("Authorization", "Bearer "+auth.BearerToken.Token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server for initialization: %w", err)
	}
	defer resp.Body.Close()

	initRespBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit for init response
	if err != nil {
		return nil, fmt.Errorf("failed to read initialize response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("initialize request failed with HTTP %d", resp.StatusCode)
	}

	var initResponse struct {
		Result struct {
			ProtocolVersion string                 `json:"protocolVersion"`
			Capabilities    map[string]interface{} `json:"capabilities"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(initRespBody, &initResponse); err != nil {
		return nil, fmt.Errorf("failed to parse initialize response: %w", err)
	}

	if initResponse.Error != nil {
		return nil, fmt.Errorf("MCP server initialize error %d: %s", initResponse.Error.Code, initResponse.Error.Message)
	}

	// Validate server info
	serverName := initResponse.Result.ServerInfo.Name
	if serverName == "" {
		serverName = "Unknown MCP Server"
	}

	serverVersion := initResponse.Result.ServerInfo.Version
	if serverVersion == "" {
		serverVersion = "1.0.0"
	}

	// Now discover tools
	tools, err := s.DiscoverTools(ctx, serverURL, auth)
	if err != nil {
		log.Printf("MCPToolDiscovery: Failed to discover tools, continuing with empty tools list: %v", err)
		tools = []models.MCPTool{}
	}

	// Create MCPFunctionality
	functionality := &models.MCPFunctionality{
		ServerInfo: models.MCPServerInfo{
			Name:         serverName,
			Version:      serverVersion,
			Protocol:     initResponse.Result.ProtocolVersion,
			Capabilities: initResponse.Result.Capabilities,
		},
		Tools:         tools,
		Prompts:       []models.MCPPrompt{},   // Not discovering prompts for now
		Resources:     []models.MCPResource{}, // Not discovering resources for now
		LastRefreshed: time.Now(),
	}

	log.Printf("MCPToolDiscovery: Successfully discovered capabilities for server '%s': %d tools, protocol %s",
		serverName, len(tools), initResponse.Result.ProtocolVersion)
	return functionality, nil
}
