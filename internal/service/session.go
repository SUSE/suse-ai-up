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

// Response types
type SessionReinitializeRequest struct {
	ForceReinitialize bool                   `json:"forceReinitialize,omitempty"`
	ClientInfo        map[string]interface{} `json:"clientInfo,omitempty"`
}

type SessionReinitializeResponse struct {
	SessionID   string `json:"sessionId"`
	Message     string `json:"message"`
	AdapterName string `json:"adapterName"`
}
