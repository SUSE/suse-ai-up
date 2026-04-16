package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/logging"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/services"
	adaptersvc "suse-ai-up/pkg/services/adapters"
)

// UnifiedMCPHandler handles unified MCP protocol requests that aggregate
// tools, resources, and prompts from all registered adapters
type UnifiedMCPHandler struct {
	adapterService   *adaptersvc.AdapterService
	userGroupService *services.UserGroupService
	httpClient       *http.Client
}

// NewUnifiedMCPHandler creates a new unified MCP handler
func NewUnifiedMCPHandler(adapterService *adaptersvc.AdapterService, userGroupService *services.UserGroupService) *UnifiedMCPHandler {
	return &UnifiedMCPHandler{
		adapterService:   adapterService,
		userGroupService: userGroupService,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Increased for slow SQL queries
		},
	}
}

// MCPRequest represents an incoming MCP JSON-RPC request
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents an MCP JSON-RPC response
type MCPResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      interface{}  `json:"id,omitempty"`
	Result  interface{}  `json:"result,omitempty"`
	Error   *MCPRPCError `json:"error,omitempty"`
}

// MCPRPCError represents a JSON-RPC error
type MCPRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Tool represents an MCP tool
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"inputSchema,omitempty"`
}

// Resource represents an MCP resource
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// Prompt represents an MCP prompt
type Prompt struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Arguments   []interface{} `json:"arguments,omitempty"`
}

// ToolsListResult represents the result of tools/list
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ResourcesListResult represents the result of resources/list
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// PromptsListResult represents the result of prompts/list
type PromptsListResult struct {
	Prompts []Prompt `json:"prompts"`
}

// ToolCallParams represents parameters for tools/call
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// HandleUnifiedMCP handles the global unified MCP endpoint
// @Summary Global Unified MCP endpoint
// @Description Aggregates tools, resources, and prompts from all accessible adapters
// @Tags mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Success 200 {object} MCPResponse "MCP response"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/mcp [post]
func (h *UnifiedMCPHandler) HandleUnifiedMCP(w http.ResponseWriter, r *http.Request) {
	h.handleUnifiedMCPInternal(w, r)
}

// HandleVirtualMCP handles a specific virtual MCP endpoint
// @Summary Virtual MCP endpoint
// @Description Aggregates tools, resources, and prompts from a specific virtual adapter
// @Tags mcp
// @Accept json
// @Produce json
// @Param X-User-ID header string false "User ID" default(default-user)
// @Param name path string true "Virtual Adapter Name"
// @Success 200 {object} MCPResponse "MCP response"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/mcp/{name} [post]
func (h *UnifiedMCPHandler) HandleVirtualMCP(w http.ResponseWriter, r *http.Request) {
	h.handleUnifiedMCPInternal(w, r)
}

func (h *UnifiedMCPHandler) handleUnifiedMCPInternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, nil, -32600, "Only POST method is supported")
		return
	}

	// Parse the incoming request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, nil, -32700, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	var req MCPRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, nil, -32700, "Invalid JSON: "+err.Error())
		return
	}

	// Get user ID from header
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	// Determine if this is a request for a specific virtual adapter
	// Path could be /api/v1/mcp or /api/v1/mcp/some-name
	path := r.URL.Path
	virtualAdapterName := ""
	if strings.HasPrefix(path, "/api/v1/mcp/") {
		virtualAdapterName = strings.TrimPrefix(path, "/api/v1/mcp/")
	}

	var adapters []models.AdapterResource
	if virtualAdapterName != "" {
		logging.ProxyLogger.Info("UnifiedMCP: Handling virtual adapter %s for user %s", virtualAdapterName, userID)
		// Get the virtual adapter
		virtualAdapter, err := h.adapterService.GetAdapter(r.Context(), userID, virtualAdapterName, h.userGroupService)
		if err != nil {
			h.sendError(w, req.ID, -32602, "Virtual adapter not found: "+virtualAdapterName)
			return
		}

		if virtualAdapter.ConnectionType != models.ConnectionTypeVirtual {
			h.sendError(w, req.ID, -32602, "Adapter is not a virtual adapter: "+virtualAdapterName)
			return
		}

		// Load source adapters
		for _, sourceID := range virtualAdapter.SourceAdapters {
			adapter, err := h.adapterService.GetAdapter(r.Context(), userID, sourceID, h.userGroupService)
			if err != nil {
				logging.ProxyLogger.Warn("UnifiedMCP: Failed to load source adapter %s for virtual adapter %s: %v", sourceID, virtualAdapterName, err)
				continue
			}
			adapters = append(adapters, *adapter)
		}
	} else {
		logging.ProxyLogger.Info("UnifiedMCP: Handling global aggregation for user %s", userID)
		var err error
		adapters, err = h.adapterService.ListAdapters(r.Context(), userID, h.userGroupService)
		if err != nil {
			h.sendError(w, req.ID, -32603, "Failed to list adapters: "+err.Error())
			return
		}
	}

	logging.ProxyLogger.Info("UnifiedMCP: Handling method %s for user %s with %d adapters", req.Method, userID, len(adapters))

	var response *MCPResponse

	switch req.Method {
	case "initialize":
		response = h.handleInitialize(r.Context(), &req)
	case "initialized":
		// No response needed for initialized notification
		w.WriteHeader(http.StatusOK)
		return
	case "tools/list":
		response = h.handleToolsList(r.Context(), &req, userID, adapters)
	case "tools/call":
		response = h.handleToolsCall(r.Context(), &req, userID, r.Header, adapters)
	case "resources/list":
		response = h.handleResourcesList(r.Context(), &req, userID, adapters)
	case "resources/read":
		response = h.handleResourcesRead(r.Context(), &req, userID, r.Header, adapters)
	case "prompts/list":
		response = h.handlePromptsList(r.Context(), &req, userID, adapters)
	case "prompts/get":
		response = h.handlePromptsGet(r.Context(), &req, userID, r.Header, adapters)
	default:
		response = &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("MCP-Protocol-Version", "2025-06-18")
	json.NewEncoder(w).Encode(response)
}

// handleInitialize handles the initialize method
func (h *UnifiedMCPHandler) handleInitialize(ctx context.Context, req *MCPRequest) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2025-06-18",
			"serverInfo": map[string]interface{}{
				"name":    "suse-ai-unified-proxy",
				"version": "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{"listChanged": false},
				"resources": map[string]interface{}{"listChanged": false},
				"prompts":   map[string]interface{}{"listChanged": false},
			},
		},
	}
}

// handleToolsList aggregates tools from a specific set of adapters
func (h *UnifiedMCPHandler) handleToolsList(ctx context.Context, req *MCPRequest, userID string, adapters []models.AdapterResource) *MCPResponse {
	var allTools []Tool
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		if adapter.ConnectionType != models.ConnectionTypeRemoteHttp || adapter.RemoteUrl == "" {
			continue
		}

		wg.Add(1)
		go func(adapter models.AdapterResource) {
			defer wg.Done()

			tools, err := h.fetchToolsFromAdapter(ctx, adapter)
			if err != nil {
				logging.ProxyLogger.Warn("UnifiedMCP: Failed to fetch tools from %s: %v", adapter.Name, err)
				return
			}

			// Prefix tool names with adapter name
			mu.Lock()
			for _, tool := range tools {
				prefixedTool := Tool{
					Name:        adapter.Name + "__" + tool.Name,
					Description: fmt.Sprintf("[%s] %s", adapter.Name, tool.Description),
					InputSchema: tool.InputSchema,
				}
				allTools = append(allTools, prefixedTool)
			}
			mu.Unlock()
		}(adapter)
	}

	wg.Wait()

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ToolsListResult{Tools: allTools},
	}
}

// handleToolsCall routes a tool call to the appropriate adapter
func (h *UnifiedMCPHandler) handleToolsCall(ctx context.Context, req *MCPRequest, userID string, headers http.Header, adapters []models.AdapterResource) *MCPResponse {
	// Parse params
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Invalid params"},
		}
	}

	var params ToolCallParams
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	// Extract adapter prefix from tool name (format: adapter__toolname)
	parts := strings.SplitN(params.Name, "__", 2)
	if len(parts) != 2 {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Tool name must be prefixed with adapter name (e.g., servicenow__get_incident)"},
		}
	}

	adapterName := parts[0]
	toolName := parts[1]

	// Find the adapter in the provided list
	var adapter *models.AdapterResource
	for _, a := range adapters {
		if a.Name == adapterName {
			adapter = &a
			break
		}
	}

	if adapter == nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Adapter not found in this context: " + adapterName},
		}
	}

	if adapter.ConnectionType != models.ConnectionTypeRemoteHttp || adapter.RemoteUrl == "" {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Adapter does not support remote MCP"},
		}
	}

	// Forward the request with unprefixed tool name
	unprefixedReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      req.ID,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": params.Arguments,
		},
	}

	return h.forwardToAdapter(ctx, adapter, &unprefixedReq, headers)
}

// handleResourcesList aggregates resources from a specific set of adapters
func (h *UnifiedMCPHandler) handleResourcesList(ctx context.Context, req *MCPRequest, userID string, adapters []models.AdapterResource) *MCPResponse {
	var allResources []Resource
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		if adapter.ConnectionType != models.ConnectionTypeRemoteHttp || adapter.RemoteUrl == "" {
			continue
		}

		wg.Add(1)
		go func(adapter models.AdapterResource) {
			defer wg.Done()

			resources, err := h.fetchResourcesFromAdapter(ctx, adapter)
			if err != nil {
				logging.ProxyLogger.Warn("UnifiedMCP: Failed to fetch resources from %s: %v", adapter.Name, err)
				return
			}

			// Prefix resource URIs with adapter name
			mu.Lock()
			for _, resource := range resources {
				prefixedResource := Resource{
					URI:         adapter.Name + "://" + resource.URI,
					Name:        fmt.Sprintf("[%s] %s", adapter.Name, resource.Name),
					Description: resource.Description,
					MimeType:    resource.MimeType,
				}
				allResources = append(allResources, prefixedResource)
			}
			mu.Unlock()
		}(adapter)
	}

	wg.Wait()

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ResourcesListResult{Resources: allResources},
	}
}

// handleResourcesRead routes a resource read to the appropriate adapter
func (h *UnifiedMCPHandler) handleResourcesRead(ctx context.Context, req *MCPRequest, userID string, headers http.Header, adapters []models.AdapterResource) *MCPResponse {
	// Parse params to get URI
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Invalid params"},
		}
	}

	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	// Extract adapter prefix from URI (format: adapter://original_uri)
	parts := strings.SplitN(params.URI, "://", 2)
	if len(parts) != 2 {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Resource URI must be prefixed with adapter name"},
		}
	}

	adapterName := parts[0]
	originalURI := parts[1]

	// Find the adapter in the provided list
	var adapter *models.AdapterResource
	for _, a := range adapters {
		if a.Name == adapterName {
			adapter = &a
			break
		}
	}

	if adapter == nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Adapter not found in this context: " + adapterName},
		}
	}

	// Forward the request with unprefixed URI
	unprefixedReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      req.ID,
		Method:  "resources/read",
		Params:  map[string]interface{}{"uri": originalURI},
	}

	return h.forwardToAdapter(ctx, adapter, &unprefixedReq, headers)
}

// handlePromptsList aggregates prompts from a specific set of adapters
func (h *UnifiedMCPHandler) handlePromptsList(ctx context.Context, req *MCPRequest, userID string, adapters []models.AdapterResource) *MCPResponse {
	var allPrompts []Prompt
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		if adapter.ConnectionType != models.ConnectionTypeRemoteHttp || adapter.RemoteUrl == "" {
			continue
		}

		wg.Add(1)
		go func(adapter models.AdapterResource) {
			defer wg.Done()

			prompts, err := h.fetchPromptsFromAdapter(ctx, adapter)
			if err != nil {
				logging.ProxyLogger.Warn("UnifiedMCP: Failed to fetch prompts from %s: %v", adapter.Name, err)
				return
			}

			// Prefix prompt names with adapter name
			mu.Lock()
			for _, prompt := range prompts {
				prefixedPrompt := Prompt{
					Name:        adapter.Name + "__" + prompt.Name,
					Description: fmt.Sprintf("[%s] %s", adapter.Name, prompt.Description),
					Arguments:   prompt.Arguments,
				}
				allPrompts = append(allPrompts, prefixedPrompt)
			}
			mu.Unlock()
		}(adapter)
	}

	wg.Wait()

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  PromptsListResult{Prompts: allPrompts},
	}
}

// handlePromptsGet routes a prompt get to the appropriate adapter
func (h *UnifiedMCPHandler) handlePromptsGet(ctx context.Context, req *MCPRequest, userID string, headers http.Header, adapters []models.AdapterResource) *MCPResponse {
	// Parse params to get prompt name
	paramsJSON, err := json.Marshal(req.Params)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Invalid params"},
		}
	}

	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	// Extract adapter prefix from prompt name (format: adapter__promptname)
	parts := strings.SplitN(params.Name, "__", 2)
	if len(parts) != 2 {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Prompt name must be prefixed with adapter name"},
		}
	}

	adapterName := parts[0]
	promptName := parts[1]

	// Find the adapter in the provided list
	var adapter *models.AdapterResource
	for _, a := range adapters {
		if a.Name == adapterName {
			adapter = &a
			break
		}
	}

	if adapter == nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Adapter not found in this context: " + adapterName},
		}
	}

	// Forward the request with unprefixed prompt name
	unprefixedReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      req.ID,
		Method:  "prompts/get",
		Params: map[string]interface{}{
			"name":      promptName,
			"arguments": params.Arguments,
		},
	}

	return h.forwardToAdapter(ctx, adapter, &unprefixedReq, headers)
}

// fetchToolsFromAdapter fetches the list of available tools from a single remote MCP adapter.
// It sends a tools/list JSON-RPC request to the adapter's remote URL and parses the response.
// Returns the list of tools or an error if the request fails or the adapter returns an error.
func (h *UnifiedMCPHandler) fetchToolsFromAdapter(ctx context.Context, adapter models.AdapterResource) ([]Tool, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]interface{}{},
	}

	resp, err := h.makeAdapterRequest(ctx, adapter.RemoteUrl, &req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("adapter error: %s", resp.Error.Message)
	}

	// Parse result
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var result ToolsListResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Tools, nil
}

// fetchResourcesFromAdapter fetches the list of available resources from a single remote MCP adapter.
// It sends a resources/list JSON-RPC request to the adapter's remote URL and parses the response.
// Returns the list of resources or an error if the request fails or the adapter returns an error.
func (h *UnifiedMCPHandler) fetchResourcesFromAdapter(ctx context.Context, adapter models.AdapterResource) ([]Resource, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/list",
		Params:  map[string]interface{}{},
	}

	resp, err := h.makeAdapterRequest(ctx, adapter.RemoteUrl, &req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("adapter error: %s", resp.Error.Message)
	}

	// Parse result
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var result ResourcesListResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Resources, nil
}

// fetchPromptsFromAdapter fetches the list of available prompts from a single remote MCP adapter.
// It sends a prompts/list JSON-RPC request to the adapter's remote URL and parses the response.
// Returns the list of prompts or an error if the request fails or the adapter returns an error.
func (h *UnifiedMCPHandler) fetchPromptsFromAdapter(ctx context.Context, adapter models.AdapterResource) ([]Prompt, error) {
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "prompts/list",
		Params:  map[string]interface{}{},
	}

	resp, err := h.makeAdapterRequest(ctx, adapter.RemoteUrl, &req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("adapter error: %s", resp.Error.Message)
	}

	// Parse result
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var result PromptsListResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Prompts, nil
}

// makeAdapterRequest makes a JSON-RPC HTTP POST request to a remote MCP adapter.
// It marshals the request to JSON, sends it to the specified URL, and parses the response.
// The request is made with the context for cancellation and timeout support.
// Returns the parsed MCP response or an error if the HTTP request or JSON parsing fails.
func (h *UnifiedMCPHandler) makeAdapterRequest(ctx context.Context, url string, req *MCPRequest) (*MCPResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, err
	}

	return &mcpResp, nil
}

// forwardToAdapter forwards a JSON-RPC request to a specific remote MCP adapter and returns the response.
// It handles the HTTP communication, including forwarding relevant headers (like X-User-ID) to the adapter.
// If the adapter has no remote URL or communication fails, it returns an appropriate error response.
// This is used by tools/call, resources/read, and prompts/get to route requests to the correct adapter.
func (h *UnifiedMCPHandler) forwardToAdapter(ctx context.Context, adapter *models.AdapterResource, req *MCPRequest, headers http.Header) *MCPResponse {
	if adapter.RemoteUrl == "" {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32602, Message: "Adapter has no remote URL"},
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32603, Message: "Failed to marshal request"},
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, adapter.RemoteUrl, bytes.NewReader(body))
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32603, Message: "Failed to create request"},
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Forward relevant headers
	if userID := headers.Get("X-User-ID"); userID != "" {
		httpReq.Header.Set("X-User-ID", userID)
	}

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32603, Message: "Failed to contact adapter: " + err.Error()},
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32603, Message: "Failed to read response"},
		}
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPRPCError{Code: -32603, Message: "Invalid response from adapter"},
		}
	}

	return &mcpResp
}

// sendError writes a JSON-RPC error response to the HTTP response writer.
// It sets the Content-Type header to application/json and encodes an MCPResponse
// with the specified error code and message. Standard JSON-RPC error codes include:
// -32700 (Parse error), -32600 (Invalid request), -32601 (Method not found),
// -32602 (Invalid params), -32603 (Internal error).
func (h *UnifiedMCPHandler) sendError(w http.ResponseWriter, id interface{}, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &MCPRPCError{Code: code, Message: message},
	})
}
