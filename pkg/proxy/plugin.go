package proxy

import (
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"

	"github.com/gin-gonic/gin"
)

// AdapterProxyPlugin defines the interface for adapter proxy plugins
type AdapterProxyPlugin interface {
	CanHandle(connectionType models.ConnectionType) bool
	ProxyRequest(c *gin.Context, adapter models.AdapterResource, sessionStore session.SessionStore) error
	GetStatus(adapter models.AdapterResource) (models.AdapterStatus, error)
	GetLogs(adapter models.AdapterResource) (string, error)
}
