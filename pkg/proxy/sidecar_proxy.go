package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
)

// SidecarProxyPlugin handles proxying requests to sidecar containers
type SidecarProxyPlugin struct {
	kubeClient *kubernetes.Clientset
	namespace  string
}

// NewSidecarProxyPlugin creates a new sidecar proxy plugin
func NewSidecarProxyPlugin(kubeClient *kubernetes.Clientset, namespace string) *SidecarProxyPlugin {
	return &SidecarProxyPlugin{
		kubeClient: kubeClient,
		namespace:  namespace,
	}
}

// CanHandle checks if this plugin can handle the connection type
func (p *SidecarProxyPlugin) CanHandle(connectionType models.ConnectionType) bool {
	return connectionType == models.ConnectionTypeSidecarStdio
}

// ProxyRequest proxies the MCP request to the sidecar container
func (p *SidecarProxyPlugin) ProxyRequest(c *gin.Context, adapter models.AdapterResource, sessionStore session.SessionStore) error {
	if adapter.SidecarConfig == nil {
		return fmt.Errorf("adapter does not have sidecar configuration")
	}

	// Construct the sidecar service URL
	sidecarURL := fmt.Sprintf("http://mcp-sidecar-%s.%s.svc.cluster.local:%d",
		adapter.ID, p.namespace, adapter.SidecarConfig.Port)

	// Build the target URL
	targetURL := sidecarURL + "/mcp"

	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Create a new request to the sidecar
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create sidecar request: %w", err)
	}

	// Copy headers
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Ensure Content-Type is set for JSON-RPC
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to proxy request to sidecar: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Set status code
	c.Status(resp.StatusCode)

	// Copy response body
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response body: %w", err)
	}

	return nil
}

// GetStatus returns the status of the sidecar deployment
func (p *SidecarProxyPlugin) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
	// For now, return a basic status
	// In a real implementation, this would check the Kubernetes deployment status
	return models.AdapterStatus{
		ReplicaStatus: "Running",
	}, nil
}

// GetLogs retrieves logs from the sidecar container
func (p *SidecarProxyPlugin) GetLogs(adapter models.AdapterResource) (string, error) {
	// For now, return a placeholder
	// In a real implementation, this would get logs from Kubernetes
	return "Sidecar container logs not available", nil
}
