package cachetest

import (
	"context"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/cache"
)

// refCache is a correct in-memory reference used only to author and prove the
// suite (dev-time red proof). It is NOT shipped as a production adapter. It
// copies on store and on return so it satisfies the byte-ownership invariants,
// and it ignores positive-TTL expiry (the deterministic suite never asserts
// expiry).
type refCache struct {
	m map[string][]byte
}

func newRefCache(t *testing.T) cache.Cache { return &refCache{m: map[string][]byte{}} }

func (c *refCache) validateKey(key string) error {
	if key == "" {
		return errorsx.New(errorsx.CodeInvalidArgument, "cachetest: empty key")
	}
	return nil
}

func (c *refCache) Get(ctx context.Context, key string) ([]byte, error) {
	if err := c.validateKey(key); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, ok := c.m[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	out := make([]byte, len(v)) // copy on return
	copy(out, v)
	return out, nil
}

func (c *refCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.validateKey(key); err != nil {
		return err
	}
	if ttl < 0 {
		return errorsx.New(errorsx.CodeInvalidArgument, "cachetest: negative ttl")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stored := make([]byte, len(value)) // copy on store
	copy(stored, value)
	c.m[key] = stored
	return nil
}

func (c *refCache) Delete(ctx context.Context, key string) error {
	if err := c.validateKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	delete(c.m, key)
	return nil
}

func (c *refCache) Exists(ctx context.Context, key string) (bool, error) {
	if err := c.validateKey(key); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, ok := c.m[key]
	return ok, nil
}

// TestReferenceImplementationSatisfiesContract proves the suite is runnable and
// GREEN against a correct implementation. Its purpose is to author the suite;
// non-vacuity is proven at dev time (see Step 4) and NOT committed as a
// red-on-purpose test.
func TestReferenceImplementationSatisfiesContract(t *testing.T) {
	RunContract(t, newRefCache)
}
