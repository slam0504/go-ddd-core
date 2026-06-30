package ratelimittest_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/ratelimit"
	"github.com/slam0504/go-ddd-core/ports/ratelimit/ratelimittest"
)

// refLimiter is a deterministic reference Limiter: for each key the first Allow
// is allowed, every subsequent Allow is denied. It uses no wall clock for its
// decision (RetryAfter is a fixed positive hint; ResetAt is left absent), so it
// is purely deterministic and concurrency-safe.
type refLimiter struct {
	mu   sync.Mutex
	seen map[string]int
}

func newRefLimiter() *refLimiter { return &refLimiter{seen: map[string]int{}} }

func (l *refLimiter) Allow(ctx context.Context, key string) (ratelimit.Result, error) {
	if key == "" {
		return ratelimit.Result{}, errorsx.New(errorsx.CodeInvalidArgument, "ratelimit: empty key")
	}
	if err := ctx.Err(); err != nil {
		return ratelimit.Result{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	const limit = 1
	n := l.seen[key]
	l.seen[key]++
	if n < limit {
		return ratelimit.Result{
			Allowed:   true,
			Limit:     limit,
			Remaining: limit - n - 1, // 0 after the one allowed call
			ResetAt:   time.Time{},   // absent: reference tracks no window
		}, nil
	}
	return ratelimit.Result{
		Allowed:    false,
		RetryAfter: time.Second, // fixed positive wait hint, no wall clock
		Limit:      limit,
		Remaining:  0,
		ResetAt:    time.Time{},
	}, nil
}

func TestReferenceLimiterSatisfiesContract(t *testing.T) {
	ratelimittest.RunContract(t, func(t *testing.T) ratelimit.Limiter {
		return newRefLimiter()
	})
}
