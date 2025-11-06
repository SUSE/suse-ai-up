package proxy

import (
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
	targetURL, err := url.Parse(adapter.RemoteUrl + "/mcp")
	if err != nil {
		return err
	}

	// Build target URL
	if c.Request.URL.RawQuery != "" {
		targetURL.RawQuery = c.Request.URL.RawQuery
	}

	// Create proxied request
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		return err
	}

	// Copy headers (excluding host)
	for k, v := range c.Request.Header {
		if k != "Host" {
			req.Header[k] = v
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

	// Copy response body
	io.Copy(c.Writer, resp.Body)

	return nil
}

func (p *RemoteHttpProxyPlugin) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
	// Simple health check
	resp, err := p.httpClient.Get(adapter.RemoteUrl + "/mcp")
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

func (p *RemoteHttpProxyPlugin) GetLogs(adapter models.AdapterResource) (string, error) {
	return "Remote server - no logs available", nil
}
