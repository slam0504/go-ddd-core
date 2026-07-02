// Package cache defines the key-value caching contract. Implementations wrap
// Redis, Memcached, in-process LRU, etc.
//
// # Context convention
//
// Every method requires a non-nil ctx (stdlib convention). Passing nil is a
// caller bug with unspecified behaviour; implementations need not nil-check it.
//
// # Key and error semantics
//
// An empty key ("") is malformed input on every method: implementations MUST
// return an errorsx.CodeInvalidArgument error before contacting the backend. A
// miss (absent key on Get) is reported as the ErrMiss sentinel, never a coded
// error. Any OTHER non-nil error means the operation could not reach a
// decision, and adapters return a coded errorsx whose CodeOf is never
// CodeUnknown.
package cache

import (
	"context"
	"errors"
	"time"
)

// Cache is a byte-level key-value cache with optional TTL per entry.
//
// TTL semantics (Set): a zero ttl means "no expiry". A negative ttl is
// malformed input and MUST return errorsx.CodeInvalidArgument before any
// backend contact — it is almost always a caller bug, and forwarding it risks
// a driver-specific "keep existing TTL" sentinel (e.g. go-redis KeepTTL == -1).
//
// Error precedence:
//   - Set:                 empty key -> negative ttl -> pre-cancelled/expired ctx -> backend
//   - Get / Delete / Exists: empty key -> pre-cancelled/expired ctx -> backend
//
// The first rung(s) are deterministic input validation, evaluated before ctx is
// observed; then a ctx already cancelled/expired returns the matching ctx error
// with no backend contact; only then a backend failure.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// ErrMiss is returned when a key is absent. Adapters must translate their
// native miss indicator into this sentinel.
var ErrMiss = errors.New("cache: miss")
