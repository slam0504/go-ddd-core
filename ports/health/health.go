// Package health defines the liveness/readiness probe contract used by
// inbound transport adapters to aggregate per-dependency status into
// `/healthz` and `/readyz` style endpoints. The core ships only the
// probe shape; registries, HTTP handlers, and aggregation policy live
// in the transport adapter that exposes them.
//
// A check returns nil to signal "healthy" and a non-nil error to signal
// "unhealthy". The error message becomes the human-readable reason in
// the adapter's diagnostic output. Implementations must respect ctx
// cancellation so an unresponsive dependency does not stall the probe.
package health

import "context"

// Check is a named health probe. Adapters that own a dependency
// (a database pool, a cache client, a downstream HTTP service)
// expose one Check per dependency so the transport adapter can
// aggregate them.
//
// Implementations must be safe for concurrent use; the transport
// adapter is expected to call Check from a probe handler that may
// be invoked many times per second.
type Check interface {
	// Name returns a stable identifier for this probe. The value is
	// surfaced to operators (e.g. as a JSON field in the probe
	// response), so it should be short, lowercase, and dependency-
	// oriented (e.g. "postgres", "redis-session", "billing-api").
	Name() string
	// Check runs the probe. It returns nil when the dependency is
	// healthy and a non-nil error describing the failure otherwise.
	// Implementations must honour ctx cancellation.
	Check(ctx context.Context) error
}

// NewCheck adapts a plain function into a Check by attaching a name.
// It is the convenience constructor for the common case where an
// adapter just wants to wrap an existing probe method (for example a
// driver's Ping) without defining a new type.
//
//	healthCheck := health.NewCheck("postgres", pool.Ping)
//
// The returned Check stores the function by value; callers that need
// to swap the probe at runtime should define their own struct type
// implementing Check directly.
func NewCheck(name string, fn func(ctx context.Context) error) Check {
	return checkFunc{name: name, fn: fn}
}

type checkFunc struct {
	name string
	fn   func(ctx context.Context) error
}

func (c checkFunc) Name() string                    { return c.name }
func (c checkFunc) Check(ctx context.Context) error { return c.fn(ctx) }
