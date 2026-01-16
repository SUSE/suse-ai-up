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

// StdioToHTTPAdapter transforms stdio MCP servers to streamable HTTP
type StdioToHTTPAdapter struct {
	stdioProxy      *LocalStdioProxyPlugin
	messageRouter   *mcp.MessageRouter
	sessionStore    session.SessionStore
	protocolHandler *mcp.ProtocolHandler
	capabilityCache *mcp.CapabilityCache

	// Session management
	sessions map[string]*StdioSession
	mutex    sync.RWMutex

	// Request correlation
	pendingRequests map[string]*PendingRequest
	requestMutex    sync.RWMutex
}

// StdioSession represents a transformed stdio session
type StdioSession struct {
	ID            string
	AdapterName   string
	Process       *runningProcess
	CreatedAt     time.Time
	LastActivity  time.Time
	IsInitialized bool

	// SSE management
	SSEConnections map[string]gin.ResponseWriter
	SSEMutex       sync.RWMutex
}

// PendingRequest tracks a request waiting for response
type PendingRequest struct {
	ID         string
	Message    *mcp.JSONRPCMessage
	ResponseCh chan *mcp.JSONRPCMessage
	Timeout    time.Duration
	CreatedAt  time.Time
}

// NewStdioToHTTPAdapter creates a new stdio-to-HTTP adapter
func NewStdioToHTTPAdapter(
	stdioProxy *LocalStdioProxyPlugin,
	messageRouter *mcp.MessageRouter,
	sessionStore session.SessionStore,
	protocolHandler *mcp.ProtocolHandler,
	capabilityCache *mcp.CapabilityCache,
) *StdioToHTTPAdapter {
	return &StdioToHTTPAdapter{
		stdioProxy:      stdioProxy,
		messageRouter:   messageRouter,
		sessionStore:    sessionStore,
		protocolHandler: protocolHandler,
		capabilityCache: capabilityCache,
		sessions:        make(map[string]*StdioSession),
		pendingRequests: make(map[string]*PendingRequest),
	}
}

// HandleRequest handles HTTP requests and transforms them to stdio
func (a *StdioToHTTPAdapter) HandleRequest(c *gin.Context, adapter models.AdapterResource) error {
	log.Printf("StdioToHTTP: Received request for adapter %s", adapter.Name)

	// Check for nil pointers
	if a.stdioProxy == nil {
		log.Printf("StdioToHTTP: ERROR - stdioProxy is nil")
		return fmt.Errorf("stdioProxy is nil")
	}
	if a.messageRouter == nil {
		log.Printf("StdioToHTTP: ERROR - messageRouter is nil")
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

	log.Printf("StdioToHTTP: Handling %s request for session %s", message.Method, sessionID)

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
func (a *StdioToHTTPAdapter) handleNotification(c *gin.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("StdioToHTTP: Handling notification %s for session %s", message.Method, sessionID)

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

	// Send notification to stdio process (no response expected)
	if err := a.sendToStdio(session, message); err != nil {
		return fmt.Errorf("failed to send notification to stdio: %w", err)
	}

	// Return immediate success for notifications
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "result": "notification sent"})
	return nil
}

// handleInitialize handles MCP initialization
func (a *StdioToHTTPAdapter) handleInitialize(c *gin.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string, body []byte) error {
	log.Printf("StdioToHTTP: Initializing session for adapter %s", adapter.Name)

	// Generate new session ID if needed
	if sessionID == "" {
		sessionID = a.generateSessionID()
		c.Header("Mcp-Session-Id", sessionID)
	}

	// Create or get session
	session := a.getOrCreateSession(sessionID, adapter)

	// Start stdio process if not running
	if session.Process == nil {
		proc, err := a.stdioProxy.getOrStartProcess(adapter)
		if err != nil {
			return fmt.Errorf("failed to start stdio process: %w", err)
		}
		session.Process = proc
	}

	// Start message listener if not already running
	if !session.IsInitialized {
		go a.startMessageListener(session, adapter)
		session.IsInitialized = true
	}

	// Forward initialize request to stdio process (don't handle locally)
	return a.forwardInitializeToStdio(c, session, message)
}

// forwardInitializeToStdio forwards initialize request to stdio process and waits for response
func (a *StdioToHTTPAdapter) forwardInitializeToStdio(c *gin.Context, session *StdioSession, message *mcp.JSONRPCMessage) error {
	// Create pending request for initialize response
	pendingReq := &PendingRequest{
		ID:         a.getRequestID(message),
		Message:    message,
		ResponseCh: make(chan *mcp.JSONRPCMessage, 1),
		Timeout:    60 * time.Second, // Increased timeout for slow-initializing MCP servers
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

	// Send initialize message to stdio process
	if err := a.sendToStdio(session, message); err != nil {
		return fmt.Errorf("failed to send initialize to stdio: %w", err)
	}

	// Wait for initialize response
	select {
	case response := <-pendingReq.ResponseCh:
		c.Header("Content-Type", "application/json")
		c.JSON(http.StatusOK, response)
		log.Printf("StdioToHTTP: Initialize completed for session %s", session.ID)
		return nil
	case <-time.After(pendingReq.Timeout):
		return fmt.Errorf("initialize timeout after %v", pendingReq.Timeout)
	case <-c.Request.Context().Done():
		return fmt.Errorf("client disconnected during initialize")
	}
}

// handleSSEStream handles Server-Sent Events streaming
func (a *StdioToHTTPAdapter) handleSSEStream(c *gin.Context, adapter models.AdapterResource, sessionID string) error {
	log.Printf("StdioToHTTP: Opening SSE stream for session %s", sessionID)

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

	// Keep connection alive
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			log.Printf("StdioToHTTP: SSE client disconnected for session %s", sessionID)
			return nil
		case <-keepalive.C:
			// Send keepalive
			a.writeSSEEvent(c.Writer, "keepalive", "")
		}
	}
}

// handleRegularRequest handles regular JSON-RPC requests
func (a *StdioToHTTPAdapter) handleRegularRequest(c *gin.Context, message *mcp.JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
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
	pendingReq := &PendingRequest{
		ID:         a.getRequestID(message),
		Message:    message,
		ResponseCh: make(chan *mcp.JSONRPCMessage, 1),
		Timeout:    60 * time.Second, // Increased timeout for slow MCP servers
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

	// Send message to stdio process
	if err := a.sendToStdio(session, message); err != nil {
		return fmt.Errorf("failed to send to stdio: %w", err)
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

// startMessageListener listens for messages from stdio process
func (a *StdioToHTTPAdapter) startMessageListener(session *StdioSession, adapter models.AdapterResource) {
	log.Printf("StdioToHTTP: Starting message listener for session %s", session.ID)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("StdioToHTTP: Message listener panic for session %s: %v", session.ID, r)
		}
	}()

	scanner := bufio.NewScanner(session.Process.stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		log.Printf("StdioToHTTP: Received from stdio: %s", line)

		// Parse JSON-RPC message
		var message mcp.JSONRPCMessage
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			log.Printf("StdioToHTTP: Failed to parse message: %v", err)
			continue
		}

		// Handle message
		a.handleStdioMessage(session, &message)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("StdioToHTTP: Scanner error for session %s: %v", session.ID, err)
	}
}

// handleStdioMessage processes messages from stdio process
func (a *StdioToHTTPAdapter) handleStdioMessage(session *StdioSession, message *mcp.JSONRPCMessage) {
	// Check if this is a response to a pending request
	if message.ID != nil {
		requestID := a.getRequestID(message)
		a.requestMutex.RLock()
		if pendingReq, exists := a.pendingRequests[requestID]; exists {
			// Send response to waiting goroutine
			select {
			case pendingReq.ResponseCh <- message:
				log.Printf("StdioToHTTP: Delivered response for request %s", requestID)
			default:
				log.Printf("StdioToHTTP: Response channel full for request %s", requestID)
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

// sendToStdio sends a message to the stdio process
func (a *StdioToHTTPAdapter) sendToStdio(session *StdioSession, message *mcp.JSONRPCMessage) error {
	if session.Process == nil || session.Process.stdin == nil {
		return fmt.Errorf("stdio process not available")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	log.Printf("StdioToHTTP: Sending to stdio: %s", string(data))

	session.Process.mutex.Lock()
	defer session.Process.mutex.Unlock()

	_, err = fmt.Fprintln(session.Process.stdin, string(data))
	if err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}

	return nil
}

// broadcastToSSE broadcasts a message to all SSE connections
func (a *StdioToHTTPAdapter) broadcastToSSE(session *StdioSession, message *mcp.JSONRPCMessage) {
	session.SSEMutex.RLock()
	defer session.SSEMutex.RUnlock()

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("StdioToHTTP: Failed to marshal SSE message: %v", err)
		return
	}

	for connID, writer := range session.SSEConnections {
		if err := a.writeSSEEvent(writer, "message", string(data)); err != nil {
			log.Printf("StdioToHTTP: Failed to write SSE to connection %s: %v", connID, err)
			// Connection might be dead, cleanup will happen on next write
		}
	}
}

// writeSSEEvent writes an SSE event
func (a *StdioToHTTPAdapter) writeSSEEvent(w gin.ResponseWriter, event, data string) error {
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
func (a *StdioToHTTPAdapter) getOrCreateSession(sessionID string, adapter models.AdapterResource) *StdioSession {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if session, exists := a.sessions[sessionID]; exists {
		session.LastActivity = time.Now()
		return session
	}

	session := &StdioSession{
		ID:             sessionID,
		AdapterName:    adapter.Name,
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		SSEConnections: make(map[string]gin.ResponseWriter),
	}

	a.sessions[sessionID] = session

	// Register in session store
	a.sessionStore.SetWithDetails(sessionID, adapter.Name, "", "stdio-http")

	return session
}

// getSession retrieves a session by ID
func (a *StdioToHTTPAdapter) getSession(sessionID string) *StdioSession {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.sessions[sessionID]
}

// extractSessionID extracts session ID from request
func (a *StdioToHTTPAdapter) extractSessionID(c *gin.Context) string {
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
func (a *StdioToHTTPAdapter) generateSessionID() string {
	return fmt.Sprintf("stdio-http-%d", time.Now().UnixNano())
}

// getRequestID extracts request ID from message
func (a *StdioToHTTPAdapter) getRequestID(message *mcp.JSONRPCMessage) string {
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
func (a *StdioToHTTPAdapter) CanHandle(connectionType models.ConnectionType) bool {
	return connectionType == models.ConnectionTypeLocalStdio
}

// GetStatus returns adapter status
func (a *StdioToHTTPAdapter) GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error) {
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
func (a *StdioToHTTPAdapter) GetLogs(adapter models.AdapterResource) (string, error) {
	return fmt.Sprintf("Stdio-to-HTTP Adapter for %s\nActive Sessions: %d\n",
		adapter.Name, len(a.sessions)), nil
}

// Cleanup cleans up resources
func (a *StdioToHTTPAdapter) Cleanup(adapterID string) error {
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

		// Clean up stdio process
		if session.Process != nil {
			a.stdioProxy.Cleanup(adapterID)
		}

		delete(a.sessions, adapterID)
	}

	return nil
}
