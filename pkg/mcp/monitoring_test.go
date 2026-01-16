package mcp

import (
	"testing"
	"time"
)

func TestDefaultMonitoringConfig(t *testing.T) {
	config := DefaultMonitoringConfig()

	if !config.EnableMetrics {
		t.Error("Expected EnableMetrics to be true")
	}

	if !config.EnableLogging {
		t.Error("Expected EnableLogging to be true")
	}

	if config.LogLevel != LogLevelInfo {
		t.Errorf("Expected LogLevel to be Info, got %v", config.LogLevel)
	}

	if config.MaxLogEntries != 10000 {
		t.Errorf("Expected MaxLogEntries to be 10000, got %d", config.MaxLogEntries)
	}
}

func TestMCPMonitor_LogOperation(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.MaxLogEntries = 10 // Small for testing
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Test logging at different levels
	monitor.LogOperation(LogLevelDebug, "TestComponent", "Debug message", "session1", "adapter1", "test-method", 0, nil, nil)
	monitor.LogOperation(LogLevelInfo, "TestComponent", "Info message", "session1", "adapter1", "test-method", 0, nil, nil)
	monitor.LogOperation(LogLevelWarn, "TestComponent", "Warning message", "session1", "adapter1", "test-method", 0, nil, nil)
	monitor.LogOperation(LogLevelError, "TestComponent", "Error message", "session1", "adapter1", "test-method", 0, nil, nil)

	// Test with error
	testErr := &testError{"test error"}
	monitor.LogOperation(LogLevelError, "TestComponent", "Error with error", "session1", "adapter1", "test-method", 0, testErr, nil)

	// Test with metadata
	metadata := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}
	monitor.LogOperation(LogLevelInfo, "TestComponent", "Message with metadata", "session1", "adapter1", "test-method", 100*time.Millisecond, nil, metadata)

	// Check logs
	logs := monitor.GetRecentLogs(100)
	if len(logs) < 5 {
		t.Errorf("Expected at least 5 log entries, got %d", len(logs))
	}

	// Find specific logs by content
	var errorLog, metadataLog *LogEntry
	for _, log := range logs {
		if log.Error == "test error" {
			errorLog = &log
		}
		if log.Duration == 100*time.Millisecond && log.Metadata != nil {
			metadataLog = &log
		}
	}

	if errorLog == nil {
		t.Error("Expected error log not found")
	} else if errorLog.Error != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", errorLog.Error)
	}

	if metadataLog == nil {
		t.Error("Expected metadata log not found")
	} else {
		if metadataLog.Duration != 100*time.Millisecond {
			t.Errorf("Expected duration 100ms, got %v", metadataLog.Duration)
		}
		if metadataLog.Metadata["key1"] != "value1" {
			t.Errorf("Expected metadata key1 to be 'value1', got %v", metadataLog.Metadata["key1"])
		}
	}
}

func TestMCPMonitor_LogLevelFiltering(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.LogLevel = LogLevelWarn // Only warn and above
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// These should be filtered out
	monitor.LogOperation(LogLevelDebug, "TestComponent", "Debug message", "", "", "", 0, nil, nil)
	monitor.LogOperation(LogLevelInfo, "TestComponent", "Info message", "", "", "", 0, nil, nil)

	// These should be logged
	monitor.LogOperation(LogLevelWarn, "TestComponent", "Warning message", "", "", "", 0, nil, nil)
	monitor.LogOperation(LogLevelError, "TestComponent", "Error message", "", "", "", 0, nil, nil)

	// Check logs - should only have 2 entries
	logs := monitor.GetRecentLogs(10)
	if len(logs) != 2 {
		t.Errorf("Expected 2 log entries after filtering, got %d", len(logs))
	}

	// Verify correct entries
	if logs[0].Level != LogLevelWarn {
		t.Errorf("Expected first log to be Warn, got %v", logs[0].Level)
	}
	if logs[1].Level != LogLevelError {
		t.Errorf("Expected second log to be Error, got %v", logs[1].Level)
	}
}

func TestMCPMonitor_RecordOperation(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.EnableMetrics = true
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Record some operations
	monitor.RecordOperation("tools/list", "session1", "adapter1", true, 100*time.Millisecond)
	monitor.RecordOperation("tools/list", "session1", "adapter1", false, 200*time.Millisecond)
	monitor.RecordOperation("tools/call", "session1", "adapter1", true, 50*time.Millisecond)
	monitor.RecordOperation("tools/list", "session2", "adapter2", true, 150*time.Millisecond)

	// Check metrics
	metrics := monitor.GetMetrics()

	// Check operation metrics
	operations, ok := metrics["operations"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected operations to be map[string]interface{}")
	}

	toolsListMetrics, ok := operations["tools/list"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected tools/list metrics to be map[string]interface{}")
	}

	if totalReqs, ok := toolsListMetrics["total_requests"].(int64); !ok || totalReqs != 3 {
		t.Errorf("Expected tools/list total_requests to be 3, got %v", totalReqs)
	}

	if successfulReqs, ok := toolsListMetrics["successful_requests"].(int64); !ok || successfulReqs != 2 {
		t.Errorf("Expected tools/list successful_requests to be 2, got %v", successfulReqs)
	}

	if failedReqs, ok := toolsListMetrics["failed_requests"].(int64); !ok || failedReqs != 1 {
		t.Errorf("Expected tools/list failed_requests to be 1, got %v", failedReqs)
	}
}

func TestMCPMonitor_GetLogsByFilter(t *testing.T) {
	config := DefaultMonitoringConfig()
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Add various log entries
	monitor.LogOperation(LogLevelInfo, "Component1", "Message1", "session1", "adapter1", "method1", 0, nil, nil)
	monitor.LogOperation(LogLevelError, "Component2", "Message2", "session2", "adapter2", "method2", 0, nil, nil)
	monitor.LogOperation(LogLevelInfo, "Component1", "Message3", "session1", "adapter3", "method3", 0, nil, nil)
	monitor.LogOperation(LogLevelWarn, "Component3", "Message4", "session3", "adapter1", "method4", 0, nil, nil)

	// Test filtering by component
	logs := monitor.GetLogsByFilter("", "Component1", "", "", "", 10)
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs for Component1, got %d", len(logs))
	}

	// Test filtering by session
	logs = monitor.GetLogsByFilter("", "", "session1", "", "", 10)
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs for session1, got %d", len(logs))
	}

	// Test filtering by level
	logs = monitor.GetLogsByFilter(LogLevelError, "", "", "", "", 10)
	if len(logs) != 1 {
		t.Errorf("Expected 1 error log, got %d", len(logs))
	}

	// Test filtering by adapter
	logs = monitor.GetLogsByFilter("", "", "", "adapter1", "", 10)
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs for adapter1, got %d", len(logs))
	}

	// Test filtering by method
	logs = monitor.GetLogsByFilter("", "", "", "", "method2", 10)
	if len(logs) != 1 {
		t.Errorf("Expected 1 log for method2, got %d", len(logs))
	}

	// Test limit
	logs = monitor.GetLogsByFilter("", "", "", "", "", 2)
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs with limit, got %d", len(logs))
	}
}

func TestMCPMonitor_ClearLogs(t *testing.T) {
	config := DefaultMonitoringConfig()
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Add some logs
	monitor.LogOperation(LogLevelInfo, "TestComponent", "Test message", "", "", "", 0, nil, nil)
	monitor.LogOperation(LogLevelError, "TestComponent", "Error message", "", "", "", 0, nil, nil)

	// Verify logs exist
	logs := monitor.GetRecentLogs(10)
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs before clear, got %d", len(logs))
	}

	// Clear logs
	monitor.ClearLogs()

	// Verify logs are cleared
	logs = monitor.GetRecentLogs(10)
	if len(logs) != 0 {
		t.Errorf("Expected 0 logs after clear, got %d", len(logs))
	}
}

func TestMCPMonitor_ResetMetrics(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.EnableMetrics = true
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Record some operations
	monitor.RecordOperation("tools/list", "session1", "adapter1", true, 100*time.Millisecond)
	monitor.RecordOperation("tools/call", "session1", "adapter1", false, 200*time.Millisecond)

	// Verify metrics exist
	metrics := monitor.GetMetrics()
	operations, ok := metrics["operations"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected operations to be map[string]interface{}")
	}

	if len(operations) == 0 {
		t.Error("Expected operations metrics to exist")
	}

	// Reset metrics
	monitor.ResetMetrics()

	// Verify metrics are reset
	metrics = monitor.GetMetrics()
	operations, ok = metrics["operations"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected operations to be map[string]interface{}")
	}

	// Check that all operation metrics are reset to zero
	for method, methodMetrics := range operations {
		if methodMetricsMap, ok := methodMetrics.(map[string]interface{}); ok {
			if totalReqs, ok := methodMetricsMap["total_requests"].(int64); !ok || totalReqs != 0 {
				t.Errorf("Expected %s total_requests to be 0 after reset, got %v", method, totalReqs)
			}
		}
	}
}

func TestOperationTimer(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.EnableMetrics = true
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Test successful operation
	timer := NewOperationTimer(monitor, "tools/list", "session1", "adapter1")
	time.Sleep(10 * time.Millisecond) // Simulate work
	timer.Finish(true)

	// Test failed operation
	timer = NewOperationTimer(monitor, "tools/call", "session1", "adapter1")
	time.Sleep(5 * time.Millisecond) // Simulate work
	timer.Finish(false)

	// Check metrics
	metrics := monitor.GetMetrics()
	operations, ok := metrics["operations"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected operations to be map[string]interface{}")
	}

	// Check tools/list metrics
	toolsListMetrics, ok := operations["tools/list"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected tools/list metrics to be map[string]interface{}")
	}

	if totalReqs, ok := toolsListMetrics["total_requests"].(int64); !ok || totalReqs != 1 {
		t.Errorf("Expected tools/list total_requests to be 1, got %v", totalReqs)
	}

	if successfulReqs, ok := toolsListMetrics["successful_requests"].(int64); !ok || successfulReqs != 1 {
		t.Errorf("Expected tools/list successful_requests to be 1, got %v", successfulReqs)
	}

	// Check tools/call metrics
	toolsCallMetrics, ok := operations["tools/call"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected tools/call metrics to be map[string]interface{}")
	}

	if totalReqs, ok := toolsCallMetrics["total_requests"].(int64); !ok || totalReqs != 1 {
		t.Errorf("Expected tools/call total_requests to be 1, got %v", totalReqs)
	}

	if failedReqs, ok := toolsCallMetrics["failed_requests"].(int64); !ok || failedReqs != 1 {
		t.Errorf("Expected tools/call failed_requests to be 1, got %v", failedReqs)
	}
}

func TestMCPMonitor_HelperFunctions(t *testing.T) {
	config := DefaultMonitoringConfig()
	monitor := NewMCPMonitor(config)
	defer monitor.Close()

	// Test helper functions
	monitor.LogRequest("session1", "adapter1", "tools/list", map[string]interface{}{"param": "value"})
	monitor.LogResponse("session1", "adapter1", "tools/list", true, 100*time.Millisecond, "result")
	monitor.LogCacheOperation("get", "adapter1", "tools/list", true, 5*time.Millisecond)
	monitor.LogSessionActivity("session1", "created", "adapter1", map[string]interface{}{"user": "test"})
	monitor.LogAdapterActivity("adapter1", "registered", map[string]interface{}{"type": "http"})
	monitor.LogError("TestComponent", "Test error", "session1", "adapter1", "tools/list", &testError{"test"}, nil)

	// Verify logs were created
	logs := monitor.GetRecentLogs(10)
	if len(logs) != 6 {
		t.Errorf("Expected 6 log entries from helper functions, got %d", len(logs))
	}

	// Verify specific log types
	var requestLog, responseLog, cacheLog, sessionLog, adapterLog, errorLog *LogEntry
	for _, log := range logs {
		switch log.Component {
		case "MessageRouter":
			if log.Message == "Processing tools/list request" {
				requestLog = &log
			} else if log.Message == "Completed tools/list request successfully" {
				responseLog = &log
			}
		case "Cache":
			cacheLog = &log
		case "SessionManager":
			sessionLog = &log
		case "AdapterManager":
			adapterLog = &log
		case "TestComponent":
			errorLog = &log
		}
	}

	if requestLog == nil {
		t.Error("Expected request log not found")
	}
	if responseLog == nil {
		t.Error("Expected response log not found")
	}
	if cacheLog == nil {
		t.Error("Expected cache log not found")
	}
	if sessionLog == nil {
		t.Error("Expected session log not found")
	}
	if adapterLog == nil {
		t.Error("Expected adapter log not found")
	}
	if errorLog == nil {
		t.Error("Expected error log not found")
	}
}

// Test helper types
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
