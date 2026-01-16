package mcp

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       LogLevel               `json:"level"`
	Message     string                 `json:"message"`
	Component   string                 `json:"component"`
	SessionID   string                 `json:"session_id,omitempty"`
	AdapterName string                 `json:"adapter_name,omitempty"`
	Method      string                 `json:"method,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// OperationMetrics tracks metrics for MCP operations
type OperationMetrics struct {
	mu              sync.RWMutex
	TotalRequests   int64         `json:"total_requests"`
	SuccessfulReqs  int64         `json:"successful_requests"`
	FailedReqs      int64         `json:"failed_requests"`
	AverageLatency  time.Duration `json:"average_latency"`
	TotalLatency    time.Duration `json:"total_latency"`
	MinLatency      time.Duration `json:"min_latency"`
	MaxLatency      time.Duration `json:"max_latency"`
	LastRequestTime time.Time     `json:"last_request_time"`
	ErrorRate       float64       `json:"error_rate"`
}

// UpdateMetrics updates operation metrics with a new request
func (om *OperationMetrics) UpdateMetrics(success bool, latency time.Duration) {
	om.mu.Lock()
	defer om.mu.Unlock()

	om.TotalRequests++
	om.TotalLatency += latency
	om.AverageLatency = om.TotalLatency / time.Duration(om.TotalRequests)
	om.LastRequestTime = time.Now()

	if success {
		om.SuccessfulReqs++
	} else {
		om.FailedReqs++
	}

	if om.MinLatency == 0 || latency < om.MinLatency {
		om.MinLatency = latency
	}
	if latency > om.MaxLatency {
		om.MaxLatency = latency
	}

	if om.TotalRequests > 0 {
		om.ErrorRate = float64(om.FailedReqs) / float64(om.TotalRequests) * 100
	}
}

// GetStats returns current operation statistics
func (om *OperationMetrics) GetStats() map[string]interface{} {
	om.mu.RLock()
	defer om.mu.RUnlock()

	return map[string]interface{}{
		"total_requests":      om.TotalRequests,
		"successful_requests": om.SuccessfulReqs,
		"failed_requests":     om.FailedReqs,
		"average_latency":     om.AverageLatency.String(),
		"min_latency":         om.MinLatency.String(),
		"max_latency":         om.MaxLatency.String(),
		"error_rate":          fmt.Sprintf("%.2f%%", om.ErrorRate),
		"last_request_time":   om.LastRequestTime.Format(time.RFC3339),
	}
}

// MonitoringConfig holds configuration for monitoring
type MonitoringConfig struct {
	EnableMetrics     bool          `json:"enable_metrics"`
	EnableLogging     bool          `json:"enable_logging"`
	LogLevel          LogLevel      `json:"log_level"`
	MetricsInterval   time.Duration `json:"metrics_interval"`
	MaxLogEntries     int           `json:"max_log_entries"`
	EnablePerformance bool          `json:"enable_performance"`
}

// DefaultMonitoringConfig returns default monitoring configuration
func DefaultMonitoringConfig() *MonitoringConfig {
	return &MonitoringConfig{
		EnableMetrics:     true,
		EnableLogging:     true,
		LogLevel:          LogLevelInfo,
		MetricsInterval:   30 * time.Second,
		MaxLogEntries:     10000,
		EnablePerformance: true,
	}
}

// MCPMonitor provides comprehensive monitoring and logging for MCP operations
type MCPMonitor struct {
	config           *MonitoringConfig
	mu               sync.RWMutex
	logEntries       []LogEntry
	operationMetrics map[string]*OperationMetrics // method -> metrics
	sessionMetrics   map[string]*OperationMetrics // sessionID -> metrics
	adapterMetrics   map[string]*OperationMetrics // adapterName -> metrics
	stopCh           chan struct{}
}

// NewMCPMonitor creates a new MCP monitor instance
func NewMCPMonitor(config *MonitoringConfig) *MCPMonitor {
	if config == nil {
		config = DefaultMonitoringConfig()
	}

	monitor := &MCPMonitor{
		config:           config,
		logEntries:       make([]LogEntry, 0),
		operationMetrics: make(map[string]*OperationMetrics),
		sessionMetrics:   make(map[string]*OperationMetrics),
		adapterMetrics:   make(map[string]*OperationMetrics),
		stopCh:           make(chan struct{}),
	}

	// Initialize metrics for common operations
	commonOperations := []string{
		"tools/list", "tools/call",
		"resources/list", "resources/read", "resources/subscribe",
		"prompts/list", "prompts/get",
		"completion/complete",
		"initialize", "capabilities",
	}

	for _, op := range commonOperations {
		monitor.operationMetrics[op] = &OperationMetrics{}
	}

	return monitor
}

// LogOperation logs an MCP operation with context
func (m *MCPMonitor) LogOperation(level LogLevel, component, message, sessionID, adapterName, method string, duration time.Duration, err error, metadata map[string]interface{}) {
	if !m.config.EnableLogging {
		return
	}

	// Filter by log level
	if !m.shouldLog(level) {
		return
	}

	entry := LogEntry{
		Timestamp:   time.Now(),
		Level:       level,
		Message:     message,
		Component:   component,
		SessionID:   sessionID,
		AdapterName: adapterName,
		Method:      method,
		Duration:    duration,
		Metadata:    metadata,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.logEntries = append(m.logEntries, entry)

	// Maintain max log entries
	if len(m.logEntries) > m.config.MaxLogEntries {
		m.logEntries = m.logEntries[1:]
	}

	// Also log to standard logger for immediate visibility
	logMessage := fmt.Sprintf("[%s] %s: %s", level, component, message)
	if sessionID != "" {
		logMessage += fmt.Sprintf(" (session: %s)", sessionID)
	}
	if adapterName != "" {
		logMessage += fmt.Sprintf(" (adapter: %s)", adapterName)
	}
	if method != "" {
		logMessage += fmt.Sprintf(" (method: %s)", method)
	}
	if duration > 0 {
		logMessage += fmt.Sprintf(" (duration: %v)", duration)
	}
	if err != nil {
		logMessage += fmt.Sprintf(" (error: %v)", err)
	}

	switch level {
	case LogLevelDebug, LogLevelInfo:
		log.Print(logMessage)
	case LogLevelWarn:
		log.Print("WARNING: " + logMessage)
	case LogLevelError, LogLevelFatal:
		log.Print("ERROR: " + logMessage)
	}
}

// RecordOperation records metrics for an operation
func (m *MCPMonitor) RecordOperation(method, sessionID, adapterName string, success bool, latency time.Duration) {
	if !m.config.EnableMetrics {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Update operation metrics
	if _, exists := m.operationMetrics[method]; !exists {
		m.operationMetrics[method] = &OperationMetrics{}
	}
	m.operationMetrics[method].UpdateMetrics(success, latency)

	// Update session metrics
	if sessionID != "" {
		if _, exists := m.sessionMetrics[sessionID]; !exists {
			m.sessionMetrics[sessionID] = &OperationMetrics{}
		}
		m.sessionMetrics[sessionID].UpdateMetrics(success, latency)
	}

	// Update adapter metrics
	if adapterName != "" {
		if _, exists := m.adapterMetrics[adapterName]; !exists {
			m.adapterMetrics[adapterName] = &OperationMetrics{}
		}
		m.adapterMetrics[adapterName].UpdateMetrics(success, latency)
	}
}

// shouldLog checks if the log level should be recorded
func (m *MCPMonitor) shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
		LogLevelFatal: 4,
	}

	currentLevel := levels[m.config.LogLevel]
	messageLevel := levels[level]

	return messageLevel >= currentLevel
}

// GetMetrics returns comprehensive monitoring metrics
func (m *MCPMonitor) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := map[string]interface{}{
		"monitoring_enabled": m.config.EnableMetrics,
		"logging_enabled":    m.config.EnableLogging,
		"log_level":          string(m.config.LogLevel),
		"total_log_entries":  len(m.logEntries),
	}

	// Operation metrics
	operationStats := make(map[string]interface{})
	for method, stats := range m.operationMetrics {
		operationStats[method] = stats.GetStats()
	}
	metrics["operations"] = operationStats

	// Session metrics (top 10 most active)
	sessionStats := make(map[string]interface{})
	for sessionID, stats := range m.sessionMetrics {
		if stats.TotalRequests > 0 {
			sessionStats[sessionID] = stats.GetStats()
		}
	}
	metrics["sessions"] = sessionStats

	// Adapter metrics
	adapterStats := make(map[string]interface{})
	for adapterName, stats := range m.adapterMetrics {
		if stats.TotalRequests > 0 {
			adapterStats[adapterName] = stats.GetStats()
		}
	}
	metrics["adapters"] = adapterStats

	return metrics
}

// GetRecentLogs returns recent log entries
func (m *MCPMonitor) GetRecentLogs(limit int) []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.logEntries) {
		limit = len(m.logEntries)
	}

	// Return the most recent entries
	start := len(m.logEntries) - limit
	if start < 0 {
		start = 0
	}

	entries := make([]LogEntry, limit)
	copy(entries, m.logEntries[start:])

	return entries
}

// GetLogsByFilter returns log entries matching the filter criteria
func (m *MCPMonitor) GetLogsByFilter(level LogLevel, component, sessionID, adapterName, method string, limit int) []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []LogEntry

	for _, entry := range m.logEntries {
		// Apply filters
		if level != "" && entry.Level != level {
			continue
		}
		if component != "" && entry.Component != component {
			continue
		}
		if sessionID != "" && entry.SessionID != sessionID {
			continue
		}
		if adapterName != "" && entry.AdapterName != adapterName {
			continue
		}
		if method != "" && entry.Method != method {
			continue
		}

		filtered = append(filtered, entry)
	}

	// Apply limit
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	return filtered
}

// ClearLogs clears all log entries
func (m *MCPMonitor) ClearLogs() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logEntries = make([]LogEntry, 0)
}

// ResetMetrics resets all metrics
func (m *MCPMonitor) ResetMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, metrics := range m.operationMetrics {
		*metrics = OperationMetrics{}
	}

	m.sessionMetrics = make(map[string]*OperationMetrics)
	m.adapterMetrics = make(map[string]*OperationMetrics)
}

// Close stops the monitoring system
func (m *MCPMonitor) Close() {
	close(m.stopCh)
}

// OperationTimer helps measure operation duration
type OperationTimer struct {
	monitor     *MCPMonitor
	startTime   time.Time
	method      string
	sessionID   string
	adapterName string
}

// NewOperationTimer creates a new operation timer
func NewOperationTimer(monitor *MCPMonitor, method, sessionID, adapterName string) *OperationTimer {
	return &OperationTimer{
		monitor:     monitor,
		startTime:   time.Now(),
		method:      method,
		sessionID:   sessionID,
		adapterName: adapterName,
	}
}

// Finish completes the operation and records metrics
func (ot *OperationTimer) Finish(success bool) {
	duration := time.Since(ot.startTime)
	if ot.monitor != nil {
		ot.monitor.RecordOperation(ot.method, ot.sessionID, ot.adapterName, success, duration)
	}
}

// LogOperationWithTimer logs an operation with automatic timing
func (m *MCPMonitor) LogOperationWithTimer(level LogLevel, component, message string, sessionID, adapterName, method string, err error, metadata map[string]interface{}) func(bool) {
	timer := NewOperationTimer(m, method, sessionID, adapterName)

	return func(success bool) {
		duration := time.Since(timer.startTime)
		m.LogOperation(level, component, message, sessionID, adapterName, method, duration, err, metadata)
		timer.Finish(success)
	}
}

// Helper functions for common logging scenarios

// LogRequest logs an incoming MCP request
func (m *MCPMonitor) LogRequest(sessionID, adapterName, method string, params interface{}) {
	metadata := make(map[string]interface{})
	if params != nil {
		metadata["params"] = params
	}

	m.LogOperation(LogLevelInfo, "MessageRouter", fmt.Sprintf("Processing %s request", method),
		sessionID, adapterName, method, 0, nil, metadata)
}

// LogResponse logs an MCP response
func (m *MCPMonitor) LogResponse(sessionID, adapterName, method string, success bool, duration time.Duration, result interface{}) {
	level := LogLevelInfo
	if !success {
		level = LogLevelError
	}

	metadata := make(map[string]interface{})
	metadata["success"] = success
	if result != nil {
		metadata["result_type"] = fmt.Sprintf("%T", result)
	}

	message := fmt.Sprintf("Completed %s request", method)
	if success {
		message += " successfully"
	} else {
		message += " with errors"
	}

	m.LogOperation(level, "MessageRouter", message,
		sessionID, adapterName, method, duration, nil, metadata)
}

// LogCacheOperation logs cache operations
func (m *MCPMonitor) LogCacheOperation(operation, adapterName, method string, hit bool, duration time.Duration) {
	level := LogLevelDebug
	if !hit {
		level = LogLevelInfo // Cache misses are more interesting
	}

	metadata := map[string]interface{}{
		"operation": operation,
		"hit":       hit,
	}

	message := fmt.Sprintf("Cache %s for %s on %s", operation, method, adapterName)
	if hit {
		message += " (hit)"
	} else {
		message += " (miss)"
	}

	m.LogOperation(level, "Cache", message, "", adapterName, method, duration, nil, metadata)
}

// LogSessionActivity logs session-related activities
func (m *MCPMonitor) LogSessionActivity(sessionID, activity string, adapterName string, metadata map[string]interface{}) {
	m.LogOperation(LogLevelInfo, "SessionManager", activity, sessionID, adapterName, "", 0, nil, metadata)
}

// LogAdapterActivity logs adapter-related activities
func (m *MCPMonitor) LogAdapterActivity(adapterName, activity string, metadata map[string]interface{}) {
	m.LogOperation(LogLevelInfo, "AdapterManager", activity, "", adapterName, "", 0, nil, metadata)
}

// LogError logs an error with context
func (m *MCPMonitor) LogError(component, message string, sessionID, adapterName, method string, err error, metadata map[string]interface{}) {
	m.LogOperation(LogLevelError, component, message, sessionID, adapterName, method, 0, err, metadata)
}
