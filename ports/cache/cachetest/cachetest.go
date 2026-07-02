// Package cachetest is the exported conformance suite for ports/cache.Cache.
// Any adapter runs RunContract against its own implementation to prove it
// honours the byte-level cache contract. The suite is deterministic and
// synchronous-only: it asserts invariants visible through the interface and
// deliberately excludes time-dependent behaviour (TTL expiry), which an
// adapter covers with its own intent test. Error assertions use errorsx.CodeOf,
// never an HTTP status. Imports only testing + the contract + errorsx (never
// pkg/errorsx/httpx) so adapters inherit no transport dependency.
package cachetest

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/cache"
)

// Factory returns a fresh, state-isolated cache.Cache for one subtest. It
// receives the subtest's *testing.T so an adapter can key isolation on
// t.Name() and register t.Cleanup (matches ratelimittest/jobstest/
// idempotencytest, which all take *testing.T). Each call MUST return an
// implementation whose keyspace does not overlap any other call's (e.g. a new
// in-memory map, or a Redis client namespaced per call).
type Factory func(t *testing.T) cache.Cache

// RunContract runs the full deterministic cache.Cache conformance suite.
func RunContract(t *testing.T, newCache Factory) {
	t.Helper()

	ctx := context.Background()

	t.Run("GetAbsentKeyReturnsErrMiss", func(t *testing.T) {
		c := newCache(t)
		_, err := c.Get(ctx, "absent")
		if !errors.Is(err, cache.ErrMiss) {
			t.Fatalf("Get(absent) err = %v, want cache.ErrMiss", err)
		}
	})

	t.Run("SetThenGetReturnsValue", func(t *testing.T) {
		c := newCache(t)
		want := []byte("hello")
		if err := c.Set(ctx, "k", want, 0); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("Get = %q, want %q", got, want)
		}
	})

	t.Run("SetOverwriteWins", func(t *testing.T) {
		c := newCache(t)
		_ = c.Set(ctx, "k", []byte("v1"), 0)
		if err := c.Set(ctx, "k", []byte("v2"), 0); err != nil {
			t.Fatalf("Set overwrite: %v", err)
		}
		got, _ := c.Get(ctx, "k")
		if !bytes.Equal(got, []byte("v2")) {
			t.Fatalf("Get after overwrite = %q, want %q", got, "v2")
		}
	})

	t.Run("OverwriteWithShorterValue", func(t *testing.T) {
		c := newCache(t)
		_ = c.Set(ctx, "k", []byte("hello"), 0)
		if err := c.Set(ctx, "k", []byte("x"), 0); err != nil {
			t.Fatalf("Set shorter: %v", err)
		}
		got, _ := c.Get(ctx, "k")
		if !bytes.Equal(got, []byte("x")) {
			t.Fatalf("Get after shorter overwrite = %q, want %q", got, "x")
		}
	})

	t.Run("EmptyValueRoundTripsAndIsNotAMiss", func(t *testing.T) {
		c := newCache(t)
		if err := c.Set(ctx, "k", []byte{}, 0); err != nil {
			t.Fatalf("Set empty value: %v", err)
		}
		got, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get empty value err = %v, want nil (an empty value is present, not a miss)", err)
		}
		if len(got) != 0 {
			t.Fatalf("Get empty value len = %d, want 0", len(got))
		}
		if ok, _ := c.Exists(ctx, "k"); !ok {
			t.Fatal("Exists(empty value) = false, want true (empty value is present)")
		}
	})

	t.Run("DeleteThenGetMisses", func(t *testing.T) {
		c := newCache(t)
		_ = c.Set(ctx, "k", []byte("v"), 0)
		if err := c.Delete(ctx, "k"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := c.Get(ctx, "k"); !errors.Is(err, cache.ErrMiss) {
			t.Fatalf("Get after Delete err = %v, want cache.ErrMiss", err)
		}
	})

	t.Run("DeleteAbsentKeyIsIdempotent", func(t *testing.T) {
		c := newCache(t)
		if err := c.Delete(ctx, "absent"); err != nil {
			t.Fatalf("Delete(absent) = %v, want nil", err)
		}
	})

	t.Run("ExistsReflectsPresence", func(t *testing.T) {
		c := newCache(t)
		if ok, err := c.Exists(ctx, "k"); err != nil || ok {
			t.Fatalf("Exists(absent) = (%v,%v), want (false,nil)", ok, err)
		}
		_ = c.Set(ctx, "k", []byte("v"), 0)
		if ok, err := c.Exists(ctx, "k"); err != nil || !ok {
			t.Fatalf("Exists(present) = (%v,%v), want (true,nil)", ok, err)
		}
		_ = c.Delete(ctx, "k")
		if ok, err := c.Exists(ctx, "k"); err != nil || ok {
			t.Fatalf("Exists(after delete) = (%v,%v), want (false,nil)", ok, err)
		}
	})

	t.Run("EmptyKeyIsInvalidArgument", func(t *testing.T) {
		c := newCache(t)
		assertInvalid := func(name string, err error) {
			t.Helper()
			if errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
				t.Fatalf("%s empty-key CodeOf = %v, want CodeInvalidArgument", name, errorsx.CodeOf(err))
			}
		}
		_, gErr := c.Get(ctx, "")
		assertInvalid("Get", gErr)
		assertInvalid("Set", c.Set(ctx, "", []byte("v"), 0))
		assertInvalid("Delete", c.Delete(ctx, ""))
		_, eErr := c.Exists(ctx, "")
		assertInvalid("Exists", eErr)
	})

	t.Run("NegativeTTLIsInvalidArgumentAndNotWritten", func(t *testing.T) {
		c := newCache(t)
		err := c.Set(ctx, "k", []byte("v"), -1*time.Second)
		if errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("Set(ttl<0) CodeOf = %v, want CodeInvalidArgument", errorsx.CodeOf(err))
		}
		// Validation failure must not have written the key.
		if _, gErr := c.Get(ctx, "k"); !errors.Is(gErr, cache.ErrMiss) {
			t.Fatalf("key written despite invalid ttl: Get err = %v, want ErrMiss", gErr)
		}
	})

	t.Run("PreCancelledCtxReturnsCtxError", func(t *testing.T) {
		c := newCache(t)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := c.Get(cancelled, "k"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Get(cancelled) err = %v, want context.Canceled", err)
		}
		if err := c.Set(cancelled, "k", []byte("v"), 0); !errors.Is(err, context.Canceled) {
			t.Fatalf("Set(cancelled) err = %v, want context.Canceled", err)
		}
		if err := c.Delete(cancelled, "k"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Delete(cancelled) err = %v, want context.Canceled", err)
		}
		if _, err := c.Exists(cancelled, "k"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Exists(cancelled) err = %v, want context.Canceled", err)
		}
	})

	t.Run("EmptyKeyPrecedesCancelledCtx", func(t *testing.T) {
		c := newCache(t)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		assertEmptyKeyWins := func(name string, code errorsx.Code, err error) {
			t.Helper()
			if code != errorsx.CodeInvalidArgument {
				t.Fatalf("%s empty key + cancelled ctx: CodeOf = %v, want CodeInvalidArgument — empty-key validation must precede ctx observation (err=%v)", name, code, err)
			}
			if errors.Is(err, context.Canceled) {
				t.Fatalf("%s empty key + cancelled ctx returned ctx error; contract precedence is empty-key first", name)
			}
		}
		_, gErr := c.Get(cancelled, "")
		assertEmptyKeyWins("Get", errorsx.CodeOf(gErr), gErr)
		sErr := c.Set(cancelled, "", []byte("v"), 0)
		assertEmptyKeyWins("Set", errorsx.CodeOf(sErr), sErr)
		dErr := c.Delete(cancelled, "")
		assertEmptyKeyWins("Delete", errorsx.CodeOf(dErr), dErr)
		_, eErr := c.Exists(cancelled, "")
		assertEmptyKeyWins("Exists", errorsx.CodeOf(eErr), eErr)
	})

	t.Run("PreExpiredCtxReturnsCtxError", func(t *testing.T) {
		c := newCache(t)
		expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Hour))
		defer cancel()
		if _, err := c.Get(expired, "k"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Get(expired deadline) err = %v, want context.DeadlineExceeded", err)
		}
		if err := c.Set(expired, "k", []byte("v"), 0); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Set(expired deadline) err = %v, want context.DeadlineExceeded", err)
		}
		if err := c.Delete(expired, "k"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Delete(expired deadline) err = %v, want context.DeadlineExceeded", err)
		}
		if _, err := c.Exists(expired, "k"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Exists(expired deadline) err = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("InputSliceNotAliased", func(t *testing.T) {
		c := newCache(t)
		v := []byte("hello")
		if err := c.Set(ctx, "k", v, 0); err != nil {
			t.Fatalf("Set: %v", err)
		}
		v[0] = 'X' // caller mutates its slice after Set
		got, _ := c.Get(ctx, "k")
		if !bytes.Equal(got, []byte("hello")) {
			t.Fatalf("cache aliased caller input: Get = %q, want %q", got, "hello")
		}
	})

	t.Run("OutputSliceNotAliased", func(t *testing.T) {
		c := newCache(t)
		_ = c.Set(ctx, "k", []byte("hello"), 0)
		a, _ := c.Get(ctx, "k")
		if len(a) > 0 {
			a[0] = 'X' // caller mutates returned slice
		}
		b, _ := c.Get(ctx, "k")
		if !bytes.Equal(b, []byte("hello")) {
			t.Fatalf("cache handed out aliased internal slice: Get = %q, want %q", b, "hello")
		}
	})
}
