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
func (h *UnifiedMCPHandler) HandleUnifiedMCP(w http.ResponseWriter, r *http.Request) {
	h.handleUnifiedMCPInternal(w, r)
}

// HandleVirtualMCP handles a specific virtual MCP endpoint
func (h *UnifiedMCPHandler) HandleVirtualMCP(w http.ResponseWriter, r *http.Request) {
	h.handleUnifiedMCPInternal(w, r)
}

func (h *UnifiedMCPHandler) handleUnifiedMCPInternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, nil, -32600, "Only POST method is supported")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, nil, -32700, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		h.sendError(w, nil, -32700, "Empty request body. A valid JSON-RPC request is required.")
		return
	}

	var req MCPRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, nil, -32700, "Invalid JSON: "+err.Error())
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	path := r.URL.Path
	virtualAdapterName := ""
	if strings.HasPrefix(path, "/api/v1/mcp/") {
		virtualAdapterName = strings.TrimPrefix(path, "/api/v1/mcp/")
	}

	var adapters []models.AdapterResource
	if virtualAdapterName != "" {
		logging.ProxyLogger.Info("UnifiedMCP: Handling virtual adapter %s for user %s", virtualAdapterName, userID)
		virtualAdapter, err := h.adapterService.GetAdapter(r.Context(), userID, virtualAdapterName, h.userGroupService)
		if err != nil {
			h.sendError(w, req.ID, -32602, "Virtual adapter not found: "+virtualAdapterName)
			return
		}

		if virtualAdapter.ConnectionType != models.ConnectionTypeVirtual {
			h.sendError(w, req.ID, -32602, "Adapter is not a virtual adapter: "+virtualAdapterName)
			return
		}

		for _, sourceID := range virtualAdapter.SourceAdapters {
			adapter, err := h.adapterService.GetAdapter(r.Context(), userID, sourceID, h.userGroupService)
			if err != nil {
				logging.ProxyLogger.Warn("UnifiedMCP: Failed to load source adapter %s for virtual adapter %s: %v", sourceID, virtualAdapterName, err)
				continue
			}

			if adapter.ConnectionType == models.ConnectionTypeVirtual {
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

// resolveAdapterURL determines the best URL to use for contacting an adapter,
// handling internal loopback cases.
func (h *UnifiedMCPHandler) resolveAdapterURL(adapter models.AdapterResource) string {
	targetURL := adapter.URL
	if targetURL == "" {
		return ""
	}

	// SPECIAL CASE: If calling ourselves (detected by /api/v1/adapters/ path), use 127.0.0.1
	// This is more robust than checking for specific IP or port.
	if strings.Contains(targetURL, "/api/v1/adapters/") {
		parts := strings.SplitN(targetURL, "/api/v1/adapters/", 2)
		if len(parts) == 2 {
			// Always use 127.0.0.1:8911 for internal proxy calls
			newURL := "http://127.0.0.1:8911/api/v1/adapters/" + parts[1]
			logging.ProxyLogger.Info("UnifiedMCP: Internal loopback redirected: %s -> %s", targetURL, newURL)
			return newURL
		}
	}
	return targetURL
}

func (h *UnifiedMCPHandler) handleToolsList(ctx context.Context, req *MCPRequest, userID string, adapters []models.AdapterResource) *MCPResponse {
	allTools := []Tool{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		targetURL := h.resolveAdapterURL(adapter)
		if targetURL == "" {
			continue
		}

		var token string
		if adapter.Authentication != nil && adapter.Authentication.Type == "bearer" && adapter.Authentication.BearerToken != nil {
			token = adapter.Authentication.BearerToken.Token
		}

		wg.Add(1)
		go func(a models.AdapterResource, url, t string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logging.ProxyLogger.Error("UnifiedMCP: Panic fetching tools from %s: %v", a.Name, r)
				}
			}()

			ctxWithTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			tools, err := h.fetchToolsFromAdapter(ctxWithTimeout, a, url, t, userID)
			if err != nil {
				logging.ProxyLogger.Warn("UnifiedMCP: Failed to fetch tools from %s: %v", a.Name, err)
				return
			}

			mu.Lock()
			for _, tool := range tools {
				allTools = append(allTools, Tool{
					Name:        a.Name + "__" + tool.Name,
					Description: fmt.Sprintf("[%s] %s", a.Name, tool.Description),
					InputSchema: tool.InputSchema,
				})
			}
			mu.Unlock()
		}(adapter, targetURL, token)
	}
	wg.Wait()
	return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Result: ToolsListResult{Tools: allTools}}
}

func (h *UnifiedMCPHandler) handleToolsCall(ctx context.Context, req *MCPRequest, userID string, headers http.Header, adapters []models.AdapterResource) *MCPResponse {
	paramsJSON, _ := json.Marshal(req.Params)
	var params ToolCallParams
	json.Unmarshal(paramsJSON, &params)

	parts := strings.SplitN(params.Name, "__", 2)
	if len(parts) != 2 {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Invalid tool name"}}
	}

	adapterName, toolName := parts[0], parts[1]
	var adapter *models.AdapterResource
	for _, a := range adapters {
		if a.Name == adapterName {
			adapter = &a
			break
		}
	}

	if adapter == nil {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Adapter not found"}}
	}

	targetURL := h.resolveAdapterURL(*adapter)
	var token string
	if adapter.Authentication != nil && adapter.Authentication.Type == "bearer" && adapter.Authentication.BearerToken != nil {
		token = adapter.Authentication.BearerToken.Token
	}

	unprefixedReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      req.ID,
		Method:  "tools/call",
		Params:  map[string]interface{}{"name": toolName, "arguments": params.Arguments},
	}

	return h.forwardToAdapter(ctx, adapter, targetURL, token, userID, &unprefixedReq, headers)
}

func (h *UnifiedMCPHandler) handleResourcesList(ctx context.Context, req *MCPRequest, userID string, adapters []models.AdapterResource) *MCPResponse {
	allResources := []Resource{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		targetURL := h.resolveAdapterURL(adapter)
		if targetURL == "" {
			continue
		}

		var token string
		if adapter.Authentication != nil && adapter.Authentication.Type == "bearer" && adapter.Authentication.BearerToken != nil {
			token = adapter.Authentication.BearerToken.Token
		}

		wg.Add(1)
		go func(a models.AdapterResource, url, t string) {
			defer wg.Done()
			ctxWithTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			resources, err := h.fetchResourcesFromAdapter(ctxWithTimeout, a, url, t, userID)
			if err != nil {
				return
			}

			mu.Lock()
			for _, resource := range resources {
				allResources = append(allResources, Resource{
					URI:         a.Name + "://" + resource.URI,
					Name:        fmt.Sprintf("[%s] %s", a.Name, resource.Name),
					Description: resource.Description,
					MimeType:    resource.MimeType,
				})
			}
			mu.Unlock()
		}(adapter, targetURL, token)
	}
	wg.Wait()
	return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Result: ResourcesListResult{Resources: allResources}}
}

func (h *UnifiedMCPHandler) handleResourcesRead(ctx context.Context, req *MCPRequest, userID string, headers http.Header, adapters []models.AdapterResource) *MCPResponse {
	paramsJSON, _ := json.Marshal(req.Params)
	var params struct {
		URI string `json:"uri"`
	}
	json.Unmarshal(paramsJSON, &params)

	parts := strings.SplitN(params.URI, "://", 2)
	if len(parts) != 2 {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Invalid URI"}}
	}

	adapterName, originalURI := parts[0], parts[1]
	var adapter *models.AdapterResource
	for _, a := range adapters {
		if a.Name == adapterName {
			adapter = &a
			break
		}
	}

	if adapter == nil {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Adapter not found"}}
	}

	targetURL := h.resolveAdapterURL(*adapter)
	var token string
	if adapter.Authentication != nil && adapter.Authentication.Type == "bearer" && adapter.Authentication.BearerToken != nil {
		token = adapter.Authentication.BearerToken.Token
	}

	unprefixedReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      req.ID,
		Method:  "resources/read",
		Params:  map[string]interface{}{"uri": originalURI},
	}

	return h.forwardToAdapter(ctx, adapter, targetURL, token, userID, &unprefixedReq, headers)
}

func (h *UnifiedMCPHandler) handlePromptsList(ctx context.Context, req *MCPRequest, userID string, adapters []models.AdapterResource) *MCPResponse {
	allPrompts := []Prompt{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		targetURL := h.resolveAdapterURL(adapter)
		if targetURL == "" {
			continue
		}

		var token string
		if adapter.Authentication != nil && adapter.Authentication.Type == "bearer" && adapter.Authentication.BearerToken != nil {
			token = adapter.Authentication.BearerToken.Token
		}

		wg.Add(1)
		go func(a models.AdapterResource, url, t string) {
			defer wg.Done()
			ctxWithTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			prompts, err := h.fetchPromptsFromAdapter(ctxWithTimeout, a, url, t, userID)
			if err != nil {
				return
			}

			mu.Lock()
			for _, prompt := range prompts {
				allPrompts = append(allPrompts, Prompt{
					Name:        a.Name + "__" + prompt.Name,
					Description: fmt.Sprintf("[%s] %s", a.Name, prompt.Description),
					Arguments:   prompt.Arguments,
				})
			}
			mu.Unlock()
		}(adapter, targetURL, token)
	}
	wg.Wait()
	return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Result: PromptsListResult{Prompts: allPrompts}}
}

func (h *UnifiedMCPHandler) handlePromptsGet(ctx context.Context, req *MCPRequest, userID string, headers http.Header, adapters []models.AdapterResource) *MCPResponse {
	paramsJSON, _ := json.Marshal(req.Params)
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}
	json.Unmarshal(paramsJSON, &params)

	parts := strings.SplitN(params.Name, "__", 2)
	if len(parts) != 2 {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Invalid prompt name"}}
	}

	adapterName, promptName := parts[0], parts[1]
	var adapter *models.AdapterResource
	for _, a := range adapters {
		if a.Name == adapterName {
			adapter = &a
			break
		}
	}

	if adapter == nil {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Adapter not found"}}
	}

	targetURL := h.resolveAdapterURL(*adapter)
	var token string
	if adapter.Authentication != nil && adapter.Authentication.Type == "bearer" && adapter.Authentication.BearerToken != nil {
		token = adapter.Authentication.BearerToken.Token
	}

	unprefixedReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      req.ID,
		Method:  "prompts/get",
		Params:  map[string]interface{}{"name": promptName, "arguments": params.Arguments},
	}

	return h.forwardToAdapter(ctx, adapter, targetURL, token, userID, &unprefixedReq, headers)
}

func (h *UnifiedMCPHandler) fetchToolsFromAdapter(ctx context.Context, adapter models.AdapterResource, url, adapterToken, userID string) ([]Tool, error) {
	req := MCPRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list", Params: map[string]interface{}{}}
	resp, err := h.makeAdapterRequest(ctx, url, adapterToken, userID, &req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("adapter error: %s", resp.Error.Message)
	}
	resultJSON, _ := json.Marshal(resp.Result)
	var result ToolsListResult
	json.Unmarshal(resultJSON, &result)
	if result.Tools == nil {
		return []Tool{}, nil
	}
	return result.Tools, nil
}

func (h *UnifiedMCPHandler) fetchResourcesFromAdapter(ctx context.Context, adapter models.AdapterResource, url, adapterToken, userID string) ([]Resource, error) {
	req := MCPRequest{JSONRPC: "2.0", ID: 1, Method: "resources/list", Params: map[string]interface{}{}}
	resp, err := h.makeAdapterRequest(ctx, url, adapterToken, userID, &req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("adapter error: %s", resp.Error.Message)
	}
	resultJSON, _ := json.Marshal(resp.Result)
	var result ResourcesListResult
	json.Unmarshal(resultJSON, &result)
	if result.Resources == nil {
		return []Resource{}, nil
	}
	return result.Resources, nil
}

func (h *UnifiedMCPHandler) fetchPromptsFromAdapter(ctx context.Context, adapter models.AdapterResource, url, adapterToken, userID string) ([]Prompt, error) {
	req := MCPRequest{JSONRPC: "2.0", ID: 1, Method: "prompts/list", Params: map[string]interface{}{}}
	resp, err := h.makeAdapterRequest(ctx, url, adapterToken, userID, &req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("adapter error: %s", resp.Error.Message)
	}
	resultJSON, _ := json.Marshal(resp.Result)
	var result PromptsListResult
	json.Unmarshal(resultJSON, &result)
	if result.Prompts == nil {
		return []Prompt{}, nil
	}
	return result.Prompts, nil
}

func (h *UnifiedMCPHandler) makeAdapterRequest(ctx context.Context, url, adapterToken, userID string, req *MCPRequest) (*MCPResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if adapterToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+adapterToken)
	}
	if userID != "" {
		httpReq.Header.Set("X-User-ID", userID)
	}
	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("adapter returned status %d: %s", resp.StatusCode, string(respBody))
	}
	respBody, _ := io.ReadAll(resp.Body)
	var mcpResp MCPResponse
	json.Unmarshal(respBody, &mcpResp)
	return &mcpResp, nil
}

func (h *UnifiedMCPHandler) forwardToAdapter(ctx context.Context, adapter *models.AdapterResource, url, adapterToken, userID string, req *MCPRequest, headers http.Header) *MCPResponse {
	if url == "" {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32602, Message: "Adapter has no URL"}}
	}
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if adapterToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+adapterToken)
	}
	if userID != "" {
		httpReq.Header.Set("X-User-ID", userID)
	} else if userIDHeader := headers.Get("X-User-ID"); userIDHeader != "" {
		httpReq.Header.Set("X-User-ID", userIDHeader)
	}
	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32603, Message: "Failed to contact adapter: " + err.Error()}}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return &MCPResponse{JSONRPC: "2.0", ID: req.ID, Error: &MCPRPCError{Code: -32603, Message: fmt.Sprintf("Adapter returned status %d: %s", resp.StatusCode, string(respBody))}}
	}
	respBody, _ := io.ReadAll(resp.Body)
	var mcpResp MCPResponse
	json.Unmarshal(respBody, &mcpResp)
	return &mcpResp
}

func (h *UnifiedMCPHandler) sendError(w http.ResponseWriter, id interface{}, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MCPResponse{JSONRPC: "2.0", ID: id, Error: &MCPRPCError{Code: code, Message: message}})
}
