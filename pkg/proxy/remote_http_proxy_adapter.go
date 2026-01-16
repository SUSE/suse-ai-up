package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RemoteHTTPProxyAdapter transforms remote HTTP MCP servers to streamable HTTP with session management
type RemoteHTTPProxyAdapter struct {
	httpClient      *http.Client
	sessionStore    session.SessionStore
	messageRouter   *mcp.MessageRouter
	protocolHandler *mcp.ProtocolHandler
	capabilityCache *mcp.CapabilityCache

	// Session management
	sessions map[string]*RemoteHTTPSession
	mutex    sync.RWMutex

	// Request correlation
	pendingRequests map[string]*RemotePendingRequest
	requestMutex    sync.RWMutex
}

// RemoteHTTPSession represents a remote HTTP session
type RemoteHTTPSession struct {
	ID            string
	AdapterName   string
	RemoteURL     string
	CreatedAt     time.Time
	LastActivity  time.Time
	IsInitialized bool

	// SSE management
	SSEConnections map[string]gin.ResponseWriter
	SSEMutex       sync.RWMutex

	// Authentication config
	AuthConfig *models.AdapterAuthConfig
}

// RemotePendingRequest tracks a request waiting for response
type RemotePendingRequest struct {
	ID         string
	Message    *mcp.JSONRPCMessage
	ResponseCh chan *mcp.JSONRPCMessage
	Timeout    time.Duration
	CreatedAt  time.Time
}

// NewRemoteHTTPProxyAdapter creates a new remote HTTP proxy adapter
func NewRemoteHTTPProxyAdapter(
	sessionStore session.SessionStore,
	messageRouter *mcp.MessageRouter,
	protocolHandler *mcp.ProtocolHandler,
	capabilityCache *mcp.CapabilityCache,
) *RemoteHTTPProxyAdapter {
	return &RemoteHTTPProxyAdapter{
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout for streaming
		},
		sessionStore:    sessionStore,
		messageRouter:   messageRouter,
		protocolHandler: protocolHandler,
		capabilityCache: capabilityCache,
		sessions:        make(map[string]*RemoteHTTPSession),
		pendingRequests: make(map[string]*RemotePendingRequest),
	}
}

// HandleRequest handles HTTP requests and proxies them to remote MCP servers
func (a *RemoteHTTPProxyAdapter) HandleRequest(c *gin.Context, adapter models.AdapterResource) error {
	log.Printf("RemoteHTTPProxy: Received request for adapter %s", adapter.Name)

	// Check for nil pointers
	if a.sessionStore == nil {
		log.Printf("RemoteHTTPProxy: ERROR - sessionStore is nil")
		return fmt.Errorf("sessionStore is nil")
	}
	if a.messageRouter == nil {
		log.Printf("RemoteHTTPProxy: ERROR - messageRouter is nil")
		return fmt.Errorf("messageRouter is nil")
	}

	// Extract session ID
	sessionID := a.extractSessionID(c)

	// Handle SSE requests first (they don't have JSON bodies)
	if c.Request.Header.Get("Accept") == "text/event-stream" {
		return a.handleSSEStream(c, adapter, sessionID)
	}

	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Parse JSON-RPC message
	var message mcp.JSONRPCMessage
	if err := json.Unmarshal(body, &message); err != nil {
		return fmt.Errorf("failed to parse JSON-RPC message: %w", err)
	}

	log.Printf("RemoteHTTPProxy: Handling %s request for session %s", message.Method, sessionID)

	// Handle different request types
	switch {
	case message.Method == "initialize":
		return a.handleInitialize(c, &message, adapter, sessionID, body)
	case strings.HasPrefix(message.Method, "notifications/"):
		return a.handleNotification(c, &message, adapter, sessionID)
	default:
		return a.handleRegularRequest(c, &message, adapter, sessionID)
	}
}

// handleNotification handles MCP notifications (no response expected)
func (a *RemoteHTTPProxyAdapter) handleNotification(c *gin.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("RemoteHTTPProxy: Handling notification %s for session %s", message.Method, sessionID)

	// Validate session
	if sessionID == "" {
		return fmt.Errorf("session ID required for notifications")
	}

	session := a.getSession(sessionID)
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update activity
	session.LastActivity = time.Now()

	// Send notification to remote server (no response expected)
	if err := a.proxyToRemote(c, session, message, false); err != nil {
		return fmt.Errorf("failed to send notification to remote server: %w", err)
	}

	// Return immediate success for notifications
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "result": "notification sent"})
	return nil
}

// handleInitialize handles MCP initialization
func (a *RemoteHTTPProxyAdapter) handleInitialize(c *gin.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string, body []byte) error {
	log.Printf("RemoteHTTPProxy: Initializing session for adapter %s", adapter.Name)

	// Generate new session ID if needed
	if sessionID == "" {
		sessionID = a.generateSessionID()
		c.Header("Mcp-Session-Id", sessionID)
	}

	// Create or get session
	session := a.getOrCreateSession(sessionID, adapter)

	// Forward initialize request to remote server and wait for response
	return a.forwardInitializeToRemote(c, session, message)
}

// forwardInitializeToRemote forwards initialize request to remote server and waits for response
func (a *RemoteHTTPProxyAdapter) forwardInitializeToRemote(c *gin.Context, session *RemoteHTTPSession, message *mcp.JSONRPCMessage) error {
	// Create pending request for initialize response
	pendingReq := &RemotePendingRequest{
		ID:         a.getRequestID(message),
		Message:    message,
		ResponseCh: make(chan *mcp.JSONRPCMessage, 1),
		Timeout:    30 * time.Second,
		CreatedAt:  time.Now(),
	}

	a.requestMutex.Lock()
	a.pendingRequests[pendingReq.ID] = pendingReq
	a.requestMutex.Unlock()

	defer func() {
		a.requestMutex.Lock()
		delete(a.pendingRequests, pendingReq.ID)
		a.requestMutex.Unlock()
	}()

	// Send initialize message to remote server
	if err := a.proxyToRemote(c, session, message, false); err != nil {
		return fmt.Errorf("failed to send initialize to remote server: %w", err)
	}

	// Wait for initialize response
	select {
	case response := <-pendingReq.ResponseCh:
		c.Header("Content-Type", "application/json")
		c.JSON(http.StatusOK, response)
		log.Printf("RemoteHTTPProxy: Initialize completed for session %s", session.ID)
		return nil
	case <-time.After(pendingReq.Timeout):
		return fmt.Errorf("initialize timeout after %v", pendingReq.Timeout)
	case <-c.Request.Context().Done():
		return fmt.Errorf("client disconnected during initialize")
	}
}

// handleSSEStream handles Server-Sent Events streaming
func (a *RemoteHTTPProxyAdapter) handleSSEStream(c *gin.Context, adapter models.AdapterResource, sessionID string) error {
	log.Printf("RemoteHTTPProxy: Opening SSE stream for session %s", sessionID)

	// Generate session ID if needed
	if sessionID == "" {
		sessionID = a.generateSessionID()
		c.Header("Mcp-Session-Id", sessionID)
	}

	// Get or create session
	session := a.getOrCreateSession(sessionID, adapter)

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Set CORS headers
	origin := c.GetHeader("Origin")
	if origin != "" && (strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1")) {
		c.Header("Access-Control-Allow-Origin", origin)
	} else {
		c.Header("Access-Control-Allow-Origin", "*")
	}

	// Flush headers immediately
	c.Writer.WriteHeader(http.StatusOK)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	// Register SSE connection
	connectionID := uuid.New().String()
	session.SSEMutex.Lock()
	if session.SSEConnections == nil {
		session.SSEConnections = make(map[string]gin.ResponseWriter)
	}
	session.SSEConnections[connectionID] = c.Writer
	session.SSEMutex.Unlock()

	// Clean up on disconnect
	defer func() {
		session.SSEMutex.Lock()
		delete(session.SSEConnections, connectionID)
		session.SSEMutex.Unlock()
	}()

	// Start SSE proxy from remote server
	go a.startSSEProxy(session, c.Writer)

	// Keep connection alive
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			log.Printf("RemoteHTTPProxy: SSE client disconnected for session %s", sessionID)
			return nil
		case <-keepalive.C:
			// Send keepalive
			a.writeSSEEvent(c.Writer, "keepalive", "")
		}
	}
}

// handleRegularRequest handles regular JSON-RPC requests
func (a *RemoteHTTPProxyAdapter) handleRegularRequest(c *gin.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	// Validate session
	if sessionID == "" {
		return fmt.Errorf("session ID required for non-initialize requests")
	}

	session := a.getSession(sessionID)
	if session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update activity
	session.LastActivity = time.Now()

	// Create pending request
	pendingReq := &RemotePendingRequest{
		ID:         a.getRequestID(message),
		Message:    message,
		ResponseCh: make(chan *mcp.JSONRPCMessage, 1),
		Timeout:    30 * time.Second,
		CreatedAt:  time.Now(),
	}

	a.requestMutex.Lock()
	a.pendingRequests[pendingReq.ID] = pendingReq
	a.requestMutex.Unlock()

	defer func() {
		a.requestMutex.Lock()
		delete(a.pendingRequests, pendingReq.ID)
		a.requestMutex.Unlock()
	}()

	// Send message to remote server
	if err := a.proxyToRemote(c, session, message, false); err != nil {
		return fmt.Errorf("failed to send to remote server: %w", err)
	}

	// Wait for response
	select {
	case response := <-pendingReq.ResponseCh:
		c.Header("Content-Type", "application/json")
		c.JSON(http.StatusOK, response)
		return nil
	case <-time.After(pendingReq.Timeout):
		return fmt.Errorf("request timeout after %v", pendingReq.Timeout)
	case <-c.Request.Context().Done():
		return fmt.Errorf("client disconnected")
	}
}

// startSSEProxy starts proxying SSE from remote server
func (a *RemoteHTTPProxyAdapter) startSSEProxy(session *RemoteHTTPSession, clientWriter gin.ResponseWriter) {
	log.Printf("RemoteHTTPProxy: Starting SSE proxy for session %s", session.ID)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("RemoteHTTPProxy: SSE proxy panic for session %s: %v", session.ID, r)
		}
	}()

	// Create request to remote server's SSE endpoint
	req, err := http.NewRequest("GET", session.RemoteURL, nil)
	if err != nil {
		log.Printf("RemoteHTTPProxy: Failed to create SSE request: %v", err)
		return
	}

	// Set headers for SSE
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Mcp-Session-Id", session.ID)

	// Apply authentication
	if session.AuthConfig != nil && session.AuthConfig.Required {
		if err := a.applyAuthentication(req, session.AuthConfig); err != nil {
			log.Printf("RemoteHTTPProxy: Failed to apply authentication: %v", err)
			return
		}
	}

	// Send request to remote server
	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Printf("RemoteHTTPProxy: Failed to connect to remote server: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("RemoteHTTPProxy: Remote server returned status %d", resp.StatusCode)
		return
	}

	// Proxy SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		log.Printf("RemoteHTTPProxy: Received SSE: %s", line)

		// Parse SSE message
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Parse JSON-RPC message
			var message mcp.JSONRPCMessage
			if err := json.Unmarshal([]byte(data), &message); err != nil {
				log.Printf("RemoteHTTPProxy: Failed to parse SSE message: %v", err)
				continue
			}

			// Handle message
			a.handleRemoteMessage(session, &message)
		}

		// Forward SSE to client
		if _, err := fmt.Fprintln(clientWriter, line); err != nil {
			log.Printf("RemoteHTTPProxy: Failed to write SSE to client: %v", err)
			break
		}

		// Flush
		if flusher, ok := clientWriter.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("RemoteHTTPProxy: SSE scanner error for session %s: %v", session.ID, err)
	}
}

// handleRemoteMessage processes messages from remote server
func (a *RemoteHTTPProxyAdapter) handleRemoteMessage(session *RemoteHTTPSession, message *mcp.JSONRPCMessage) {
	// Check if this is a response to a pending request
	if message.ID != nil {
		requestID := a.getRequestID(message)
		a.requestMutex.RLock()
		if pendingReq, exists := a.pendingRequests[requestID]; exists {
			// Send response to waiting goroutine
			select {
			case pendingReq.ResponseCh <- message:
				log.Printf("RemoteHTTPProxy: Delivered response for request %s", requestID)
			default:
				log.Printf("RemoteHTTPProxy: Response channel full for request %s", requestID)
			}
			a.requestMutex.RUnlock()
			return
		}
		a.requestMutex.RUnlock()
	}

	// This is a notification or unsolicited message
	// Forward to all SSE connections
	a.broadcastToSSE(session, message)
}

// proxyToRemote proxies a message to the remote server
func (a *RemoteHTTPProxyAdapter) proxyToRemote(c *gin.Context, session *RemoteHTTPSession, message *mcp.JSONRPCMessage, isSSE bool) error {
	// Marshal message
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create request to remote server
	var req *http.Request
	if isSSE {
		req, err = http.NewRequest("GET", session.RemoteURL, nil)
	} else {
		req, err = http.NewRequest("POST", session.RemoteURL, strings.NewReader(string(data)))
	}
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if !isSSE {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", session.ID)

	// Apply authentication
	if session.AuthConfig != nil && session.AuthConfig.Required {
		if err := a.applyAuthentication(req, session.AuthConfig); err != nil {
			return fmt.Errorf("failed to apply authentication: %w", err)
		}
	}

	// Send request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("remote server returned status %d", resp.StatusCode)
	}

	// For non-SSE requests, read and handle response
	if !isSSE {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		var response mcp.JSONRPCMessage
		if err := json.Unmarshal(body, &response); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		// Handle response
		a.handleRemoteMessage(session, &response)
	}

	return nil
}

// applyAuthentication applies authentication to HTTP request
func (a *RemoteHTTPProxyAdapter) applyAuthentication(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth == nil || !auth.Required {
		return nil // No authentication required
	}

	switch auth.Type {
	case "bearer":
		return a.applyBearerAuth(req, auth)
	case "oauth":
		return a.applyOAuthAuth(req, auth)
	case "basic":
		return a.applyBasicAuth(req, auth)
	case "apikey":
		return a.applyAPIKeyAuth(req, auth)
	default:
		return fmt.Errorf("unsupported authentication type: %s", auth.Type)
	}
}

// applyBearerAuth applies bearer authentication to request
func (a *RemoteHTTPProxyAdapter) applyBearerAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	var token string

	// Check bearer token configuration
	if auth.BearerToken != nil && auth.BearerToken.Token != "" {
		token = auth.BearerToken.Token
	}

	if token == "" {
		return fmt.Errorf("no bearer token available")
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// applyOAuthAuth applies OAuth authentication to request
func (a *RemoteHTTPProxyAdapter) applyOAuthAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	// For now, this is a placeholder
	// In a full implementation, this would handle OAuth token management
	return fmt.Errorf("OAuth authentication not yet implemented for request signing")
}

// applyBasicAuth applies basic authentication to request
func (a *RemoteHTTPProxyAdapter) applyBasicAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
	if auth.Basic == nil {
		return fmt.Errorf("basic authentication configuration not found")
	}

	req.SetBasicAuth(auth.Basic.Username, auth.Basic.Password)
	return nil
}

// applyAPIKeyAuth applies API key authentication to request
func (a *RemoteHTTPProxyAdapter) applyAPIKeyAuth(req *http.Request, auth *models.AdapterAuthConfig) error {
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

// broadcastToSSE broadcasts a message to all SSE connections
func (a *RemoteHTTPProxyAdapter) broadcastToSSE(session *RemoteHTTPSession, message *mcp.JSONRPCMessage) {
	session.SSEMutex.RLock()
	defer session.SSEMutex.RUnlock()

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("RemoteHTTPProxy: Failed to marshal SSE message: %v", err)
		return
	}

	for connID, writer := range session.SSEConnections {
		if err := a.writeSSEEvent(writer, "message", string(data)); err != nil {
			log.Printf("RemoteHTTPProxy: Failed to write SSE to connection %s: %v", connID, err)
			// Connection might be dead, cleanup will happen on next write
		}
	}
}

// writeSSEEvent writes an SSE event
func (a *RemoteHTTPProxyAdapter) writeSSEEvent(w gin.ResponseWriter, event, data string) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}

	if data != "" {
		lines := strings.Split(data, "\n")
		for _, line := range lines {
			if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
				return err
			}
		}
	}

	// End event
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}

	// Flush
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// getOrCreateSession gets existing session or creates new one
func (a *RemoteHTTPProxyAdapter) getOrCreateSession(sessionID string, adapter models.AdapterResource) *RemoteHTTPSession {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if session, exists := a.sessions[sessionID]; exists {
		session.LastActivity = time.Now()
		return session
	}

	session := &RemoteHTTPSession{
		ID:             sessionID,
		AdapterName:    adapter.Name,
		RemoteURL:      adapter.RemoteUrl,
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		SSEConnections: make(map[string]gin.ResponseWriter),
		AuthConfig:     adapter.Authentication,
	}

	a.sessions[sessionID] = session

	// Register in session store
	a.sessionStore.SetWithDetails(sessionID, adapter.Name, "", "remote-http")

	return session
}

// getSession retrieves a session by ID
func (a *RemoteHTTPProxyAdapter) getSession(sessionID string) *RemoteHTTPSession {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.sessions[sessionID]
}

// extractSessionID extracts session ID from request
func (a *RemoteHTTPProxyAdapter) extractSessionID(c *gin.Context) string {
	sessionID := c.GetHeader("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = c.GetHeader("mcp-session-id")
	}
	if sessionID == "" {
		sessionID = c.Query("sessionId")
	}
	return sessionID
}

// generateSessionID generates a new session ID
func (a *RemoteHTTPProxyAdapter) generateSessionID() string {
	return fmt.Sprintf("remote-http-%d", time.Now().UnixNano())
}

// getRequestID extracts request ID from message
func (a *RemoteHTTPProxyAdapter) getRequestID(message *mcp.JSONRPCMessage) string {
	if message.ID == nil {
		return ""
	}

	switch v := message.ID.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', 0, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// CanHandle checks if this adapter can handle the connection type
func (a *RemoteHTTPProxyAdapter) CanHandle(connectionType models.ConnectionType) bool {
	return connectionType == models.ConnectionTypeRemoteHttp
}

// GetStatus returns adapter status
func (a *RemoteHTTPProxyAdapter) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
	a.mutex.RLock()
	activeSessions := 0
	for _, session := range a.sessions {
		if time.Since(session.LastActivity) < 5*time.Minute {
			activeSessions++
		}
	}
	a.mutex.RUnlock()

	status := "Ready"
	if activeSessions == 0 {
		status = "Idle"
	} else if activeSessions > 10 {
		status = "Busy"
	}

	return models.AdapterStatus{
		ReplicaStatus: status,
	}, nil
}

// GetLogs returns adapter logs
func (a *RemoteHTTPProxyAdapter) GetLogs(adapter models.AdapterResource) (string, error) {
	return fmt.Sprintf("Remote HTTP Proxy Adapter for %s\nActive Sessions: %d\n",
		adapter.Name, len(a.sessions)), nil
}

// Cleanup cleans up resources
func (a *RemoteHTTPProxyAdapter) Cleanup(adapterID string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if session, exists := a.sessions[adapterID]; exists {
		// Close SSE connections
		session.SSEMutex.Lock()
		for _, writer := range session.SSEConnections {
			// Try to close connection
			if writer != nil {
				writer.WriteHeader(http.StatusGone)
			}
		}
		session.SSEMutex.Unlock()

		delete(a.sessions, adapterID)
	}

	return nil
}
