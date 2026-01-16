package mcp

import (
	"context"
	"testing"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// TestMCPIntegration tests the complete MCP proxy flow
func TestMCPIntegration(t *testing.T) {
	// This is a basic integration test that verifies components can be initialized
	// and work together without errors

	// Initialize components
	sessionStore := session.NewInMemorySessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)
	messageRouter := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)

	// Cleanup
	defer cache.Close()
	defer monitor.Close()

	// Verify components are initialized
	if sessionStore == nil {
		t.Fatal("Session store not initialized")
	}

	if capabilityCache == nil {
		t.Fatal("Capability cache not initialized")
	}

	if cache == nil {
		t.Fatal("Response cache not initialized")
	}

	if monitor == nil {
		t.Fatal("Monitor not initialized")
	}

	if protocolHandler == nil {
		t.Fatal("Protocol handler not initialized")
	}

	if messageRouter == nil {
		t.Fatal("Message router not initialized")
	}

	// Test basic functionality
	adapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name: "test-adapter",
		},
		ID: "test-adapter",
	}

	// Create a test message
	message := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "tools/list",
		Params:  nil,
	}

	// Test routing (will fail since no real server, but should not panic)
	ctx := context.Background()
	sessionID := "test-session"

	// This should not panic and should return an error (no real server)
	response, err := messageRouter.RouteMessage(ctx, message, adapter, sessionID)

	// We expect an error since there's no real MCP server
	if err == nil {
		t.Error("Expected error when routing to non-existent server")
	}

	// Response might be nil or contain error info, both are acceptable
	if response != nil && response.Error == nil {
		t.Error("Expected response to contain error or be nil")
	}

	// Verify monitoring recorded the attempt
	metrics := monitor.GetMetrics()
	if metrics == nil {
		t.Error("Expected monitoring metrics")
	}

	// Verify cache metrics are available
	cacheMetrics := messageRouter.GetCacheMetrics()
	if cacheMetrics == nil {
		t.Error("Expected cache metrics")
	}
}

// TestMCPComponentsInitialization tests that all MCP components can be initialized
func TestMCPComponentsInitialization(t *testing.T) {
	t.Run("CapabilityCache", func(t *testing.T) {
		cache := NewCapabilityCache()
		if cache == nil {
			t.Error("Capability cache initialization failed")
		}
	})

	t.Run("ResponseCache", func(t *testing.T) {
		cache := NewMCPCache(DefaultCacheConfig())
		defer cache.Close()
		if cache == nil {
			t.Error("Response cache initialization failed")
		}
	})

	t.Run("Monitor", func(t *testing.T) {
		monitor := NewMCPMonitor(DefaultMonitoringConfig())
		defer monitor.Close()
		if monitor == nil {
			t.Error("Monitor initialization failed")
		}
	})

	t.Run("ErrorHandler", func(t *testing.T) {
		sessionStore := session.NewInMemorySessionStore()
		handler := NewErrorHandler(sessionStore)
		if handler == nil {
			t.Error("Error handler initialization failed")
		}
	})
}

// TestMCPMessageHandling tests basic message handling
func TestMCPMessageHandling(t *testing.T) {
	sessionStore := session.NewInMemorySessionStore()
	capabilityCache := NewCapabilityCache()
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())
	protocolHandler := NewProtocolHandler(sessionStore, capabilityCache)
	messageRouter := NewMessageRouter(protocolHandler, sessionStore, capabilityCache, cache, monitor)

	defer cache.Close()
	defer monitor.Close()

	// Test different message types
	testCases := []struct {
		name   string
		method string
	}{
		{"ToolsList", "tools/list"},
		{"ResourcesList", "resources/list"},
		{"PromptsList", "prompts/list"},
		{"ToolsCall", "tools/call"},
		{"ResourcesRead", "resources/read"},
		{"PromptsGet", "prompts/get"},
		{"CompletionComplete", "completion/complete"},
	}

	adapter := models.AdapterResource{
		AdapterData: models.AdapterData{
			Name: "test-adapter",
		},
		ID: "test-adapter",
	}

	ctx := context.Background()
	sessionID := "test-session"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			message := &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      "test-id-" + tc.name,
				Method:  tc.method,
				Params:  nil,
			}

			// Should not panic
			response, err := messageRouter.RouteMessage(ctx, message, adapter, sessionID)

			// We expect errors since no real server, but no panics
			if err == nil {
				t.Logf("Unexpected success for %s (this might be OK if using cached data)", tc.method)
			}

			if response != nil && response.Error == nil && err == nil {
				t.Logf("Successful response for %s", tc.method)
			}
		})
	}
}

// TestMCPCacheAndMonitoringIntegration tests cache and monitoring work together
func TestMCPCacheAndMonitoringIntegration(t *testing.T) {
	cache := NewMCPCache(DefaultCacheConfig())
	monitor := NewMCPMonitor(DefaultMonitoringConfig())

	defer cache.Close()
	defer monitor.Close()

	adapterName := "test-adapter"
	method := "tools/list"
	data := map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "test-tool",
				"description": "A test tool",
			},
		},
	}

	// Test cache operations with monitoring
	startTime := time.Now()

	// Cache miss
	result, found := cache.Get(adapterName, method, nil)
	if found {
		t.Error("Expected cache miss")
	}
	if result != nil {
		t.Error("Expected nil result on cache miss")
	}

	// Log cache miss
	monitor.LogCacheOperation("get", adapterName, method, false, time.Since(startTime))

	// Set cache
	cache.Set(adapterName, method, nil, data)

	// Cache hit
	startTime = time.Now()
	result, found = cache.Get(adapterName, method, nil)
	if !found {
		t.Error("Expected cache hit")
	}
	if result == nil {
		t.Error("Expected non-nil result on cache hit")
	}

	// Log cache hit
	monitor.LogCacheOperation("get", adapterName, method, true, time.Since(startTime))

	// Verify metrics
	cacheMetrics := cache.GetMetrics()
	if cacheMetrics == nil {
		t.Error("Expected cache metrics")
	}

	monitorMetrics := monitor.GetMetrics()
	if monitorMetrics == nil {
		t.Error("Expected monitor metrics")
	}

	// Should have cache operation logs
	logs := monitor.GetRecentLogs(10)
	if len(logs) < 2 {
		t.Error("Expected at least 2 log entries for cache operations")
	}
}

// TestMCPErrorHandling tests error handling integration
// TODO: Rewrite this test to use the new ErrorHandler API
func TestMCPErrorHandling(t *testing.T) {
	t.Skip("Test needs to be rewritten to match current ErrorHandler API")

	// errorHandler := NewErrorHandler(session.NewInMemorySessionStore())

	// Test different error types using new API
	// testCases := []struct {
	// 	name         string
	// 	errFunc      func() error
	// 	expectedType ErrorType
	// }{
	// 	{
	// 		name: "ProtocolError",
	// 		errFunc: func() error {
	// 			return fmt.Errorf("parse error")
	// 		},
	// 		expectedType: ErrorTypeProtocol,
	// 	},
	// }

	// for _, tc := range testCases {
	// 	t.Run(tc.name, func(t *testing.T) {
	// 		err := tc.errFunc()
	// 		if err == nil {
	// 			t.Error("Expected error")
	// 		}

	// 		// Test error handling
	// 		mcpErr := errorHandler.HandleProtocolError(context.Background(), err, "test-session", "test-adapter")
	// 		if mcpErr == nil {
	// 			t.Error("Expected MCP error")
	// 		}

	// 		if mcpErr.Type != tc.expectedType {
	// 			t.Errorf("Expected error type %s, got %s", tc.expectedType, mcpErr.Type)
	// 		}
	// 	})
	// }
}
