package middleware

import (
	"log"
	"suse-ai-up/pkg/proxy"
	"time"
)

// TimingMiddleware measures request execution time
type TimingMiddleware struct {
	logger *log.Logger
}

// NewTimingMiddleware creates a new timing middleware
func NewTimingMiddleware() *TimingMiddleware {
	return &TimingMiddleware{
		logger: log.Default(),
	}
}

// OnRequest measures request timing
func (m *TimingMiddleware) OnRequest(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	start := time.Now()

	result, err := next(ctx)

	duration := time.Since(start)
	m.logger.Printf("[TIMING] %s completed in %v", ctx.Method, duration)

	return result, err
}

// OnMessage handles all messages
func (m *TimingMiddleware) OnMessage(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

// Other hooks delegate to OnRequest for simplicity
func (m *TimingMiddleware) OnNotification(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnCallTool(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnReadResource(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnGetPrompt(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnListTools(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnListResources(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnListPrompts(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}

func (m *TimingMiddleware) OnInitialize(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error) {
	return m.OnRequest(ctx, next)
}
