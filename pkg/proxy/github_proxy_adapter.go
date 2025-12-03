package proxy

import (
	"github.com/gin-gonic/gin"
	"suse-ai-up/pkg/mcp"
	"suse-ai-up/pkg/models"
	"suse-ai-up/pkg/session"
)

// GitHubRemoteHTTPProxyAdapter extends RemoteHTTPProxyAdapter for GitHub-specific MCP handling
type GitHubRemoteHTTPProxyAdapter struct {
	*RemoteHTTPProxyAdapter
	githubToken string
}

// NewGitHubRemoteHTTPProxyAdapter creates a new GitHub-specific remote HTTP proxy adapter
func NewGitHubRemoteHTTPProxyAdapter(sessionStore session.SessionStore, messageRouter *mcp.MessageRouter, protocolHandler *mcp.ProtocolHandler, capabilityCache *mcp.CapabilityCache, adapter models.AdapterResource) *GitHubRemoteHTTPProxyAdapter {
	baseAdapter := NewRemoteHTTPProxyAdapter(sessionStore, messageRouter, protocolHandler, capabilityCache)
	githubToken := ""

	// Extract GitHub token from environment variables
	if adapter.EnvironmentVariables != nil {
		if token, exists := adapter.EnvironmentVariables["GITHUB_PAT"]; exists {
			githubToken = token
		}
	}

	return &GitHubRemoteHTTPProxyAdapter{
		RemoteHTTPProxyAdapter: baseAdapter,
		githubToken:            githubToken,
	}
}

// HandleRequest handles MCP requests with GitHub-specific authentication
func (a *GitHubRemoteHTTPProxyAdapter) HandleRequest(c *gin.Context, adapter models.AdapterResource) error {
	// Get or create session
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		sessionID = c.GetHeader("Mcp-Session-Id")
	}

	session := a.getOrCreateSession(sessionID, adapter)

	// Apply GitHub-specific authentication to the session
	if a.githubToken != "" {
		// Override the remote URL for GitHub
		session.RemoteURL = "https://api.githubcopilot.com/mcp/"
	}

	// Call the base implementation
	return a.RemoteHTTPProxyAdapter.HandleRequest(c, adapter)
}
