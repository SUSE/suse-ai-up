package service

import (
	"fmt"
	"sync"

	"suse-ai-up/pkg/models"
)

// InMemoryMCPServerStore is a simple in-memory store for MCP servers
type InMemoryMCPServerStore struct {
	mu      sync.RWMutex
	servers map[string]*models.MCPServer
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

	if _, exists := s.servers[server.ID]; exists {
		return fmt.Errorf("MCP server with ID %s already exists", server.ID)
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
		return nil, fmt.Errorf("MCP server with ID %s not found", id)
	}

	return server, nil
}

// UpdateMCPServer updates an existing MCP server
func (s *InMemoryMCPServerStore) UpdateMCPServer(id string, updated *models.MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[id]; !exists {
		return fmt.Errorf("MCP server with ID %s not found", id)
	}

	updated.ID = id // Ensure ID doesn't change
	s.servers[id] = updated
	return nil
}

// DeleteMCPServer deletes an MCP server by ID
func (s *InMemoryMCPServerStore) DeleteMCPServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[id]; !exists {
		return fmt.Errorf("MCP server with ID %s not found", id)
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
