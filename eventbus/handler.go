package eventbus

import (
	"context"

	"github.com/slam0504/go-ddd-core/domain"
)

// EventHandler consumes a typed DomainEvent. Returning a nil error signals
// ack; returning a non-nil error tells the subscriber to nack so the broker
// may redeliver according to its retry policy.
type EventHandler[E domain.DomainEvent] interface {
	Handle(ctx context.Context, event E) error
}

// HandlerFunc adapts a function to the EventHandler interface.
type HandlerFunc[E domain.DomainEvent] func(ctx context.Context, event E) error

func (f HandlerFunc[E]) Handle(ctx context.Context, event E) error {
	return f(ctx, event)
}

// DispatchFunc is the type-erased form of an EventHandler used by Router.
type DispatchFunc func(ctx context.Context, event domain.DomainEvent) error

// Middleware wraps a DispatchFunc with cross-cutting concerns (logging,
// tracing, retry, dedup, etc.).
type Middleware func(next DispatchFunc) DispatchFunc

// Chain composes middlewares so the first one is outermost.
func Chain(mws ...Middleware) Middleware {
	return func(next DispatchFunc) DispatchFunc {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}
