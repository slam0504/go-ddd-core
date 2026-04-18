package command

import (
	"context"
	"sync"
)

// InMemoryBus is a zero-dependency Bus implementation suitable for in-process
// dispatching, testing, and examples. It supports attaching middlewares that
// wrap every handler uniformly.
type InMemoryBus struct {
	mu          sync.RWMutex
	handlers    map[string]DispatchFunc
	middlewares []Middleware
}

// NewInMemoryBus constructs an InMemoryBus with optional global middlewares.
func NewInMemoryBus(mws ...Middleware) *InMemoryBus {
	return &InMemoryBus{
		handlers:    make(map[string]DispatchFunc),
		middlewares: mws,
	}
}

// RegisterHandler stores the dispatcher under the given command name.
func (b *InMemoryBus) RegisterHandler(name string, fn DispatchFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = Apply(fn, b.middlewares...)
}

// Dispatch looks up the handler for cmd and invokes it.
func (b *InMemoryBus) Dispatch(ctx context.Context, cmd Command) (any, error) {
	b.mu.RLock()
	fn, ok := b.handlers[cmd.CommandName()]
	b.mu.RUnlock()
	if !ok {
		return nil, ErrHandlerNotFound
	}
	return fn(ctx, cmd)
}
