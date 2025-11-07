package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// StreamableHTTPTransport handles MCP Streamable HTTP transport
type StreamableHTTPTransport struct {
	sessionStore    session.SessionStore
	protocolHandler *ProtocolHandler
	messageRouter   *MessageRouter
	httpClient      *http.Client
}

// NewStreamableHTTPTransport creates a new Streamable HTTP transport
func NewStreamableHTTPTransport(sessionStore session.SessionStore, protocolHandler *ProtocolHandler, messageRouter *MessageRouter) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{
		sessionStore:    sessionStore,
		protocolHandler: protocolHandler,
		messageRouter:   messageRouter,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout for streaming
		},
	}
}

// HandleRequest handles an incoming MCP request via Streamable HTTP
func (sht *StreamableHTTPTransport) HandleRequest(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource) {
	log.Printf("StreamableHTTP: Handling %s request for adapter %s", r.Method, adapter.Name)

	// Extract session ID from headers
	sessionID := sht.extractSessionID(r)

	// Validate MCP protocol version
	if protocolVersion := r.Header.Get("MCP-Protocol-Version"); protocolVersion != "" && protocolVersion != ProtocolVersion {
		sht.writeErrorResponse(w, nil, -32600, fmt.Sprintf("Unsupported protocol version: %s", protocolVersion))
		return
	}

	// Set required response headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, MCP-Protocol-Version, Mcp-Session-Id")

	// Handle OPTIONS requests for CORS
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case "POST":
		sht.handlePostRequest(w, r, adapter, sessionID)
	case "GET":
		sht.handleGetRequest(w, r, adapter, sessionID)
	default:
		sht.writeErrorResponse(w, nil, -32601, fmt.Sprintf("Method not allowed: %s", r.Method))
	}
}

// handlePostRequest handles POST requests (client sending messages)
func (sht *StreamableHTTPTransport) handlePostRequest(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource, sessionID string) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sht.writeErrorResponse(w, nil, -32700, fmt.Sprintf("Failed to read request body: %v", err))
		return
	}

	// Validate MCP message
	if err := ValidateMCPMessage(body); err != nil {
		sht.writeErrorResponse(w, nil, -32600, fmt.Sprintf("Invalid MCP message: %v", err))
		return
	}

	// Parse JSON-RPC message
	var message JSONRPCMessage
	if err := json.Unmarshal(body, &message); err != nil {
		sht.writeErrorResponse(w, nil, -32700, fmt.Sprintf("Failed to parse JSON-RPC message: %v", err))
		return
	}

	// Handle the message
	response, err := sht.messageRouter.RouteMessage(r.Context(), &message, adapter, sessionID)
	if err != nil {
		sht.writeErrorResponse(w, message.ID, -32603, fmt.Sprintf("Failed to route message: %v", err))
		return
	}

	// Check if this is a request that should open an SSE stream
	if sht.shouldOpenSSEStream(&message) {
		sht.handleSSEStream(w, r, adapter, sessionID, &message)
		return
	}

	// For regular requests, return JSON response
	sht.writeJSONResponse(w, response)
}

// handleGetRequest handles GET requests (client opening SSE stream)
func (sht *StreamableHTTPTransport) handleGetRequest(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource, sessionID string) {
	// Check if client wants SSE stream
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		sht.writeErrorResponse(w, nil, -32600, "GET requests must accept text/event-stream")
		return
	}

	// Handle SSE stream
	sht.handleSSEStream(w, r, adapter, sessionID, nil)
}

// shouldOpenSSEStream determines if a request should open an SSE stream
func (sht *StreamableHTTPTransport) shouldOpenSSEStream(message *JSONRPCMessage) bool {
	// For now, only initialize requests open streams
	// In a full implementation, this could be more sophisticated
	return message.Method == "initialize"
}

// handleSSEStream handles Server-Sent Events streaming
func (sht *StreamableHTTPTransport) handleSSEStream(w http.ResponseWriter, r *http.Request, adapter models.AdapterResource, sessionID string, initMessage *JSONRPCMessage) {
	log.Printf("StreamableHTTP: Opening SSE stream for adapter %s", adapter.Name)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Handle Last-Event-ID for resumable streams
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		log.Printf("StreamableHTTP: Client requested to resume from event ID: %s", lastEventID)
		// In a full implementation, this would resume from the specified event
	}

	// Create or get session
	if sessionID == "" {
		sessionID = sht.generateSessionID()
		log.Printf("StreamableHTTP: Generated new session ID: %s", sessionID)
	}

	// Store session
	sht.sessionStore.SetWithDetails(sessionID, adapter.Name, "", "SSE")

	// Set MCP session ID for tracking
	sht.sessionStore.SetMCPSessionID(sessionID, sessionID)

	// Set session ID in response header
	w.Header().Set("Mcp-Session-Id", sessionID)

	// If this is an initialize request, handle it first
	if initMessage != nil {
		response, err := sht.protocolHandler.HandleMessage(r.Context(), []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"method":"initialize","params":{}}`, initMessage.ID)), adapter, sessionID)
		if err != nil {
			sht.writeSSEError(w, initMessage.ID, -32603, fmt.Sprintf("Initialize failed: %v", err))
			return
		}

		// Send initialize response
		if err := sht.streamSSEMessage(w, response); err != nil {
			log.Printf("StreamableHTTP: Failed to stream initialize response: %v", err)
			return
		}
	}

	// Establish connection to target server for streaming
	targetURL, err := sht.buildTargetURL(adapter)
	if err != nil {
		sht.writeSSEError(w, nil, -32603, fmt.Sprintf("Failed to build target URL: %v", err))
		return
	}

	// Create streaming request to target
	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		sht.writeSSEError(w, nil, -32603, fmt.Sprintf("Failed to create streaming request: %v", err))
		return
	}

	// Set headers for target request
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	// Apply authentication
	if err := sht.applyAuthentication(req, adapter); err != nil {
		sht.writeSSEError(w, nil, -32603, fmt.Sprintf("Failed to apply authentication: %v", err))
		return
	}

	// Send request to target
	resp, err := sht.httpClient.Do(req)
	if err != nil {
		sht.writeSSEError(w, nil, -32603, fmt.Sprintf("Failed to connect to target: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		sht.writeSSEError(w, nil, -32603, fmt.Sprintf("Target server returned status %d", resp.StatusCode))
		return
	}

	// Stream SSE from target to client
	sht.proxySSEStream(w, resp.Body, sessionID)
}

// proxySSEStream proxies SSE stream from target to client
func (sht *StreamableHTTPTransport) proxySSEStream(w http.ResponseWriter, targetBody io.Reader, sessionID string) {
	scanner := bufio.NewScanner(targetBody)
	eventID := 1

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			// Parse and potentially modify SSE data
			data := strings.TrimPrefix(line, "data: ")

			// Add event ID for resumability
			if err := WriteSSEEvent(w, "", fmt.Sprintf(`{"id":"%d","data":%s}`, eventID, data)); err != nil {
				log.Printf("StreamableHTTP: Failed to write SSE event: %v", err)
				break
			}

			eventID++
		} else if line == "" {
			// Empty line indicates end of event
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				log.Printf("StreamableHTTP: Failed to write SSE separator: %v", err)
				break
			}
		} else {
			// Pass through other SSE lines (event:, retry:, etc.)
			if _, err := fmt.Fprintln(w, line); err != nil {
				log.Printf("StreamableHTTP: Failed to write SSE line: %v", err)
				break
			}
		}

		// Flush to ensure immediate delivery
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("StreamableHTTP: SSE stream error: %v", err)
	}
}

// extractSessionID extracts session ID from request
func (sht *StreamableHTTPTransport) extractSessionID(r *http.Request) string {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		sessionID = r.Header.Get("mcp-session-id")
	}
	if sessionID == "" {
		sessionID = r.URL.Query().Get("sessionId")
	}
	return sessionID
}

// generateSessionID generates a new session ID
func (sht *StreamableHTTPTransport) generateSessionID() string {
	return fmt.Sprintf("mcp-sess-%d", time.Now().UnixNano())
}

// buildTargetURL builds target URL for the adapter
func (sht *StreamableHTTPTransport) buildTargetURL(adapter models.AdapterResource) (string, error) {
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
		return fmt.Sprintf("http://%s-service.adapter.svc.cluster.local:8000/mcp", adapter.Name), nil
	default:
		return "", fmt.Errorf("unsupported connection type: %s", adapter.ConnectionType)
	}
}

// applyAuthentication applies authentication to the request
func (sht *StreamableHTTPTransport) applyAuthentication(req *http.Request, adapter models.AdapterResource) error {
	if adapter.Authentication == nil || !adapter.Authentication.Required {
		return nil // No authentication required
	}

	switch adapter.Authentication.Type {
	case "bearer":
		return sht.applyBearerAuth(req, adapter)
	case "oauth":
		return sht.applyOAuthAuth(req, adapter)
	case "basic":
		return sht.applyBasicAuth(req, adapter)
	case "apikey":
		return sht.applyAPIKeyAuth(req, adapter)
	default:
		return fmt.Errorf("unsupported authentication type: %s", adapter.Authentication.Type)
	}
}

// applyBearerAuth applies bearer authentication
func (sht *StreamableHTTPTransport) applyBearerAuth(req *http.Request, adapter models.AdapterResource) error {
	token := adapter.Authentication.Token // Legacy field

	// Check new bearer token configuration
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
func (sht *StreamableHTTPTransport) applyOAuthAuth(req *http.Request, adapter models.AdapterResource) error {
	// For now, this is a placeholder
	// In a full implementation, this would handle OAuth token management
	return fmt.Errorf("OAuth authentication not yet implemented in streamable HTTP transport")
}

// applyBasicAuth applies basic authentication
func (sht *StreamableHTTPTransport) applyBasicAuth(req *http.Request, adapter models.AdapterResource) error {
	if adapter.Authentication.Basic == nil {
		return fmt.Errorf("basic authentication configuration not found")
	}

	req.SetBasicAuth(adapter.Authentication.Basic.Username, adapter.Authentication.Basic.Password)
	return nil
}

// applyAPIKeyAuth applies API key authentication
func (sht *StreamableHTTPTransport) applyAPIKeyAuth(req *http.Request, adapter models.AdapterResource) error {
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

// writeJSONResponse writes a JSON response
func (sht *StreamableHTTPTransport) writeJSONResponse(w http.ResponseWriter, message *JSONRPCMessage) {
	w.Header().Set("Content-Type", "application/json")

	if message.Error != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if err := json.NewEncoder(w).Encode(message); err != nil {
		log.Printf("StreamableHTTP: Failed to encode JSON response: %v", err)
	}
}

// writeErrorResponse writes a JSON-RPC error response
func (sht *StreamableHTTPTransport) writeErrorResponse(w http.ResponseWriter, id interface{}, code int, message string) {
	response := CreateErrorResponse(id, code, message, nil)
	sht.writeJSONResponse(w, response)
}

// writeSSEError writes an error via SSE
func (sht *StreamableHTTPTransport) writeSSEError(w http.ResponseWriter, id interface{}, code int, message string) {
	errorResponse := CreateErrorResponse(id, code, message, nil)

	if err := sht.streamSSEMessage(w, errorResponse); err != nil {
		log.Printf("StreamableHTTP: Failed to write SSE error: %v", err)
	}
}

// streamSSEMessage streams a JSON-RPC message via SSE
func (sht *StreamableHTTPTransport) streamSSEMessage(w http.ResponseWriter, message *JSONRPCMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return WriteSSEEvent(w, "message", string(data))
}
