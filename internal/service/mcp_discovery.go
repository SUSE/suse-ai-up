package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
)

// MCPDiscoveryService handles discovery of MCP server capabilities
type MCPDiscoveryService struct {
	httpClient *http.Client
}

// NewMCPDiscoveryService creates a new MCP discovery service
func NewMCPDiscoveryService() *MCPDiscoveryService {
	return &MCPDiscoveryService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// DiscoverCapabilities discovers MCP server capabilities from the given adapter
func (mds *MCPDiscoveryService) DiscoverCapabilities(adapter models.AdapterResource) (*models.MCPFunctionality, error) {
	log.Printf("MCPDiscoveryService: Discovering capabilities for adapter %s", adapter.Name)

	// Determine the MCP endpoint URL
	mcpURL, err := mds.getMCPURL(adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to determine MCP URL: %w", err)
	}

	// Initialize the MCP server
	serverInfo, err := mds.initializeServer(mcpURL, adapter.Authentication)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP server: %w", err)
	}

	// Discover tools, prompts, and resources
	tools, err := mds.discoverTools(mcpURL, adapter.Authentication)
	if err != nil {
		log.Printf("MCPDiscoveryService: Failed to discover tools: %v", err)
		// Continue even if tools discovery fails
	}

	prompts, err := mds.discoverPrompts(mcpURL, adapter.Authentication)
	if err != nil {
		log.Printf("MCPDiscoveryService: Failed to discover prompts: %v", err)
		// Continue even if prompts discovery fails
	}

	resources, err := mds.discoverResources(mcpURL, adapter.Authentication)
	if err != nil {
		log.Printf("MCPDiscoveryService: Failed to discover resources: %v", err)
		// Continue even if resources discovery fails
	}

	functionality := &models.MCPFunctionality{
		ServerInfo:    *serverInfo,
		Tools:         tools,
		Prompts:       prompts,
		Resources:     resources,
		LastRefreshed: time.Now(),
	}

	log.Printf("MCPDiscoveryService: Discovered %d tools, %d prompts, %d resources for adapter %s",
		len(tools), len(prompts), len(resources), adapter.Name)

	return functionality, nil
}

// getMCPURL determines the MCP endpoint URL for the adapter
func (mds *MCPDiscoveryService) getMCPURL(adapter models.AdapterResource) (string, error) {
	switch adapter.ConnectionType {
	case models.ConnectionTypeRemoteHttp:
		if adapter.RemoteUrl == "" {
			return "", fmt.Errorf("remote URL is required for RemoteHttp connection type")
		}
		// Don't append /mcp if it's already included
		url := strings.TrimSuffix(adapter.RemoteUrl, "/")
		if !strings.HasSuffix(url, "/mcp") {
			url += "/mcp"
		}
		return url, nil
	case models.ConnectionTypeStreamableHttp:
		// For deployed adapters, use the proxy endpoint
		return fmt.Sprintf("http://localhost:8911/adapters/%s/mcp", adapter.Name), nil
	default:
		return "", fmt.Errorf("unsupported connection type for MCP discovery: %s", adapter.ConnectionType)
	}
}

// initializeServer initializes the MCP server and returns server info
func (mds *MCPDiscoveryService) initializeServer(mcpURL string, auth *models.AdapterAuthConfig) (*models.MCPServerInfo, error) {
	// Create initialize request
	initRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "mcp-discovery",
				"version": "1.0.0",
			},
		},
	}

	// Send request
	response, err := mds.sendMCPRequest(mcpURL, initRequest, auth)
	if err != nil {
		return nil, err
	}

	// Parse response
	var result map[string]interface{}
	if response["result"] != nil {
		result = response["result"].(map[string]interface{})
	} else {
		return nil, fmt.Errorf("initialize response missing result")
	}

	// Extract server info
	serverInfoData, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("server info not found in initialize response")
	}

	serverInfo := &models.MCPServerInfo{
		Name:         getString(serverInfoData, "name", "Unknown Server"),
		Version:      getString(serverInfoData, "version", "unknown"),
		Protocol:     getString(serverInfoData, "protocol", "unknown"),
		Capabilities: make(map[string]interface{}),
	}

	if caps, ok := serverInfoData["capabilities"].(map[string]interface{}); ok {
		serverInfo.Capabilities = caps
	}

	return serverInfo, nil
}

// discoverTools discovers available tools from the MCP server
func (mds *MCPDiscoveryService) discoverTools(mcpURL string, auth *models.AdapterAuthConfig) ([]models.MCPTool, error) {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	response, err := mds.sendMCPRequest(mcpURL, request, auth)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if response["result"] != nil {
		result = response["result"].(map[string]interface{})
	} else {
		return nil, fmt.Errorf("tools/list response missing result")
	}

	toolsData, ok := result["tools"].([]interface{})
	if !ok {
		return []models.MCPTool{}, nil // No tools available
	}

	var tools []models.MCPTool
	for _, toolData := range toolsData {
		toolMap, ok := toolData.(map[string]interface{})
		if !ok {
			continue
		}

		tool := models.MCPTool{
			Name:        getString(toolMap, "name", ""),
			Description: getString(toolMap, "description", ""),
		}

		if inputSchema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
			tool.InputSchema = inputSchema
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

// discoverPrompts discovers available prompts from the MCP server
func (mds *MCPDiscoveryService) discoverPrompts(mcpURL string, auth *models.AdapterAuthConfig) ([]models.MCPPrompt, error) {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "prompts/list",
		"params":  map[string]interface{}{},
	}

	response, err := mds.sendMCPRequest(mcpURL, request, auth)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if response["result"] != nil {
		result = response["result"].(map[string]interface{})
	} else {
		return nil, fmt.Errorf("prompts/list response missing result")
	}

	promptsData, ok := result["prompts"].([]interface{})
	if !ok {
		return []models.MCPPrompt{}, nil // No prompts available
	}

	var prompts []models.MCPPrompt
	for _, promptData := range promptsData {
		promptMap, ok := promptData.(map[string]interface{})
		if !ok {
			continue
		}

		prompt := models.MCPPrompt{
			Name:        getString(promptMap, "name", ""),
			Description: getString(promptMap, "description", ""),
		}

		// Parse arguments if present
		if argsData, ok := promptMap["arguments"].([]interface{}); ok {
			for _, argData := range argsData {
				argMap, ok := argData.(map[string]interface{})
				if !ok {
					continue
				}

				arg := models.MCPArgument{
					Name:        getString(argMap, "name", ""),
					Description: getString(argMap, "description", ""),
					Required:    getBool(argMap, "required", false),
				}

				prompt.Arguments = append(prompt.Arguments, arg)
			}
		}

		prompts = append(prompts, prompt)
	}

	return prompts, nil
}

// discoverResources discovers available resources from the MCP server
func (mds *MCPDiscoveryService) discoverResources(mcpURL string, auth *models.AdapterAuthConfig) ([]models.MCPResource, error) {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "resources/list",
		"params":  map[string]interface{}{},
	}

	response, err := mds.sendMCPRequest(mcpURL, request, auth)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if response["result"] != nil {
		result = response["result"].(map[string]interface{})
	} else {
		return nil, fmt.Errorf("resources/list response missing result")
	}

	resourcesData, ok := result["resources"].([]interface{})
	if !ok {
		return []models.MCPResource{}, nil // No resources available
	}

	var resources []models.MCPResource
	for _, resourceData := range resourcesData {
		resourceMap, ok := resourceData.(map[string]interface{})
		if !ok {
			continue
		}

		resource := models.MCPResource{
			URI:         getString(resourceMap, "uri", ""),
			Name:        getString(resourceMap, "name", ""),
			Description: getString(resourceMap, "description", ""),
			MimeType:    getString(resourceMap, "mimeType", ""),
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// sendMCPRequest sends a JSON-RPC request to the MCP server
func (mds *MCPDiscoveryService) sendMCPRequest(mcpURL string, request map[string]interface{}, auth *models.AdapterAuthConfig) (map[string]interface{}, error) {
	// Marshal request
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", mcpURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Add authentication if required
	if auth != nil && auth.Required {
		switch auth.Type {
		case "bearer":
			if auth.BearerToken != nil && auth.BearerToken.Token != "" {
				req.Header.Set("Authorization", "Bearer "+auth.BearerToken.Token)
			}
			// Add other auth types as needed
		}
	}

	// Send request
	resp, err := mds.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("MCP server returned status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Parse JSON-RPC response
	var response map[string]interface{}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for JSON-RPC error
	if response["error"] != nil {
		errorData := response["error"].(map[string]interface{})
		message := getString(errorData, "message", "Unknown error")
		code := getInt(errorData, "code", -1)
		return nil, fmt.Errorf("MCP server error (code %d): %s", code, message)
	}

	return response, nil
}

// Helper functions for extracting values from maps
func getString(m map[string]interface{}, key, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

func getBool(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return defaultValue
}

func getInt(m map[string]interface{}, key string, defaultValue int) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return defaultValue
}
