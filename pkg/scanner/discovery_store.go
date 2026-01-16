package scanner

import (
	"crypto/md5"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"suse-ai-up/pkg/models"
)

// InMemoryDiscoveryStore implements DiscoveryStore with in-memory storage
type InMemoryDiscoveryStore struct {
	servers map[string]*models.DiscoveredServer
	mutex   sync.RWMutex
}

// NewInMemoryDiscoveryStore creates a new in-memory discovery store
func NewInMemoryDiscoveryStore() *InMemoryDiscoveryStore {
	return &InMemoryDiscoveryStore{
		servers: make(map[string]*models.DiscoveredServer),
	}
}

// generateFingerprint creates a unique fingerprint for a discovered server
func generateFingerprint(server *models.DiscoveredServer) string {
	var parts []string

	// Use server name if available
	if server.Name != "" && server.Name != "Unknown MCP Server" {
		parts = append(parts, server.Name)
	}

	// Add server version and protocol version if available
	if server.ServerVersion != "" {
		parts = append(parts, server.ServerVersion)
	}
	if server.ProtocolVersion != "" {
		parts = append(parts, server.ProtocolVersion)
	}

	// Add capabilities hash if available
	if server.Capabilities != nil {
		capStr := fmt.Sprintf("%v", server.Capabilities)
		hash := fmt.Sprintf("%x", md5.Sum([]byte(capStr)))[:8] // First 8 chars of MD5
		parts = append(parts, hash)
	}

	// If no identity information, fall back to address-based fingerprint
	if len(parts) == 0 {
		return fmt.Sprintf("address-%s", strings.ReplaceAll(server.Address, "/", "-"))
	}

	return strings.Join(parts, "|")
}

// Save stores a discovered server with deduplication
func (s *InMemoryDiscoveryStore) Save(server *models.DiscoveredServer) error {
	if server == nil {
		log.Printf("ERROR: Attempted to save nil server")
		return fmt.Errorf("server cannot be nil")
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Update LastSeen
	server.LastSeen = time.Now()

	log.Printf("DEBUG: Saving server ID=%s, Name=%s, Address=%s", server.ID, server.Name, server.Address)

	// Check for existing server with same fingerprint (deduplication)
	fingerprint := generateFingerprint(server)
	log.Printf("DEBUG: Generated fingerprint: %s", fingerprint)

	if existingServerByFingerprint, err := s.FindByFingerprint(fingerprint); err == nil {
		log.Printf("DEBUG: Found existing server with same fingerprint, updating: %s", existingServerByFingerprint.ID)
		// Server with same identity exists - update it instead of creating duplicate
		server.ID = existingServerByFingerprint.ID // Keep the original ID

		// Safely merge/update information from the new discovery
		if server.Capabilities != nil {
			existingServerByFingerprint.Capabilities = server.Capabilities
		}
		if server.Tools != nil {
			existingServerByFingerprint.Tools = server.Tools
		}
		if server.Resources != nil {
			existingServerByFingerprint.Resources = server.Resources
		}
		if server.Prompts != nil {
			existingServerByFingerprint.Prompts = server.Prompts
		}
		if server.AuthInfo != nil {
			existingServerByFingerprint.AuthInfo = server.AuthInfo
		}
		if server.ServerVersion != "" {
			existingServerByFingerprint.ServerVersion = server.ServerVersion
		}
		if server.ProtocolVersion != "" {
			existingServerByFingerprint.ProtocolVersion = server.ProtocolVersion
		}
		// Update metadata and vulnerability score
		if server.Metadata != nil {
			existingServerByFingerprint.Metadata = server.Metadata
		}
		if server.VulnerabilityScore != "" {
			existingServerByFingerprint.VulnerabilityScore = server.VulnerabilityScore
		}
		existingServerByFingerprint.LastSeen = server.LastSeen

		// Store the updated existing server
		s.servers[existingServerByFingerprint.ID] = existingServerByFingerprint
		log.Printf("DEBUG: Successfully updated existing server: %s", existingServerByFingerprint.ID)
		return nil
	} else {
		log.Printf("DEBUG: No existing server with fingerprint %s found", fingerprint)
	}

	// No existing server found - check if the current ID is already in use
	if _, exists := s.servers[server.ID]; exists {
		log.Printf("DEBUG: ID collision detected for %s, generating new ID", server.ID)
		// ID collision - this shouldn't happen with proper ID generation, but handle it
		// Generate a new unique ID
		counter := 1
		newID := server.ID
		for {
			newID = fmt.Sprintf("%s-%d", server.ID, counter)
			if _, exists := s.servers[newID]; !exists {
				break
			}
			counter++
		}
		server.ID = newID
		log.Printf("DEBUG: Generated new ID: %s", newID)
	}

	// Store the new server
	s.servers[server.ID] = server
	log.Printf("DEBUG: Successfully saved new server: %s", server.ID)
	return nil
}

// GetAll returns all discovered servers
func (s *InMemoryDiscoveryStore) GetAll() ([]models.DiscoveredServer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	servers := make([]models.DiscoveredServer, 0, len(s.servers))
	for _, server := range s.servers {
		servers = append(servers, *server)
	}

	return servers, nil
}

// GetByID returns a specific discovered server by ID
func (s *InMemoryDiscoveryStore) GetByID(id string) (*models.DiscoveredServer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	server, exists := s.servers[id]
	if !exists {
		return nil, fmt.Errorf("server not found: %s", id)
	}

	// Return a copy to prevent external modification
	serverCopy := *server
	return &serverCopy, nil
}

// FindByFingerprint finds a server by its fingerprint (used for deduplication)
func (s *InMemoryDiscoveryStore) FindByFingerprint(fingerprint string) (*models.DiscoveredServer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, server := range s.servers {
		serverFingerprint := generateFingerprint(server)
		if serverFingerprint == fingerprint {
			// Return a copy to prevent external modification
			serverCopy := *server
			return &serverCopy, nil
		}
	}

	return nil, fmt.Errorf("server with fingerprint not found: %s", fingerprint)
}

// UpdateLastSeen updates the last seen time for a server
func (s *InMemoryDiscoveryStore) UpdateLastSeen(id string, lastSeen time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	server, exists := s.servers[id]
	if !exists {
		return fmt.Errorf("server not found: %s", id)
	}

	server.LastSeen = lastSeen
	return nil
}

// RemoveStale removes servers that haven't been seen for longer than the threshold
func (s *InMemoryDiscoveryStore) RemoveStale(threshold time.Duration) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	cutoff := time.Now().Add(-threshold)
	for id, server := range s.servers {
		if server.LastSeen.Before(cutoff) {
			delete(s.servers, id)
		}
	}

	return nil
}

// Delete removes a server from the store
func (s *InMemoryDiscoveryStore) Delete(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.servers[id]; !exists {
		return fmt.Errorf("server not found: %s", id)
	}

	delete(s.servers, id)
	return nil
}

// GetServerCount returns the total number of stored servers
func (s *InMemoryDiscoveryStore) GetServerCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return len(s.servers)
}

// Clear removes all servers from the store
func (s *InMemoryDiscoveryStore) Clear() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.servers = make(map[string]*models.DiscoveredServer)
	return nil
}
