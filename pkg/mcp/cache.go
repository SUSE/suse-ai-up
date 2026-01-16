package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CacheEntry represents a cached response with metadata
type CacheEntry struct {
	Data      interface{}   `json:"data"`
	Timestamp time.Time     `json:"timestamp"`
	TTL       time.Duration `json:"ttl"`
	HitCount  int64         `json:"hit_count"`
	ETag      string        `json:"etag,omitempty"`
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Since(e.Timestamp) > e.TTL
}

// IsValid checks if the cache entry is still valid and not expired
func (e *CacheEntry) IsValid() bool {
	return !e.IsExpired() && e.Data != nil
}

// CacheConfig holds configuration for the MCP cache
type CacheConfig struct {
	// Default TTL for different types of cached data
	CapabilitiesTTL  time.Duration `json:"capabilities_ttl"`
	ToolsListTTL     time.Duration `json:"tools_list_ttl"`
	PromptsListTTL   time.Duration `json:"prompts_list_ttl"`
	ResourcesListTTL time.Duration `json:"resources_list_ttl"`
	ResponseTTL      time.Duration `json:"response_ttl"`

	// Cache size limits
	MaxEntries     int   `json:"max_entries"`
	MaxMemoryUsage int64 `json:"max_memory_usage"` // in bytes

	// Cache behavior
	EnableCompression bool          `json:"enable_compression"`
	EnableMetrics     bool          `json:"enable_metrics"`
	CleanupInterval   time.Duration `json:"cleanup_interval"`
}

// DefaultCacheConfig returns a default configuration for MCP caching
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		CapabilitiesTTL:   5 * time.Minute,
		ToolsListTTL:      2 * time.Minute,
		PromptsListTTL:    2 * time.Minute,
		ResourcesListTTL:  2 * time.Minute,
		ResponseTTL:       30 * time.Second,
		MaxEntries:        1000,
		MaxMemoryUsage:    100 * 1024 * 1024, // 100MB
		EnableCompression: false,
		EnableMetrics:     true,
		CleanupInterval:   1 * time.Minute,
	}
}

// CacheMetrics tracks cache performance metrics
type CacheMetrics struct {
	mu             sync.RWMutex
	Hits           int64         `json:"hits"`
	Misses         int64         `json:"misses"`
	Evictions      int64         `json:"evictions"`
	Entries        int64         `json:"entries"`
	MemoryUsage    int64         `json:"memory_usage"`
	AverageHitTime time.Duration `json:"average_hit_time"`
	TotalHitTime   time.Duration `json:"total_hit_time"`
}

// CacheStats returns current cache statistics
func (m *CacheMetrics) CacheStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hitRate := float64(0)
	if m.Hits+m.Misses > 0 {
		hitRate = float64(m.Hits) / float64(m.Hits+m.Misses) * 100
	}

	avgHitTime := time.Duration(0)
	if m.Hits > 0 {
		avgHitTime = m.TotalHitTime / time.Duration(m.Hits)
	}

	return map[string]interface{}{
		"hits":             m.Hits,
		"misses":           m.Misses,
		"hit_rate":         fmt.Sprintf("%.2f%%", hitRate),
		"evictions":        m.Evictions,
		"entries":          m.Entries,
		"memory_usage":     fmt.Sprintf("%.2f MB", float64(m.MemoryUsage)/1024/1024),
		"average_hit_time": avgHitTime.String(),
	}
}

// IncrementHit increments hit counter and updates timing
func (m *CacheMetrics) IncrementHit(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Hits++
	m.TotalHitTime += duration
}

// IncrementMiss increments miss counter
func (m *CacheMetrics) IncrementMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Misses++
}

// IncrementEvictions increments eviction counter
func (m *CacheMetrics) IncrementEvictions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Evictions++
}

// UpdateEntries updates entry count and memory usage
func (m *CacheMetrics) UpdateEntries(count int, memory int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Entries = int64(count)
	m.MemoryUsage = memory
}

// MCPCache provides intelligent caching for MCP operations
type MCPCache struct {
	mu      sync.RWMutex
	config  *CacheConfig
	cache   map[string]*CacheEntry
	metrics *CacheMetrics
	stopCh  chan struct{}
}

// NewMCPCache creates a new MCP cache instance
func NewMCPCache(config *CacheConfig) *MCPCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &MCPCache{
		config:  config,
		cache:   make(map[string]*CacheEntry),
		metrics: &CacheMetrics{},
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupRoutine()

	return cache
}

// generateCacheKey creates a unique cache key for the given parameters
func (c *MCPCache) generateCacheKey(adapterName, method string, params interface{}) string {
	h := sha256.New()

	// Include adapter name and method
	h.Write([]byte(adapterName))
	h.Write([]byte(":"))
	h.Write([]byte(method))

	// Include parameters if provided
	if params != nil {
		paramBytes, _ := json.Marshal(params)
		h.Write([]byte(":"))
		h.Write(paramBytes)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// getTTLForMethod returns appropriate TTL for different MCP methods
func (c *MCPCache) getTTLForMethod(method string) time.Duration {
	switch method {
	case "initialize", "capabilities":
		return c.config.CapabilitiesTTL
	case "tools/list":
		return c.config.ToolsListTTL
	case "prompts/list":
		return c.config.PromptsListTTL
	case "resources/list":
		return c.config.ResourcesListTTL
	default:
		return c.config.ResponseTTL
	}
}

// Get retrieves a cached entry
func (c *MCPCache) Get(adapterName, method string, params interface{}) (interface{}, bool) {
	start := time.Now()
	defer func() {
		if c.config.EnableMetrics {
			c.metrics.IncrementHit(time.Since(start))
		}
	}()

	key := c.generateCacheKey(adapterName, method, params)

	c.mu.RLock()
	entry, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists || !entry.IsValid() {
		if c.config.EnableMetrics {
			c.metrics.IncrementMiss()
		}
		return nil, false
	}

	// Update hit count
	c.mu.Lock()
	entry.HitCount++
	c.mu.Unlock()

	return entry.Data, true
}

// Set stores data in the cache
func (c *MCPCache) Set(adapterName, method string, params interface{}, data interface{}, etag ...string) {
	key := c.generateCacheKey(adapterName, method, params)
	ttl := c.getTTLForMethod(method)

	entry := &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       ttl,
		HitCount:  0,
	}

	if len(etag) > 0 {
		entry.ETag = etag[0]
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.cache) >= c.config.MaxEntries {
		c.evictLRU()
	}

	c.cache[key] = entry
	c.updateMetrics()
}

// Invalidate removes entries matching the given criteria
func (c *MCPCache) Invalidate(adapterName, method string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	keysToDelete := make([]string, 0)

	for key := range c.cache {
		// Simple pattern matching - in a real implementation, you might want regex
		if adapterName != "" && method != "" {
			// Invalidate specific method for specific adapter
			if c.keyMatches(key, adapterName, method) {
				keysToDelete = append(keysToDelete, key)
			}
		} else if adapterName != "" {
			// Invalidate all methods for specific adapter
			if c.keyMatchesAdapter(key, adapterName) {
				keysToDelete = append(keysToDelete, key)
			}
		} else if method != "" {
			// Invalidate specific method for all adapters
			if c.keyMatchesMethod(key, method) {
				keysToDelete = append(keysToDelete, key)
			}
		}
	}

	for _, key := range keysToDelete {
		delete(c.cache, key)
	}

	c.updateMetrics()
}

// InvalidateAll clears the entire cache
func (c *MCPCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CacheEntry)
	c.updateMetrics()
}

// evictLRU removes least recently used entries
func (c *MCPCache) evictLRU() {
	if len(c.cache) == 0 {
		return
	}

	// Find entry with lowest hit count (simple LRU approximation)
	var oldestKey string
	var lowestHitCount int64 = -1

	for key, entry := range c.cache {
		if lowestHitCount == -1 || entry.HitCount < lowestHitCount {
			oldestKey = key
			lowestHitCount = entry.HitCount
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
		if c.config.EnableMetrics {
			c.metrics.IncrementEvictions()
		}
	}
}

// cleanupRoutine periodically removes expired entries
func (c *MCPCache) cleanupRoutine() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup removes expired entries
func (c *MCPCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	keysToDelete := make([]string, 0)

	for key, entry := range c.cache {
		if entry.IsExpired() {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.cache, key)
	}

	c.updateMetrics()
}

// keyMatches checks if a cache key matches the given adapter and method
func (c *MCPCache) keyMatches(key, adapterName, method string) bool {
	// Generate expected key and compare
	expectedKey := c.generateCacheKey(adapterName, method, nil)
	return key == expectedKey
}

// keyMatchesAdapter checks if a cache key matches the given adapter
func (c *MCPCache) keyMatchesAdapter(key, adapterName string) bool {
	// Simple check - see if the key starts with the adapter's hash
	// This is a simplified approach
	testKey := c.generateCacheKey(adapterName, "test", nil)
	return len(key) >= len(testKey) && key[:len(testKey)] == testKey[:len(testKey)]
}

// keyMatchesMethod checks if a cache key matches the given method
func (c *MCPCache) keyMatchesMethod(key, method string) bool {
	// This is harder to determine without storing components separately
	// For now, we'll use a simple approach
	testKey1 := c.generateCacheKey("adapter1", method, nil)

	// Check if the key pattern matches the method
	if len(key) >= len(testKey1) {
		suffix1 := testKey1[len(c.generateCacheKey("adapter1", "", nil)):]
		suffix2 := key[len(c.generateCacheKey("adapter1", "", nil)):]
		if len(suffix2) >= len(suffix1) && suffix2[:len(suffix1)] == suffix1 {
			return true
		}
	}
	return false
}

// updateMetrics updates cache metrics
func (c *MCPCache) updateMetrics() {
	if !c.config.EnableMetrics {
		return
	}

	// Calculate memory usage (rough estimate)
	var memory int64
	for _, entry := range c.cache {
		if data, err := json.Marshal(entry); err == nil {
			memory += int64(len(data))
		}
	}

	c.metrics.UpdateEntries(len(c.cache), memory)
}

// GetMetrics returns current cache metrics
func (c *MCPCache) GetMetrics() map[string]interface{} {
	if !c.config.EnableMetrics {
		return map[string]interface{}{"metrics_enabled": false}
	}

	return c.metrics.CacheStats()
}

// Close stops the cache cleanup routine
func (c *MCPCache) Close() {
	close(c.stopCh)
}

// CacheableResponse represents a response that can be cached
type CacheableResponse struct {
	Data    interface{}       `json:"data"`
	ETag    string            `json:"etag,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// IsCacheable checks if a response should be cached based on the method and status
func IsCacheable(method string, statusCode int) bool {
	// Only cache successful GET-like operations
	if statusCode >= 200 && statusCode < 300 {
		switch method {
		case "tools/list", "prompts/list", "resources/list", "capabilities":
			return true
		default:
			// Don't cache state-changing operations
			return false
		}
	}
	return false
}
