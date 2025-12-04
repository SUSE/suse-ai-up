package clients

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"suse-ai-up/pkg/models"
)

// MCPServerStore interface for storing and retrieving MCP servers
type MCPServerStore interface {
	CreateMCPServer(server *models.MCPServer) error
	GetMCPServer(id string) (*models.MCPServer, error)
	UpdateMCPServer(id string, updated *models.MCPServer) error
	DeleteMCPServer(id string) error
	ListMCPServers() []*models.MCPServer
}

// ScanStore interface for storing and retrieving scan results
type ScanStore interface {
	SaveScan(scanID string, config models.ScanConfig, results []models.DiscoveredServer) error
	GetScan(scanID string) (models.ScanConfig, []models.DiscoveredServer, error)
	ListScans() []string
	DeleteScan(scanID string) error
}

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

// InMemoryScanStore implements ScanStore with in-memory storage
type InMemoryScanStore struct {
	scans map[string]ScanResult
	mu    sync.RWMutex
}

// ScanResult holds scan configuration and results
type ScanResult struct {
	Config  models.ScanConfig
	Results []models.DiscoveredServer
}

// NewInMemoryScanStore creates a new in-memory scan store
func NewInMemoryScanStore() *InMemoryScanStore {
	return &InMemoryScanStore{
		scans: make(map[string]ScanResult),
	}
}

// SaveScan stores scan results
func (s *InMemoryScanStore) SaveScan(scanID string, config models.ScanConfig, results []models.DiscoveredServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.scans[scanID] = ScanResult{
		Config:  config,
		Results: results,
	}

	return nil
}

// GetScan retrieves scan results
func (s *InMemoryScanStore) GetScan(scanID string) (models.ScanConfig, []models.DiscoveredServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result, exists := s.scans[scanID]
	if !exists {
		return models.ScanConfig{}, nil, ErrNotFound
	}

	return result.Config, result.Results, nil
}

// ListScans returns all scan IDs
func (s *InMemoryScanStore) ListScans() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	scanIDs := make([]string, 0, len(s.scans))
	for id := range s.scans {
		scanIDs = append(scanIDs, id)
	}

	return scanIDs
}

// DeleteScan removes a scan
func (s *InMemoryScanStore) DeleteScan(scanID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.scans[scanID]; !exists {
		return ErrNotFound
	}

	delete(s.scans, scanID)
	return nil
}
