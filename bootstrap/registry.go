// Package bootstrap assembles adapters into a running application. The
// package defines the Module lifecycle, an AdapterRegistry for driver
// selection, and an App container tying configuration, modules, and
// graceful shutdown together.
package bootstrap

import (
	"context"
	"fmt"
	"sync"
)

// AdapterFactory builds a concrete adapter from raw configuration. The cfg
// argument is the slice of configuration that applies to this adapter —
// bootstrap extracts it from the Provider based on the adapter kind.
type AdapterFactory func(ctx context.Context, cfg any) (any, error)

// AdapterRegistry maps (kind, driver) → factory. Kind is a stable name for
// the abstract capability (e.g. "messaging", "cache"); driver is the
// concrete implementation (e.g. "kafka", "redis").
type AdapterRegistry struct {
	mu        sync.RWMutex
	factories map[string]map[string]AdapterFactory
}

// NewAdapterRegistry constructs an empty registry.
func NewAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{factories: map[string]map[string]AdapterFactory{}}
}

// Register binds a factory to (kind, driver). Re-registering the same pair
// overwrites the existing factory.
func (r *AdapterRegistry) Register(kind, driver string, f AdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.factories[kind]; !ok {
		r.factories[kind] = map[string]AdapterFactory{}
	}
	r.factories[kind][driver] = f
}

// Resolve looks up a factory. Returns ErrAdapterNotFound if (kind, driver)
// is unregistered.
func (r *AdapterRegistry) Resolve(kind, driver string) (AdapterFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	drivers, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("%w: kind=%q", ErrAdapterNotFound, kind)
	}
	f, ok := drivers[driver]
	if !ok {
		return nil, fmt.Errorf("%w: kind=%q driver=%q", ErrAdapterNotFound, kind, driver)
	}
	return f, nil
}

// Build looks up and invokes the factory in one step.
func (r *AdapterRegistry) Build(ctx context.Context, kind, driver string, cfg any) (any, error) {
	f, err := r.Resolve(kind, driver)
	if err != nil {
		return nil, err
	}
	return f(ctx, cfg)
}

// ErrAdapterNotFound is returned when the registry has no factory for a
// given (kind, driver) pair.
var ErrAdapterNotFound = fmt.Errorf("bootstrap: adapter not found")
