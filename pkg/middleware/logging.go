package middleware

import (
	"log"
	"suse-ai-up/pkg/proxy"
)

// LoggingMiddleware provides request/response logging
type LoggingMiddleware struct {
	logger *log.Logger
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware() *LoggingMiddleware {
	return &LoggingMiddleware{
		logger: log.Default(),
	}
}

// OnMessage logs all MCP messages
func (m *LoggingMiddleware) OnMessage(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	m.logger.Printf("[LOG] Processing %s from %s", ctx.Method, ctx.Source)

	result, err := next(ctx)

	if err != nil {
		m.logger.Printf("[LOG] Error in %s: %v", ctx.Method, err)
	} else {
		m.logger.Printf("[LOG] Completed %s", ctx.Method)
	}

	return result, err
}

// Other hooks delegate to OnMessage for simplicity
func (m *LoggingMiddleware) OnRequest(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnNotification(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnCallTool(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnReadResource(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnGetPrompt(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnListTools(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnListResources(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnListPrompts(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}

func (m *LoggingMiddleware) OnInitialize(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnMessage(ctx, next)
}
