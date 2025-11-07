package scanner

import (
	"fmt"
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

// Save stores a discovered server
func (s *InMemoryDiscoveryStore) Save(server *models.DiscoveredServer) error {
	if server == nil {
		return fmt.Errorf("server cannot be nil")
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Update LastSeen
	server.LastSeen = time.Now()

	// Store or update the server
	s.servers[server.ID] = server

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
