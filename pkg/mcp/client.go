package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
)

// InitializeRequest represents the MCP initialize request
type InitializeRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int              `json:"id"`
	Method  string           `json:"method"`
	Params  InitializeParams `json:"params"`
}

// ClientCapabilities represents client capabilities
type ClientCapabilities struct {
	Sampling *struct{} `json:"sampling,omitempty"`
}

// InitializeResponse represents the MCP initialize response
type InitializeResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      interface{}      `json:"id"` // Can be string, number, or null
	Result  InitializeResult `json:"result,omitempty"`
	Error   *struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

// ListToolsRequest represents the MCP list tools request
type ListToolsRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
}

// ListToolsResponse represents the MCP list tools response
type ListToolsResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"` // Can be string, number, or null
	Result  struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description,omitempty"`
			InputSchema map[string]interface{} `json:"inputSchema"`
		} `json:"tools"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ListResourcesRequest represents the MCP list resources request
type ListResourcesRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
}

// ListResourcesResponse represents the MCP list resources response
type ListResourcesResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"` // Can be string, number, or null
	Result  struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name,omitempty"`
			Description string `json:"description,omitempty"`
			MimeType    string `json:"mimeType,omitempty"`
		} `json:"resources"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ListPromptsRequest represents the MCP list prompts request
type ListPromptsRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
}

// ListPromptsResponse represents the MCP list prompts response
type ListPromptsResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Prompts []struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Arguments   []struct {
				Name        string `json:"name"`
				Description string `json:"description,omitempty"`
				Required    bool   `json:"required,omitempty"`
			} `json:"arguments,omitempty"`
		} `json:"prompts"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client represents an MCP protocol client for deep server interrogation
type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient creates a new MCP client
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// InterrogateServer performs deep interrogation of an MCP server
func (c *Client) InterrogateServer(ctx context.Context) (*models.McpCapabilities, *models.MCPServerInfo, []models.McpTool, []models.McpResource, []models.McpPrompt, *models.AuthAnalysis, error) {
	// Initialize connection
	capabilities, serverInfo, authAnalysis, err := c.initialize(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, authAnalysis, err
	}

	// Extract tools if supported
	var tools []models.McpTool
	if capabilities != nil && capabilities.Tools {
		tools, err = c.listTools(ctx)
		if err != nil {
			log.Printf("Failed to list tools: %v", err)
			// Continue with other interrogations even if tools fail
		}
	}

	// Extract resources if supported
	var resources []models.McpResource
	if capabilities != nil && capabilities.Resources {
		resources, err = c.listResources(ctx)
		if err != nil {
			log.Printf("Failed to list resources: %v", err)
			// Continue with other interrogations even if resources fail
		}
	}

	// Extract prompts if supported
	var prompts []models.McpPrompt
	if capabilities != nil && capabilities.Prompts {
		prompts, err = c.listPrompts(ctx)
		if err != nil {
			log.Printf("Failed to list prompts: %v", err)
			// Continue with other interrogations even if prompts fail
		}
	}

	return capabilities, serverInfo, tools, resources, prompts, authAnalysis, nil
}

// initialize performs MCP initialization and returns capabilities
func (c *Client) initialize(ctx context.Context) (*models.McpCapabilities, *models.MCPServerInfo, *models.AuthAnalysis, error) {
	req := InitializeRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities:    map[string]interface{}{},
			ClientInfo: ClientInfo{
				Name:    "mcp-discovery",
				Version: "1.0.0",
			},
		},
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal initialize request: %w", err)
	}

	resp, authAnalysis, err := c.makeMCPRequest(ctx, jsonData)
	if err != nil {
		return nil, nil, authAnalysis, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, authAnalysis, fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response is JSON or SSE
	bodyStr := string(body)
	trimmedBody := strings.TrimSpace(bodyStr)

	// Handle SSE responses (Server-Sent Events)
	if strings.Contains(bodyStr, "event:") && strings.Contains(bodyStr, "data:") {
		return c.handleSSEResponse(resp, bodyStr)
	}

	// Check if response is JSON
	if !strings.HasPrefix(trimmedBody, "{") {
		// Handle non-JSON responses (HTML error pages, etc.)
		bodyPreview := bodyStr
		if len(bodyStr) > 200 {
			bodyPreview = bodyStr[:200]
		}
		return nil, nil, c.analyzeNonJSONResponse(resp, bodyStr), fmt.Errorf("non-JSON response received - server may not be MCP compatible: %s", bodyPreview)
	}

	var initResp InitializeResponse
	if err := json.Unmarshal(body, &initResp); err != nil {
		// Safely limit response body for error message
		bodyPreview := bodyStr
		if len(bodyStr) > 500 {
			bodyPreview = bodyStr[:500]
		}
		return nil, nil, authAnalysis, fmt.Errorf("failed to unmarshal initialize response: %w. Response: %s", err, bodyPreview)
	}

	if initResp.Error != nil {
		// Analyze error for authentication requirements
		authAnalysis = c.analyzeMCPError(initResp.Error)
		return nil, nil, authAnalysis, fmt.Errorf("MCP initialize error: %s", initResp.Error.Message)
	}

	// Extract capabilities
	capabilities := &models.McpCapabilities{
		Tools:        initResp.Result.Capabilities["tools"] != nil,
		Prompts:      initResp.Result.Capabilities["prompts"] != nil,
		Resources:    initResp.Result.Capabilities["resources"] != nil,
		Logging:      initResp.Result.Capabilities["logging"] != nil,
		Completions:  initResp.Result.Capabilities["completions"] != nil,
		Experimental: initResp.Result.Capabilities["experimental"] != nil,
	}

	serverInfo := &models.MCPServerInfo{
		Name:     initResp.Result.ServerInfo.Name,
		Version:  initResp.Result.ServerInfo.Version,
		Protocol: initResp.Result.ProtocolVersion,
		Capabilities: map[string]interface{}{
			"tools":        capabilities.Tools,
			"prompts":      capabilities.Prompts,
			"resources":    capabilities.Resources,
			"logging":      capabilities.Logging,
			"completions":  capabilities.Completions,
			"experimental": capabilities.Experimental,
		},
	}

	return capabilities, serverInfo, authAnalysis, nil
}

// listTools retrieves available tools from the MCP server
func (c *Client) listTools(ctx context.Context) ([]models.McpTool, error) {
	req := ListToolsRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tools request: %w", err)
	}

	resp, _, err := c.makeMCPRequest(ctx, jsonData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read tools response: %w", err)
	}

	// Check for non-JSON responses
	bodyStr := string(body)
	if !strings.HasPrefix(strings.TrimSpace(bodyStr), "{") {
		bodyPreview := bodyStr
		if len(bodyStr) > 200 {
			bodyPreview = bodyStr[:200]
		}
		return nil, fmt.Errorf("non-JSON response for tools list: %s", bodyPreview)
	}

	var toolsResp ListToolsResponse
	if err := json.Unmarshal(body, &toolsResp); err != nil {
		bodyPreview := bodyStr
		if len(bodyStr) > 300 {
			bodyPreview = bodyStr[:300]
		}
		return nil, fmt.Errorf("failed to unmarshal tools response: %w. Response: %s", err, bodyPreview)
	}

	if toolsResp.Error != nil {
		return nil, fmt.Errorf("tools list error: %s", toolsResp.Error.Message)
	}

	var tools []models.McpTool
	for _, tool := range toolsResp.Result.Tools {
		tools = append(tools, models.McpTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	return tools, nil
}

// listResources retrieves available resources from the MCP server
func (c *Client) listResources(ctx context.Context) ([]models.McpResource, error) {
	req := ListResourcesRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "resources/list",
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resources request: %w", err)
	}

	resp, _, err := c.makeMCPRequest(ctx, jsonData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read resources response: %w", err)
	}

	// Check for non-JSON responses
	bodyStr := string(body)
	if !strings.HasPrefix(strings.TrimSpace(bodyStr), "{") {
		bodyPreview := bodyStr
		if len(bodyStr) > 200 {
			bodyPreview = bodyStr[:200]
		}
		return nil, fmt.Errorf("non-JSON response for resources list: %s", bodyPreview)
	}

	var resourcesResp ListResourcesResponse
	if err := json.Unmarshal(body, &resourcesResp); err != nil {
		bodyPreview := bodyStr
		if len(bodyStr) > 300 {
			bodyPreview = bodyStr[:300]
		}
		return nil, fmt.Errorf("failed to unmarshal resources response: %w. Response: %s", err, bodyPreview)
	}

	if resourcesResp.Error != nil {
		return nil, fmt.Errorf("resources list error: %s", resourcesResp.Error.Message)
	}

	var resources []models.McpResource
	for _, resource := range resourcesResp.Result.Resources {
		resources = append(resources, models.McpResource{
			URI:         resource.URI,
			Name:        resource.Name,
			Description: resource.Description,
			MimeType:    resource.MimeType,
		})
	}

	return resources, nil
}

// listPrompts retrieves available prompts from the MCP server
func (c *Client) listPrompts(ctx context.Context) ([]models.McpPrompt, error) {
	req := ListPromptsRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "prompts/list",
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prompts request: %w", err)
	}

	resp, _, err := c.makeMCPRequest(ctx, jsonData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts response: %w", err)
	}

	// Check for non-JSON responses
	bodyStr := string(body)
	if !strings.HasPrefix(strings.TrimSpace(bodyStr), "{") {
		bodyPreview := bodyStr
		if len(bodyStr) > 200 {
			bodyPreview = bodyStr[:200]
		}
		return nil, fmt.Errorf("non-JSON response for prompts list: %s", bodyPreview)
	}

	var promptsResp ListPromptsResponse
	if err := json.Unmarshal(body, &promptsResp); err != nil {
		bodyPreview := bodyStr
		if len(bodyStr) > 300 {
			bodyPreview = bodyStr[:300]
		}
		return nil, fmt.Errorf("failed to unmarshal prompts response: %w. Response: %s", err, bodyPreview)
	}

	if promptsResp.Error != nil {
		return nil, fmt.Errorf("prompts list error: %s", promptsResp.Error.Message)
	}

	var prompts []models.McpPrompt
	for _, prompt := range promptsResp.Result.Prompts {
		var args []models.McpArgument
		for _, arg := range prompt.Arguments {
			args = append(args, models.McpArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			})
		}

		prompts = append(prompts, models.McpPrompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Arguments:   args,
		})
	}

	return prompts, nil
}

// makeMCPRequest makes an HTTP request to the MCP server
func (c *Client) makeMCPRequest(ctx context.Context, jsonData []byte) (*http.Response, *models.AuthAnalysis, error) {
	// Try different endpoints that MCP servers might use
	endpoints := []string{"/mcp", "/"}

	var lastErr error
	var lastResp *http.Response

	for _, endpoint := range endpoints {
		url := c.baseURL + endpoint

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("User-Agent", "mcp-discovery/1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// If we get a successful response or a client/server error (not network error), return it
		if resp.StatusCode >= 200 && resp.StatusCode < 600 {
			// Analyze response for authentication requirements
			authAnalysis := c.analyzeHTTPResponse(resp)
			return resp, authAnalysis, nil
		}

		// Store this response and continue trying other endpoints
		if lastResp != nil {
			lastResp.Body.Close()
		}
		lastResp = resp
		lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// If we have a response, analyze it for auth
	var authAnalysis *models.AuthAnalysis
	if lastResp != nil {
		authAnalysis = c.analyzeHTTPResponse(lastResp)
	}

	return lastResp, authAnalysis, lastErr
}

// analyzeHTTPResponse analyzes HTTP response for authentication requirements
func (c *Client) analyzeHTTPResponse(resp *http.Response) *models.AuthAnalysis {
	analysis := &models.AuthAnalysis{
		Required:           false,
		Type:               "none",
		DetectedMechanisms: []string{},
		Vulnerabilities:    []string{},
		Confidence:         "high",
	}

	// Check for authentication-required status codes
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		analysis.Required = true
		analysis.Vulnerabilities = append(analysis.Vulnerabilities, "authentication_required")

		// Check WWW-Authenticate header for auth type
		authHeader := resp.Header.Get("WWW-Authenticate")
		if authHeader != "" {
			if strings.Contains(strings.ToLower(authHeader), "basic") {
				analysis.Type = "basic"
				analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "basic")
			} else if strings.Contains(strings.ToLower(authHeader), "bearer") {
				analysis.Type = "bearer"
				analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "bearer")
			}
		}

		// Check for common auth headers
		if resp.Header.Get("X-API-Key") != "" {
			if analysis.Type == "none" {
				analysis.Type = "apikey"
			}
			analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "api_key")
		}

		if resp.Header.Get("Authorization") != "" {
			if analysis.Type == "none" {
				analysis.Type = "bearer"
			}
			analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "bearer")
		}
	}

	return analysis
}

// analyzeMCPError analyzes MCP error response for authentication requirements
func (c *Client) analyzeMCPError(mcpError *struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}) *models.AuthAnalysis {
	analysis := &models.AuthAnalysis{
		Required:           false,
		Type:               "missing",
		DetectedMechanisms: []string{},
		Vulnerabilities:    []string{},
		Confidence:         "medium",
	}

	if mcpError != nil {
		// Check error code for auth-related errors
		if mcpError.Code == -32602 || mcpError.Code == -32000 { // Invalid params or server error
			analysis.Required = true
			analysis.Vulnerabilities = append(analysis.Vulnerabilities, "authentication_required")
		}

		// Check error message for auth clues
		msg := strings.ToLower(mcpError.Message)
		if strings.Contains(msg, "auth") || strings.Contains(msg, "token") ||
			strings.Contains(msg, "credential") || strings.Contains(msg, "unauthorized") {
			analysis.Required = true
			analysis.Type = "bearer" // Assume bearer token by default
			analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "bearer")
		}

		// Check for API key requirements
		if strings.Contains(msg, "api_key") || strings.Contains(msg, "api-key") ||
			strings.Contains(msg, "api key") {
			analysis.Required = true
			analysis.Type = "missing"
			analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "api_key")
			analysis.Vulnerabilities = append(analysis.Vulnerabilities, "missing_api_key")
		}
	}

	return analysis
}

// handleSSEResponse parses Server-Sent Events responses and continues with normal processing
func (c *Client) handleSSEResponse(resp *http.Response, bodyStr string) (*models.McpCapabilities, *models.MCPServerInfo, *models.AuthAnalysis, error) {
	// Parse SSE format: "event: message\ndata: {json}\n\n"
	lines := strings.Split(bodyStr, "\n")
	var jsonData string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			jsonData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if jsonData == "" {
		return nil, nil, &models.AuthAnalysis{
			Required:           false,
			Type:               "none",
			DetectedMechanisms: []string{},
			Vulnerabilities:    []string{},
			Confidence:         "low",
		}, fmt.Errorf("no JSON data found in SSE response")
	}

	// Continue with normal JSON processing using the extracted data
	var initResp InitializeResponse
	if err := json.Unmarshal([]byte(jsonData), &initResp); err != nil {
		// Safely limit response body for error message
		bodyPreview := jsonData
		if len(jsonData) > 500 {
			bodyPreview = jsonData[:500]
		}
		return nil, nil, &models.AuthAnalysis{
			Required:           false,
			Type:               "none",
			DetectedMechanisms: []string{},
			Vulnerabilities:    []string{},
			Confidence:         "low",
		}, fmt.Errorf("failed to unmarshal SSE JSON data: %w. Data: %s", err, bodyPreview)
	}

	if initResp.Error != nil {
		// Analyze error for authentication requirements
		authAnalysis := c.analyzeMCPError(initResp.Error)
		return nil, nil, authAnalysis, fmt.Errorf("MCP initialize error: %s", initResp.Error.Message)
	}

	// Extract capabilities
	capabilities := &models.McpCapabilities{
		Tools:        initResp.Result.Capabilities["tools"] != nil,
		Prompts:      initResp.Result.Capabilities["prompts"] != nil,
		Resources:    initResp.Result.Capabilities["resources"] != nil,
		Logging:      initResp.Result.Capabilities["logging"] != nil,
		Completions:  initResp.Result.Capabilities["completions"] != nil,
		Experimental: initResp.Result.Capabilities["experimental"] != nil,
	}

	serverInfo := &models.MCPServerInfo{
		Name:         initResp.Result.ServerInfo.Name,
		Version:      initResp.Result.ServerInfo.Version,
		Protocol:     initResp.Result.ProtocolVersion,
		Capabilities: initResp.Result.Capabilities,
	}

	return capabilities, serverInfo, &models.AuthAnalysis{
		Required:           false,
		Type:               "none",
		DetectedMechanisms: []string{},
		Vulnerabilities:    []string{},
		Confidence:         "low",
	}, nil
}

// analyzeNonJSONResponse analyzes non-JSON responses for useful information
func (c *Client) analyzeNonJSONResponse(resp *http.Response, bodyStr string) *models.AuthAnalysis {
	analysis := &models.AuthAnalysis{
		Required:           false,
		Type:               "missing", // Changed from "unknown" to "missing"
		DetectedMechanisms: []string{},
		Vulnerabilities:    []string{},
		Confidence:         "low",
	}

	// Check for common error patterns
	bodyLower := strings.ToLower(bodyStr)

	if strings.Contains(bodyLower, "unauthorized") || strings.Contains(bodyLower, "401") {
		analysis.Required = true
		analysis.Type = "missing"
		analysis.Vulnerabilities = append(analysis.Vulnerabilities, "authentication_required")
	}

	if strings.Contains(bodyLower, "forbidden") || strings.Contains(bodyLower, "403") {
		analysis.Required = true
		analysis.Type = "missing"
		analysis.Vulnerabilities = append(analysis.Vulnerabilities, "access_forbidden")
	}

	// Check for API key requirements
	if strings.Contains(bodyLower, "api_key") || strings.Contains(bodyLower, "api-key") {
		analysis.Required = true
		analysis.Type = "missing"
		analysis.DetectedMechanisms = append(analysis.DetectedMechanisms, "api_key")
		analysis.Vulnerabilities = append(analysis.Vulnerabilities, "missing_api_key")
	}

	return analysis
}

// ValidateCapabilities performs basic validation of discovered capabilities
func (c *Client) ValidateCapabilities(ctx context.Context, capabilities *models.McpCapabilities, tools []models.McpTool, resources []models.McpResource, prompts []models.McpPrompt) (*models.CapabilityValidation, error) {
	validation := &models.CapabilityValidation{
		ToolsValid:     true,
		ResourcesValid: true,
		PromptsValid:   true,
		Issues:         []string{},
	}

	// Validate tools
	if capabilities.Tools && len(tools) == 0 {
		validation.ToolsValid = false
		validation.Issues = append(validation.Issues, "server claims to support tools but returned empty list")
	}

	// Validate resources
	if capabilities.Resources && len(resources) == 0 {
		validation.ResourcesValid = false
		validation.Issues = append(validation.Issues, "server claims to support resources but returned empty list")
	}

	// Validate prompts
	if capabilities.Prompts && len(prompts) == 0 {
		validation.PromptsValid = false
		validation.Issues = append(validation.Issues, "server claims to support prompts but returned empty list")
	}

	// Validate tool schemas
	for _, tool := range tools {
		if tool.InputSchema == nil {
			validation.ToolsValid = false
			validation.Issues = append(validation.Issues, fmt.Sprintf("tool '%s' missing input schema", tool.Name))
		}
	}

	// Validate prompt arguments
	for _, prompt := range prompts {
		for _, arg := range prompt.Arguments {
			if arg.Required && arg.Description == "" {
				validation.PromptsValid = false
				validation.Issues = append(validation.Issues, fmt.Sprintf("required prompt argument '%s' missing description", arg.Name))
			}
		}
	}

	return validation, nil
}
