# jobstest delivery/timing suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an exported `jobstest.RunDeliveryContract` that distills the interface-observable delivery/timing invariants of `ports/jobs` into a reusable conformance suite, validated against a wall-clock in-memory reference backend.

**Architecture:** A new `delivery.go` in package `jobstest` adds `DeliveryBounds`/`DeliveryFixture`/`DeliveryFactory` types and `RunDeliveryContract(t, factory)` — 11 subtests that start a real `jobs.Worker`, drive it through `jobs.Enqueuer` + a test handler, and assert only interface-observable behavior (no backend introspection, no fault injection). A wall-clock in-memory reference backend in `delivery_test.go` (a `jobs.Enqueuer` + `jobs.Worker` over a shared store) both unit-tests directly and drives the suite; deliberately-broken variants prove the suite is non-vacuous.

**Tech Stack:** Go, stdlib `context`/`time`/`sync`/`testing`, `github.com/slam0504/go-ddd-core/pkg/errorsx`, `github.com/slam0504/go-ddd-core/ports/jobs`.

**Design spec:** `docs/superpowers/specs/2026-07-01-jobstest-delivery-suite-design.md` (commit `479dc9e`).

## Global Constraints

- Module path: `github.com/slam0504/go-ddd-core`; suite lives in the existing `ports/jobs/jobstest` package.
- `gofmt -l ports/jobs/` MUST produce no output; `go build ./...`, `go vet ./ports/jobs/...`, `go test ./ports/jobs/...` all clean before each commit.
- `RunDeliveryContract` and the reference backend assert ONLY interface-observable behavior (`jobs.Enqueuer`/`jobs.Worker`/test handler): no `Inspector`, no state classification, no fault-injection hooks.
- All `DeliveryBounds` fields are REQUIRED; a non-positive value → `t.Fatalf`. No core defaults (single-source: bounds are the adapter's promise).
- Fixture preconditions (documented, adapter responsibility): each `factory(t)` returns an isolated store; every `NewWorker()` shares the `Enqueuer`'s store; the fixture MUST configure failed handler attempts to be redelivered within `RedeliverWithin` (retry-enabled).
- The delivery suite waits on adapter-declared bounds, never hardcoded sleeps-then-assert; handler signals via buffered channel + `select` with the bound.
- Branch: `feat/jobstest-delivery-suite` (already created, off `main`).

---

### Task 1: Contract types + wall-clock in-memory reference backend

**Files:**
- Create: `ports/jobs/jobstest/delivery.go` (types only in this task)
- Test: `ports/jobs/jobstest/delivery_test.go` (reference backend + direct unit tests)

**Interfaces:**
- Consumes: `jobs.Enqueuer`, `jobs.Worker`, `jobs.Handler`, `jobs.HandlerFunc`, `jobs.Job`, `jobs.Task`, `jobs.JobInfo` (existing); `errorsx.New`, `errorsx.CodeInvalidArgument`, `errorsx.CodeAlreadyExists`, `errorsx.CodeOf`.
- Produces:
  - `type jobstest.DeliveryBounds struct { ShutdownWithin, DeliverWithin, RedeliverWithin, ProcessAtDelay time.Duration }`
  - `type jobstest.DeliveryFixture struct { Enqueuer jobs.Enqueuer; NewWorker func() jobs.Worker; Bounds DeliveryBounds }`
  - `type jobstest.DeliveryFactory func(t *testing.T) DeliveryFixture`
  - Test-only reference backend: `refStore` / `refEnqueuer` / `refWorker` / `newRefFixture(t) DeliveryFixture`.

- [ ] **Step 1: Write the failing test** (reference backend direct unit tests)

Create `ports/jobs/jobstest/delivery_test.go`:

```go
package jobstest_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/jobs"
	"github.com/slam0504/go-ddd-core/ports/jobs/jobstest"
)

// --- wall-clock in-memory reference backend (test-only) ---

type refJob struct {
	id        string
	typ       string
	payload   []byte
	processAt time.Time
	attempts  int
	claimed   bool
	done      bool
}

type refStore struct {
	mu   sync.Mutex
	seq  int
	jobs map[string]*refJob
}

func newRefStore() *refStore { return &refStore{jobs: map[string]*refJob{}} }

func (s *refStore) enqueue(job jobs.Job) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("job-%d", s.seq)
	s.jobs[id] = &refJob{
		id:        id,
		typ:       job.Type,
		payload:   append([]byte(nil), job.Payload...), // snapshot-before-submit
		processAt: job.ProcessAt,
	}
	return id
}

type claimed struct {
	id, typ string
	payload []byte
}

// claim atomically picks one eligible (ProcessAt <= now), unclaimed, undone job
// whose type has a handler, marks it claimed, bumps attempts, and returns a
// value snapshot (payload copied per delivery). Returns ok=false if none.
func (s *refStore) claim(now time.Time, has func(string) bool) (claimed, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.done || j.claimed {
			continue
		}
		if !j.processAt.IsZero() && j.processAt.After(now) {
			continue
		}
		if !has(j.typ) {
			continue
		}
		j.claimed = true
		j.attempts++
		return claimed{id: j.id, typ: j.typ, payload: append([]byte(nil), j.payload...)}, true
	}
	return claimed{}, false
}

func (s *refStore) complete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.done = true
	}
}

func (s *refStore) retry(id string, after time.Duration, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.claimed = false
		j.processAt = now.Add(after) // redeliver after a short delay
	}
}

type refEnqueuer struct{ store *refStore }

func (e *refEnqueuer) Enqueue(ctx context.Context, job jobs.Job) (jobs.JobInfo, error) {
	if job.Type == "" {
		return jobs.JobInfo{}, errorsx.New(errorsx.CodeInvalidArgument, "jobs: empty Type")
	}
	if err := ctx.Err(); err != nil {
		return jobs.JobInfo{}, err
	}
	return jobs.JobInfo{ID: e.store.enqueue(job)}, nil
}

type refWorker struct {
	store      *refStore
	mu         sync.Mutex
	handlers   map[string]jobs.Handler
	retryDelay time.Duration
	poll       time.Duration
}

func (w *refWorker) Register(jobType string, h jobs.Handler) error {
	if jobType == "" || h == nil {
		return errorsx.New(errorsx.CodeInvalidArgument, "jobs: bad Register")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, dup := w.handlers[jobType]; dup {
		return errorsx.New(errorsx.CodeAlreadyExists, "jobs: duplicate Register")
	}
	w.handlers[jobType] = h
	return nil
}

func (w *refWorker) has(typ string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.handlers[typ]
	return ok
}

func (w *refWorker) handlerFor(typ string) jobs.Handler {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.handlers[typ]
}

func (w *refWorker) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil // pre-cancelled: return nil without starting
	}
	t := time.NewTicker(w.poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil // liveness: do not wait for in-flight stragglers
		case <-t.C:
			c, ok := w.store.claim(time.Now(), w.has)
			if !ok {
				continue
			}
			h := w.handlerFor(c.typ)
			if h == nil {
				continue
			}
			go func(c claimed, h jobs.Handler) {
				task := jobs.Task{ID: c.id, Type: c.typ, Payload: c.payload}
				if err := h.Handle(ctx, task); err != nil {
					w.store.retry(c.id, w.retryDelay, time.Now())
				} else {
					w.store.complete(c.id)
				}
			}(c, h)
		}
	}
}

// newRefFixture builds a DeliveryFixture over one shared store: retry-enabled
// (failed handler redelivered after retryDelay), wall-clock ProcessAt, multiple
// Workers over the same store.
func newRefFixture(t *testing.T) jobstest.DeliveryFixture {
	t.Helper()
	store := newRefStore()
	return jobstest.DeliveryFixture{
		Enqueuer: &refEnqueuer{store: store},
		NewWorker: func() jobs.Worker {
			return &refWorker{
				store:      store,
				handlers:   map[string]jobs.Handler{},
				retryDelay: 20 * time.Millisecond,
				poll:       10 * time.Millisecond,
			}
		},
		Bounds: jobstest.DeliveryBounds{
			ShutdownWithin:  2 * time.Second,
			DeliverWithin:   2 * time.Second,
			RedeliverWithin: 2 * time.Second,
			ProcessAtDelay:  300 * time.Millisecond,
		},
	}
}

// --- direct unit tests on the reference backend (independent of the suite) ---

func TestRefBackend_EnqueueSnapshotAndID(t *testing.T) {
	fx := newRefFixture(t)
	payload := []byte("orig")
	info, err := fx.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: payload})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if info.ID == "" {
		t.Fatal("empty JobInfo.ID")
	}
	payload[0] = 'X' // mutate after enqueue; the snapshot must be isolated
	w := fx.NewWorker()
	got := make(chan string, 1)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- string(task.Payload)
		return nil
	})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	cancel, done := runRef(w)
	defer func() { cancel(); <-done }()
	select {
	case p := <-got:
		if p != "orig" {
			t.Fatalf("handler saw %q, want \"orig\" (snapshot isolation)", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job never delivered")
	}
}

func TestRefBackend_RetryOnError(t *testing.T) {
	fx := newRefFixture(t)
	w := fx.NewWorker()
	var attempts atomic.Int32
	ok := make(chan struct{}, 1)
	if err := w.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		if attempts.Add(1) == 1 {
			return errorsx.New(errorsx.CodeInternal, "transient")
		}
		ok <- struct{}{}
		return nil
	})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	cancel, done := runRef(w)
	defer func() { cancel(); <-done }()
	if _, err := fx.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case <-ok:
		if attempts.Load() < 2 {
			t.Fatalf("succeeded after %d attempts, want >= 2", attempts.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no retry")
	}
}

func TestRefBackend_NotBeforeProcessAt(t *testing.T) {
	fx := newRefFixture(t)
	w := fx.NewWorker()
	at := time.Now().Add(300 * time.Millisecond)
	fired := make(chan time.Time, 1)
	if err := w.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		fired <- time.Now()
		return nil
	})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	cancel, done := runRef(w)
	defer func() { cancel(); <-done }()
	if _, err := fx.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "t", ProcessAt: at}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case got := <-fired:
		if got.Before(at) {
			t.Fatalf("fired at %v, before ProcessAt %v", got, at)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled job never fired")
	}
}

func TestRefBackend_MultiWorkerClaimIsolation(t *testing.T) {
	fx := newRefFixture(t)
	var delivered atomic.Int32
	got := make(chan struct{}, 1)
	reg := func(w jobs.Worker) {
		if err := w.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			delivered.Add(1)
			select {
			case got <- struct{}{}:
			default:
			}
			return nil
		})); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	w1, w2 := fx.NewWorker(), fx.NewWorker()
	reg(w1)
	reg(w2)
	c1, d1 := runRef(w1)
	c2, d2 := runRef(w2)
	defer func() { c1(); <-d1; c2(); <-d2 }()
	if _, err := fx.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("job never delivered by either worker")
	}
	time.Sleep(200 * time.Millisecond) // give a buggy double-claim time to show
	if n := delivered.Load(); n != 1 {
		t.Fatalf("job delivered %d times across two workers, want exactly 1 (claim isolation)", n)
	}
}

func TestRefBackend_RunNilOnCancel(t *testing.T) {
	fx := newRefFixture(t)
	w := fx.NewWorker()
	cancel, done := runRef(w)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within bound")
	}
}

// runRef starts w.Run in a goroutine; cancel stops it, done receives its return.
func runRef(w jobs.Worker) (context.CancelFunc, <-chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return cancel, done
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ports/jobs/jobstest/`
Expected: FAIL — build error `undefined: jobstest.DeliveryFixture` / `jobstest.DeliveryBounds` (the types do not exist yet).

- [ ] **Step 3: Write minimal implementation** (types only)

Create `ports/jobs/jobstest/delivery.go`:

```go
package jobstest

import (
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/ports/jobs"
)

// DeliveryBounds are the adapter-declared timing bounds RunDeliveryContract waits
// on. All are REQUIRED; a non-positive value is a t.Fatalf (there is no core
// default — a bound is the adapter's own promise, not something core can guess).
// Per the ports/jobs single-source rule, each MUST be the adapter's own exported
// constant/option value.
type DeliveryBounds struct {
	// ShutdownWithin bounds Run returning nil after its ctx is cancelled.
	ShutdownWithin time.Duration
	// DeliverWithin bounds a freshly-eligible job reaching its handler.
	DeliverWithin time.Duration
	// RedeliverWithin bounds a failed handler attempt being redelivered.
	RedeliverWithin time.Duration
	// ProcessAtDelay is the future offset the suite uses for not-before-ProcessAt
	// tests. The declared value MUST cover the scheduler granularity, the worker
	// poll interval, and the expected backend-clock skew in the test environment;
	// a too-small value makes the not-before assertion flaky.
	ProcessAtDelay time.Duration
}

// DeliveryFixture is one isolated backing store for a delivery subtest: an
// Enqueuer plus a factory for Workers over that SAME store, plus the declared
// bounds. Worker.Run is once-per-instance, so tests that stop one Worker and
// expect a new instance to deliver need NewWorker to spawn a second Worker over
// the same store; a single Worker field cannot express that.
type DeliveryFixture struct {
	// Enqueuer submits jobs to the shared backing store.
	Enqueuer jobs.Enqueuer
	// NewWorker returns a fresh Worker over the SAME backing store as Enqueuer.
	// The fixture MUST configure failed handler attempts to be redelivered within
	// Bounds.RedeliverWithin; the suite assumes at least one retry attempt and a
	// retry delay short enough for the declared bound.
	NewWorker func() jobs.Worker
	// Bounds are this fixture's declared timing bounds.
	Bounds DeliveryBounds
}

// DeliveryFactory returns a FRESH, ISOLATED DeliveryFixture for one subtest: a
// backing store with no jobs and no registrations shared with any other call.
// Register cleanup via t.Cleanup.
type DeliveryFactory func(t *testing.T) DeliveryFixture
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ports/jobs/jobstest/ -run TestRefBackend -v`
Expected: PASS (5 reference-backend unit tests).
Then: `gofmt -l ports/jobs/jobstest/` → no output; `go vet ./ports/jobs/jobstest/` → clean; `go build ./...` → clean.

- [ ] **Step 5: Commit**

```bash
git add ports/jobs/jobstest/delivery.go ports/jobs/jobstest/delivery_test.go
git commit -m "$(cat <<'EOF'
feat(jobstest): add delivery fixture types + wall-clock reference backend

DeliveryBounds/DeliveryFixture/DeliveryFactory (delivery.go) plus a test-only
wall-clock in-memory reference backend (jobs.Enqueuer + jobs.Worker over a
shared store): snapshot-before-submit, ProcessAt eligibility, retry-on-error,
exact-type claim, atomic claim isolation across workers, Run-nil-on-cancel.
Direct unit tests prove each behavior; the delivery suite lands next.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `RunDeliveryContract` suite (11 subtests) + non-vacuity + package doc

**Files:**
- Modify: `ports/jobs/jobstest/delivery.go` (add `RunDeliveryContract` + helpers)
- Modify: `ports/jobs/jobstest/delivery_test.go` (add contract-driving test + non-vacuity)
- Modify: `ports/jobs/jobstest/jobstest.go:1-15` (package doc: point delivery-side invariants at `RunDeliveryContract`)

**Interfaces:**
- Consumes: `jobstest.DeliveryFactory`, `jobstest.DeliveryFixture`, `jobstest.DeliveryBounds`, the reference backend + `newRefFixture` (Task 1); `jobs.*`, `errorsx.CodeOf`, `errorsx.CodeAlreadyExists`.
- Produces: `func jobstest.RunDeliveryContract(t *testing.T, factory DeliveryFactory)`.

- [ ] **Step 1: Prove the suite is non-vacuous (failing test with broken fixtures)**

Append to `ports/jobs/jobstest/delivery_test.go`:

```go
// noRetryFixture is newRefFixture with a Worker that does NOT redeliver a failed
// handler (it drops it) — must make FailedAttemptRedelivered FAIL.
func noRetryFixture(t *testing.T) jobstest.DeliveryFixture {
	t.Helper()
	fx := newRefFixture(t)
	store := fx.Enqueuer.(*refEnqueuer).store
	fx.NewWorker = func() jobs.Worker {
		return &noRetryWorker{refWorker{store: store, handlers: map[string]jobs.Handler{}, poll: 10 * time.Millisecond}}
	}
	return fx
}

type noRetryWorker struct{ refWorker }

func (w *noRetryWorker) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	t := time.NewTicker(w.poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			c, ok := w.store.claim(time.Now(), w.has)
			if !ok {
				continue
			}
			h := w.handlerFor(c.typ)
			if h == nil {
				continue
			}
			go func(c claimed, h jobs.Handler) {
				task := jobs.Task{ID: c.id, Type: c.typ, Payload: c.payload}
				if err := h.Handle(ctx, task); err != nil {
					w.store.complete(c.id) // BUG: drops the failed job instead of retrying
				} else {
					w.store.complete(c.id)
				}
			}(c, h)
		}
	}
}

func TestDeliveryContract_NonVacuous(t *testing.T) {
	// A no-retry backend MUST fail the suite (FailedAttemptRedelivered at least).
	// Run it as an expected-failing subtest and assert it failed via t.Run's bool
	// — this is the robust way to observe a *testing.T-based suite failing.
	ok := t.Run("no-retry-must-fail", func(st *testing.T) {
		jobstest.RunDeliveryContract(st, noRetryFixture)
	})
	if ok {
		t.Fatal("RunDeliveryContract passed against a no-retry backend; the suite is vacuous")
	}
}
```

Run: `go test ./ports/jobs/jobstest/ -run TestDeliveryContract_NonVacuous -v`
Expected: FAIL — build error `undefined: jobstest.RunDeliveryContract` (suite not written yet).

- [ ] **Step 2: Write the suite**

Append to `ports/jobs/jobstest/delivery.go`:

```go
import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/jobs"
)

// (NOTE: merge these imports INTO the existing delivery.go import block — do not
// write a second import(...). The existing block already has "testing", "time",
// and jobs; add "context", "sync", "sync/atomic", and errorsx.)

// runWorker starts w.Run in a goroutine; the returned cancel stops it and done
// receives Run's return value.
func runWorker(w jobs.Worker) (context.CancelFunc, <-chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return cancel, done
}

// assertRunNilWithin asserts Run returned nil within bound.
func assertRunNilWithin(t *testing.T, done <-chan error, bound time.Duration, what string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: Run returned %v, want nil", what, err)
		}
	case <-time.After(bound):
		t.Fatalf("%s: Run did not return within declared bound %v", what, bound)
	}
}

func (b DeliveryBounds) validate(t *testing.T) {
	t.Helper()
	if b.ShutdownWithin <= 0 {
		t.Fatalf("DeliveryBounds.ShutdownWithin = %v, must be > 0", b.ShutdownWithin)
	}
	if b.DeliverWithin <= 0 {
		t.Fatalf("DeliveryBounds.DeliverWithin = %v, must be > 0", b.DeliverWithin)
	}
	if b.RedeliverWithin <= 0 {
		t.Fatalf("DeliveryBounds.RedeliverWithin = %v, must be > 0", b.RedeliverWithin)
	}
	if b.ProcessAtDelay <= 0 {
		t.Fatalf("DeliveryBounds.ProcessAtDelay = %v, must be > 0", b.ProcessAtDelay)
	}
}

// RunDeliveryContract runs the delivery/timing conformance suite against a real,
// running Worker. It starts Workers, waits on the fixture's declared bounds, and
// asserts only what is observable through jobs.Enqueuer / jobs.Worker / a test
// handler — no backend introspection, no fault injection. Recoverability,
// retention, and fault-classification invariants stay in the adapter's own
// tag-gate tests.
func RunDeliveryContract(t *testing.T, factory DeliveryFactory) {
	t.Helper()

	t.Run("FailedAttemptRedelivered", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		var attempts atomic.Int32
		ok := make(chan struct{}, 1)
		mustRegister(t, w, "a:retry", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			if attempts.Add(1) == 1 {
				return errorsx.New(errorsx.CodeInternal, "transient")
			}
			ok <- struct{}{}
			return nil
		}))
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "a shutdown") }()
		mustEnqueue(t, fx, jobs.Job{Type: "a:retry"})
		select {
		case <-ok:
			if attempts.Load() < 2 {
				t.Fatalf("succeeded after %d attempts, want >= 2 (redelivery)", attempts.Load())
			}
		case <-time.After(fx.Bounds.RedeliverWithin):
			t.Fatalf("failed attempt not redelivered within RedeliverWithin %v", fx.Bounds.RedeliverWithin)
		}
	})

	t.Run("NotBeforeProcessAt", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		at := time.Now().Add(fx.Bounds.ProcessAtDelay)
		fired := make(chan time.Time, 1)
		mustRegister(t, w, "b:sched", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			fired <- time.Now()
			return nil
		}))
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "b shutdown") }()
		mustEnqueue(t, fx, jobs.Job{Type: "b:sched", ProcessAt: at})
		select {
		case got := <-fired:
			if got.Before(at) {
				t.Fatalf("dispatched at %v, before ProcessAt %v", got, at)
			}
		case <-time.After(fx.Bounds.ProcessAtDelay + fx.Bounds.DeliverWithin):
			t.Fatal("scheduled job never fired within ProcessAtDelay + DeliverWithin")
		}
	})

	t.Run("PastProcessAtEligible", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		fired := make(chan struct{}, 1)
		mustRegister(t, w, "p:past", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			fired <- struct{}{}
			return nil
		}))
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "p shutdown") }()
		mustEnqueue(t, fx, jobs.Job{Type: "p:past", ProcessAt: time.Now().Add(-time.Hour)})
		select {
		case <-fired:
		case <-time.After(fx.Bounds.DeliverWithin):
			t.Fatal("past-ProcessAt job not delivered within DeliverWithin")
		}
	})

	t.Run("RunReturnsNilOnCancel", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		started := make(chan struct{}, 1)
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		mustRegister(t, w, "c:straggler", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			started <- struct{}{}
			<-release // deliberately ignores ctx: the stuck straggler
			return nil
		}))
		cancel, done := runWorker(w)
		mustEnqueue(t, fx, jobs.Job{Type: "c:straggler"})
		select {
		case <-started:
		case <-time.After(fx.Bounds.DeliverWithin):
			t.Fatal("handler never started")
		}
		cancel() // running worker, in-flight straggler ignoring ctx
		assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "c shutdown")
	})

	t.Run("PayloadMutationIsolated", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		var attempts atomic.Int32
		second := make(chan string, 1)
		mustRegister(t, w, "d:mut", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
			if attempts.Add(1) == 1 {
				for i := range task.Payload {
					task.Payload[i] = 'X'
				}
				return errorsx.New(errorsx.CodeInternal, "transient")
			}
			second <- string(task.Payload)
			return nil
		}))
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "d shutdown") }()
		mustEnqueue(t, fx, jobs.Job{Type: "d:mut", Payload: []byte("orig")})
		select {
		case got := <-second:
			if got != "orig" {
				t.Fatalf("redelivery saw mutated payload %q, want \"orig\"", got)
			}
		case <-time.After(fx.Bounds.RedeliverWithin):
			t.Fatal("no redelivery")
		}
	})

	t.Run("IDStableAcrossRedeliveries", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		var attempts atomic.Int32
		var mu sync.Mutex
		var seen []string
		twice := make(chan struct{}, 1)
		mustRegister(t, w, "e:id", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
			mu.Lock()
			seen = append(seen, task.ID)
			mu.Unlock()
			if attempts.Add(1) == 1 {
				return errorsx.New(errorsx.CodeInternal, "transient")
			}
			twice <- struct{}{}
			return nil
		}))
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "e shutdown") }()
		info := mustEnqueue(t, fx, jobs.Job{Type: "e:id"})
		select {
		case <-twice:
		case <-time.After(fx.Bounds.RedeliverWithin):
			t.Fatal("no redelivery")
		}
		mu.Lock()
		defer mu.Unlock()
		for i, id := range seen {
			if id != info.ID {
				t.Fatalf("delivery %d had ID %q, want stable %q", i, id, info.ID)
			}
		}
	})

	t.Run("HandlerCtxCancelledOnShutdown", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		inHandler := make(chan struct{}, 1)
		ctxErr := make(chan error, 1)
		mustRegister(t, w, "j:wait", jobs.HandlerFunc(func(hctx context.Context, _ jobs.Task) error {
			inHandler <- struct{}{}
			<-hctx.Done()
			ctxErr <- hctx.Err()
			return hctx.Err()
		}))
		cancel, done := runWorker(w)
		mustEnqueue(t, fx, jobs.Job{Type: "j:wait"})
		select {
		case <-inHandler:
		case <-time.After(fx.Bounds.DeliverWithin):
			t.Fatal("handler never started")
		}
		cancel()
		select {
		case err := <-ctxErr:
			if err == nil {
				t.Fatal("handler ctx was not cancelled on Run cancel")
			}
		case <-time.After(fx.Bounds.ShutdownWithin):
			t.Fatal("handler ctx never observed cancellation")
		}
		assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "j shutdown")
	})

	t.Run("ExactTypeDispatchNoPrefix", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		var fires atomic.Int32
		fired := make(chan struct{}, 2)
		mustRegister(t, w, "t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			fires.Add(1)
			fired <- struct{}{}
			return nil
		}))
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "k shutdown") }()
		// exact first: confirm the handler fires, then drain the signal.
		mustEnqueue(t, fx, jobs.Job{Type: "t"})
		select {
		case <-fired:
		case <-time.After(fx.Bounds.DeliverWithin):
			t.Fatal("exact-type job not delivered")
		}
		// prefix-neighbor: must NOT reach the "t" handler.
		mustEnqueue(t, fx, jobs.Job{Type: "t:x"})
		select {
		case <-fired:
			t.Fatal("prefix-neighbor type \"t:x\" was dispatched to the \"t\" handler (want exact-match only)")
		case <-time.After(fx.Bounds.DeliverWithin):
			// good: no second fire within the bound.
		}
		if fires.Load() != 1 {
			t.Fatalf("handler fired %d times, want exactly 1 (only the exact type)", fires.Load())
		}
	})

	t.Run("DuplicateRegisterKeepsOriginal", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		h1 := make(chan struct{}, 1)
		h2 := make(chan struct{}, 1)
		mustRegister(t, w, "o:dup", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			h1 <- struct{}{}
			return nil
		}))
		if code := errorsx.CodeOf(w.Register("o:dup", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			h2 <- struct{}{}
			return nil
		}))); code != errorsx.CodeAlreadyExists {
			t.Fatalf("duplicate Register: code = %v, want CodeAlreadyExists", code)
		}
		cancel, done := runWorker(w)
		defer func() { cancel(); assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "o shutdown") }()
		mustEnqueue(t, fx, jobs.Job{Type: "o:dup"})
		select {
		case <-h1:
		case <-h2:
			t.Fatal("second (rejected) handler received the job; the original must stay installed")
		case <-time.After(fx.Bounds.DeliverWithin):
			t.Fatal("job never delivered to the original handler")
		}
	})

	t.Run("NewWorkerDeliversAfterStop", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		// ProcessAt in the future so w1 (cancelled at once) can never dequeue it.
		mustEnqueue(t, fx, jobs.Job{Type: "r:job", ProcessAt: time.Now().Add(fx.Bounds.ProcessAtDelay)})
		w1 := fx.NewWorker()
		mustRegister(t, w1, "r:job", jobs.HandlerFunc(func(context.Context, jobs.Task) error { return nil }))
		cancel1, done1 := runWorker(w1)
		cancel1() // stop before the scheduled job becomes eligible
		assertRunNilWithin(t, done1, fx.Bounds.ShutdownWithin, "r w1 shutdown")
		w2 := fx.NewWorker()
		got := make(chan struct{}, 1)
		mustRegister(t, w2, "r:job", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
			got <- struct{}{}
			return nil
		}))
		cancel2, done2 := runWorker(w2)
		defer func() { cancel2(); assertRunNilWithin(t, done2, fx.Bounds.ShutdownWithin, "r w2 shutdown") }()
		select {
		case <-got:
		case <-time.After(fx.Bounds.ProcessAtDelay + fx.Bounds.DeliverWithin):
			t.Fatal("new Worker did not deliver the job left by the stopped worker")
		}
	})

	t.Run("ConcurrentEnqueueSmoke", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		mustRegister(t, w, "l:job", jobs.HandlerFunc(func(context.Context, jobs.Task) error { return nil }))
		cancel, done := runWorker(w)
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = fx.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "l:job"})
			}()
		}
		wg.Wait()
		cancel()
		assertRunNilWithin(t, done, fx.Bounds.ShutdownWithin, "l shutdown")
	})
}

func mustRegister(t *testing.T, w jobs.Worker, jobType string, h jobs.Handler) {
	t.Helper()
	if err := w.Register(jobType, h); err != nil {
		t.Fatalf("Register(%q): %v", jobType, err)
	}
}

func mustEnqueue(t *testing.T, fx DeliveryFixture, job jobs.Job) jobs.JobInfo {
	t.Helper()
	info, err := fx.Enqueuer.Enqueue(context.Background(), job)
	if err != nil {
		t.Fatalf("Enqueue(%q): %v", job.Type, err)
	}
	return info
}
```

- [ ] **Step 3: Add the passing (GREEN) contract test against the reference**

Append to `ports/jobs/jobstest/delivery_test.go`:

```go
func TestReferenceBackendSatisfiesDeliveryContract(t *testing.T) {
	jobstest.RunDeliveryContract(t, newRefFixture)
}
```

- [ ] **Step 4: Run the non-vacuity + GREEN tests**

Run: `go test ./ports/jobs/jobstest/ -run 'TestDeliveryContract_NonVacuous|TestReferenceBackendSatisfiesDeliveryContract' -v`
Expected:
- `TestDeliveryContract_NonVacuous` PASS (its inner `no-retry-must-fail` subtest FAILS as designed → the outer test asserts that and passes).
- `TestReferenceBackendSatisfiesDeliveryContract` PASS (all 11 subtests green against the reference).

- [ ] **Step 5: Update the package doc**

Modify `ports/jobs/jobstest/jobstest.go:1-15` — replace the "Delivery-side invariants … are NOT in this suite" paragraph:

Old:
```go
// Delivery-side invariants (payload snapshot isolation, exact-type dispatch,
// at-least-once redelivery, shutdown recoverability) need a running worker
// and backend-specific observation, so they are NOT in this suite: adapters
// must cover them with their own intent tests (see the tag-gate acceptance
// criteria recorded in the repository's .agent/decisions.md), and core
// demonstrates them against a local fake in ports/jobs's own tests.
```

New:
```go
// The interface-observable half of the delivery/timing invariants (failed-attempt
// redelivery, not-before-ProcessAt, exact-type dispatch, payload-copy isolation,
// stable Task.ID, handler-ctx-cancelled-on-shutdown, Run-returns-nil-on-cancel,
// per-key isolation across workers) lives in RunDeliveryContract, which runs a
// real Worker against adapter-declared timing bounds. Introspection- and
// fault-injection-bound invariants (recoverable-state classification, retention,
// accepted-but-ack-lost, unreachable backend, unhandled-job policy) remain in the
// adapter's own tag-gate intent tests (see .agent/decisions.md); core demonstrates
// them against a local fake in ports/jobs's own tests.
```

- [ ] **Step 6: Run the full package + repo checks**

Run: `go test ./ports/jobs/...`
Expected: PASS (existing `RunContract` tests + reference unit tests + `RunDeliveryContract` + non-vacuity).
Then: `gofmt -l ports/jobs/` → no output; `go vet ./ports/jobs/...`; `go build ./...` → clean.

- [ ] **Step 7: Commit**

```bash
git add ports/jobs/jobstest/delivery.go ports/jobs/jobstest/delivery_test.go ports/jobs/jobstest/jobstest.go
git commit -m "$(cat <<'EOF'
feat(jobstest): add RunDeliveryContract delivery/timing conformance suite

Eleven interface-observable delivery subtests driven against a real Worker with
adapter-declared bounds (ShutdownWithin/DeliverWithin/RedeliverWithin/
ProcessAtDelay): FailedAttemptRedelivered, NotBeforeProcessAt, PastProcessAt-
Eligible, RunReturnsNilOnCancel (ctx-ignoring straggler + liveness),
PayloadMutationIsolated, IDStableAcrossRedeliveries, HandlerCtxCancelledOnShutdown,
ExactTypeDispatchNoPrefix (exact-first, no unhandled-policy dependency),
DuplicateRegisterKeepsOriginal, NewWorkerDeliversAfterStop, ConcurrentEnqueueSmoke.
No introspection, no fault injection. Validated against the reference backend; a
no-retry backend proves the suite is non-vacuous. Package doc updated.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**1. Spec coverage** (against `2026-07-01-jobstest-delivery-suite-design.md`):

| Spec section | Task |
|---|---|
| §2 `DeliveryBounds`/`DeliveryFixture`/`DeliveryFactory` (incl. `ProcessAtDelay`, `NewWorker`) | Task 1 Step 3 |
| §3 fixture preconditions (isolated store, shared store, retry-enabled) | Task 1 (reference honours them) + `delivery.go` doc comments |
| §4 11 subtests with mechanics (incl. `FailedAttemptRedelivered` rename, exact-first (k), straggler (c)) | Task 2 Step 2 |
| §5 determinism (channel-signal + select-with-bound; `assertRunNilWithin` at teardown) | Task 2 Step 2 helpers |
| §6 wall-clock reference backend + non-vacuity via broken backend | Task 1 (reference) + Task 2 Step 1 (no-retry non-vacuity) |
| §7 file organisation | Task 1 & 2 file paths |
| §8 verification (gofmt/vet/build/test) | Task 1 Step 4, Task 2 Step 6 |
| package doc update | Task 2 Step 5 |

No gaps. (Excluded criteria g/t/f/h/n/s/u are intentionally not tasks — they stay adapter-side per §1.)

**2. Placeholder scan:** No TBD/TODO. Non-vacuity uses the `t.Run`-returns-bool form (an expected-failing subtest asserted via the returned bool). All steps have complete code. The Task 2 Step 2 import block is a MERGE into the existing `delivery.go` block (existing: `testing`, `time`, `jobs`; add `context`, `sync`, `sync/atomic`, `errorsx`) — not a second `import (...)`.

**3. Type consistency:** `DeliveryBounds`/`DeliveryFixture`/`DeliveryFactory`/`RunDeliveryContract`/`newRefFixture`/`runWorker`/`assertRunNilWithin`/`mustRegister`/`mustEnqueue`/`refStore`/`refWorker`/`claimed` spelled identically across Task 1 → Task 2. `runRef` (Task 1 test helper) and `runWorker` (Task 2 suite helper) are distinct on purpose: `runRef` lives in the test file for the direct unit tests; `runWorker` is the suite's own helper in `delivery.go`.
