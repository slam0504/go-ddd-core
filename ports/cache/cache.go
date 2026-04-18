// Package cache defines the key-value caching contract. Implementations wrap
// Redis, Memcached, in-process LRU, etc.
package cache

import (
	"context"
	"errors"
	"time"
)

// Cache is a byte-level key-value cache with optional TTL per entry.
// Implementations should treat a zero TTL as "no expiry".
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// TypedCache[T] is a convenience generic layer on top of Cache that handles
// marshalling. Codec is provided by the caller so core stays unopinionated.
type TypedCache[T any] interface {
	Get(ctx context.Context, key string) (T, error)
	Set(ctx context.Context, key string, value T, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// ErrMiss is returned when a key is absent. Adapters must translate their
// native miss indicator into this sentinel.
var ErrMiss = errors.New("cache: miss")
