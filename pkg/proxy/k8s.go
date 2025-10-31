package proxy

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
)

// K8sProxyPlugin handles K8s-based adapters
type K8sProxyPlugin struct {
	kubeClient *clients.KubeClientWrapper
}

func NewK8sProxyPlugin(kubeClient *clients.KubeClientWrapper) *K8sProxyPlugin {
	return &K8sProxyPlugin{kubeClient: kubeClient}
}

func (p *K8sProxyPlugin) CanHandle(connectionType models.ConnectionType) bool {
	return connectionType == models.ConnectionTypeSSE || connectionType == models.ConnectionTypeStreamableHttp
}

func (p *K8sProxyPlugin) ProxyRequest(c *gin.Context, adapter models.AdapterResource, sessionStore session.SessionStore) error {
	// For local development, proxy to the MCP server URL from environment variables
	targetURL := adapter.EnvironmentVariables["MCP_PROXY_URL"]
	if targetURL == "" {
		targetURL = "http://localhost:8000/mcp" // Default for development
	}

	// Parse the target URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return err
	}

	// Ensure the path is /mcp for MCP requests
	parsedURL.Path = "/mcp"

	// Add query parameters if present
	if c.Request.URL.RawQuery != "" {
		parsedURL.RawQuery = c.Request.URL.RawQuery
	}

	// Create proxied request
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, parsedURL.String(), c.Request.Body)
	if err != nil {
		return err
	}

	// Copy headers (excluding host)
	for k, v := range c.Request.Header {
		if k != "Host" {
			req.Header[k] = v
		}
	}

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		c.Header(k, strings.Join(v, ","))
	}
	c.Status(resp.StatusCode)

	// Copy response body
	io.Copy(c.Writer, resp.Body)

	return nil
}

func (p *K8sProxyPlugin) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
	// For local development, check if the MCP server is reachable
	targetURL := adapter.EnvironmentVariables["MCP_PROXY_URL"]
	if targetURL == "" {
		targetURL = "http://localhost:8000/mcp"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(targetURL)
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

func (p *K8sProxyPlugin) GetLogs(adapter models.AdapterResource) (string, error) {
	return "Local MCP server - check server logs directly", nil
}
