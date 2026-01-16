package middleware

import (
	"fmt"
	"suse-ai-up/pkg/proxy"
)

// Middleware defines the interface for MCP middleware
type Middleware interface {
	OnMessage(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnRequest(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnNotification(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnCallTool(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnReadResource(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnGetPrompt(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnListTools(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnListResources(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnListPrompts(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
	OnInitialize(ctx *proxy.MiddlewareContext, next proxy.MiddlewareHandler) (interface{}, error)
}

// MiddlewarePipeline manages the execution of middleware chain
type MiddlewarePipeline struct {
	middlewares  []Middleware
	finalHandler proxy.MiddlewareHandler
}

// NewMiddlewarePipeline creates a new middleware pipeline
func NewMiddlewarePipeline(middlewares []Middleware) *MiddlewarePipeline {
	return &MiddlewarePipeline{
		middlewares: middlewares,
	}
}

// SetFinalHandler sets the final handler that processes the actual request
func (p *MiddlewarePipeline) SetFinalHandler(handler proxy.MiddlewareHandler) {
	p.finalHandler = handler
}

// Execute runs the middleware pipeline
func (p *MiddlewarePipeline) Execute(ctx *proxy.MiddlewareContext) (interface{}, error) {
	// Build the middleware chain
	handler := p.buildChain(ctx)
	return handler(ctx)
}

// buildChain creates the middleware execution chain
func (p *MiddlewarePipeline) buildChain(ctx *proxy.MiddlewareContext) proxy.MiddlewareHandler {
	// Start with the final handler
	finalHandler := p.finalHandler
	if finalHandler == nil {
		// Default handler if none provided
		finalHandler = func(ctx *proxy.MiddlewareContext) (interface{}, error) {
			return nil, fmt.Errorf("no final handler configured")
		}
	}

	// Wrap with middleware in reverse order
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		middleware := p.middlewares[i]
		next := finalHandler
		finalHandler = p.createMiddlewareHandler(middleware, next)
	}

	return finalHandler
}

// createMiddlewareHandler creates a handler for a specific middleware
func (p *MiddlewarePipeline) createMiddlewareHandler(mw Middleware, next proxy.MiddlewareHandler) proxy.MiddlewareHandler {
	return func(ctx *proxy.MiddlewareContext) (interface{}, error) {
		// Call the appropriate hook based on message type and method
		switch ctx.Type {
		case "request":
			switch ctx.Message.Method {
			case "initialize":
				return mw.OnInitialize(ctx, next)
			case "tools/call":
				return mw.OnCallTool(ctx, next)
			case "tools/list":
				return mw.OnListTools(ctx, next)
			case "resources/read":
				return mw.OnReadResource(ctx, next)
			case "resources/list":
				return mw.OnListResources(ctx, next)
			case "prompts/get":
				return mw.OnGetPrompt(ctx, next)
			case "prompts/list":
				return mw.OnListPrompts(ctx, next)
			default:
				return mw.OnRequest(ctx, next)
			}
		case "notification":
			return mw.OnNotification(ctx, next)
		default:
			return mw.OnMessage(ctx, next)
		}
	}
}
