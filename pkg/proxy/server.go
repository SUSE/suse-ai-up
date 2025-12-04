package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AsProxy creates a new MCP proxy server (equivalent to FastMCP.as_proxy())
func AsProxy(clientFactory func() *ProxyClient, name string) *MCPProxyServer {
	server := &MCPProxyServer{
		clientFactory: clientFactory,
		name:          name,
		middlewares:   []Middleware{},
	}

	return server
}

// AsProxyFromConfig creates a proxy server from MCPConfig (equivalent to FastMCP.as_proxy(config))
func AsProxyFromConfig(config *MCPConfig, name string) *MCPProxyServer {
	if len(config.MCPServers) == 1 {
		// Single server proxy
		for _, serverConfig := range config.MCPServers {
			clientFactory := func() *ProxyClient {
				return NewProxyClient(serverConfig.URL, serverConfig.AuthToken)
			}
			return AsProxy(clientFactory, name)
		}
	}

	// Multi-server proxy with prefixing (for future implementation)
	return AsProxy(func() *ProxyClient { return nil }, name)
}

// AddMiddleware adds middleware to the proxy server
func (s *MCPProxyServer) AddMiddleware(mw Middleware) {
	s.middlewares = append(s.middlewares, mw)
}

// HandleMCPRequest handles incoming MCP requests
func (s *MCPProxyServer) HandleMCPRequest(ctx context.Context, message *JSONRPCMessage) (interface{}, error) {
	// Process directly (middleware integration can be added later)
	return s.processMCPMessageDirect(message)
}

// processMCPMessageDirect processes the actual MCP message directly
func (s *MCPProxyServer) processMCPMessageDirect(message *JSONRPCMessage) (interface{}, error) {
	client := s.clientFactory()
	if client == nil {
		return nil, fmt.Errorf("no client factory configured")
	}

	// Route based on method
	switch message.Method {
	case "initialize":
		return client.Initialize(context.Background(), message.Params.(map[string]interface{}))

	case "tools/call":
		params := message.Params.(map[string]interface{})
		name := params["name"].(string)
		args := params["arguments"].(map[string]interface{})
		return client.CallTool(context.Background(), name, args)

	case "tools/list":
		return client.ListTools(context.Background())

	case "resources/read":
		params := message.Params.(map[string]interface{})
		uri := params["uri"].(string)
		return client.ReadResource(context.Background(), uri)

	case "resources/list":
		return client.ListResources(context.Background())

	case "prompts/get":
		params := message.Params.(map[string]interface{})
		name := params["name"].(string)
		return client.GetPrompt(context.Background(), name)

	case "prompts/list":
		return client.ListPrompts(context.Background())

	default:
		// Forward unknown methods to backend
		return client.forwardRequest(context.Background(), message.Method, message.Params)
	}
}

// HandleJSONRPC processes JSON-RPC formatted requests
func (s *MCPProxyServer) HandleJSONRPC(requestData []byte) ([]byte, error) {
	// Parse JSON-RPC request
	var request JSONRPCMessage
	if err := json.Unmarshal(requestData, &request); err != nil {
		return s.createJSONRPCError(nil, -32700, "Parse error", err.Error())
	}

	// Handle the request
	result, err := s.HandleMCPRequest(context.Background(), &request)
	if err != nil {
		return s.createJSONRPCError(request.ID, -32603, "Internal error", err.Error())
	}

	// Create JSON-RPC response
	response := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  result,
	}

	responseData, err := json.Marshal(response)
	if err != nil {
		return s.createJSONRPCError(request.ID, -32603, "Internal error", "Failed to marshal response")
	}

	return responseData, nil
}

// createJSONRPCError creates a JSON-RPC error response
func (s *MCPProxyServer) createJSONRPCError(id interface{}, code int, message, data string) ([]byte, error) {
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

	return json.Marshal(response)
}

// GetName returns the proxy server name
func (s *MCPProxyServer) GetName() string {
	return s.name
}

// GetTool returns tool information (placeholder - would be implemented based on actual MCP server capabilities)
func (s *MCPProxyServer) GetTool(name string) (*Tool, error) {
	// This is a placeholder - in a real implementation, this would query the backend server
	// or maintain a cache of discovered tools
	return nil, fmt.Errorf("tool not found: %s", name)
}

// GetResource returns resource information (placeholder)
func (s *MCPProxyServer) GetResource(uri string) (*Resource, error) {
	return nil, fmt.Errorf("resource not found: %s", uri)
}

// GetPrompt returns prompt information (placeholder)
func (s *MCPProxyServer) GetPrompt(name string) (*Prompt, error) {
	return nil, fmt.Errorf("prompt not found: %s", name)
}

// ListTools returns available tools (placeholder)
func (s *MCPProxyServer) ListTools() ([]*Tool, error) {
	// This would normally query the backend server for available tools
	return []*Tool{}, nil
}

// ListResources returns available resources (placeholder)
func (s *MCPProxyServer) ListResources() ([]*Resource, error) {
	return []*Resource{}, nil
}

// ListPrompts returns available prompts (placeholder)
func (s *MCPProxyServer) ListPrompts() ([]*Prompt, error) {
	return []*Prompt{}, nil
}

// GetStats returns basic proxy statistics (for future monitoring)
func (s *MCPProxyServer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"name":        s.name,
		"middlewares": len(s.middlewares),
		"uptime":      time.Since(time.Now()), // Placeholder
	}
}
