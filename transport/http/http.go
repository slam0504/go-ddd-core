// Package http defines the inbound HTTP server contract. Adapters wrap
// stdlib net/http, chi, echo, gin, etc.
package http

import (
	"context"
	"net/http"
)

// Server runs an HTTP listener. Adapters are responsible for socket binding,
// TLS, and protocol details. Start must not block; Stop must drain.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Addr() string
}

// Router is a minimal routing contract. Adapters satisfying this interface
// can register handlers regardless of the underlying mux library.
type Router interface {
	Handle(method, pattern string, h http.Handler)
	Use(mws ...Middleware)
	Group(prefix string, mws ...Middleware) Router
}

// Middleware wraps an http.Handler to add cross-cutting behaviour.
type Middleware func(next http.Handler) http.Handler

// Chain composes middlewares so the first one is outermost.
func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// HandlerProvider is implemented by components that want to contribute
// routes to the Server — typically application or GraphQL modules.
type HandlerProvider interface {
	RegisterRoutes(r Router)
}
