package session

import (
	"fmt"
	"sync"
	"time"
)

// SessionInfo holds session data
type SessionInfo struct {
	TargetAddress string
	CreatedAt     time.Time
}

// TokenInfo holds OAuth token information
type TokenInfo struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
	TokenType    string    `json:"tokenType"`
	Scope        string    `json:"scope,omitempty"`
	IssuedAt     time.Time `json:"issuedAt"`
}

// AuthorizationInfo holds authorization state
type AuthorizationInfo struct {
	Status           string    `json:"status"` // "unauthorized", "authorizing", "authorized", "expired", "failed"
	AuthorizationURL string    `json:"authorizationUrl,omitempty"`
	ErrorMessage     string    `json:"errorMessage,omitempty"`
	AuthorizedAt     time.Time `json:"authorizedAt,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
	AccessToken      string    `json:"accessToken,omitempty"`
	RefreshToken     string    `json:"refreshToken,omitempty"`
	TokenType        string    `json:"tokenType,omitempty"`
	Scope            string    `json:"scope,omitempty"`
}

// MCPClientInfo holds MCP client information
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerInfo holds MCP server information
type MCPServerInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

// SessionDetails holds detailed session information
type SessionDetails struct {
	SessionID         string             `json:"sessionId"`
	AdapterName       string             `json:"adapterName"`
	TargetAddress     string             `json:"targetAddress"`
	ConnectionType    string             `json:"connectionType"`
	CreatedAt         time.Time          `json:"createdAt"`
	LastActivity      time.Time          `json:"lastActivity"`
	Status            string             `json:"status"`
	TokenInfo         *TokenInfo         `json:"tokenInfo,omitempty"`
	AuthorizationInfo *AuthorizationInfo `json:"authorizationInfo,omitempty"`

	// MCP-specific fields
	MCPSessionID    string                 `json:"mcpSessionId,omitempty"`
	MCPCapabilities map[string]interface{} `json:"mcpCapabilities,omitempty"`
	MCPClientInfo   *MCPClientInfo         `json:"mcpClientInfo,omitempty"`
	MCPServerInfo   *MCPServerInfo         `json:"mcpServerInfo,omitempty"`
}

// SessionStore interface defines session management operations
type SessionStore interface {
	Set(sessionID, targetAddress string) error
	Get(sessionID string) (string, bool)
	SetWithDetails(sessionID, adapterName, targetAddress, connectionType string) error
	ListByAdapter(adapterName string) ([]SessionDetails, error)
	GetDetails(sessionID string) (*SessionDetails, error)
	Delete(sessionID string) error
	DeleteByAdapter(adapterName string) error
	UpdateActivity(sessionID string) error
	CleanupExpired(maxAge time.Duration) error
	// Token management methods
	SetTokenInfo(sessionID string, tokenInfo *TokenInfo) error
	GetTokenInfo(sessionID string) (*TokenInfo, error)
	SetAuthorizationInfo(sessionID string, authInfo *AuthorizationInfo) error
	GetAuthorizationInfo(sessionID string) (*AuthorizationInfo, error)
	IsTokenValid(sessionID string) bool
	RefreshToken(sessionID, newAccessToken string, expiresAt time.Time) error
	// MCP-specific methods
	SetMCPSessionID(sessionID, mcpSessionID string) error
	GetMCPSessionID(sessionID string) (string, error)
	SetMCPCapabilities(sessionID string, capabilities map[string]interface{}) error
	GetMCPCapabilities(sessionID string) (map[string]interface{}, error)
	SetMCPClientInfo(sessionID string, clientInfo *MCPClientInfo) error
	GetMCPClientInfo(sessionID string) (*MCPClientInfo, error)
	SetMCPServerInfo(sessionID string, serverInfo *MCPServerInfo) error
	GetMCPServerInfo(sessionID string) (*MCPServerInfo, error)
	FindByMCPSessionID(mcpSessionID string) (*SessionDetails, error)
	GetActiveMCPSessions() ([]SessionDetails, error)
}

// InMemorySessionStore is a simple in-memory session store
type InMemorySessionStore struct {
	sessions map[string]SessionDetails
	mutex    sync.RWMutex
}

// NewInMemorySessionStore creates a new in-memory session store
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]SessionDetails),
	}
}

// Set sets a session (backward compatibility)
func (s *InMemorySessionStore) Set(sessionID, targetAddress string) error {
	return s.SetWithDetails(sessionID, "", targetAddress, "")
}

// Get gets a session (backward compatibility)
func (s *InMemorySessionStore) Get(sessionID string) (string, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	details, exists := s.sessions[sessionID]
	return details.TargetAddress, exists
}

// SetWithDetails sets a session with full details
func (s *InMemorySessionStore) SetWithDetails(sessionID, adapterName, targetAddress, connectionType string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	now := time.Now()
	s.sessions[sessionID] = SessionDetails{
		SessionID:      sessionID,
		AdapterName:    adapterName,
		TargetAddress:  targetAddress,
		ConnectionType: connectionType,
		CreatedAt:      now,
		LastActivity:   now,
		Status:         "active",
	}
	return nil
}

// ListByAdapter returns all sessions for a specific adapter
func (s *InMemorySessionStore) ListByAdapter(adapterName string) ([]SessionDetails, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var sessions []SessionDetails
	for _, session := range s.sessions {
		if session.AdapterName == adapterName {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

// GetDetails returns detailed information about a session
func (s *InMemorySessionStore) GetDetails(sessionID string) (*SessionDetails, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	details, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return &details, nil
}

// Delete removes a session
func (s *InMemorySessionStore) Delete(sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.sessions[sessionID]; !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(s.sessions, sessionID)
	return nil
}

// DeleteByAdapter removes all sessions for a specific adapter
func (s *InMemorySessionStore) DeleteByAdapter(adapterName string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	deleted := 0
	for sessionID, session := range s.sessions {
		if session.AdapterName == adapterName {
			delete(s.sessions, sessionID)
			deleted++
		}
	}
	return nil
}

// UpdateActivity updates the last activity timestamp for a session
func (s *InMemorySessionStore) UpdateActivity(sessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.LastActivity = time.Now()
	s.sessions[sessionID] = session
	return nil
}

// CleanupExpired removes sessions inactive longer than maxAge
func (s *InMemorySessionStore) CleanupExpired(maxAge time.Duration) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-maxAge)

	for sessionID, session := range s.sessions {
		if session.LastActivity.Before(cutoff) {
			delete(s.sessions, sessionID)
		}
	}
	return nil
}

// SetTokenInfo sets token information for a session
func (s *InMemorySessionStore) SetTokenInfo(sessionID string, tokenInfo *TokenInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.TokenInfo = tokenInfo
	s.sessions[sessionID] = session
	return nil
}

// GetTokenInfo retrieves token information for a session
func (s *InMemorySessionStore) GetTokenInfo(sessionID string) (*TokenInfo, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if session.TokenInfo == nil {
		return nil, fmt.Errorf("no token info for session: %s", sessionID)
	}

	return session.TokenInfo, nil
}

// SetAuthorizationInfo sets authorization information for a session
func (s *InMemorySessionStore) SetAuthorizationInfo(sessionID string, authInfo *AuthorizationInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.AuthorizationInfo = authInfo
	s.sessions[sessionID] = session
	return nil
}

// GetAuthorizationInfo retrieves authorization information for a session
func (s *InMemorySessionStore) GetAuthorizationInfo(sessionID string) (*AuthorizationInfo, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if session.AuthorizationInfo == nil {
		return nil, fmt.Errorf("no authorization info for session: %s", sessionID)
	}

	return session.AuthorizationInfo, nil
}

// IsTokenValid checks if the session has a valid (non-expired) token
func (s *InMemorySessionStore) IsTokenValid(sessionID string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return false
	}

	if session.TokenInfo == nil {
		return false
	}

	// Add some buffer time (5 minutes) before actual expiration
	buffer := 5 * time.Minute
	return session.TokenInfo.ExpiresAt.After(time.Now().Add(buffer))
}

// RefreshToken updates the access token and expiration for a session
func (s *InMemorySessionStore) RefreshToken(sessionID, newAccessToken string, expiresAt time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.TokenInfo == nil {
		session.TokenInfo = &TokenInfo{}
	}

	session.TokenInfo.AccessToken = newAccessToken
	session.TokenInfo.ExpiresAt = expiresAt
	session.TokenInfo.IssuedAt = time.Now()

	s.sessions[sessionID] = session
	return nil
}

// SetMCPSessionID sets the MCP session ID for a session
func (s *InMemorySessionStore) SetMCPSessionID(sessionID, mcpSessionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.MCPSessionID = mcpSessionID
	s.sessions[sessionID] = session
	return nil
}

// GetMCPSessionID retrieves the MCP session ID for a session
func (s *InMemorySessionStore) GetMCPSessionID(sessionID string) (string, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	return session.MCPSessionID, nil
}

// SetMCPCapabilities sets the MCP capabilities for a session
func (s *InMemorySessionStore) SetMCPCapabilities(sessionID string, capabilities map[string]interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.MCPCapabilities = capabilities
	s.sessions[sessionID] = session
	return nil
}

// GetMCPCapabilities retrieves the MCP capabilities for a session
func (s *InMemorySessionStore) GetMCPCapabilities(sessionID string) (map[string]interface{}, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session.MCPCapabilities, nil
}

// SetMCPClientInfo sets the MCP client information for a session
func (s *InMemorySessionStore) SetMCPClientInfo(sessionID string, clientInfo *MCPClientInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.MCPClientInfo = clientInfo
	s.sessions[sessionID] = session
	return nil
}

// GetMCPClientInfo retrieves the MCP client information for a session
func (s *InMemorySessionStore) GetMCPClientInfo(sessionID string) (*MCPClientInfo, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session.MCPClientInfo, nil
}

// SetMCPServerInfo sets the MCP server information for a session
func (s *InMemorySessionStore) SetMCPServerInfo(sessionID string, serverInfo *MCPServerInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.MCPServerInfo = serverInfo
	s.sessions[sessionID] = session
	return nil
}

// GetMCPServerInfo retrieves the MCP server information for a session
func (s *InMemorySessionStore) GetMCPServerInfo(sessionID string) (*MCPServerInfo, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session.MCPServerInfo, nil
}

// FindByMCPSessionID finds a session by its MCP session ID
func (s *InMemorySessionStore) FindByMCPSessionID(mcpSessionID string) (*SessionDetails, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, session := range s.sessions {
		if session.MCPSessionID == mcpSessionID {
			return &session, nil
		}
	}

	return nil, fmt.Errorf("no session found with MCP session ID: %s", mcpSessionID)
}

// GetActiveMCPSessions returns all active MCP sessions
func (s *InMemorySessionStore) GetActiveMCPSessions() ([]SessionDetails, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var mcpSessions []SessionDetails
	for _, session := range s.sessions {
		if session.MCPSessionID != "" && session.Status == "active" {
			mcpSessions = append(mcpSessions, session)
		}
	}

	return mcpSessions, nil
}
