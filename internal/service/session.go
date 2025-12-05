package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/clients"
	"suse-ai-up/pkg/session"
)

// SessionManagementService handles session management operations
type SessionManagementService struct {
	sessionStore session.SessionStore
	store        clients.AdapterResourceStore
}

// NewSessionManagementService creates a new session management service
func NewSessionManagementService(sessionStore session.SessionStore, store clients.AdapterResourceStore) *SessionManagementService {
	return &SessionManagementService{
		sessionStore: sessionStore,
		store:        store,
	}
}

// ListSessions handles GET /adapters/{name}/sessions
// @Summary List all sessions for an adapter
// @Description Retrieve all active sessions for a specific MCP server adapter
// @Tags sessions
// @Produce json
// @Param name path string true "Adapter name"
// @Success 200 {object} SessionListResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/sessions [get]
func (sms *SessionManagementService) ListAdapterSessions(c *gin.Context) {
	name := c.Param("name")

	// Verify adapter exists
	ctx := context.Background()
	adapter, err := sms.store.Get(ctx, name)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	sessions, err := sms.sessionStore.ListByAdapter(name)
	if err != nil {
		log.Printf("SessionManagementService: Failed to list sessions for adapter %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve sessions"})
		return
	}

	response := SessionListResponse{
		AdapterName: name,
		Sessions:    sessions,
	}
	c.JSON(http.StatusOK, response)
}

// GetSession handles GET /adapters/{name}/sessions/{sessionId}
// @Summary Get session details
// @Description Retrieve detailed information about a specific session
// @Tags sessions
// @Produce json
// @Param name path string true "Adapter name"
// @Param sessionId path string true "Session ID"
// @Success 200 {object} session.SessionDetails
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/sessions/{sessionId} [get]
func (sms *SessionManagementService) GetAdapterSession(c *gin.Context) {
	name := c.Param("name")
	sessionID := c.Param("sessionId")

	// Verify adapter exists
	ctx := context.Background()
	adapter, err := sms.store.Get(ctx, name)
	if err != nil || adapter == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	details, err := sms.sessionStore.GetDetails(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	// Verify session belongs to the adapter
	if details.AdapterName != name {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found for this adapter"})
		return
	}

	c.JSON(http.StatusOK, details)
}

// DeleteSession handles DELETE /adapters/{name}/sessions/{sessionId}
// @Summary Delete a session
// @Description Invalidate and remove a specific session
// @Tags sessions
// @Produce json
// @Param name path string true "Adapter name"
// @Param sessionId path string true "Session ID"
// @Success 200 {object} SessionDeleteResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/sessions/{sessionId} [delete]
func (sms *SessionManagementService) DeleteSession(c *gin.Context) {
	name := c.Param("name")
	sessionID := c.Param("sessionId")

	// Verify adapter exists
	ctx := context.Background()
	_, err := sms.store.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// Verify session exists and belongs to adapter
	details, err := sms.sessionStore.GetDetails(sessionID)
	if err != nil || details.AdapterName != name {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	err = sms.sessionStore.Delete(sessionID)
	if err != nil {
		log.Printf("SessionManagementService: Failed to delete session %s: %v", sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete session"})
		return
	}

	response := SessionDeleteResponse{
		Message:   "Session deleted successfully",
		SessionID: sessionID,
	}
	c.JSON(http.StatusOK, response)
}

// ReinitializeSession handles POST /adapters/{name}/sessions
// @Summary Reinitialize a session
// @Description Create a new session by reinitializing the MCP connection
// @Tags sessions
// @Accept json
// @Produce json
// @Param name path string true "Adapter name"
// @Param request body SessionReinitializeRequest true "Reinitialization parameters"
// @Success 200 {object} SessionReinitializeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/sessions [post]
func (sms *SessionManagementService) ReinitializeSession(c *gin.Context) {
	name := c.Param("name")

	var req SessionReinitializeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify adapter exists
	ctx := context.Background()
	adapter, err := sms.store.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	// For now, return a placeholder response
	// In a full implementation, this would trigger MCP initialization
	sessionID := fmt.Sprintf("session-%d", time.Now().Unix())

	err = sms.sessionStore.SetWithDetails(sessionID, name, "reinitialized", string(adapter.ConnectionType))
	if err != nil {
		log.Printf("SessionManagementService: Failed to create session for adapter %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	response := SessionReinitializeResponse{
		SessionID:   sessionID,
		Message:     "Session reinitialized successfully",
		AdapterName: name,
	}
	c.JSON(http.StatusOK, response)
}

// DeleteAllSessions handles DELETE /adapters/{name}/sessions
// @Summary Delete all sessions for an adapter
// @Description Remove all active sessions for a specific adapter
// @Tags sessions
// @Produce json
// @Param name path string true "Adapter name"
// @Success 200 {object} BulkSessionDeleteResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/adapters/{name}/sessions [delete]
func (sms *SessionManagementService) DeleteAllSessions(c *gin.Context) {
	name := c.Param("name")

	// Verify adapter exists
	ctx := context.Background()
	_, err := sms.store.Get(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Adapter not found"})
		return
	}

	err = sms.sessionStore.DeleteByAdapter(name)
	if err != nil {
		log.Printf("SessionManagementService: Failed to delete sessions for adapter %s: %v", name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete sessions"})
		return
	}

	response := BulkSessionDeleteResponse{
		Message:     "All sessions deleted successfully",
		AdapterName: name,
	}
	c.JSON(http.StatusOK, response)
}

// Response types
type SessionListResponse struct {
	AdapterName string                   `json:"adapterName"`
	Sessions    []session.SessionDetails `json:"sessions"`
}

type SessionDeleteResponse struct {
	Message   string `json:"message"`
	SessionID string `json:"sessionId"`
}

type SessionReinitializeRequest struct {
	ForceReinitialize bool                   `json:"forceReinitialize,omitempty"`
	ClientInfo        map[string]interface{} `json:"clientInfo,omitempty"`
}

type SessionReinitializeResponse struct {
	SessionID   string `json:"sessionId"`
	Message     string `json:"message"`
	AdapterName string `json:"adapterName"`
}

type BulkSessionDeleteResponse struct {
	Message     string `json:"message"`
	AdapterName string `json:"adapterName"`
}
