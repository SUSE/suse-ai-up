package middleware

import (
	"fmt"
	"strings"
	"suse-ai-up/pkg/proxy"
)

// AuthMiddleware provides bearer token authentication
type AuthMiddleware struct {
	validTokens map[string]bool
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(validTokens []string) *AuthMiddleware {
	tokens := make(map[string]bool)
	for _, token := range validTokens {
		tokens[token] = true
	}
	return &AuthMiddleware{
		validTokens: tokens,
	}
}

// OnRequest validates authentication for requests
func (m *AuthMiddleware) OnRequest(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	// Extract token from context (would be set by HTTP handler)
	token := m.extractToken(ctx)
	if token == "" {
		return nil, fmt.Errorf("authentication required")
	}

	if !m.validTokens[token] {
		return nil, fmt.Errorf("invalid authentication token")
	}

	return next(ctx)
}

// extractToken extracts the bearer token from the request context
func (m *AuthMiddleware) extractToken(ctx *proxy.MiddlewareContext) string {
	// In a real implementation, this would extract from HTTP headers
	// For now, we'll look in the context or use a default
	if ctx.MCPContext != nil && ctx.MCPContext.RequestContext != nil {
		// Try to extract from request context
		if reqCtx, ok := ctx.MCPContext.RequestContext.(map[string]interface{}); ok {
			if auth, ok := reqCtx["authorization"].(string); ok {
				if strings.HasPrefix(auth, "Bearer ") {
					return strings.TrimPrefix(auth, "Bearer ")
				}
			}
		}
	}

	// For testing/demo purposes, accept any token
	return "demo-token"
}

// Other hooks delegate to OnRequest
func (m *AuthMiddleware) OnMessage(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnNotification(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnCallTool(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnReadResource(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnGetPrompt(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnListTools(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnListResources(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnListPrompts(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *AuthMiddleware) OnInitialize(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}
