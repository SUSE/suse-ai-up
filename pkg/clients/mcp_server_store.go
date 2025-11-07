package clients

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"suse-ai-up/pkg/models"
)

var (
	// ErrNotFound is returned when a resource is not found
	ErrNotFound = errors.New("resource not found")
)

// generateID generates a random hex ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// InMemoryMCPServerStore implements MCPServerStore interface using in-memory storage
type InMemoryMCPServerStore struct {
	servers map[string]*models.MCPServer
	mu      sync.RWMutex
}

// NewInMemoryMCPServerStore creates a new in-memory MCP server store
func NewInMemoryMCPServerStore() *InMemoryMCPServerStore {
	return &InMemoryMCPServerStore{
		servers: make(map[string]*models.MCPServer),
	}
}

// CreateMCPServer creates a new MCP server
func (s *InMemoryMCPServerStore) CreateMCPServer(server *models.MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if server.ID == "" {
		server.ID = generateID()
	}

	s.servers[server.ID] = server
	return nil
}

// GetMCPServer retrieves an MCP server by ID
func (s *InMemoryMCPServerStore) GetMCPServer(id string) (*models.MCPServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	server, exists := s.servers[id]
	if !exists {
		return nil, ErrNotFound
	}

	return server, nil
}

// UpdateMCPServer updates an existing MCP server
func (s *InMemoryMCPServerStore) UpdateMCPServer(id string, updated *models.MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[id]; !exists {
		return ErrNotFound
	}

	updated.ID = id
	s.servers[id] = updated
	return nil
}

// DeleteMCPServer deletes an MCP server by ID
func (s *InMemoryMCPServerStore) DeleteMCPServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[id]; !exists {
		return ErrNotFound
	}

	delete(s.servers, id)
	return nil
}

// ListMCPServers returns all MCP servers
func (s *InMemoryMCPServerStore) ListMCPServers() []*models.MCPServer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	servers := make([]*models.MCPServer, 0, len(s.servers))
	for _, server := range s.servers {
		servers = append(servers, server)
	}

	return servers
}
