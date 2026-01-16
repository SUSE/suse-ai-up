package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/session"
)

// ErrorType represents different types of MCP errors
type ErrorType string

const (
	ErrorTypeProtocol   ErrorType = "protocol"
	ErrorTypeTransport  ErrorType = "transport"
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeParsing    ErrorType = "parsing"
	ErrorTypeCapability ErrorType = "capability"
	ErrorTypeSession    ErrorType = "session"
)

// MCPError represents a comprehensive MCP error
type MCPError struct {
	Type        ErrorType              `json:"type"`
	Code        int                    `json:"code"`
	Message     string                 `json:"message"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	SessionID   string                 `json:"sessionId,omitempty"`
	AdapterName string                 `json:"adapterName,omitempty"`
	Retryable   bool                   `json:"retryable"`
	Suggestions []string               `json:"suggestions,omitempty"`
}

// Error implements the error interface
func (e *MCPError) Error() string {
	return fmt.Sprintf("[%s] %d: %s", e.Type, e.Code, e.Message)
}

// ErrorHandler handles MCP errors with comprehensive logging and recovery
type ErrorHandler struct {
	sessionStore session.SessionStore
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(sessionStore session.SessionStore) *ErrorHandler {
	return &ErrorHandler{
		sessionStore: sessionStore,
	}
}

// HandleProtocolError handles MCP protocol errors
func (eh *ErrorHandler) HandleProtocolError(ctx context.Context, err error, sessionID, adapterName string) *MCPError {
	mcpErr := &MCPError{
		Type:        ErrorTypeProtocol,
		Timestamp:   time.Now(),
		SessionID:   sessionID,
		AdapterName: adapterName,
		Retryable:   false,
		Details:     make(map[string]interface{}),
		Suggestions: []string{},
	}

	// Analyze the error to determine specific details
	if strings.Contains(err.Error(), "parse error") {
		mcpErr.Code = ErrCodeParseError
		mcpErr.Message = "Failed to parse MCP message"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check message format and JSON syntax"
		mcpErr.Suggestions = []string{
			"Verify JSON-RPC 2.0 message format",
			"Check for proper UTF-8 encoding",
			"Ensure required fields are present",
		}
	} else if strings.Contains(err.Error(), "invalid request") {
		mcpErr.Code = ErrCodeInvalidRequest
		mcpErr.Message = "Invalid MCP request"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check request structure and parameters"
		mcpErr.Suggestions = []string{
			"Verify method name is correct",
			"Check parameter types and values",
			"Ensure protocol version is '2.0'",
		}
	} else if strings.Contains(err.Error(), "method not found") {
		mcpErr.Code = ErrCodeMethodNotFound
		mcpErr.Message = "MCP method not supported"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check if method is supported by the target server"
		mcpErr.Suggestions = []string{
			"Verify method name spelling",
			"Check server capabilities",
			"Ensure server supports the requested method",
		}
	} else if strings.Contains(err.Error(), "invalid params") {
		mcpErr.Code = ErrCodeInvalidParams
		mcpErr.Message = "Invalid method parameters"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check parameter types and values"
		mcpErr.Suggestions = []string{
			"Verify parameter names",
			"Check parameter types",
			"Ensure required parameters are provided",
		}
	} else {
		mcpErr.Code = ErrCodeInternalError
		mcpErr.Message = "Internal MCP protocol error"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Suggestions = []string{
			"Try the request again",
			"Contact support if the issue persists",
		}
	}

	eh.logError(mcpErr)
	return mcpErr
}

// HandleTransportError handles transport-level errors
func (eh *ErrorHandler) HandleTransportError(ctx context.Context, err error, sessionID, adapterName string) *MCPError {
	mcpErr := &MCPError{
		Type:        ErrorTypeTransport,
		Timestamp:   time.Now(),
		SessionID:   sessionID,
		AdapterName: adapterName,
		Retryable:   true,
		Details:     make(map[string]interface{}),
		Suggestions: []string{},
	}

	// Analyze transport error
	if strings.Contains(err.Error(), "connection refused") {
		mcpErr.Code = -32010 // Custom connection error code
		mcpErr.Message = "Connection to target server refused"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check if target server is running"
		mcpErr.Suggestions = []string{
			"Verify target server is running",
			"Check network connectivity",
			"Ensure correct port and address",
		}
	} else if strings.Contains(err.Error(), "timeout") {
		mcpErr.Code = -32011 // Custom timeout error code
		mcpErr.Message = "Request timeout"
		mcpErr.Type = ErrorTypeTimeout
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Increase timeout or check server performance"
		mcpErr.Suggestions = []string{
			"Increase request timeout",
			"Check server performance",
			"Try again later",
		}
	} else if strings.Contains(err.Error(), "network") {
		mcpErr.Code = -32012 // Custom network error code
		mcpErr.Message = "Network error"
		mcpErr.Type = ErrorTypeNetwork
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check network connectivity"
		mcpErr.Suggestions = []string{
			"Check network connection",
			"Verify DNS resolution",
			"Check firewall settings",
		}
	} else {
		mcpErr.Code = ErrCodeInternalError
		mcpErr.Message = "Transport error"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Suggestions = []string{
			"Try the request again",
			"Check network connectivity",
		}
	}

	eh.logError(mcpErr)
	return mcpErr
}

// HandleAuthError handles authentication errors
func (eh *ErrorHandler) HandleAuthError(ctx context.Context, err error, sessionID, adapterName string) *MCPError {
	mcpErr := &MCPError{
		Type:        ErrorTypeAuth,
		Timestamp:   time.Now(),
		SessionID:   sessionID,
		AdapterName: adapterName,
		Retryable:   false,
		Details:     make(map[string]interface{}),
		Suggestions: []string{},
	}

	// Analyze auth error
	if strings.Contains(err.Error(), "unauthorized") {
		mcpErr.Code = ErrCodeMCPUnauthorized
		mcpErr.Message = "Authentication failed"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check authentication credentials"
		mcpErr.Suggestions = []string{
			"Verify authentication token",
			"Check token expiration",
			"Refresh authentication token",
		}
	} else if strings.Contains(err.Error(), "forbidden") {
		mcpErr.Code = ErrCodeMCPNotFound // Using existing code
		mcpErr.Message = "Access forbidden"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Check access permissions"
		mcpErr.Suggestions = []string{
			"Check access permissions",
			"Verify user roles",
			"Contact administrator",
		}
	} else {
		mcpErr.Code = ErrCodeInternalError
		mcpErr.Message = "Authentication error"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Suggestions = []string{
			"Check authentication configuration",
			"Verify credentials",
		}
	}

	eh.logError(mcpErr)
	return mcpErr
}

// HandleValidationError handles validation errors
func (eh *ErrorHandler) HandleValidationError(ctx context.Context, err error, sessionID, adapterName string, validationDetails map[string]interface{}) *MCPError {
	mcpErr := &MCPError{
		Type:        ErrorTypeValidation,
		Code:        ErrCodeInvalidParams,
		Message:     "Validation error",
		Timestamp:   time.Now(),
		SessionID:   sessionID,
		AdapterName: adapterName,
		Retryable:   false,
		Details:     validationDetails,
		Suggestions: []string{
			"Check input parameters",
			"Verify data format",
			"Ensure required fields are provided",
		},
	}

	if err != nil {
		if mcpErr.Details == nil {
			mcpErr.Details = make(map[string]interface{})
		}
		mcpErr.Details["originalError"] = err.Error()
	}

	eh.logError(mcpErr)
	return mcpErr
}

// HandleSessionError handles session-related errors
func (eh *ErrorHandler) HandleSessionError(ctx context.Context, err error, sessionID, adapterName string) *MCPError {
	mcpErr := &MCPError{
		Type:        ErrorTypeSession,
		Timestamp:   time.Now(),
		SessionID:   sessionID,
		AdapterName: adapterName,
		Retryable:   true,
		Details:     make(map[string]interface{}),
		Suggestions: []string{},
	}

	// Analyze session error
	if strings.Contains(err.Error(), "not found") {
		mcpErr.Code = ErrCodeMCPNotFound
		mcpErr.Message = "Session not found"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Session may have expired"
		mcpErr.Suggestions = []string{
			"Start a new session",
			"Check session ID",
			"Verify session is still active",
		}
	} else if strings.Contains(err.Error(), "expired") {
		mcpErr.Code = ErrCodeMCPUnauthorized
		mcpErr.Message = "Session expired"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Details["suggestion"] = "Session has expired, please reconnect"
		mcpErr.Suggestions = []string{
			"Start a new session",
			"Refresh session token",
		}
	} else {
		mcpErr.Code = ErrCodeInternalError
		mcpErr.Message = "Session error"
		mcpErr.Details["originalError"] = err.Error()
		mcpErr.Suggestions = []string{
			"Try the request again",
			"Restart session if needed",
		}
	}

	eh.logError(mcpErr)
	return mcpErr
}

// ToJSONRPCResponse converts MCPError to JSON-RPC error response
func (eh *ErrorHandler) ToJSONRPCResponse(id interface{}, mcpErr *MCPError) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    mcpErr.Code,
			Message: mcpErr.Message,
			Data:    mcpErr.Details,
		},
	}
}

// ToHTTPResponse converts MCPError to HTTP response
func (eh *ErrorHandler) ToHTTPResponse(mcpErr *MCPError) (int, map[string]interface{}) {
	statusCode := http.StatusInternalServerError
	response := map[string]interface{}{
		"error": mcpErr,
	}

	// Map MCP error codes to HTTP status codes
	switch mcpErr.Code {
	case ErrCodeParseError, ErrCodeInvalidRequest, ErrCodeInvalidParams:
		statusCode = http.StatusBadRequest
	case ErrCodeMethodNotFound:
		statusCode = http.StatusNotFound
	case ErrCodeMCPUnauthorized:
		statusCode = http.StatusUnauthorized
	case ErrCodeMCPNotFound:
		statusCode = http.StatusNotFound
	case -32010, -32011, -32012: // Custom transport errors
		statusCode = http.StatusBadGateway
	}

	return statusCode, response
}

// logError logs the error with appropriate level
func (eh *ErrorHandler) logError(mcpErr *MCPError) {
	logMessage := fmt.Sprintf("MCP Error [%s] %d: %s", mcpErr.Type, mcpErr.Code, mcpErr.Message)

	if mcpErr.SessionID != "" {
		logMessage += fmt.Sprintf(" (Session: %s", mcpErr.SessionID)
	}

	if mcpErr.AdapterName != "" {
		logMessage += fmt.Sprintf(", Adapter: %s)", mcpErr.AdapterName)
	}

	if mcpErr.Retryable {
		log.Printf("RETRYABLE - %s", logMessage)
	} else {
		log.Printf("FATAL - %s", logMessage)
	}

	// Log details if available
	if mcpErr.Details != nil {
		if detailsJSON, err := json.Marshal(mcpErr.Details); err == nil {
			log.Printf("Error Details: %s", string(detailsJSON))
		}
	}

	// Log suggestions
	if len(mcpErr.Suggestions) > 0 {
		log.Printf("Suggestions: %v", mcpErr.Suggestions)
	}
}

// IsRetryable checks if an error is retryable
func (eh *ErrorHandler) IsRetryable(err error) bool {
	if mcpErr, ok := err.(*MCPError); ok {
		return mcpErr.Retryable
	}

	// Check error string for retryable conditions
	errStr := err.Error()
	retryableStrings := []string{
		"timeout",
		"connection refused",
		"network",
		"temporary",
		"retry",
	}

	for _, retryableStr := range retryableStrings {
		if strings.Contains(strings.ToLower(errStr), retryableStr) {
			return true
		}
	}

	return false
}

// GetRetryDelay calculates appropriate retry delay based on error type
func (eh *ErrorHandler) GetRetryDelay(err error, attempt int) time.Duration {
	if mcpErr, ok := err.(*MCPError); ok {
		switch mcpErr.Type {
		case ErrorTypeTimeout:
			return time.Duration(attempt) * time.Second
		case ErrorTypeNetwork:
			return time.Duration(attempt*2) * time.Second
		case ErrorTypeTransport:
			return time.Duration(attempt) * 500 * time.Millisecond
		default:
			return time.Second
		}
	}

	// Default exponential backoff
	return time.Duration(attempt) * time.Second
}
