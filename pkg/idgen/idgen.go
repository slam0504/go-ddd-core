// Package idgen defines the ID generation contract used for domain entity
// IDs, event IDs, and trace correlation. Core ships a UUIDv7 default based
// on google/uuid; downstream services can swap in Snowflake, ULID, etc.
package idgen

import (
	"github.com/google/uuid"
)

// Generator produces opaque string IDs. Implementations must be safe for
// concurrent use.
type Generator interface {
	New() string
}

// GeneratorFunc adapts a function to the Generator interface.
type GeneratorFunc func() string

func (f GeneratorFunc) New() string { return f() }

// UUIDv4 returns a Generator producing random UUIDs.
func UUIDv4() Generator {
	return GeneratorFunc(uuid.NewString)
}

// UUIDv7 returns a Generator producing time-ordered UUIDv7 values. Falls
// back to UUIDv4 if the runtime cannot supply v7 (should not happen on
// modern google/uuid).
func UUIDv7() Generator {
	return GeneratorFunc(func() string {
		if id, err := uuid.NewV7(); err == nil {
			return id.String()
		}
		return uuid.NewString()
	})
}
