package query

import (
	"context"
	"sync"
)

// InMemoryBus is a zero-dependency Bus for in-process query dispatching.
type InMemoryBus struct {
	mu          sync.RWMutex
	handlers    map[string]DispatchFunc
	middlewares []Middleware
}

func NewInMemoryBus(mws ...Middleware) *InMemoryBus {
	return &InMemoryBus{
		handlers:    make(map[string]DispatchFunc),
		middlewares: mws,
	}
}

func (b *InMemoryBus) RegisterHandler(name string, fn DispatchFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = Apply(fn, b.middlewares...)
}

func (b *InMemoryBus) Dispatch(ctx context.Context, q Query) (any, error) {
	b.mu.RLock()
	fn, ok := b.handlers[q.QueryName()]
	b.mu.RUnlock()
	if !ok {
		return nil, ErrHandlerNotFound
	}
	return fn(ctx, q)
}
