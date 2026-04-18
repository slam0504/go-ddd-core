// Package query defines the read side of CQRS: Query, Handler, and Bus.
// Queries must be side-effect free.
package query

import "context"

// Query is the input to a read-side use case.
type Query interface {
	QueryName() string
}

// Handler executes a Query and returns a typed result.
type Handler[Q Query, R any] interface {
	Handle(ctx context.Context, q Q) (R, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc[Q Query, R any] func(ctx context.Context, q Q) (R, error)

func (f HandlerFunc[Q, R]) Handle(ctx context.Context, q Q) (R, error) {
	return f(ctx, q)
}

// DispatchFunc is the type-erased form of a query handler.
type DispatchFunc func(ctx context.Context, q Query) (any, error)

// Bus dispatches queries to their registered handler.
type Bus interface {
	Dispatch(ctx context.Context, q Query) (any, error)
	RegisterHandler(name string, fn DispatchFunc)
}

// Register binds a typed Handler to the bus.
func Register[Q Query, R any](bus Bus, h Handler[Q, R]) {
	var zero Q
	bus.RegisterHandler(zero.QueryName(), func(ctx context.Context, q Query) (any, error) {
		typed, ok := q.(Q)
		if !ok {
			return nil, ErrQueryMismatch
		}
		return h.Handle(ctx, typed)
	})
}
