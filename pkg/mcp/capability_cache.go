package mcp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"suse-ai-up/pkg/models"
)

// CapabilityCache caches MCP server capabilities
type CapabilityCache struct {
	cache map[string]*CachedCapabilities
	mutex sync.RWMutex
}

// CachedCapabilities represents cached capabilities with expiration
type CachedCapabilities struct {
	Capabilities map[string]interface{} `json:"capabilities"`
	ExpiresAt    time.Time              `json:"expiresAt"`
	ServerInfo   models.MCPServerInfo   `json:"serverInfo"`
}

// NewCapabilityCache creates a new capability cache
func NewCapabilityCache() *CapabilityCache {
	cache := &CapabilityCache{
		cache: make(map[string]*CachedCapabilities),
	}

	// Start cleanup goroutine
	go cache.startCleanup()

	return cache
}

// GetCapabilities retrieves capabilities for an adapter
func (cc *CapabilityCache) GetCapabilities(ctx context.Context, adapter models.AdapterResource) (map[string]interface{}, error) {
	cacheKey := cc.getCacheKey(adapter)

	cc.mutex.RLock()
	cached, exists := cc.cache[cacheKey]
	cc.mutex.RUnlock()

	if exists && time.Now().Before(cached.ExpiresAt) {
		log.Printf("CapabilityCache: Using cached capabilities for %s", adapter.Name)
		return cached.Capabilities, nil
	}

	// Cache miss or expired - discover capabilities
	log.Printf("CapabilityCache: Discovering capabilities for %s", adapter.Name)
	capabilities, err := cc.discoverCapabilities(ctx, adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to discover capabilities: %w", err)
	}

	// Cache the result
	cc.mutex.Lock()
	cc.cache[cacheKey] = &CachedCapabilities{
		Capabilities: capabilities,
		ExpiresAt:    time.Now().Add(5 * time.Minute), // Cache for 5 minutes
		ServerInfo: models.MCPServerInfo{
			Name:     adapter.Name,
			Version:  "1.0.0",
			Protocol: "MCP",
		},
	}
	cc.mutex.Unlock()

	return capabilities, nil
}

// InvalidateCache invalidates cached capabilities for an adapter
func (cc *CapabilityCache) InvalidateCache(adapter models.AdapterResource) {
	cacheKey := cc.getCacheKey(adapter)

	cc.mutex.Lock()
	delete(cc.cache, cacheKey)
	cc.mutex.Unlock()

	log.Printf("CapabilityCache: Invalidated cache for %s", adapter.Name)
}

// getCacheKey generates a cache key for an adapter
func (cc *CapabilityCache) getCacheKey(adapter models.AdapterResource) string {
	return fmt.Sprintf("%s-%s-%s", adapter.Name, adapter.ConnectionType, adapter.RemoteUrl)
}

// discoverCapabilities discovers capabilities from the target MCP server
func (cc *CapabilityCache) discoverCapabilities(ctx context.Context, adapter models.AdapterResource) (map[string]interface{}, error) {
	// For now, return default capabilities
	// In a full implementation, this would connect to the target server
	// and discover its actual capabilities

	capabilities := map[string]interface{}{
		"tools": map[string]interface{}{
			"listChanged": true,
		},
		"resources": map[string]interface{}{
			"subscribe":   true,
			"listChanged": true,
		},
		"prompts": map[string]interface{}{
			"listChanged": true,
		},
		"completion": map[string]interface{}{},
	}

	// If adapter has discovered functionality, use that
	if adapter.MCPFunctionality != nil {
		capabilities = cc.buildCapabilitiesFromFunctionality(adapter.MCPFunctionality)
	}

	return capabilities, nil
}

// buildCapabilitiesFromFunctionality builds capabilities from discovered functionality
func (cc *CapabilityCache) buildCapabilitiesFromFunctionality(functionality *models.MCPFunctionality) map[string]interface{} {
	capabilities := map[string]interface{}{}

	// Tools capability
	if functionality.Tools != nil && len(functionality.Tools) > 0 {
		capabilities["tools"] = map[string]interface{}{
			"listChanged": true,
		}
	}

	// Resources capability
	if functionality.Resources != nil && len(functionality.Resources) > 0 {
		capabilities["resources"] = map[string]interface{}{
			"subscribe":   true,
			"listChanged": true,
		}
	}

	// Prompts capability
	if functionality.Prompts != nil && len(functionality.Prompts) > 0 {
		capabilities["prompts"] = map[string]interface{}{
			"listChanged": true,
		}
	}

	// Completion capability (always available if we have tools, prompts, or resources)
	if len(capabilities) > 0 {
		capabilities["completion"] = map[string]interface{}{}
	}

	return capabilities
}

// startCleanup starts a background goroutine to clean up expired cache entries
func (cc *CapabilityCache) startCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cc.cleanup()
	}
}

// cleanup removes expired cache entries
func (cc *CapabilityCache) cleanup() {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	now := time.Now()
	for key, cached := range cc.cache {
		if now.After(cached.ExpiresAt) {
			delete(cc.cache, key)
		}
	}
}

// GetCachedCapabilities returns the cached capabilities without discovering new ones
func (cc *CapabilityCache) GetCachedCapabilities(adapter models.AdapterResource) (*CachedCapabilities, bool) {
	cacheKey := cc.getCacheKey(adapter)

	cc.mutex.RLock()
	cached, exists := cc.cache[cacheKey]
	cc.mutex.RUnlock()

	if exists && time.Now().Before(cached.ExpiresAt) {
		return cached, true
	}

	return nil, false
}

// SetCapabilities manually sets capabilities for an adapter
func (cc *CapabilityCache) SetCapabilities(adapter models.AdapterResource, capabilities map[string]interface{}, serverInfo models.MCPServerInfo) {
	cacheKey := cc.getCacheKey(adapter)

	cc.mutex.Lock()
	cc.cache[cacheKey] = &CachedCapabilities{
		Capabilities: capabilities,
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		ServerInfo:   serverInfo,
	}
	cc.mutex.Unlock()

	log.Printf("CapabilityCache: Set capabilities for %s", adapter.Name)
}
