package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// MCPProxyHandler handles HTTP requests for MCP proxying
type MCPProxyHandler struct {
	proxy *MCPProxyServer
}

// NewMCPProxyHandler creates a new MCP proxy handler
func NewMCPProxyHandler(proxy *MCPProxyServer) *MCPProxyHandler {
	return &MCPProxyHandler{
		proxy: proxy,
	}
}

// HandleMCP handles the main MCP JSON-RPC endpoint
func (h *MCPProxyHandler) HandleMCP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication
	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendJSONRPCError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	// Handle JSON-RPC request
	response, err := h.proxy.HandleJSONRPC(body)
	if err != nil {
		h.sendJSONRPCError(w, nil, -32603, "Internal error", err.Error())
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

// HandleToolsList handles GET /mcp/{id}/tools
func (h *MCPProxyHandler) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create tools/list request
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  "tools/list",
	}

	result, err := h.proxy.HandleMCPRequest(r.Context(), message)
	if err != nil {
		h.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.sendJSONResponse(w, result)
}

// HandleToolCall handles POST /mcp/{id}/tools/{name}/call
func (h *MCPProxyHandler) HandleToolCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract tool name from URL
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[1] != "tools" || pathParts[3] != "call" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	toolName := pathParts[2]

	// Read request body for arguments
	var args map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		h.sendJSONError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create tools/call request
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	result, err := h.proxy.HandleMCPRequest(r.Context(), message)
	if err != nil {
		h.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.sendJSONResponse(w, result)
}

// HandleResourcesList handles GET /mcp/{id}/resources
func (h *MCPProxyHandler) HandleResourcesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create resources/list request
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  "resources/list",
	}

	result, err := h.proxy.HandleMCPRequest(r.Context(), message)
	if err != nil {
		h.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.sendJSONResponse(w, result)
}

// HandleResourceRead handles GET /mcp/{id}/resources/{uri}
func (h *MCPProxyHandler) HandleResourceRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.authenticateRequest(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract resource URI from URL
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[1] != "resources" {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	// Reconstruct URI from remaining path parts
	uri := strings.Join(pathParts[2:], "/")

	// Create resources/read request
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  "resources/read",
		Params: map[string]interface{}{
			"uri": uri,
		},
	}

	result, err := h.proxy.HandleMCPRequest(r.Context(), message)
	if err != nil {
		h.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.sendJSONResponse(w, result)
}

// authenticateRequest checks if the request is authenticated
func (h *MCPProxyHandler) authenticateRequest(r *http.Request) bool {
	// For now, accept any request (authentication will be added later)
	return true
}

// sendJSONResponse sends a JSON response
func (h *MCPProxyHandler) sendJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// sendJSONError sends a JSON error response
func (h *MCPProxyHandler) sendJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}

// sendJSONRPCError sends a JSON-RPC error response
func (h *MCPProxyHandler) sendJSONRPCError(w http.ResponseWriter, id interface{}, code int, message, data string) {
	error := &JSONRPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}

	response := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error:   error,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
