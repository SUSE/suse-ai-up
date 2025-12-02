package mcp

import (
	"fmt"
	"sync"
	"time"
)

// MCPSessionManager manages MCP sessions with lifecycle tracking
type MCPSessionManager struct {
	sessions map[string]*MCPSession
	mu       sync.RWMutex
}

// NewMCPSessionManager creates a new MCP session manager
func NewMCPSessionManager() *MCPSessionManager {
	return &MCPSessionManager{
		sessions: make(map[string]*MCPSession),
	}
}

// CreateSession creates a new MCP session
func (sm *MCPSessionManager) CreateSession(adapterName string, clientInfo MCPClientInfo) *MCPSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionID := generateSessionID()
	session := &MCPSession{
		ID:           sessionID,
		AdapterName:  adapterName,
		Initialized:  false,
		ClientInfo:   clientInfo,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	sm.sessions[sessionID] = session
	return session
}

// GetSession retrieves a session by ID
func (sm *MCPSessionManager) GetSession(sessionID string) *MCPSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.sessions[sessionID]
}

// UpdateSessionCapabilities updates session capabilities after initialization
func (sm *MCPSessionManager) UpdateSessionCapabilities(sessionID string, capabilities MCPCapabilities, serverInfo *MCPClientInfo) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return CreateMCPError(MCP_RESOURCE_NOT_FOUND, "Session not found", nil)
	}

	session.Capabilities = capabilities
	session.ServerInfo = serverInfo
	session.Initialized = true
	session.LastActivity = time.Now()

	return nil
}

// UpdateSessionActivity updates the last activity timestamp
func (sm *MCPSessionManager) UpdateSessionActivity(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.LastActivity = time.Now()
	}
}

// CloseSession removes a session
func (sm *MCPSessionManager) CloseSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.sessions, sessionID)
}

// ListSessions returns all active sessions
func (sm *MCPSessionManager) ListSessions() []*MCPSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*MCPSession, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// CleanupExpiredSessions removes sessions that haven't been active for the specified duration
func (sm *MCPSessionManager) CleanupExpiredSessions(maxAge time.Duration) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, session := range sm.sessions {
		if session.LastActivity.Before(cutoff) {
			delete(sm.sessions, id)
			removed++
		}
	}

	return removed
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	// Simple implementation - in production, use crypto/rand
	return "session-" + time.Now().Format("20060102150405") + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000)
}
