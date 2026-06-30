# ports/ratelimit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the `ports/ratelimit` inbound request-throttling contract (the last A-quadrant gap in `docs/roadmap.md`) plus its deterministic-only `ratelimittest` conformance suite.

**Architecture:** A single `Limiter` interface whose `Allow(ctx, key)` returns the throttling decision as **data** (`Result.Allowed`), never as an error. `Result` also carries accurate-or-absent advisory quota metadata. A `ratelimittest.RunContract` suite verifies any implementation against the non-timing invariants, validated in-package against a deterministic reference limiter. No throttling algorithm, config, or storage lives in core — those are adapter concerns.

**Tech Stack:** Go (repo's existing toolchain), stdlib `context`/`time`/`errors`/`testing`, `github.com/slam0504/go-ddd-core/pkg/errorsx`.

**Design spec:** `docs/superpowers/specs/2026-06-30-ports-ratelimit-design.md` (commit `a6080cf`).

## Global Constraints

- Module path: `github.com/slam0504/go-ddd-core` — single-module repo; the contract lives under `ports/ratelimit/`.
- `gofmt -l ports/` MUST produce no output (CI runs golangci-lint; gofmt failures break the build).
- `go build ./...`, `go vet ./...`, `go test ./ports/ratelimit/...` all clean before each commit.
- Non-nil ctx convention: every ctx-taking method requires a non-nil ctx; no runtime nil-check.
- Coded errors use `pkg/errorsx`: validation → `errorsx.CodeInvalidArgument`; backend failure → coded error whose `errorsx.CodeOf` is NOT `CodeUnknown`.
- File organisation mirrors `ports/jobs` and `ports/idempotency`: `ratelimit.go` + `ratelimit_test.go` + `ratelimittest/ratelimittest.go` + `ratelimittest/ratelimittest_test.go`.
- The conformance suite is deterministic-only (no `time.Sleep`, no timers, no waits) — it asserts solely on interface return values, exactly like `jobstest`.
- `UnknownCount = -1` is the ONLY "absent" value for `Limit`/`Remaining`; a real `0` is a known value. `ResetAt` absent is `time.Time{}` (`IsZero`).
- Branch: `feat/ports-ratelimit` (already created, off `db25165`).

---

### Task 1: Contract types (`Result`, sentinels, `Limiter` interface)

**Files:**
- Create: `ports/ratelimit/ratelimit.go`
- Test: `ports/ratelimit/ratelimit_test.go`

**Interfaces:**
- Consumes: nothing (leaf package; imports only stdlib `context`/`time`).
- Produces:
  - `const ratelimit.UnknownCount = -1`
  - `type ratelimit.Result struct { Allowed bool; RetryAfter time.Duration; Limit int; Remaining int; ResetAt time.Time }`
  - `func (Result) HasLimit() bool` — `Limit >= 0`
  - `func (Result) HasRemaining() bool` — `Remaining >= 0`
  - `type ratelimit.Limiter interface { Allow(ctx context.Context, key string) (Result, error) }`

- [ ] **Step 1: Write the failing test**

Create `ports/ratelimit/ratelimit_test.go`:

```go
package ratelimit_test

import (
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/ports/ratelimit"
)

func TestUnknownCountIsNegativeOne(t *testing.T) {
	if ratelimit.UnknownCount != -1 {
		t.Fatalf("UnknownCount = %d, want -1 (mirrors pagination.Page.Total's absent sentinel)", ratelimit.UnknownCount)
	}
}

func TestHasLimit(t *testing.T) {
	cases := []struct {
		limit int
		want  bool
	}{
		{ratelimit.UnknownCount, false}, // -1 is absent
		{0, true},                       // a real limit of 0 is KNOWN, not absent
		{5, true},
	}
	for _, c := range cases {
		if got := (ratelimit.Result{Limit: c.limit}).HasLimit(); got != c.want {
			t.Fatalf("Result{Limit:%d}.HasLimit() = %v, want %v", c.limit, got, c.want)
		}
	}
}

func TestHasRemaining(t *testing.T) {
	cases := []struct {
		remaining int
		want      bool
	}{
		{ratelimit.UnknownCount, false}, // -1 is absent
		{0, true},                       // window exhausted: Remaining 0 is KNOWN, not absent
		{3, true},
	}
	for _, c := range cases {
		if got := (ratelimit.Result{Remaining: c.remaining}).HasRemaining(); got != c.want {
			t.Fatalf("Result{Remaining:%d}.HasRemaining() = %v, want %v", c.remaining, got, c.want)
		}
	}
}

func TestZeroValueResultTreatsCountsAsKnownZero(t *testing.T) {
	// A zero-value Result has Limit==0 / Remaining==0, which HasLimit/HasRemaining
	// report as KNOWN (not absent). Adapters that cannot honestly produce a count
	// MUST set it to UnknownCount explicitly; they cannot rely on the zero value
	// to mean absent. This pins the trap so it is documented, not discovered.
	var r ratelimit.Result
	if !r.HasLimit() {
		t.Fatal("zero-value Result.HasLimit() = false; want true (Limit==0 is known real 0; absent must be UnknownCount)")
	}
	if !r.HasRemaining() {
		t.Fatal("zero-value Result.HasRemaining() = false; want true (Remaining==0 is known)")
	}
}

func TestResultIsComparableValue(t *testing.T) {
	// Result is a flat value struct (no pointers); equal field values compare equal.
	a := ratelimit.Result{Allowed: true, RetryAfter: time.Second, Limit: 10, Remaining: 9}
	b := ratelimit.Result{Allowed: true, RetryAfter: time.Second, Limit: 10, Remaining: 9}
	if a != b {
		t.Fatal("identical Result values compare unequal; Result must be a comparable value type")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ports/ratelimit/`
Expected: FAIL — build error `package github.com/slam0504/go-ddd-core/ports/ratelimit is not in std` / `undefined: ratelimit.UnknownCount` (the package does not exist yet).

- [ ] **Step 3: Write minimal implementation**

Create `ports/ratelimit/ratelimit.go` (doc comments are the contract — copy verbatim from the design spec):

```go
// Package ratelimit defines the inbound request-throttling contract. A Limiter
// answers a single question — "may this key proceed right now?" — and returns
// the decision as data (Result.Allowed), never as an error. It also carries
// advisory quota metadata (limit / remaining / reset) that an HTTP transport
// adapter MAY surface as rate-limit response hints — Retry-After plus whatever
// quota headers the service's HTTP convention uses. Core fixes neither the
// throttling algorithm (token-bucket, sliding-window, GCRA, …), the
// rate/burst/window configuration, nor the storage backend — all live in
// adapters.
//
// Every ctx-taking method requires a non-nil ctx (stdlib convention).
package ratelimit

import (
	"context"
	"time"
)

// UnknownCount marks Limit/Remaining as absent — the limiter cannot honestly
// produce the value (e.g. a token-bucket has no discrete window, or an upstream
// quota exceeds what an int can hold). It is distinct from a real 0. Mirrors
// pkg/pagination.Page.Total's -1 "not computed" sentinel.
const UnknownCount = -1

// Result is the outcome of one Allow call. Allowed is the decision; the rest is
// advisory metadata for response headers (see field docs for each obligation).
type Result struct {
	// Allowed MUST be exact — it is the decision itself.
	Allowed bool
	// RetryAfter MUST be present. When Allowed it MUST be 0. When !Allowed it is
	// a conservative wait hint: it MUST NOT be less than the limiter's known
	// earliest-retry time (under-estimating is a bug), and MAY be larger (a
	// conservative over-estimate is fine — the client just waits longer).
	// Retrying before it elapses carries NO success guarantee; it is NOT a
	// guarantee of denial. Per IETF draft-ietf-httpapi-ratelimit-headers-11,
	// reset/retry timing is a hint (a server MAY alter quota between requests),
	// not a hard "denied until then" promise. Reports on the safe side like jobs
	// Job.ProcessAt's "no earlier than", but without ProcessAt's hard guarantee.
	RetryAfter time.Duration
	// Limit / Remaining / ResetAt are accurate-or-absent advisory metadata:
	// each is either a value the limiter genuinely computed (carrying that
	// algorithm's inherent precision; a distributed limiter's stale value
	// counts) or a defined "absent" sentinel — NEVER a fabricated placeholder.
	// They are advisory-only: a consumer MAY surface them as headers but MUST
	// NOT use them to make its own allow/deny decision (that is Allowed's job;
	// Remaining is TOCTOU-stale the instant it is read). A consumer MUST omit
	// the corresponding header when the value is absent (see HasLimit /
	// HasRemaining / ResetAt.IsZero) — it MUST NOT serialise UnknownCount.
	//
	// Multi-policy projection: a limiter MAY enforce several quota policies at
	// once (IETF draft-11 RateLimit-Policy is a list, e.g. 50/60s AND
	// 1000/3600s). Result carries at most ONE policy's metadata, so the adapter
	// MUST project the policy that BOUND this decision — on a denial, the policy
	// that denied; on an allow, the most-constraining policy it can honestly
	// represent. If no single policy is a faithful representative, the fields
	// MUST be absent rather than a fabricated blend. Surfacing every policy is an
	// adapter-layer concern, outside this single-Result contract.
	Limit     int       // UnknownCount means absent.
	Remaining int       // UnknownCount means absent.
	ResetAt   time.Time // IsZero means absent.
}

// HasLimit reports whether Limit carries a real value (not absent).
func (r Result) HasLimit() bool { return r.Limit >= 0 }

// HasRemaining reports whether Remaining carries a real value (not absent).
func (r Result) HasRemaining() bool { return r.Remaining >= 0 }

// Limiter decides whether a key may proceed. Implementations MUST be safe for
// concurrent use — middleware calls Allow on every inbound request.
type Limiter interface {
	// Allow reports whether key may proceed right now, as Result data. Ordinary
	// quota exhaustion is NOT an error: it MUST return Result{Allowed:false}
	// (with a RetryAfter wait hint), nil. The Limiter NEVER returns
	// errorsx.CodeRateLimited for ordinary denial. Because Allowed==false is data
	// (there is no error value), HTTP middleware SHOULD emit 429 directly from the
	// Allowed==false decision rather than route it through an error-translation
	// pipeline (httpx.Translate takes an error, which this is not); only if a
	// particular transport pipeline requires an error object should it mint a
	// CodeRateLimited error inside that adapter.
	//
	// A non-nil error means the limiter could not reach a decision, in two
	// classes with fixed precedence (same shape as jobs.Enqueuer):
	//   CLASS 1 — validation: an empty key is malformed input (a missing
	//     partition key, not an "anonymous" caller) → errorsx.CodeInvalidArgument;
	//     deterministic, nothing consumed.
	//   CLASS 2 — ctx / backend: a ctx already cancelled or past its deadline at
	//     entry returns the matching ctx error (errors.Is context.Canceled /
	//     context.DeadlineExceeded) with NO backend contact; otherwise a backend
	//     failure is a coded errorsx whose CodeOf is NOT CodeUnknown
	//     (unreachable → CodeUnavailable; unclassifiable → CodeInternal).
	//   Precedence: empty key → pre-cancelled/expired ctx → backend.
	Allow(ctx context.Context, key string) (Result, error)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ports/ratelimit/`
Expected: PASS (all 5 tests).
Then: `gofmt -l ports/ratelimit/ratelimit.go ports/ratelimit/ratelimit_test.go` → no output; `go vet ./ports/ratelimit/` → clean.

- [ ] **Step 5: Commit**

```bash
git add ports/ratelimit/ratelimit.go ports/ratelimit/ratelimit_test.go
git commit -m "$(cat <<'EOF'
feat(ports/ratelimit): add inbound request-throttling contract

Limiter.Allow(ctx, key) (Result, error): decision is data (Result.Allowed)
not error. Result carries accurate-or-absent advisory metadata (UnknownCount
= -1 / IsZero sentinels); HasLimit/HasRemaining predicates. A real 0 is known,
not absent — adapters must set UnknownCount explicitly.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `ratelimittest` conformance suite + reference limiter

**Files:**
- Create: `ports/ratelimit/ratelimittest/ratelimittest.go`
- Create (test): `ports/ratelimit/ratelimittest/ratelimittest_test.go`

**Interfaces:**
- Consumes: `ratelimit.Limiter`, `ratelimit.Result`, `ratelimit.UnknownCount` (Task 1); `errorsx.CodeOf`, `errorsx.New`, `errorsx.CodeInvalidArgument`.
- Produces:
  - `type ratelimittest.Factory func(t *testing.T) ratelimit.Limiter` — returns a fresh, isolated limiter per subtest, configured so that for any single key the FIRST `Allow` is allowed and the SECOND (same key) is denied.
  - `func ratelimittest.RunContract(t *testing.T, factory Factory)` — the deterministic-only suite.

- [ ] **Step 1: Write the suite**

Create `ports/ratelimit/ratelimittest/ratelimittest.go`:

```go
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
		res, err := l.Allow(context.Background(), keyA)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if res.Limit < 0 && res.Limit != ratelimit.UnknownCount {
			t.Fatalf("Result.Limit = %d: a negative Limit must be exactly UnknownCount (%d), never another negative", res.Limit, ratelimit.UnknownCount)
		}
		if res.Remaining < 0 && res.Remaining != ratelimit.UnknownCount {
			t.Fatalf("Result.Remaining = %d: a negative Remaining must be exactly UnknownCount (%d)", res.Remaining, ratelimit.UnknownCount)
		}
		if res.HasLimit() && res.HasRemaining() && res.Remaining > res.Limit {
			t.Fatalf("Remaining (%d) > Limit (%d) while both known; remaining can never exceed the limit", res.Remaining, res.Limit)
		}
	})
}
```

- [ ] **Step 2: Prove the suite is not vacuous (failing test with a deliberately-broken limiter)**

Create `ports/ratelimit/ratelimittest/ratelimittest_test.go` with a STUB limiter that always allows and never validates — this must make the suite FAIL, proving the suite actually catches violations:

```go
package ratelimittest_test

import (
	"context"
	"testing"

	"github.com/slam0504/go-ddd-core/ports/ratelimit"
	"github.com/slam0504/go-ddd-core/ports/ratelimit/ratelimittest"
)

// brokenLimiter always allows and never validates — used only to prove the
// suite is not vacuous. Replaced by refLimiter in Step 3.
type brokenLimiter struct{}

func (brokenLimiter) Allow(context.Context, string) (ratelimit.Result, error) {
	return ratelimit.Result{Allowed: true}, nil
}

func TestReferenceLimiterSatisfiesContract(t *testing.T) {
	ratelimittest.RunContract(t, func(t *testing.T) ratelimit.Limiter {
		return brokenLimiter{}
	})
}
```

Run: `go test ./ports/ratelimit/ratelimittest/ -run TestReferenceLimiterSatisfiesContract -v`
Expected: FAIL — at least `EmptyKeyInvalidArgument` (no validation → code CodeUnknown), `FirstAllowedSecondDenied` (second still allowed), `RetryAfterInvariant` (denied case never reached / RetryAfter 0), and `PreCancelledCtx` (ctx ignored) fail. This confirms the suite has teeth.

- [ ] **Step 3: Replace the stub with the real reference limiter (make it pass)**

Replace the entire body of `ports/ratelimit/ratelimittest/ratelimittest_test.go`:

```go
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
```

- [ ] **Step 4: Run the full suite to verify it passes**

Run: `go test ./ports/ratelimit/...`
Expected: PASS (Task 1 unit tests + all 8 RunContract subtests via the reference limiter).
Then: `gofmt -l ports/ratelimit/` → no output; `go vet ./ports/ratelimit/...` → clean; `go build ./...` → clean.

- [ ] **Step 5: Commit**

```bash
git add ports/ratelimit/ratelimittest/ratelimittest.go ports/ratelimit/ratelimittest/ratelimittest_test.go
git commit -m "$(cat <<'EOF'
feat(ports/ratelimit): add deterministic-only ratelimittest conformance suite

RunContract asserts the non-timing invariants: empty-key CodeInvalidArgument +
precedence over ctx errors, pre-cancelled/expired ctx, first-allowed-then-denied
depletion, denial-is-data, RetryAfter invariant (allowed=0 / denied>0), key
isolation, accurate-or-absent metadata shape. Validated against an in-package
deterministic reference limiter. No sleeps, no timers — cannot flake.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**1. Spec coverage** (against `2026-06-30-ports-ratelimit-design.md`):

| Spec section | Task |
|---|---|
| §2 `Result` + `UnknownCount` + `HasLimit`/`HasRemaining` | Task 1 |
| §2 `Limiter.Allow` interface + doc | Task 1 |
| §3 `Allowed` exact / `RetryAfter` hint / accurate-or-absent / overflow→absent / advisory-only | Task 1 (types + doc), Task 2 (MetadataShape, RetryAfterInvariant) |
| §4 error: empty key → CodeInvalidArgument; ctx precedence | Task 2 (EmptyKeyInvalidArgument, EmptyKeyPrecedesCancelledCtx, PreCancelledCtx, PreExpiredCtx) |
| §4 denial-is-data | Task 2 (FirstAllowedSecondDenied) |
| §5 deterministic-only suite + fresh-per-subtest + RetryAfter invariant | Task 2 (whole suite, Factory contract) |
| §6 file organisation | Tasks 1 & 2 file paths |
| §7 verification (gofmt/build/vet/test) | Task 1 Step 4, Task 2 Step 4 |

No gaps. (Note: §4 backend-failure `CodeUnavailable`/`CodeInternal` is an adapter obligation — the contract states it; a real-backend adapter test exercises it, out of scope for the core deterministic suite. Documented in the `Limiter.Allow` doc.)

**2. Placeholder scan:** No TBD/TODO/"handle edge cases"/"similar to". Every code step shows complete code.

**3. Type consistency:** `Result`, `UnknownCount`, `HasLimit`, `HasRemaining`, `Limiter`, `Allow`, `Factory`, `RunContract` are spelled identically across Task 1 → Task 2 and match the design spec §2/§6.
