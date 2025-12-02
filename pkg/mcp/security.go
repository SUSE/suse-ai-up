package mcp

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// UserConsentRequest represents a request for user consent
type UserConsentRequest struct {
	Type                string   `json:"type"` // tool_execution, resource_access, data_sharing, sampling_request
	Resource            string   `json:"resource,omitempty"`
	Tool                string   `json:"tool,omitempty"`
	Description         string   `json:"description"`
	RequiredPermissions []string `json:"requiredPermissions"`
	RiskLevel           string   `json:"riskLevel"` // low, medium, high
}

// UserConsentResponse represents a response to a consent request
type UserConsentResponse struct {
	Approved           bool                   `json:"approved"`
	GrantedPermissions []string               `json:"grantedPermissions,omitempty"`
	ExpiresAt          *time.Time             `json:"expiresAt,omitempty"`
	Conditions         map[string]interface{} `json:"conditions,omitempty"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	Timestamp    time.Time              `json:"timestamp"`
	SessionID    string                 `json:"sessionId"`
	AdapterName  string                 `json:"adapterName"`
	UserID       string                 `json:"userId,omitempty"`
	Action       string                 `json:"action"`
	Resource     string                 `json:"resource,omitempty"`
	Success      bool                   `json:"success"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	RiskLevel    string                 `json:"riskLevel,omitempty"`
	ConsentID    string                 `json:"consentId,omitempty"`
}

// MCPSecurityService handles MCP security and authorization
type MCPSecurityService struct {
	auditLogs []AuditLog
	// In a production system, this would be persisted to a database
}

// NewMCPSecurityService creates a new MCP security service
func NewMCPSecurityService() *MCPSecurityService {
	return &MCPSecurityService{
		auditLogs: make([]AuditLog, 0),
	}
}

// RequestUserConsent requests user consent for a sensitive operation
func (s *MCPSecurityService) RequestUserConsent(sessionID, adapterName string, request UserConsentRequest) (*UserConsentResponse, error) {
	log.Printf("User consent requested for session %s, adapter %s: %s", sessionID, adapterName, request.Description)

	// Assess risk level if not provided
	if request.RiskLevel == "" {
		request.RiskLevel = s.assessRisk(request)
	}

	// For now, auto-approve low-risk operations and deny high-risk ones
	// In a real implementation, this would involve user interaction
	var approved bool
	var grantedPermissions []string

	switch request.RiskLevel {
	case "low":
		approved = true
		grantedPermissions = request.RequiredPermissions
	case "medium":
		// For medium risk, approve but with conditions
		approved = true
		grantedPermissions = request.RequiredPermissions
		expiresAt := time.Now().Add(1 * time.Hour)
		response := &UserConsentResponse{
			Approved:           true,
			GrantedPermissions: grantedPermissions,
			ExpiresAt:          &expiresAt,
			Conditions: map[string]interface{}{
				"maxRequestsPerHour": 10,
				"auditRequired":      true,
			},
		}
		s.logAuditEvent(sessionID, adapterName, "consent_granted", "", true, "", request.RiskLevel, "auto-approved-medium-risk")
		return response, nil
	case "high":
		approved = false
	default:
		approved = false
	}

	response := &UserConsentResponse{
		Approved:           approved,
		GrantedPermissions: grantedPermissions,
	}

	action := "consent_denied"
	if approved {
		action = "consent_granted"
	}
	s.logAuditEvent(sessionID, adapterName, action, "", approved, "", request.RiskLevel, "auto-decision")

	return response, nil
}

// ValidateToolExecution validates if a tool execution is allowed
func (s *MCPSecurityService) ValidateToolExecution(sessionID, adapterName, toolName string, parameters map[string]interface{}) (bool, error) {
	riskLevel := s.assessToolRisk(toolName, parameters)

	if riskLevel == "high" {
		consent, err := s.RequestUserConsent(sessionID, adapterName, UserConsentRequest{
			Type:                "tool_execution",
			Tool:                toolName,
			Description:         fmt.Sprintf("Execute tool '%s' with parameters: %v", toolName, parameters),
			RequiredPermissions: []string{"tool.execute"},
			RiskLevel:           riskLevel,
		})
		if err != nil {
			s.logAuditEvent(sessionID, adapterName, "tool_validation_failed", toolName, false, err.Error(), riskLevel, "")
			return false, err
		}
		if !consent.Approved {
			s.logAuditEvent(sessionID, adapterName, "tool_execution_denied", toolName, false, "consent denied", riskLevel, "")
			return false, CreateMCPError(MCP_TOOL_EXECUTION_DENIED, "Tool execution denied by user consent", nil)
		}
	}

	s.logAuditEvent(sessionID, adapterName, "tool_execution_allowed", toolName, true, "", riskLevel, "")
	return true, nil
}

// ValidateResourceAccess validates if resource access is allowed
func (s *MCPSecurityService) ValidateResourceAccess(sessionID, adapterName, resourceURI, accessType string) (bool, error) {
	riskLevel := s.assessResourceRisk(resourceURI, accessType)

	if riskLevel == "high" {
		consent, err := s.RequestUserConsent(sessionID, adapterName, UserConsentRequest{
			Type:                "resource_access",
			Resource:            resourceURI,
			Description:         fmt.Sprintf("%s access to resource '%s'", strings.Title(accessType), resourceURI),
			RequiredPermissions: []string{fmt.Sprintf("resource.%s", accessType)},
			RiskLevel:           riskLevel,
		})
		if err != nil {
			s.logAuditEvent(sessionID, adapterName, "resource_validation_failed", resourceURI, false, err.Error(), riskLevel, "")
			return false, err
		}
		if !consent.Approved {
			s.logAuditEvent(sessionID, adapterName, "resource_access_denied", resourceURI, false, "consent denied", riskLevel, "")
			return false, CreateMCPError(MCP_RESOURCE_ACCESS_DENIED, "Resource access denied by user consent", nil)
		}
	}

	s.logAuditEvent(sessionID, adapterName, "resource_access_allowed", resourceURI, true, "", riskLevel, "")
	return true, nil
}

// LogAuditEvent logs an audit event
func (s *MCPSecurityService) LogAuditEvent(sessionID, adapterName, action, resource string, success bool, errorMessage, riskLevel, consentID string) {
	event := AuditLog{
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		AdapterName:  adapterName,
		Action:       action,
		Resource:     resource,
		Success:      success,
		ErrorMessage: errorMessage,
		RiskLevel:    riskLevel,
		ConsentID:    consentID,
		Metadata:     make(map[string]interface{}),
	}

	s.auditLogs = append(s.auditLogs, event)
	log.Printf("Audit: %s - %s - %s - Success: %v", sessionID, action, resource, success)
}

// GetAuditLogs retrieves audit logs with optional filtering
func (s *MCPSecurityService) GetAuditLogs(sessionID, adapterName, action string, limit int) []AuditLog {
	var filtered []AuditLog

	for _, log := range s.auditLogs {
		if sessionID != "" && log.SessionID != sessionID {
			continue
		}
		if adapterName != "" && log.AdapterName != adapterName {
			continue
		}
		if action != "" && log.Action != action {
			continue
		}
		filtered = append(filtered, log)
	}

	// Return most recent logs up to limit
	if len(filtered) > limit && limit > 0 {
		start := len(filtered) - limit
		filtered = filtered[start:]
	}

	return filtered
}

// assessRisk assesses the overall risk level for a request
func (s *MCPSecurityService) assessRisk(request UserConsentRequest) string {
	switch request.Type {
	case "tool_execution":
		return s.assessToolRisk(request.Tool, nil)
	case "resource_access":
		return s.assessResourceRisk(request.Resource, "read")
	case "data_sharing":
		return "high"
	case "sampling_request":
		return "medium"
	default:
		return "medium"
	}
}

// assessToolRisk assesses the risk level of a tool execution
func (s *MCPSecurityService) assessToolRisk(toolName string, parameters map[string]interface{}) string {
	// Define high-risk tools
	highRiskTools := []string{
		"execute_shell",
		"run_command",
		"delete_file",
		"modify_system",
		"access_sensitive_data",
	}

	// Check if tool is in high-risk list
	for _, highRiskTool := range highRiskTools {
		if strings.Contains(strings.ToLower(toolName), highRiskTool) {
			return "high"
		}
	}

	// Check for sensitive parameters
	if parameters != nil {
		for key := range parameters {
			if strings.Contains(strings.ToLower(key), "password") ||
				strings.Contains(strings.ToLower(key), "secret") ||
				strings.Contains(strings.ToLower(key), "token") {
				return "high"
			}
		}
	}

	// Default to medium risk for unknown tools
	return "medium"
}

// assessResourceRisk assesses the risk level of resource access
func (s *MCPSecurityService) assessResourceRisk(resourceURI, accessType string) string {
	// High-risk resource patterns
	highRiskPatterns := []string{
		"/etc/",
		"/var/log",
		"/home/",
		"/root/",
		"password",
		"secret",
		"private",
	}

	// Check resource URI for sensitive patterns
	resourceLower := strings.ToLower(resourceURI)
	for _, pattern := range highRiskPatterns {
		if strings.Contains(resourceLower, pattern) {
			return "high"
		}
	}

	// Write access is higher risk than read access
	if accessType == "write" {
		return "medium"
	}

	return "low"
}

// logAuditEvent is a helper method to log audit events
func (s *MCPSecurityService) logAuditEvent(sessionID, adapterName, action, resource string, success bool, errorMessage, riskLevel, consentID string) {
	s.LogAuditEvent(sessionID, adapterName, action, resource, success, errorMessage, riskLevel, consentID)
}
