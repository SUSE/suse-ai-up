package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NewProxyClient creates a new proxy client for communicating with remote MCP servers
func NewProxyClient(remoteURL, authToken string) *ProxyClient {
	return &ProxyClient{
		remoteURL: remoteURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CallTool forwards a tool call to the remote MCP server
func (pc *ProxyClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	return pc.forwardRequest(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
}

// ReadResource forwards a resource read request to the remote MCP server
func (pc *ProxyClient) ReadResource(ctx context.Context, uri string) (interface{}, error) {
	return pc.forwardRequest(ctx, "resources/read", map[string]interface{}{
		"uri": uri,
	})
}

// GetPrompt forwards a prompt retrieval request to the remote MCP server
func (pc *ProxyClient) GetPrompt(ctx context.Context, name string) (interface{}, error) {
	return pc.forwardRequest(ctx, "prompts/get", map[string]interface{}{
		"name": name,
	})
}

// ListTools forwards a tools listing request to the remote MCP server
func (pc *ProxyClient) ListTools(ctx context.Context) (interface{}, error) {
	return pc.forwardRequest(ctx, "tools/list", nil)
}

// ListResources forwards a resources listing request to the remote MCP server
func (pc *ProxyClient) ListResources(ctx context.Context) (interface{}, error) {
	return pc.forwardRequest(ctx, "resources/list", nil)
}

// ListPrompts forwards a prompts listing request to the remote MCP server
func (pc *ProxyClient) ListPrompts(ctx context.Context) (interface{}, error) {
	return pc.forwardRequest(ctx, "prompts/list", nil)
}

// Initialize forwards an initialize request to the remote MCP server
func (pc *ProxyClient) Initialize(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	return pc.forwardRequest(ctx, "initialize", params)
}

// forwardRequest sends a JSON-RPC request to the remote MCP server
func (pc *ProxyClient) forwardRequest(ctx context.Context, method string, params interface{}) (interface{}, error) {
	// Create JSON-RPC request
	request := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  method,
		Params:  params,
	}

	// Serialize request
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", pc.remoteURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")

	// Add authentication if provided
	if pc.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+pc.authToken)
	}

	// Send request
	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(responseBody))
	}

	// Parse JSON-RPC response
	var response JSONRPCMessage
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for JSON-RPC error
	if response.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", response.Error.Code, response.Error.Message)
	}

	return response.Result, nil
}

// generateRequestID generates a unique request ID
func generateRequestID() int64 {
	return time.Now().UnixNano()
}
