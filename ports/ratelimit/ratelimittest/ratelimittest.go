// Package ratelimittest provides an exported, deterministic-only conformance
// suite for ports/ratelimit.Limiter implementations.
//
// RunContract verifies only the invariants visible through interface return
// values: empty-key validation and its precedence over ctx errors, pre-
// cancelled / pre-expired ctx handling, first-allowed-then-denied depletion,
// denial-is-data (not error), the RetryAfter invariant, per-key isolation, and
// metadata shape (accurate-or-absent). It never sleeps and waits on no timer,
// so it cannot flake.
//
// Timing-dependent behaviour — window recovery after RetryAfter, exact reset
// instants, RetryAfter decay, high-concurrency exact allow counts — is NOT in
// this suite. Those are adapter-level concerns; adapters cover them with their
// own intent tests, mirroring jobstest's "synchronous-only" boundary.
package ratelimittest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/ratelimit"
)

// Factory returns a fresh, isolated Limiter for one subtest, configured with a
// deterministic profile: for any single key the FIRST Allow is allowed and the
// SECOND (same key) is denied. State MUST be isolated across calls (no shared
// counters); register any cleanup via t.Cleanup.
type Factory func(t *testing.T) ratelimit.Limiter

// RunContract runs the deterministic-only contract suite. It calls factory once
// per subtest and asserts only on interface return values (Result fields,
// errorsx.CodeOf, errors.Is); it never sleeps or waits on a timer.
func RunContract(t *testing.T, factory Factory) {
	t.Helper()

	const keyA, keyB = "ratelimittest:a", "ratelimittest:b"

	t.Run("EmptyKeyInvalidArgument", func(t *testing.T) {
		l := factory(t)
		res, err := l.Allow(context.Background(), "")
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Allow with empty key: code = %v, want CodeInvalidArgument — an empty key is a missing partition key, not an anonymous caller (err=%v)", code, err)
		}
		if res.Allowed {
			t.Fatal("Allow with empty key returned Allowed=true; an error return must not also allow")
		}
	})

	t.Run("EmptyKeyPrecedesCancelledCtx", func(t *testing.T) {
		l := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := l.Allow(ctx, "")
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("empty key + cancelled ctx: code = %v, want CodeInvalidArgument — validation precedes ctx observation (err=%v)", code, err)
		}
		if errors.Is(err, context.Canceled) {
			t.Fatal("empty key + cancelled ctx returned the ctx error; fixed precedence is empty-key first")
		}
	})

	t.Run("EmptyKeyPrecedesExpiredDeadline", func(t *testing.T) {
		l := factory(t)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		_, err := l.Allow(ctx, "")
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("empty key + expired deadline: code = %v, want CodeInvalidArgument — validation precedes ctx observation (err=%v)", code, err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("empty key + expired deadline returned the ctx error; fixed precedence is empty-key first")
		}
	})

	t.Run("PreCancelledCtx", func(t *testing.T) {
		l := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := l.Allow(ctx, keyA)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-cancelled ctx with valid key: err = %v, want errors.Is(context.Canceled)", err)
		}
	})

	t.Run("PreExpiredCtx", func(t *testing.T) {
		l := factory(t)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		_, err := l.Allow(ctx, keyA)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("pre-expired deadline with valid key: err = %v, want errors.Is(context.DeadlineExceeded)", err)
		}
	})

	t.Run("FirstAllowedSecondDenied", func(t *testing.T) {
		l := factory(t)
		first, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("first Allow: unexpected err %v", err)
		}
		if !first.Allowed {
			t.Fatal("first Allow on a fresh key: Allowed=false, want true (deterministic profile)")
		}
		second, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("second Allow: unexpected err %v — ordinary denial is DATA, not an error", err)
		}
		if second.Allowed {
			t.Fatal("second Allow on the same key: Allowed=true, want false (deterministic profile)")
		}
	})

	t.Run("RetryAfterInvariant", func(t *testing.T) {
		l := factory(t)
		allowed, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("first Allow: %v", err)
		}
		if allowed.RetryAfter != 0 {
			t.Fatalf("allowed Result.RetryAfter = %v, want 0", allowed.RetryAfter)
		}
		denied, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("second Allow: %v", err)
		}
		if denied.RetryAfter <= 0 {
			t.Fatalf("denied Result.RetryAfter = %v, want > 0 — a denial needs a wait hint; Result{}, nil would fail here", denied.RetryAfter)
		}
	})

	t.Run("KeyIsolation", func(t *testing.T) {
		l := factory(t)
		if _, err := l.Allow(context.Background(), keyA); err != nil {
			t.Fatalf("deplete keyA #1: %v", err)
		}
		if _, err := l.Allow(context.Background(), keyA); err != nil {
			t.Fatalf("deplete keyA #2: %v", err)
		}
		res, err := l.Allow(context.Background(), keyB)
		if err != nil {
			t.Fatalf("first Allow on keyB: %v", err)
		}
		if !res.Allowed {
			t.Fatal("keyB first Allow denied after keyA was depleted; keys must be isolated buckets")
		}
	})

	t.Run("MetadataShape", func(t *testing.T) {
		l := factory(t)
		check := func(res ratelimit.Result, label string) {
			if res.Limit < 0 && res.Limit != ratelimit.UnknownCount {
				t.Fatalf("%s: Result.Limit = %d: a negative Limit must be exactly UnknownCount (%d), never another negative", label, res.Limit, ratelimit.UnknownCount)
			}
			if res.Remaining < 0 && res.Remaining != ratelimit.UnknownCount {
				t.Fatalf("%s: Result.Remaining = %d: a negative Remaining must be exactly UnknownCount (%d)", label, res.Remaining, ratelimit.UnknownCount)
			}
			if res.HasLimit() && res.HasRemaining() && res.Remaining > res.Limit {
				t.Fatalf("%s: Remaining (%d) > Limit (%d) while both known; remaining can never exceed the limit", label, res.Remaining, res.Limit)
			}
		}
		allowed, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("first Allow: %v", err)
		}
		check(allowed, "allowed result")
		denied, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("second Allow: %v — denial must be data, not an error", err)
		}
		check(denied, "denied result")
	})
}
