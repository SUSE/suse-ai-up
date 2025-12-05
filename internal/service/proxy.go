package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/proxy"
	"suse-ai-up/pkg/session"
)

// ProxyHandler handles proxying requests
type ProxyHandler struct {
	sessionStore session.SessionStore
	kubeClient   *clients.KubeClientWrapper
	httpClient   *http.Client
	plugins      map[string]proxy.AdapterProxyPlugin
	store        clients.AdapterResourceStore
}

// NewProxyHandler creates a new ProxyHandler
func NewProxyHandler(store session.SessionStore, kubeClient *clients.KubeClientWrapper, adapterStore clients.AdapterResourceStore) *ProxyHandler {
	plugins := make(map[string]proxy.AdapterProxyPlugin)
	plugins["k8s"] = proxy.NewK8sProxyPlugin(kubeClient)
	plugins["remoteHttp"] = proxy.NewRemoteHttpProxyPlugin()
	plugins["localStdio"] = proxy.NewLocalStdioProxyPlugin()

	return &ProxyHandler{
		sessionStore: store,
		kubeClient:   kubeClient,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		plugins:      plugins,
		store:        adapterStore,
	}
}

// ForwardStreamableHttp handles POST /adapters/:name/mcp
func (ph *ProxyHandler) ForwardStreamableHttp(c *gin.Context) {
	name := c.Param("name")
	log.Printf("ForwardStreamableHttp: Request for adapter %s", name)

	// Get adapter from store
	ctx := context.Background()
	userID := c.GetString("userId")
	if userID == "" {
		userID = "default-user"
	}
	adapter, err := ph.store.Get(ctx, name)
	if err != nil || adapter.CreatedBy != userID {
		log.Printf("ForwardStreamableHttp: Adapter %s not found", name)
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Find plugin
	var plugin proxy.AdapterProxyPlugin
	switch adapter.ConnectionType {
	case models.ConnectionTypeRemoteHttp:
		plugin = ph.plugins["remoteHttp"]
	case models.ConnectionTypeLocalStdio:
		plugin = ph.plugins["localStdio"]
	case models.ConnectionTypeSSE, models.ConnectionTypeStreamableHttp:
		plugin = ph.plugins["k8s"]
	default:
		log.Printf("ForwardStreamableHttp: Unsupported connection type %s", adapter.ConnectionType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported connection type"})
		return
	}

	if plugin == nil {
		log.Printf("ForwardStreamableHttp: Plugin not found for %s", adapter.ConnectionType)
		c.Status(http.StatusInternalServerError)
		return
	}

	// Update session activity and details if session ID is present
	sessionID := ph.getSessionID(c)
	if sessionID != "" {
		if err := ph.sessionStore.UpdateActivity(sessionID); err != nil {
			log.Printf("ForwardStreamableHttp: Failed to update session activity for %s: %v", sessionID, err)
			// Don't fail the request for this
		}

		// Update session with adapter details if not already set
		if details, err := ph.sessionStore.GetDetails(sessionID); err == nil && details.AdapterName == "" {
			// Update session with adapter information
			ph.sessionStore.SetWithDetails(sessionID, name, details.TargetAddress, string(adapter.ConnectionType))
		}
	}

	// Proxy using plugin
	if err := plugin.ProxyRequest(c, *adapter, ph.sessionStore); err != nil {
		log.Printf("ForwardStreamableHttp: Proxy error: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	}
}

// ForwardMessages handles POST /adapters/:name/messages
// swagger:route POST /adapters/{name}/messages adapters forwardMessages
//
// # Forward messages
//
// Proxies the request to the adapter's messages endpoint.
//
// responses:
//
//	200:
//	400: ErrorResponse
//	503: ErrorResponse
func (ph *ProxyHandler) ForwardMessages(c *gin.Context) {
	sessionID := ph.getSessionID(c)
	if sessionID == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	target, exists := ph.sessionStore.Get(sessionID)
	if !exists {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// Proxy the request
	ph.proxyRequest(c, target, sessionID)
}

// ForwardSSE handles GET /adapters/:name/sse
// swagger:route GET /adapters/{name}/sse adapters forwardSSE
//
// # Forward SSE
//
// Proxies the request to the adapter's SSE endpoint.
//
// responses:
//
//	200:
//	503: ErrorResponse
func (ph *ProxyHandler) ForwardSSE(c *gin.Context) {
	name := c.Param("name")
	ctx := context.Background()

	target, err := ph.getNewSessionTarget(name, ctx)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// Proxy SSE request
	ph.proxySSERequest(c, target, name)
}

// Helper methods
func (ph *ProxyHandler) getNewSessionTarget(name string, ctx context.Context) (string, error) {
	// Get pod addresses for load balancing
	addresses, err := ph.getPodAddresses(name, ctx)
	if err != nil {
		return "", err
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("no healthy pods found for adapter %s", name)
	}

	// Simple round-robin: return first available pod
	return addresses[0], nil
}

// getPodAddresses returns addresses of healthy pods for an adapter
func (ph *ProxyHandler) getPodAddresses(adapterName string, ctx context.Context) ([]string, error) {
	// In local development mode (no Kubernetes), return a mock local address
	if ph.kubeClient == nil {
		log.Printf("ProxyHandler: Using local mode for adapter %s, targeting localhost:8000", adapterName)
		// For local testing, assume the MCP server is running on localhost:8000
		// In a real scenario, this would be configurable per adapter
		return []string{"http://localhost:8000"}, nil
	}

	podList, err := ph.kubeClient.ListPods("adapter",
		fmt.Sprintf("app=%s,type=mcp-adapter", adapterName),
		"status.phase=Running", ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var addresses []string
	for _, pod := range podList.Items {
		if ph.isPodReady(&pod) {
			address := fmt.Sprintf("http://%s.%s-service.adapter.svc.cluster.local:8000",
				pod.Name, adapterName)
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

// isPodReady checks if a pod is ready
func (ph *ProxyHandler) isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// getSessionID extracts session ID from request headers
func (ph *ProxyHandler) getSessionID(c *gin.Context) string {
	sessionID := c.GetHeader("mcp-session-id")
	if sessionID == "" {
		sessionID = c.GetHeader("session_id")
	}
	if sessionID == "" {
		sessionID = c.Query("session_id")
	}
	return sessionID
}

func (ph *ProxyHandler) proxyRequest(c *gin.Context, targetAddress, sessionID string) {
	// Build target URL - for MCP, use the base MCP endpoint
	// The gateway path (/adapters/:name/mcp) should map to the target's MCP endpoint
	targetURL := targetAddress + "/mcp"
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}
	log.Printf("ProxyHandler: Proxying to target URL: %s", targetURL)

	// Create proxied request
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	// Copy headers (excluding host)
	for k, v := range c.Request.Header {
		if k != "Host" {
			req.Header[k] = v
		}
	}

	// Add authorization header if we have a valid token for this session
	if sessionID != "" && ph.sessionStore.IsTokenValid(sessionID) {
		if tokenInfo, err := ph.sessionStore.GetTokenInfo(sessionID); err == nil && tokenInfo != nil {
			authHeader := fmt.Sprintf("Bearer %s", tokenInfo.AccessToken)
			req.Header.Set("Authorization", authHeader)
			log.Printf("ProxyHandler: Added authorization header for session %s", sessionID)
		}
	}

	// Send request
	resp, err := ph.httpClient.Do(req)
	if err != nil {
		log.Printf("ProxyHandler: HTTP request failed: %v", err)
		c.Status(http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Handle 401 Unauthorized responses for OAuth flows
	if resp.StatusCode == http.StatusUnauthorized {
		// Check for WWW-Authenticate header indicating OAuth protected resource
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		if strings.Contains(wwwAuth, "oauth-protected-resource") {
			log.Printf("ProxyHandler: Detected OAuth protected resource, status: %s", wwwAuth)

			// Extract resource metadata URL from WWW-Authenticate header
			// Format: Bearer error="invalid_token", error_description="...", resource_metadata="https://..."
			resourceMetadataURL := ph.extractResourceMetadataURL(wwwAuth)
			if resourceMetadataURL != "" {
				log.Printf("ProxyHandler: Resource metadata URL: %s", resourceMetadataURL)

				// In full implementation, this would trigger OAuth discovery and authorization
				// For now, return a helpful error message
				// Get adapter name from gin context
				adapterName := c.Param("name")
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":                 "oauth_authorization_required",
					"message":               "This MCP server requires OAuth authorization",
					"resource_metadata_url": resourceMetadataURL,
					"adapter":               adapterName,
				})
				return
			}
		}
	}

	// Copy response headers
	for k, v := range resp.Header {
		c.Header(k, strings.Join(v, ","))
	}
	c.Status(resp.StatusCode)

	// Copy response body
	io.Copy(c.Writer, resp.Body)

	// If new session, extract and store session ID
	if sessionID == "" {
		if newSessionID := ph.extractSessionID(resp); newSessionID != "" {
			log.Printf("ProxyHandler: Extracted new session ID: %s for target: %s", newSessionID, targetAddress)
			// Note: adapterName is not available here, will be set when session is first used
			ph.sessionStore.Set(newSessionID, targetAddress)
		} else {
			log.Printf("ProxyHandler: No session ID found in response headers")
		}
	}
}

// extractResourceMetadataURL extracts the resource metadata URL from WWW-Authenticate header
func (ph *ProxyHandler) extractResourceMetadataURL(wwwAuth string) string {
	// Parse WWW-Authenticate header for resource_metadata parameter
	// Format: Bearer error="invalid_token", resource_metadata="https://example.com/.well-known/oauth-protected-resource"
	parts := strings.Split(wwwAuth, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, `resource_metadata="`) {
			// Extract URL from quotes
			url := strings.TrimPrefix(part, `resource_metadata="`)
			url = strings.TrimSuffix(url, `"`)
			return url
		}
	}
	return ""
}

// extractSessionID extracts session ID from response headers
func (ph *ProxyHandler) extractSessionID(resp *http.Response) string {
	sessionID := resp.Header.Get("mcp-session-id")
	if sessionID == "" {
		sessionID = resp.Header.Get("session-id")
	}
	return sessionID
}

func (ph *ProxyHandler) proxySSERequest(c *gin.Context, targetAddress, adapterName string) {
	// Build target URL
	targetURL := targetAddress + "/mcp"
	log.Printf("ProxyHandler: Proxying SSE to target URL: %s", targetURL)

	// First, establish a session by making an initialize request
	sessionID, err := ph.establishSession(targetAddress)
	if err != nil {
		log.Printf("ProxyHandler: Failed to establish session: %v", err)
		c.Status(http.StatusServiceUnavailable)
		return
	}

	log.Printf("ProxyHandler: Established session ID: %s", sessionID)

	// Update session with adapter details
	ph.sessionStore.SetWithDetails(sessionID, adapterName, targetAddress, "SSE")

	// Now make the SSE request with the session ID
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", targetURL, nil)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	// Copy headers (excluding host)
	for k, v := range c.Request.Header {
		if k != "Host" {
			req.Header[k] = v
		}
	}

	// Set required headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("mcp-session-id", sessionID)

	// Send request
	resp, err := ph.httpClient.Do(req)
	if err != nil {
		log.Printf("ProxyHandler: SSE request failed: %v", err)
		c.Status(http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		c.Header(k, strings.Join(v, ","))
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	// Store the session for future use
	ph.sessionStore.Set(sessionID, targetAddress)

	// Stream the SSE response
	io.Copy(c.Writer, resp.Body)
}

// establishSession makes an initialize request to establish a session
func (ph *ProxyHandler) establishSession(targetAddress string) (string, error) {
	targetURL := targetAddress + "/mcp"

	// Create initialize request
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"mcp-gateway","version":"1.0"}}}`
	req, err := http.NewRequest("POST", targetURL, strings.NewReader(initBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := ph.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("initialize request failed with status %d", resp.StatusCode)
	}

	sessionID := resp.Header.Get("mcp-session-id")
	if sessionID == "" {
		sessionID = resp.Header.Get("session-id")
	}

	if sessionID == "" {
		return "", fmt.Errorf("no session ID in initialize response")
	}

	return sessionID, nil
}
