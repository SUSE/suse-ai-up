package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// MessageRouter handles routing of MCP messages to appropriate handlers
type MessageRouter struct {
	protocolHandler *ProtocolHandler
	sessionStore    session.SessionStore
	capabilityCache *CapabilityCache
	cache           *MCPCache
	monitor         *MCPMonitor
	httpClient      *http.Client
}

// NewMessageRouter creates a new MCP message router
func NewMessageRouter(protocolHandler *ProtocolHandler, sessionStore session.SessionStore, capabilityCache *CapabilityCache, cache *MCPCache, monitor *MCPMonitor) *MessageRouter {
	return &MessageRouter{
		protocolHandler: protocolHandler,
		sessionStore:    sessionStore,
		capabilityCache: capabilityCache,
		cache:           cache,
		monitor:         monitor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RouteMessage routes an MCP message to the appropriate handler
func (mr *MessageRouter) RouteMessage(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	startTime := time.Now()

	// Log incoming request
	if mr.monitor != nil {
		mr.monitor.LogRequest(sessionID, adapter.Name, message.Method, message.Params)
	}

	log.Printf("MessageRouter: Routing method %s for adapter %s", message.Method, adapter.Name)

	var response *JSONRPCMessage
	var err error

	switch message.Method {
	case "tools/list":
		response, err = mr.handleToolsList(ctx, message, adapter, sessionID)
	case "tools/call":
		response, err = mr.handleToolsCall(ctx, message, adapter, sessionID)
	case "resources/list":
		response, err = mr.handleResourcesList(ctx, message, adapter, sessionID)
	case "resources/read":
		response, err = mr.handleResourcesRead(ctx, message, adapter, sessionID)
	case "resources/subscribe":
		response, err = mr.handleResourcesSubscribe(ctx, message, adapter, sessionID)
	case "prompts/list":
		response, err = mr.handlePromptsList(ctx, message, adapter, sessionID)
	case "prompts/get":
		response, err = mr.handlePromptsGet(ctx, message, adapter, sessionID)
	case "completion/complete":
		response, err = mr.handleCompletionComplete(ctx, message, adapter, sessionID)
	default:
		// For unknown methods, try to proxy to target server
		response, err = mr.proxyToTarget(ctx, message, adapter, sessionID)
	}

	// Log completion and metrics
	duration := time.Since(startTime)
	success := err == nil && response != nil

	if mr.monitor != nil {
		mr.monitor.LogResponse(sessionID, adapter.Name, message.Method, success, duration, response)
		mr.monitor.RecordOperation(message.Method, sessionID, adapter.Name, success, duration)
	}

	return response, err
}

// handleToolsList handles tools/list requests
func (mr *MessageRouter) handleToolsList(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling tools/list for %s", adapter.Name)

	// Check cache first if available
	if mr.cache != nil {
		startTime := time.Now()
		if cached, found := mr.cache.Get(adapter.Name, "tools/list", message.Params); found {
			duration := time.Since(startTime)
			log.Printf("MessageRouter: Cache hit for tools/list on %s", adapter.Name)
			if mr.monitor != nil {
				mr.monitor.LogCacheOperation("get", adapter.Name, "tools/list", true, duration)
			}
			return CreateSuccessResponse(message.ID, cached), nil
		} else {
			duration := time.Since(startTime)
			if mr.monitor != nil {
				mr.monitor.LogCacheOperation("get", adapter.Name, "tools/list", false, duration)
			}
		}
	}

	// If adapter has discovered functionality, return cached tools
	if adapter.MCPFunctionality != nil && len(adapter.MCPFunctionality.Tools) > 0 {
		result := map[string]interface{}{
			"tools": adapter.MCPFunctionality.Tools,
		}

		// Cache the result
		if mr.cache != nil {
			mr.cache.Set(adapter.Name, "tools/list", message.Params, result)
		}

		return CreateSuccessResponse(message.ID, result), nil
	}

	// Otherwise, proxy to target server
	response, err := mr.proxyToTarget(ctx, message, adapter, sessionID)
	if err == nil && response != nil && mr.cache != nil {
		// Cache the successful response
		if response.Result != nil {
			mr.cache.Set(adapter.Name, "tools/list", message.Params, response.Result)
		}
	}
	return response, err
}

// handleToolsCall handles tools/call requests
func (mr *MessageRouter) handleToolsCall(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling tools/call for %s", adapter.Name)
	return mr.proxyToTarget(ctx, message, adapter, sessionID)
}

// handleResourcesList handles resources/list requests
func (mr *MessageRouter) handleResourcesList(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling resources/list for %s", adapter.Name)

	// Check cache first if available
	if mr.cache != nil {
		if cached, found := mr.cache.Get(adapter.Name, "resources/list", message.Params); found {
			log.Printf("MessageRouter: Cache hit for resources/list on %s", adapter.Name)
			return CreateSuccessResponse(message.ID, cached), nil
		}
	}

	// If adapter has discovered functionality, return cached resources
	if adapter.MCPFunctionality != nil && len(adapter.MCPFunctionality.Resources) > 0 {
		result := map[string]interface{}{
			"resources": adapter.MCPFunctionality.Resources,
		}

		// Cache the result
		if mr.cache != nil {
			mr.cache.Set(adapter.Name, "resources/list", message.Params, result)
		}

		return CreateSuccessResponse(message.ID, result), nil
	}

	// Otherwise, proxy to target server
	response, err := mr.proxyToTarget(ctx, message, adapter, sessionID)
	if err == nil && response != nil && mr.cache != nil {
		// Cache the successful response
		if response.Result != nil {
			mr.cache.Set(adapter.Name, "resources/list", message.Params, response.Result)
		}
	}
	return response, err
}

// GetCacheMetrics returns current cache performance metrics
func (mr *MessageRouter) GetCacheMetrics() map[string]interface{} {
	if mr.cache == nil {
		return map[string]interface{}{"cache_enabled": false}
	}
	return mr.cache.GetMetrics()
}

// handleResourcesRead handles resources/read requests
func (mr *MessageRouter) handleResourcesRead(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling resources/read for %s", adapter.Name)
	return mr.proxyToTarget(ctx, message, adapter, sessionID)
}

// handleResourcesSubscribe handles resources/subscribe requests
func (mr *MessageRouter) handleResourcesSubscribe(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling resources/subscribe for %s", adapter.Name)
	return mr.proxyToTarget(ctx, message, adapter, sessionID)
}

// handlePromptsList handles prompts/list requests
func (mr *MessageRouter) handlePromptsList(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling prompts/list for %s", adapter.Name)

	// Check cache first if available
	if mr.cache != nil {
		if cached, found := mr.cache.Get(adapter.Name, "prompts/list", message.Params); found {
			log.Printf("MessageRouter: Cache hit for prompts/list on %s", adapter.Name)
			return CreateSuccessResponse(message.ID, cached), nil
		}
	}

	// If adapter has discovered functionality, return cached prompts
	if adapter.MCPFunctionality != nil && len(adapter.MCPFunctionality.Prompts) > 0 {
		result := map[string]interface{}{
			"prompts": adapter.MCPFunctionality.Prompts,
		}

		// Cache the result
		if mr.cache != nil {
			mr.cache.Set(adapter.Name, "prompts/list", message.Params, result)
		}

		return CreateSuccessResponse(message.ID, result), nil
	}

	// Otherwise, proxy to target server
	response, err := mr.proxyToTarget(ctx, message, adapter, sessionID)
	if err == nil && response != nil && mr.cache != nil {
		// Cache the successful response
		if response.Result != nil {
			mr.cache.Set(adapter.Name, "prompts/list", message.Params, response.Result)
		}
	}
	return response, err
}

// handlePromptsGet handles prompts/get requests
func (mr *MessageRouter) handlePromptsGet(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling prompts/get for %s", adapter.Name)
	return mr.proxyToTarget(ctx, message, adapter, sessionID)
}

// handleCompletionComplete handles completion/complete requests
func (mr *MessageRouter) handleCompletionComplete(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Handling completion/complete for %s", adapter.Name)
	return mr.proxyToTarget(ctx, message, adapter, sessionID)
}

// proxyToTarget proxies a message to the target MCP server
func (mr *MessageRouter) proxyToTarget(ctx context.Context, message *JSONRPCMessage, adapter models.AdapterResource, sessionID string) (*JSONRPCMessage, error) {
	log.Printf("MessageRouter: Proxying %s to target server for %s", message.Method, adapter.Name)

	// Build target URL based on adapter connection type
	targetURL, err := mr.buildTargetURL(adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to build target URL: %w", err)
	}

	// Marshal message
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, strings.NewReader(string(messageBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", ProtocolVersion)

	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	// Apply authentication if configured
	if err := mr.applyAuthentication(req, adapter); err != nil {
		return nil, fmt.Errorf("failed to apply authentication: %w", err)
	}

	// Send request
	resp, err := mr.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Handle response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("target server returned status %d", resp.StatusCode)
	}

	// Parse response
	var response JSONRPCMessage
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// buildTargetURL builds the target URL for the adapter
func (mr *MessageRouter) buildTargetURL(adapter models.AdapterResource) (string, error) {
	switch adapter.ConnectionType {
	case models.ConnectionTypeRemoteHttp:
		if adapter.RemoteUrl == "" {
			return "", fmt.Errorf("remote URL is required for RemoteHttp connection")
		}
		return adapter.RemoteUrl, nil
	case models.ConnectionTypeStreamableHttp, models.ConnectionTypeSSE:
		// For Kubernetes-deployed adapters, construct service URL
		if adapter.RemoteUrl != "" {
			return adapter.RemoteUrl, nil
		}
		// For sidecar-based adapters, use sidecar service URL
		if adapter.SidecarConfig != nil {
			port := adapter.SidecarConfig.Port
			if port == 0 {
				port = 8000
			}
			return fmt.Sprintf("http://mcp-sidecar-%s.suse-ai-up-mcp.svc.cluster.local:%d/mcp", adapter.ID, port), nil
		}
		// Fallback for non-sidecar adapters
		return fmt.Sprintf("http://%s-service.adapter.svc.cluster.local:8000/mcp", adapter.Name), nil
	case models.ConnectionTypeLocalStdio:
		// For local stdio, we need to handle differently
		return "", fmt.Errorf("local stdio connections not yet supported in message router")
	default:
		return "", fmt.Errorf("unsupported connection type: %s", adapter.ConnectionType)
	}
}

// applyAuthentication applies authentication to the request
func (mr *MessageRouter) applyAuthentication(req *http.Request, adapter models.AdapterResource) error {
	if adapter.Authentication == nil || !adapter.Authentication.Required {
		return nil // No authentication required
	}

	switch adapter.Authentication.Type {
	case "bearer":
		return mr.applyBearerAuth(req, adapter)
	case "oauth":
		return mr.applyOAuthAuth(req, adapter)
	case "basic":
		return mr.applyBasicAuth(req, adapter)
	case "apikey":
		return mr.applyAPIKeyAuth(req, adapter)
	default:
		return fmt.Errorf("unsupported authentication type: %s", adapter.Authentication.Type)
	}
}

// applyBearerAuth applies bearer authentication
func (mr *MessageRouter) applyBearerAuth(req *http.Request, adapter models.AdapterResource) error {
	var token string

	// Check bearer token configuration
	if adapter.Authentication.BearerToken != nil && adapter.Authentication.BearerToken.Token != "" {
		token = adapter.Authentication.BearerToken.Token
	}

	if token == "" {
		return fmt.Errorf("no bearer token available")
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// applyOAuthAuth applies OAuth authentication
func (mr *MessageRouter) applyOAuthAuth(req *http.Request, adapter models.AdapterResource) error {
	// For now, this is a placeholder
	// In a full implementation, this would handle OAuth token management
	return fmt.Errorf("OAuth authentication not yet implemented in message router")
}

// applyBasicAuth applies basic authentication
func (mr *MessageRouter) applyBasicAuth(req *http.Request, adapter models.AdapterResource) error {
	if adapter.Authentication.Basic == nil {
		return fmt.Errorf("basic authentication configuration not found")
	}

	req.SetBasicAuth(adapter.Authentication.Basic.Username, adapter.Authentication.Basic.Password)
	return nil
}

// applyAPIKeyAuth applies API key authentication
func (mr *MessageRouter) applyAPIKeyAuth(req *http.Request, adapter models.AdapterResource) error {
	if adapter.Authentication.APIKey == nil {
		return fmt.Errorf("API key configuration not found")
	}

	location := adapter.Authentication.APIKey.Location
	name := adapter.Authentication.APIKey.Name
	key := adapter.Authentication.APIKey.Key

	switch location {
	case "header":
		req.Header.Set(name, key)
	case "query":
		// Add to query parameters
		if req.URL == nil {
			return fmt.Errorf("request URL is nil")
		}
		query := req.URL.Query()
		query.Set(name, key)
		req.URL.RawQuery = query.Encode()
	case "cookie":
		// Add cookie
		req.AddCookie(&http.Cookie{Name: name, Value: key})
	default:
		return fmt.Errorf("unsupported API key location: %s", location)
	}

	return nil
}

// HandleNotification handles MCP notifications
func (mr *MessageRouter) HandleNotification(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Handling notification %s for %s", notification.Method, adapter.Name)

	switch notification.Method {
	case "notifications/tools/list_changed":
		return mr.handleToolsListChanged(ctx, notification, adapter, sessionID)
	case "notifications/resources/list_changed":
		return mr.handleResourcesListChanged(ctx, notification, adapter, sessionID)
	case "notifications/resources/updated":
		return mr.handleResourcesUpdated(ctx, notification, adapter, sessionID)
	case "notifications/prompts/list_changed":
		return mr.handlePromptsListChanged(ctx, notification, adapter, sessionID)
	case "notifications/cancelled":
		return mr.handleCancelled(ctx, notification, adapter, sessionID)
	case "notifications/progress":
		return mr.handleProgress(ctx, notification, adapter, sessionID)
	default:
		log.Printf("MessageRouter: Unknown notification method: %s", notification.Method)
		return nil
	}
}

// handleToolsListChanged handles tools/list_changed notifications
func (mr *MessageRouter) handleToolsListChanged(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Tools list changed for %s", adapter.Name)

	// Invalidate capability cache for this adapter
	mr.capabilityCache.InvalidateCache(adapter)

	// Invalidate response cache for tools
	if mr.cache != nil {
		mr.cache.Invalidate(adapter.Name, "tools/list")
	}

	// Update session capabilities if available
	if sessionID != "" {
		// Get fresh capabilities
		freshCapabilities, err := mr.capabilityCache.GetCapabilities(ctx, adapter)
		if err == nil {
			mr.sessionStore.SetMCPCapabilities(sessionID, freshCapabilities)
		}
	}

	// Forward notification to client via SSE if they have an active connection
	// This would be handled by the streamable transport

	return nil
}

// handleResourcesListChanged handles resources/list_changed notifications
func (mr *MessageRouter) handleResourcesListChanged(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Resources list changed for %s", adapter.Name)

	// Invalidate capability cache for this adapter
	mr.capabilityCache.InvalidateCache(adapter)

	// Invalidate response cache for resources
	if mr.cache != nil {
		mr.cache.Invalidate(adapter.Name, "resources/list")
	}

	// Update session capabilities if available
	if sessionID != "" {
		freshCapabilities, err := mr.capabilityCache.GetCapabilities(ctx, adapter)
		if err == nil {
			mr.sessionStore.SetMCPCapabilities(sessionID, freshCapabilities)
		}
	}

	return nil
}

// handleResourcesUpdated handles resources/updated notifications
func (mr *MessageRouter) handleResourcesUpdated(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Resource updated for %s", adapter.Name)

	// Extract resource information from notification if available
	if notification.Params != nil {
		if params, ok := notification.Params.(map[string]interface{}); ok {
			if resourceURI, exists := params["uri"]; exists {
				log.Printf("MessageRouter: Resource %s updated", resourceURI)
			}
		}
	}

	// Forward notification to client via SSE if they have an active connection
	// This would be handled by the streamable transport

	return nil
}

// handlePromptsListChanged handles prompts/list_changed notifications
func (mr *MessageRouter) handlePromptsListChanged(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Prompts list changed for %s", adapter.Name)

	// Invalidate capability cache for this adapter
	mr.capabilityCache.InvalidateCache(adapter)

	// Invalidate response cache for prompts
	if mr.cache != nil {
		mr.cache.Invalidate(adapter.Name, "prompts/list")
	}

	// Update session capabilities if available
	if sessionID != "" {
		freshCapabilities, err := mr.capabilityCache.GetCapabilities(ctx, adapter)
		if err == nil {
			mr.sessionStore.SetMCPCapabilities(sessionID, freshCapabilities)
		}
	}

	// Forward notification to client via SSE if they have an active connection
	// This would be handled by the streamable transport

	return nil
}

// handleCancelled handles cancelled notifications
func (mr *MessageRouter) handleCancelled(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Operation cancelled for %s", adapter.Name)

	// Extract cancellation details if available
	if notification.Params != nil {
		if params, ok := notification.Params.(map[string]interface{}); ok {
			if requestID, exists := params["requestId"]; exists {
				log.Printf("MessageRouter: Request %v cancelled", requestID)
			}
			if reason, exists := params["reason"]; exists {
				log.Printf("MessageRouter: Cancellation reason: %v", reason)
			}
		}
	}

	return nil
}

// handleProgress handles progress notifications
func (mr *MessageRouter) handleProgress(ctx context.Context, notification *JSONRPCMessage, adapter models.AdapterResource, sessionID string) error {
	log.Printf("MessageRouter: Progress notification for %s", adapter.Name)

	// Extract progress details if available
	if notification.Params != nil {
		if params, ok := notification.Params.(map[string]interface{}); ok {
			if progressToken, exists := params["progressToken"]; exists {
				log.Printf("MessageRouter: Progress token: %v", progressToken)
			}
			if progress, exists := params["progress"]; exists {
				log.Printf("MessageRouter: Progress: %v", progress)
			}
			if total, exists := params["total"]; exists {
				log.Printf("MessageRouter: Total: %v", total)
			}
		}
	}

	return nil
}

// ParseCursor parses pagination cursor from parameters
func (mr *MessageRouter) ParseCursor(params map[string]interface{}) (string, error) {
	if params == nil {
		return "", nil
	}

	cursor, ok := params["cursor"]
	if !ok {
		return "", nil
	}

	switch v := cursor.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', 0, 64), nil
	case int:
		return strconv.Itoa(v), nil
	default:
		return "", fmt.Errorf("invalid cursor type: %T", cursor)
	}
}
