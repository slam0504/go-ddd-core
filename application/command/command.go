// Package command defines the write side of CQRS: Command, Handler, and Bus.
// The package contains no infrastructure — transports, persistence, and
// messaging live in adapters selected by downstream services.
package command

import "context"

// Command is the input to a write-side use case.
type Command interface {
	CommandName() string
}

// Handler executes a Command and returns a typed result.
type Handler[C Command, R any] interface {
	Handle(ctx context.Context, cmd C) (R, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc[C Command, R any] func(ctx context.Context, cmd C) (R, error)

func (f HandlerFunc[C, R]) Handle(ctx context.Context, cmd C) (R, error) {
	return f(ctx, cmd)
}

// DispatchFunc is the type-erased form of a handler. Bus implementations
// store dispatchers of this type keyed by command name.
type DispatchFunc func(ctx context.Context, cmd Command) (any, error)

// Bus dispatches commands to their registered handler.
type Bus interface {
	Dispatch(ctx context.Context, cmd Command) (any, error)
	RegisterHandler(name string, fn DispatchFunc)
}

// Register binds a typed Handler to the bus. It is a free function because
// interface methods cannot introduce their own type parameters.
func Register[C Command, R any](bus Bus, h Handler[C, R]) {
	var zero C
	bus.RegisterHandler(zero.CommandName(), func(ctx context.Context, cmd Command) (any, error) {
		typed, ok := cmd.(C)
		if !ok {
			return nil, ErrCommandMismatch
		}
		return h.Handle(ctx, typed)
	})
}
