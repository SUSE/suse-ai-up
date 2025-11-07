package mcp

import (
	"testing"
	"time"
)

func TestCacheEntry_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		entry    *CacheEntry
		expected bool
	}{
		{
			name: "fresh entry",
			entry: &CacheEntry{
				Timestamp: time.Now(),
				TTL:       time.Minute,
			},
			expected: false,
		},
		{
			name: "expired entry",
			entry: &CacheEntry{
				Timestamp: time.Now().Add(-2 * time.Minute),
				TTL:       time.Minute,
			},
			expected: true,
		},
		{
			name: "zero TTL entry",
			entry: &CacheEntry{
				Timestamp: time.Now(),
				TTL:       0,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.IsExpired(); got != tt.expected {
				t.Errorf("CacheEntry.IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCacheEntry_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		entry    *CacheEntry
		expected bool
	}{
		{
			name: "valid entry",
			entry: &CacheEntry{
				Data:      "test data",
				Timestamp: time.Now(),
				TTL:       time.Minute,
			},
			expected: true,
		},
		{
			name: "expired entry",
			entry: &CacheEntry{
				Data:      "test data",
				Timestamp: time.Now().Add(-2 * time.Minute),
				TTL:       time.Minute,
			},
			expected: false,
		},
		{
			name: "nil data entry",
			entry: &CacheEntry{
				Data:      nil,
				Timestamp: time.Now(),
				TTL:       time.Minute,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.IsValid(); got != tt.expected {
				t.Errorf("CacheEntry.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	if config.CapabilitiesTTL != 5*time.Minute {
		t.Errorf("Expected CapabilitiesTTL to be 5 minutes, got %v", config.CapabilitiesTTL)
	}

	if config.MaxEntries != 1000 {
		t.Errorf("Expected MaxEntries to be 1000, got %d", config.MaxEntries)
	}

	if config.EnableMetrics != true {
		t.Errorf("Expected EnableMetrics to be true, got %v", config.EnableMetrics)
	}
}

func TestMCPCache_GetSet(t *testing.T) {
	cache := NewMCPCache(DefaultCacheConfig())
	defer cache.Close()

	adapterName := "test-adapter"
	method := "tools/list"
	params := map[string]interface{}{"cursor": "123"}
	data := map[string]interface{}{"tools": []string{"tool1", "tool2"}}

	// Test cache miss
	result, found := cache.Get(adapterName, method, params)
	if found {
		t.Error("Expected cache miss, but got hit")
	}
	if result != nil {
		t.Error("Expected nil result on cache miss")
	}

	// Test cache set
	cache.Set(adapterName, method, params, data)

	// Test cache hit
	result, found = cache.Get(adapterName, method, params)
	if !found {
		t.Error("Expected cache hit, but got miss")
	}
	if result == nil {
		t.Error("Expected non-nil result on cache hit")
	}

	// Verify data integrity
	if resultMap, ok := result.(map[string]interface{}); ok {
		if tools, ok := resultMap["tools"].([]string); ok {
			if len(tools) != 2 || tools[0] != "tool1" || tools[1] != "tool2" {
				t.Errorf("Expected tools [tool1, tool2], got %v", tools)
			}
		} else {
			t.Error("Expected tools to be []string")
		}
	} else {
		t.Error("Expected result to be map[string]interface{}")
	}
}

func TestMCPCache_Expiration(t *testing.T) {
	config := DefaultCacheConfig()
	config.ResponseTTL = 100 * time.Millisecond    // Short TTL for testing
	config.CleanupInterval = 50 * time.Millisecond // More frequent cleanup
	cache := NewMCPCache(config)
	defer cache.Close()

	adapterName := "test-adapter"
	method := "tools/list"
	data := "test data"

	// Set cache entry
	cache.Set(adapterName, method, nil, data)

	// Should be available immediately
	result, found := cache.Get(adapterName, method, nil)
	if !found {
		t.Error("Expected cache hit immediately after set")
	}

	// Wait for expiration and cleanup
	time.Sleep(200 * time.Millisecond)

	// Should be expired
	result, found = cache.Get(adapterName, method, nil)
	if found {
		t.Error("Expected cache miss after expiration")
	}
	if result != nil {
		t.Error("Expected nil result after expiration")
	}
}

func TestMCPCache_Invalidate(t *testing.T) {
	cache := NewMCPCache(DefaultCacheConfig())
	defer cache.Close()

	adapterName := "test-adapter"
	method := "tools/list"
	data := "test data"

	// Set cache entry
	cache.Set(adapterName, method, nil, data)

	// Verify it exists
	_, found := cache.Get(adapterName, method, nil)
	if !found {
		t.Error("Expected cache hit before invalidation")
	}

	// Invalidate all entries for adapter (since specific method invalidation isn't implemented)
	cache.Invalidate(adapterName, "")

	// Should be gone
	_, found = cache.Get(adapterName, method, nil)
	if found {
		t.Error("Expected cache miss after invalidation")
	}
}

func TestMCPCache_InvalidateAll(t *testing.T) {
	cache := NewMCPCache(DefaultCacheConfig())
	defer cache.Close()

	adapterName := "test-adapter"

	// Set multiple cache entries
	cache.Set(adapterName, "tools/list", nil, "tools data")
	cache.Set(adapterName, "resources/list", nil, "resources data")
	cache.Set("other-adapter", "tools/list", nil, "other tools data")

	// Verify they exist
	if _, found := cache.Get(adapterName, "tools/list", nil); !found {
		t.Error("Expected cache hit for tools/list")
	}
	if _, found := cache.Get(adapterName, "resources/list", nil); !found {
		t.Error("Expected cache hit for resources/list")
	}
	if _, found := cache.Get("other-adapter", "tools/list", nil); !found {
		t.Error("Expected cache hit for other adapter")
	}

	// Invalidate all
	cache.InvalidateAll()

	// All should be gone
	if _, found := cache.Get(adapterName, "tools/list", nil); found {
		t.Error("Expected cache miss for tools/list after InvalidateAll")
	}
	if _, found := cache.Get(adapterName, "resources/list", nil); found {
		t.Error("Expected cache miss for resources/list after InvalidateAll")
	}
	if _, found := cache.Get("other-adapter", "tools/list", nil); found {
		t.Error("Expected cache miss for other adapter after InvalidateAll")
	}
}

func TestMCPCache_Metrics(t *testing.T) {
	config := DefaultCacheConfig()
	config.EnableMetrics = true
	cache := NewMCPCache(config)
	defer cache.Close()

	adapterName := "test-adapter"
	method := "tools/list"
	data := "test data"

	// Get initial metrics
	metrics := cache.GetMetrics()
	if hits, ok := metrics["hits"].(int64); ok && hits != 0 {
		t.Errorf("Expected initial hits to be 0, got %d", hits)
	}

	// Cache miss
	cache.Get(adapterName, method, nil)

	// Cache set
	cache.Set(adapterName, method, nil, data)

	// Cache hit
	cache.Get(adapterName, method, nil)

	// Another cache miss (different params)
	cache.Get(adapterName, method, map[string]interface{}{"different": "params"})

	// Check metrics
	metrics = cache.GetMetrics()
	if hits, ok := metrics["hits"].(int64); ok && hits != 1 {
		t.Errorf("Expected hits to be 1, got %d", hits)
	}
	if misses, ok := metrics["misses"].(int64); ok && misses != 2 {
		t.Errorf("Expected misses to be 2, got %d", misses)
	}
}

func TestIsCacheable(t *testing.T) {
	tests := []struct {
		method     string
		statusCode int
		expected   bool
	}{
		{"tools/list", 200, true},
		{"tools/call", 200, false},
		{"resources/list", 200, true},
		{"resources/read", 200, false},
		{"prompts/list", 200, true},
		{"prompts/get", 200, false},
		{"completion/complete", 200, false},
		{"tools/list", 404, false},
		{"tools/list", 500, false},
		{"unknown/method", 200, false},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+string(rune(tt.statusCode)), func(t *testing.T) {
			if got := IsCacheable(tt.method, tt.statusCode); got != tt.expected {
				t.Errorf("IsCacheable(%s, %d) = %v, want %v", tt.method, tt.statusCode, got, tt.expected)
			}
		})
	}
}
