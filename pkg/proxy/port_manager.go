package proxy

import (
	"fmt"
	"sync"
)

// PortManager manages port allocation for sidecar containers
type PortManager struct {
	minPort   int
	maxPort   int
	allocated map[string]int // adapterID -> port
	mutex     sync.RWMutex
}

// NewPortManager creates a new port manager
func NewPortManager(minPort, maxPort int) *PortManager {
	return &PortManager{
		minPort:   minPort,
		maxPort:   maxPort,
		allocated: make(map[string]int),
	}
}

// AllocatePort allocates a random port for the given adapter ID
func (pm *PortManager) AllocatePort(adapterID string) (int, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if already allocated
	if port, exists := pm.allocated[adapterID]; exists {
		return port, nil
	}

	// Find an available port
	for port := pm.minPort; port <= pm.maxPort; port++ {
		if !pm.isPortUsed(port) {
			pm.allocated[adapterID] = port
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", pm.minPort, pm.maxPort)
}

// ReleasePort releases the port allocated for the given adapter ID
func (pm *PortManager) ReleasePort(adapterID string) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	delete(pm.allocated, adapterID)
}

// isPortUsed checks if a port is already allocated
func (pm *PortManager) isPortUsed(port int) bool {
	for _, allocatedPort := range pm.allocated {
		if allocatedPort == port {
			return true
		}
	}
	return false
}

// GetAllocatedPort returns the allocated port for an adapter ID
func (pm *PortManager) GetAllocatedPort(adapterID string) (int, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	port, exists := pm.allocated[adapterID]
	return port, exists
}
