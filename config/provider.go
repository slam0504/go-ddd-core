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
