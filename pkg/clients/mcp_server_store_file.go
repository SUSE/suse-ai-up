package clients

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"suse-ai-up/pkg/models"
)

// FileMCPServerStore implements MCPServerStore interface using file-based storage
type FileMCPServerStore struct {
	filePath string
	servers  map[string]*models.MCPServer
	mu       sync.RWMutex
	crypto   *StorageCrypto
}

// NewFileMCPServerStore creates a new file-based MCP server store
func NewFileMCPServerStore(filePath string, crypto *StorageCrypto) *FileMCPServerStore {
	store := &FileMCPServerStore{
		filePath: filePath,
		servers:  make(map[string]*models.MCPServer),
		crypto:   crypto,
	}

	// Load existing servers from file
	if err := store.loadFromFile(); err != nil {
		fmt.Printf("Warning: Failed to load MCP servers from file: %v\n", err)
	}

	return store
}

// CreateMCPServer creates a new MCP server
func (s *FileMCPServerStore) CreateMCPServer(server *models.MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if server.ID == "" {
		server.ID = generateID()
	}

	s.servers[server.ID] = server
	return s.saveToFile()
}

// GetMCPServer retrieves an MCP server by ID
func (s *FileMCPServerStore) GetMCPServer(id string) (*models.MCPServer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	server, exists := s.servers[id]
	if !exists {
		return nil, ErrNotFound
	}

	return server, nil
}

// UpdateMCPServer updates an existing MCP server
func (s *FileMCPServerStore) UpdateMCPServer(id string, updated *models.MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[id]; !exists {
		return ErrNotFound
	}

	updated.ID = id
	s.servers[id] = updated
	return s.saveToFile()
}

// DeleteMCPServer deletes an MCP server by ID
func (s *FileMCPServerStore) DeleteMCPServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.servers[id]; !exists {
		return ErrNotFound
	}

	delete(s.servers, id)
	return s.saveToFile()
}

// ListMCPServers returns all MCP servers
func (s *FileMCPServerStore) ListMCPServers() []*models.MCPServer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	servers := make([]*models.MCPServer, 0, len(s.servers))
	for _, server := range s.servers {
		servers = append(servers, server)
	}

	return servers
}

// loadFromFile loads servers from the JSON file
func (s *FileMCPServerStore) loadFromFile() error {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil // File doesn't exist, start with empty store
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read MCP server file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file
	}

	// Decrypt if crypto is enabled
	if s.crypto != nil {
		decrypted, err := s.crypto.Decrypt(data)
		if err != nil {
			return fmt.Errorf("failed to decrypt MCP server file: %w", err)
		}
		data = decrypted
	}

	var servers []*models.MCPServer
	if err := json.Unmarshal(data, &servers); err != nil {
		return fmt.Errorf("failed to parse MCP server file: %w", err)
	}

	// Convert to map
	for _, server := range servers {
		s.servers[server.ID] = server
	}

	return nil
}

// saveToFile saves servers to the JSON file
func (s *FileMCPServerStore) saveToFile() error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Convert map to slice
	servers := make([]*models.MCPServer, 0, len(s.servers))
	for _, server := range s.servers {
		servers = append(servers, server)
	}

	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal servers: %w", err)
	}

	// Encrypt if crypto is enabled
	if s.crypto != nil {
		encrypted, err := s.crypto.Encrypt(data)
		if err != nil {
			return fmt.Errorf("failed to encrypt MCP server file: %w", err)
		}
		data = encrypted
	}

	// Write to temporary file first
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempFile, s.filePath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}
