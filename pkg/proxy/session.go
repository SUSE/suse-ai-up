package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SessionManager manages MCP proxy sessions
type SessionManager struct {
	sessions map[string]*ProxySession
	mutex    sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ProxySession),
	}
}

// CreateSession creates a new isolated session for a request
func (sm *SessionManager) CreateSession(client *ProxyClient) *ProxySession {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sessionID := fmt.Sprintf("session-%d", generateRequestID())
	session := &ProxySession{
		id:       sessionID,
		client:   client,
		created:  time.Now(),
		lastUsed: time.Now(),
	}

	sm.sessions[sessionID] = session
	return session
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*ProxySession, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[sessionID]
	return session, exists
}

// UpdateSessionActivity updates the last used time for a session
func (sm *SessionManager) UpdateSessionActivity(sessionID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.lastUsed = time.Now()
	}
}

// RemoveSession removes a session
func (sm *SessionManager) RemoveSession(sessionID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	delete(sm.sessions, sessionID)
}

// CleanupExpiredSessions removes sessions that haven't been used for a certain duration
func (sm *SessionManager) CleanupExpiredSessions(maxAge time.Duration) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	for id, session := range sm.sessions {
		if now.Sub(session.lastUsed) > maxAge {
			delete(sm.sessions, id)
		}
	}
}

// GetActiveSessionCount returns the number of active sessions
func (sm *SessionManager) GetActiveSessionCount() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return len(sm.sessions)
}

// GetID returns the session ID
func (ps *ProxySession) GetID() string {
	return ps.id
}

// GetClient returns the proxy client for this session
func (ps *ProxySession) GetClient() *ProxyClient {
	return ps.client
}

// UpdateActivity updates the last used time
func (ps *ProxySession) UpdateActivity() {
	ps.lastUsed = time.Now()
}

// GetAge returns how long the session has existed
func (ps *ProxySession) GetAge() time.Duration {
	return time.Since(ps.created)
}

// GetIdleTime returns how long since the session was last used
func (ps *ProxySession) GetIdleTime() time.Duration {
	return time.Since(ps.lastUsed)
}

// IsExpired checks if the session has expired based on max age
func (ps *ProxySession) IsExpired(maxAge time.Duration) bool {
	return ps.GetIdleTime() > maxAge
}

// Close cleans up the session
func (ps *ProxySession) Close() {
	// Any cleanup logic for the session
}

// SessionAwareProxyServer extends MCPProxyServer with session management
type SessionAwareProxyServer struct {
	*MCPProxyServer
	sessionManager *SessionManager
	maxSessionAge  time.Duration
}

// NewSessionAwareProxyServer creates a new session-aware proxy server
func NewSessionAwareProxyServer(clientFactory func() *ProxyClient, name string, maxSessionAge time.Duration) *SessionAwareProxyServer {
	baseServer := AsProxy(clientFactory, name)
	sessionManager := NewSessionManager()

	return &SessionAwareProxyServer{
		MCPProxyServer: baseServer,
		sessionManager: sessionManager,
		maxSessionAge:  maxSessionAge,
	}
}

// HandleMCPRequestWithSession handles MCP requests with session isolation
func (s *SessionAwareProxyServer) HandleMCPRequestWithSession(ctx context.Context, message *JSONRPCMessage) (interface{}, error) {
	// Create a fresh session for this request
	client := s.clientFactory()
	session := s.sessionManager.CreateSession(client)

	// Update session activity
	defer func() {
		session.UpdateActivity()
		s.sessionManager.UpdateSessionActivity(session.GetID())
	}()

	// Process the request with the isolated session
	return s.processMCPMessageWithSession(message, session)
}

// processMCPMessageWithSession processes MCP messages using an isolated session
func (s *SessionAwareProxyServer) processMCPMessageWithSession(message *JSONRPCMessage, session *ProxySession) (interface{}, error) {
	client := session.GetClient()
	if client == nil {
		return nil, fmt.Errorf("no client available for session")
	}

	// Route based on method with session isolation
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

// CleanupExpiredSessions removes old sessions
func (s *SessionAwareProxyServer) CleanupExpiredSessions() {
	s.sessionManager.CleanupExpiredSessions(s.maxSessionAge)
}

// GetSessionStats returns session statistics
func (s *SessionAwareProxyServer) GetSessionStats() map[string]interface{} {
	return map[string]interface{}{
		"active_sessions": s.sessionManager.GetActiveSessionCount(),
		"max_session_age": s.maxSessionAge.String(),
	}
}

// StartSessionCleanup starts a goroutine to periodically clean up expired sessions
func (s *SessionAwareProxyServer) StartSessionCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			s.CleanupExpiredSessions()
		}
	}()
}
