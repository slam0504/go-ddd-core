package eventbus

import (
	"context"
	"errors"
	"sync"

	"github.com/slam0504/go-ddd-core/domain"
)

// Router maps decoded event names to typed handlers and runs them against a
// Subscriber. Unlike watermill's own Router, this one operates on already
// decoded DomainEvents so application code is free of transport types.
type Router struct {
	sub         Subscriber
	middlewares []Middleware

	mu        sync.RWMutex
	handlers  map[string][]DispatchFunc
	onUnknown func(ctx context.Context, env Envelope)
}

// NewRouter constructs a Router that pulls from sub. Middlewares wrap every
// registered handler.
func NewRouter(sub Subscriber, mws ...Middleware) *Router {
	return &Router{
		sub:         sub,
		middlewares: mws,
		handlers:    make(map[string][]DispatchFunc),
	}
}

// OnUnknown installs a callback invoked when an envelope's event name has no
// registered handler. The default behaviour is to silently ack such messages
// (a consumer may deliberately share a topic with unrelated events); set a
// callback to log, count, or route them elsewhere. The callback runs before
// the envelope is acked; returning does not block the router loop.
func (r *Router) OnUnknown(fn func(ctx context.Context, env Envelope)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onUnknown = fn
}

// RegisterHandler registers a type-erased dispatcher for an event name.
// Multiple handlers per name are supported and invoked sequentially.
func (r *Router) RegisterHandler(name string, fn DispatchFunc) {
	wrapped := Chain(r.middlewares...)(fn)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = append(r.handlers[name], wrapped)
}

// Register binds a typed EventHandler[E] to the router.
func Register[E domain.DomainEvent](r *Router, name string, h EventHandler[E]) {
	r.RegisterHandler(name, func(ctx context.Context, e domain.DomainEvent) error {
		typed, ok := e.(E)
		if !ok {
			return ErrEventMismatch
		}
		return h.Handle(ctx, typed)
	})
}

// Run subscribes to topic and dispatches envelopes until ctx is cancelled or
// the subscriber closes. On handler error the envelope is nacked; on success
// (or when no handler is registered) it is acked.
func (r *Router) Run(ctx context.Context, topic string) error {
	ch, err := r.sub.Subscribe(ctx, topic)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case env, ok := <-ch:
			if !ok {
				return nil
			}
			if err := r.dispatch(ctx, env); err != nil {
				env.Nack()
				continue
			}
			env.Ack()
		}
	}
}

func (r *Router) dispatch(ctx context.Context, env Envelope) error {
	r.mu.RLock()
	fns := r.handlers[env.Name]
	onUnknown := r.onUnknown
	r.mu.RUnlock()
	if len(fns) == 0 {
		if onUnknown != nil {
			onUnknown(ctx, env)
		}
		return nil
	}
	for _, fn := range fns {
		if err := fn(ctx, env.Event); err != nil {
			return err
		}
	}
	return nil
}

// ErrEventMismatch indicates the decoded event is not of the expected type.
var ErrEventMismatch = errors.New("eventbus: event type mismatch")
