package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
)

// RemoteHttpProxyPlugin handles remote HTTP MCP servers
type RemoteHttpProxyPlugin struct {
	httpClient *http.Client
}

func NewRemoteHttpProxyPlugin() *RemoteHttpProxyPlugin {
	return &RemoteHttpProxyPlugin{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *RemoteHttpProxyPlugin) CanHandle(connectionType models.ConnectionType) bool {
	return connectionType == models.ConnectionTypeRemoteHttp
}

func (p *RemoteHttpProxyPlugin) ProxyRequest(c *gin.Context, adapter models.AdapterResource, sessionStore session.SessionStore) error {
	var targetURL *url.URL
	var err error
	var isGitHubAdapter bool

	// Check if this is a GitHub adapter (created from GitHub MCP server)
	if strings.HasPrefix(adapter.Name, "github-") {
		isGitHubAdapter = true
		targetURL, err = url.Parse("https://api.githubcopilot.com/mcp/")
		if err != nil {
			return err
		}
	} else {
		// Use the RemoteUrl directly - it should already include the full MCP endpoint
		targetURL, err = url.Parse(adapter.RemoteUrl)
		if err != nil {
			return err
		}

		// For MCP protocol servers, ensure the path includes /mcp
		if adapter.Protocol == models.ServerProtocolMCP && targetURL.Path == "" {
			targetURL.Path = "/mcp"
		}
	}

	// Build target URL
	if c.Request.URL.RawQuery != "" {
		targetURL.RawQuery = c.Request.URL.RawQuery
	}

	// Read and validate request body if it's JSON
	var bodyReader io.Reader = c.Request.Body
	if c.Request.Header.Get("Content-Type") == "application/json" {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		// Validate MCP protocol
		if err := p.validateMCPMessage(bytes.NewReader(bodyBytes)); err != nil {
			return fmt.Errorf("invalid MCP protocol message: %w", err)
		}

		// Create a new reader for the request body
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create proxied request
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), bodyReader)
	if err != nil {
		return err
	}

	// Copy headers (excluding host)
	for k, v := range c.Request.Header {
		if k != "Host" {
			req.Header[k] = v
		}
	}

	// Apply backend authentication if required by backend server
	backendAuthRequired := adapter.EnvironmentVariables["MCP_BACKEND_AUTH_REQUIRED"] == "true"
	if backendAuthRequired && adapter.Authentication != nil {
		if err := p.applyBackendAuthentication(req, adapter.Authentication); err != nil {
			return fmt.Errorf("failed to apply backend authentication: %w", err)
		}
	}

	// Apply GitHub authentication for GitHub adapters
	if isGitHubAdapter {
		if githubToken, exists := adapter.EnvironmentVariables["GITHUB_PAT"]; exists && githubToken != "" {
			req.Header.Set("Authorization", "Bearer "+githubToken)
			req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		} else {
			return fmt.Errorf("GitHub adapter requires GITHUB_PAT environment variable")
		}
	}

	// Ensure Accept header includes text/event-stream for MCP compatibility
	req.Header.Set("Accept", "application/json, text/event-stream")

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		c.Header(k, strings.Join(v, ","))
	}
	c.Status(resp.StatusCode)

	// Handle response body - parse SSE if needed
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		if err := p.handleSSEResponse(c, resp.Body); err != nil {
			return fmt.Errorf("failed to parse SSE response: %w", err)
		}
	} else {
		if _, err := io.Copy(c.Writer, resp.Body); err != nil {
			return fmt.Errorf("failed to copy response body: %w", err)
		}
	}

	return nil
}

// handleSSEResponse parses Server-Sent Events and extracts JSON data
func (p *RemoteHttpProxyPlugin) handleSSEResponse(c *gin.Context, body io.Reader) error {
	// Read the entire response body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	bodyStr := string(bodyBytes)
	lines := strings.Split(bodyStr, "\n")

	c.Header("Content-Type", "application/json")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			jsonData := strings.TrimPrefix(line, "data: ")
			if jsonData != "" && jsonData != "[DONE]" {
				_, err := c.Writer.Write([]byte(jsonData))
				if err != nil {
					return err
				}
				// For MCP responses, we typically only have one data line
				break
			}
		}
	}

	return nil
}

func (p *RemoteHttpProxyPlugin) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
	// Simple health check - use RemoteUrl directly since it includes full endpoint
	resp, err := p.httpClient.Get(adapter.RemoteUrl)
	if err != nil {
		return models.AdapterStatus{ReplicaStatus: "Unavailable"}, nil
	}
	resp.Body.Close()

	status := "Healthy"
	if resp.StatusCode != http.StatusOK {
		status = "Degraded"
	}

	return models.AdapterStatus{ReplicaStatus: status}, nil
}

// applyBackendAuthentication applies authentication to HTTP request based on adapter config
func (p *RemoteHttpProxyPlugin) applyBackendAuthentication(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth == nil || !auth.Required {
		return nil // No authentication required
	}

	switch auth.Type {
	case "bearer":
		return p.applyBearerAuth(req, auth)
	case "oauth":
		return p.applyOAuthAuth(req, auth)
	case "basic":
		return p.applyBasicAuth(req, auth)
	case "apikey":
		return p.applyAPIKeyAuth(req, auth)
	default:
		return fmt.Errorf("unsupported authentication type: %s", auth.Type)
	}
}

// applyBearerAuth applies bearer authentication to request
func (p *RemoteHttpProxyPlugin) applyBearerAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	var token string

	// Check bearer token configuration
	if auth.BearerToken != nil && auth.BearerToken.Token != "" {
		token = auth.BearerToken.Token
		// Note: Dynamic token generation would require token manager integration
	}

	if token == "" {
		return fmt.Errorf("no bearer token available")
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// applyOAuthAuth applies OAuth authentication to request
func (p *RemoteHttpProxyPlugin) applyOAuthAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth.OAuth == nil {
		return fmt.Errorf("OAuth configuration not found")
	}

	// Check if we have the required OAuth configuration
	if auth.OAuth.ClientID == "" || auth.OAuth.ClientSecret == "" || auth.OAuth.TokenURL == "" {
		return fmt.Errorf("OAuth configuration incomplete: missing client credentials or token URL")
	}

	// For now, OAuth token acquisition is not implemented
	// In a full implementation, this would:
	// 1. Check for cached access token
	// 2. If expired, request new token from TokenURL using client credentials
	// 3. Cache the new token
	// 4. Set Authorization header with Bearer token
	return fmt.Errorf("OAuth token acquisition not yet implemented - configure with pre-acquired access token using bearer auth type")
}

// applyBasicAuth applies basic authentication to request
func (p *RemoteHttpProxyPlugin) applyBasicAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth.Basic == nil {
		return fmt.Errorf("basic authentication configuration not found")
	}

	req.SetBasicAuth(auth.Basic.Username, auth.Basic.Password)
	return nil
}

// applyAPIKeyAuth applies API key authentication to request
func (p *RemoteHttpProxyPlugin) applyAPIKeyAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth.APIKey == nil {
		return fmt.Errorf("API key configuration not found")
	}

	location := strings.ToLower(auth.APIKey.Location)
	name := auth.APIKey.Name
	key := auth.APIKey.Key

	switch location {
	case "header":
		req.Header.Set(name, key)
	case "query":
		// Add to query parameters
		if req.URL == nil {
			return fmt.Errorf("request URL is nil")
		}
		query := req.URL.Query()
		query.Set(name, key)
		req.URL.RawQuery = query.Encode()
	case "cookie":
		// Add cookie
		req.AddCookie(&http.Cookie{Name: name, Value: key})
	default:
		return fmt.Errorf("unsupported API key location: %s", location)
	}

	return nil
}

func (p *RemoteHttpProxyPlugin) GetLogs(adapter models.AdapterResource) (string, error) {
	return "Remote server - no logs available", nil
}

// validateMCPMessage validates that the request body is a valid MCP JSON-RPC 2.0 message
func (p *RemoteHttpProxyPlugin) validateMCPMessage(body io.Reader) error {
	var message map[string]interface{}
	if err := json.NewDecoder(body).Decode(&message); err != nil {
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
				"tools/list":          true,
				"tools/call":          true,
				"resources/list":      true,
				"resources/read":      true,
				"prompts/list":        true,
				"prompts/get":         true,
				"completion/complete": true,
			}

			if !validMethods[methodStr] && !strings.HasPrefix(methodStr, "tools/") &&
				!strings.HasPrefix(methodStr, "resources/") && !strings.HasPrefix(methodStr, "prompts/") {
				// Allow custom methods but log for debugging
				fmt.Printf("MCP Validation: Unknown method '%s' - allowing but may be unsupported\n", methodStr)
			}
		}
	}

	return nil
}
