package mcp

import (
	"time"
)

// MCPMessage represents a JSON-RPC 2.0 compliant MCP message
type MCPMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCP Client/Server Info
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCP Capabilities
type MCPCapabilities struct {
	Resources *struct {
		Subscribe   bool `json:"subscribe,omitempty"`
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"resources,omitempty"`
	Prompts *struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"prompts,omitempty"`
	Tools *struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"tools,omitempty"`
	Logging  *struct{} `json:"logging,omitempty"`
	Sampling *struct{} `json:"sampling,omitempty"`
}

// MCP Resource types
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type MCPResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// MCP Prompt types
type MCPPrompt struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCP Tool types
type MCPTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"inputSchema"`
}

type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type MCPContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// MCP Initialize Request/Response
type MCPInitializeRequest struct {
	Method string `json:"method"`
	Params struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    MCPCapabilities `json:"capabilities"`
		ClientInfo      MCPClientInfo   `json:"clientInfo"`
	} `json:"params"`
}

type MCPInitializeResponse struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    MCPCapabilities `json:"capabilities"`
	ServerInfo      MCPClientInfo   `json:"serverInfo"`
}

// MCP Session represents a session with an MCP server
type MCPSession struct {
	ID           string          `json:"id"`
	AdapterName  string          `json:"adapterName"`
	Initialized  bool            `json:"initialized"`
	Capabilities MCPCapabilities `json:"capabilities"`
	ServerInfo   *MCPClientInfo  `json:"serverInfo,omitempty"`
	ClientInfo   MCPClientInfo   `json:"clientInfo"`
	CreatedAt    time.Time       `json:"createdAt"`
	LastActivity time.Time       `json:"lastActivity"`
}

// MCP Error Codes
const (
	// JSON-RPC 2.0 standard errors
	MCP_PARSE_ERROR      = -32700
	MCP_INVALID_REQUEST  = -32600
	MCP_METHOD_NOT_FOUND = -32601
	MCP_INVALID_PARAMS   = -32602
	MCP_INTERNAL_ERROR   = -32603

	// MCP-specific errors
	MCP_CONNECTION_ERROR          = -32000
	MCP_AUTHENTICATION_ERROR      = -32001
	MCP_AUTHORIZATION_ERROR       = -32002
	MCP_RESOURCE_NOT_FOUND        = -32003
	MCP_CAPABILITY_NOT_SUPPORTED  = -32004
	MCP_USER_CONSENT_REQUIRED     = -32005
	MCP_TOOL_EXECUTION_DENIED     = -32006
	MCP_RESOURCE_ACCESS_DENIED    = -32007
	MCP_PROTOCOL_VERSION_MISMATCH = -32008
)

// CreateMCPError creates a new MCP error
func CreateMCPError(code int, message string, data interface{}) *MCPError {
	return &MCPError{
		Code:      code,
		Message:   message,
		Details:   map[string]interface{}{"data": data},
		Timestamp: time.Now(),
		Retryable: false,
	}
}

// HandleMCPError converts various error types to MCP errors
func HandleMCPError(err error) *MCPError {
	if err == nil {
		return nil
	}

	// Check if it's already an MCP error
	if mcpErr, ok := err.(*MCPError); ok {
		return mcpErr
	}

	// Default to internal error
	return CreateMCPError(MCP_INTERNAL_ERROR, err.Error(), nil)
}
