package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// ProtocolVersion represents the MCP protocol version
const ProtocolVersion = "2025-06-18"

// JSONRPCMessage represents a JSON-RPC 2.0 message
type JSONRPCMessage struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Method  string        `json:"method,omitempty"`
	Params  interface{}   `json:"params,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface
func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC Error %d: %s", e.Code, e.Message)
}

// MCPError codes
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// Standard MCP error codes
const (
	ErrCodeMCPParseError     = -32700
	ErrCodeMCPInvalidRequest = -32600
	ErrCodeMCPMethodNotFound = -32601
	ErrCodeMCPInvalidParams  = -32602
	ErrCodeMCPInternalError  = -32603
	ErrCodeMCPUnauthorized   = -32001
	ErrCodeMCPNotFound       = -32002
)

// InitializeParams represents MCP initialization parameters
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// ClientInfo represents client information
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult represents MCP initialization result
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}

// ServerInfo represents server information
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ProtocolHandler handles MCP protocol operations
type ProtocolHandler struct {
	sessionStore    session.SessionStore
	capabilityCache *CapabilityCache
}

// NewProtocolHandler creates a new MCP protocol handler
func NewProtocolHandler(sessionStore session.SessionStore, capabilityCache *CapabilityCache) *ProtocolHandler {
	return &ProtocolHandler{
		sessionStore:    sessionStore,
		capabilityCache: capabilityCache,
	}
}

// HandleMessage processes an incoming MCP message
func (ph *ProtocolHandler) HandleMessage(ctx context.Context, message []byte, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	// Parse JSON-RPC message
	var rpcMessage JSONRPCMessage
	if err := json.Unmarshal(message, &rpcMessage); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Validate JSON-RPC 2.0 structure
	if rpcMessage.JSONRPC != "2.0" {
		return nil, fmt.Errorf("invalid JSON-RPC version")
	}

	// Handle different message types
	if rpcMessage.Method != "" {
		// This is a request
		return ph.handleRequest(ctx, &rpcMessage, adapter, sessionID)
	} else if rpcMessage.Result != nil || rpcMessage.Error != nil {
		// This is a response - forward to appropriate handler
		return ph.handleResponse(&rpcMessage, adapter, sessionID)
	}

	// Invalid message
	return nil, fmt.Errorf("invalid JSON-RPC message: missing method, result, or error")
}

// handleRequest processes an MCP request
func (ph *ProtocolHandler) handleRequest(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MCP Protocol: Handling request method: %s", message.Method)

	switch message.Method {
	case "initialize":
		return ph.handleInitialize(ctx, message, adapter, sessionID)
	case "initialized":
		return ph.handleInitialized(ctx, message, adapter, sessionID)
	default:
		// For other methods, we need to proxy to the target MCP server
		return ph.proxyRequest(ctx, message, adapter, sessionID)
	}
}

// handleInitialize processes MCP initialization
func (ph *ProtocolHandler) handleInitialize(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MCP Protocol: Handling initialize request")

	// Parse initialize parameters
	var params InitializeParams
	if message.Params != nil {
		if paramsBytes, err := json.Marshal(message.Params); err == nil {
			if err := json.Unmarshal(paramsBytes, &params); err != nil {
				return nil, fmt.Errorf("invalid initialize parameters: %w", err)
			}
		}
	}

	// Get target server capabilities
	capabilities, err := ph.capabilityCache.GetCapabilities(ctx, adapter)
	if err != nil {
		log.Printf("MCP Protocol: Failed to get capabilities: %v", err)
		// Return basic capabilities for now
		capabilities = map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
			"resources": map[string]interface{}{
				"subscribe":   true,
				"listChanged": true,
			},
			"prompts": map[string]interface{}{
				"listChanged": true,
			},
		}
	}

	// Create initialize result
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    capabilities,
		ServerInfo: ServerInfo{
			Name:    fmt.Sprintf("MCP Proxy for %s", adapter.Name),
			Version: "1.0.0",
		},
	}

	// Store session info
	if sessionID != "" {
		ph.sessionStore.SetWithDetails(sessionID, adapter.Name, "", "MCP")

		// Store MCP-specific session information
		ph.sessionStore.SetMCPServerInfo(sessionID, &session.MCPServerInfo{
			Name:     adapter.Name,
			Version:  "1.0.0",
			Protocol: "MCP",
		})

		// Store capabilities in session
		ph.sessionStore.SetMCPCapabilities(sessionID, capabilities)
	}

	response := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      message.ID,
		Result:  result,
	}

	return response, nil
}

// handleInitialized processes MCP initialized notification
func (ph *ProtocolHandler) handleInitialized(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MCP Protocol: Client initialized for adapter %s", adapter.Name)

	// This is a notification, so no response needed
	// Just update session state
	if sessionID != "" {
		ph.sessionStore.UpdateActivity(sessionID)
	}

	return nil, nil
}

// handleResponse processes an MCP response
func (ph *ProtocolHandler) handleResponse(message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MCP Protocol: Handling response for ID: %v", message.ID)

	// For now, just forward the response
	// In a full implementation, this would handle response correlation
	return message, nil
}

// proxyRequest forwards a request to the target MCP server
func (ph *ProtocolHandler) proxyRequest(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MCP Protocol: Proxying request %s to adapter %s", message.Method, adapter.Name)

	// This will be implemented when we create the message router
	// For now, return method not found
	return nil, fmt.Errorf("method '%s' not yet implemented in proxy", message.Method)
}

// ValidateMCPMessage validates that a message is a valid MCP JSON-RPC 2.0 message
func ValidateMCPMessage(data []byte) error {
	var message map[string]interface{}
	if err := json.Unmarshal(data, &message); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Check for JSON-RPC 2.0 structure
	jsonrpc, ok := message["jsonrpc"]
	if !ok {
		return fmt.Errorf("missing 'jsonrpc' field")
	}

	jsonrpcStr, ok := jsonrpc.(string)
	if !ok || jsonrpcStr != "2.0" {
		return fmt.Errorf("invalid or missing 'jsonrpc' version, expected '2.0'")
	}

	// Should have either 'method' (request) or 'result'/'error' (response)
	hasMethod := message["method"] != nil
	hasResult := message["result"] != nil
	hasError := message["error"] != nil

	if !hasMethod && !hasResult && !hasError {
		return fmt.Errorf("invalid MCP message: must have 'method', 'result', or 'error' field")
	}

	// Additional MCP-specific validation
	if hasMethod {
		if _, ok := message["method"].(string); !ok {
			return fmt.Errorf("'method' must be a string")
		}

		// Validate common MCP method names
		if methodStr, ok := message["method"].(string); ok {
			validMethods := map[string]bool{
				"initialize":          true,
				"initialized":         true,
				"tools/list":          true,
				"tools/call":          true,
				"resources/list":      true,
				"resources/read":      true,
				"resources/subscribe": true,
				"prompts/list":        true,
				"prompts/get":         true,
				"completion/complete": true,
			}

			if !validMethods[methodStr] && !strings.HasPrefix(methodStr, "tools/") &&
				!strings.HasPrefix(methodStr, "resources/") && !strings.HasPrefix(methodStr, "prompts/") {
				// Allow custom methods but log for debugging
				log.Printf("MCP Validation: Unknown method '%s' - allowing but may be unsupported", methodStr)
			}
		}
	}

	return nil
}

// CreateErrorResponse creates a JSON-RPC error response
func CreateErrorResponse(id interface{}, code int, message string, data interface{}) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// CreateSuccessResponse creates a JSON-RPC success response
func CreateSuccessResponse(id interface{}, result interface{}) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// CreateNotification creates a JSON-RPC notification
func CreateNotification(method string, params interface{}) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}

// WriteSSEEvent writes an SSE event to the response writer
func WriteSSEEvent(w http.ResponseWriter, event string, data string) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}

	// Write data lines
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}

	// End event
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}

	// Flush to ensure immediate delivery
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// StreamSSEMessage streams a JSON-RPC message via SSE
func StreamSSEMessage(w http.ResponseWriter, message *JSONRPCMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return WriteSSEEvent(w, "message", string(data))
}

// ReadSSEMessage reads and parses SSE messages from a reader
func ReadSSEMessage(reader io.Reader) ([]*JSONRPCMessage, error) {
	var messages []*JSONRPCMessage

	// Simple SSE parser - in production, use a proper SSE library
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var currentMessage strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "data: ") {
			messageData := strings.TrimPrefix(line, "data: ")
			if messageData == "" {
				// Empty data line indicates end of message
				if currentMessage.Len() > 0 {
					var rpcMessage JSONRPCMessage
					if err := json.Unmarshal([]byte(currentMessage.String()), &rpcMessage); err == nil {
						messages = append(messages, &rpcMessage)
					}
					currentMessage.Reset()
				}
			} else {
				if currentMessage.Len() > 0 {
					currentMessage.WriteString("\n")
				}
				currentMessage.WriteString(messageData)
			}
		}
	}

	// Handle final message if no empty data line at end
	if currentMessage.Len() > 0 {
		var rpcMessage JSONRPCMessage
		if err := json.Unmarshal([]byte(currentMessage.String()), &rpcMessage); err == nil {
			messages = append(messages, &rpcMessage)
		}
	}

	return messages, nil
}
