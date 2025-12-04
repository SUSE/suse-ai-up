// Package proxy provides MCP proxying functionality with middleware support
package proxy

import (
	"context"
	"net/http"
	"time"
)

// MCPProxy represents a configured MCP server proxy
type MCPProxy struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RemoteURL string    `json:"remote_url"` // External MCP server URL
	AuthToken string    `json:"auth_token"` // Client bearer token for accessing this proxy
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

// ProxyConfig holds global proxy server configuration
type ProxyConfig struct {
	Port      string `json:"port"`
	AuthToken string `json:"auth_token"` // Global auth token for all proxies
	LogLevel  string `json:"log_level"`
}

// MCPConfig represents the MCP server configuration format (inspired by gofastmcp)
type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig defines a single MCP server configuration
type ServerConfig struct {
	URL       string            `json:"url"`
	Transport string            `json:"transport"` // Always "http" for now
	AuthToken string            `json:"authToken,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// ProxyClient handles communication with remote MCP servers
type ProxyClient struct {
	remoteURL        string
	transport        Transport
	authToken        string
	httpClient       *http.Client
	advancedFeatures *AdvancedFeaturesHandler
}

// Transport defines the interface for MCP server communication
type Transport interface {
	SendRequest(ctx context.Context, method string, params interface{}) (interface{}, error)
}

// ProxySession represents an isolated session for a single request
type ProxySession struct {
	id       string
	client   *ProxyClient
	created  time.Time
	lastUsed time.Time
}

// MCPProxyServer provides the main proxy functionality with middleware support
type MCPProxyServer struct {
	clientFactory func() *ProxyClient
	name          string
	middlewares   []Middleware
	pipeline      *MiddlewarePipeline
}

// Middleware defines the interface for MCP middleware
type Middleware interface {
	OnMessage(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnRequest(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnNotification(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnCallTool(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnReadResource(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnGetPrompt(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnListTools(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnListResources(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnListPrompts(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
	OnInitialize(ctx *MiddlewareContext, next MiddlewareHandler) (interface{}, error)
}

// MiddlewareContext provides context for middleware operations
type MiddlewareContext struct {
	Method         string
	Source         string // "client" or "server"
	Type           string // "request" or "notification"
	Message        *JSONRPCMessage
	Timestamp      time.Time
	MCPContext     *MCPContext // May be nil during initialization
	FastMCPContext *FastMCPContext
}

// MiddlewareHandler represents the next handler in the middleware chain
type MiddlewareHandler func(ctx *MiddlewareContext) (interface{}, error)

// MiddlewarePipeline manages the execution of middleware chain
type MiddlewarePipeline struct {
	middlewares []Middleware
}

// JSONRPCMessage represents a JSON-RPC 2.0 message
type JSONRPCMessage struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Method  string        `json:"method"`
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

// MCPContext holds MCP-specific context information
type MCPContext struct {
	SessionID      string
	RequestID      string
	RequestContext interface{} // HTTP request context if available
}

// FastMCPContext holds FastMCP-specific context
type FastMCPContext struct {
	FastMCP interface{} // Reference to FastMCP server instance
}
