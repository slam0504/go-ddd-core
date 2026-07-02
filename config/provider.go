// Package config defines the configuration loading contract (Provider) and
// ships a viper-backed ViperProvider as an in-process default.
//
// A concrete provider living in core is a DELIBERATE exception, not
// architecture drift: core is infrastructure-client-free rather than
// literally interface-only. Configuration loads during bootstrap, before any
// adapter is wired, and the provider performs no infrastructure-client
// behaviour — it opens no network-broker, database, or HTTP-framework
// connection. Drivers that talk to infrastructure belong in go-ddd-adapters;
// in-process bootstrap defaults (this provider, the CQRS in-memory buses) may
// live in core. Rationale: docs/anti-patterns.md "Design boundaries" and the
// repository's .agent/decisions.md.
package config

import (
	"context"
)

// Provider loads configuration and unmarshals it into a destination struct.
// Implementations may watch the underlying source and emit change events.
type Provider interface {
	// Load unmarshals the current config snapshot into dest.
	Load(ctx context.Context, dest any) error

	// Get returns the raw value at the dotted path (e.g. "messaging.driver").
	Get(path string) any

	// OnChange registers a callback invoked after any reload.
	OnChange(fn func())
}

// ChangeListener is a convenience typed callback for config hot reload.
type ChangeListener func()
