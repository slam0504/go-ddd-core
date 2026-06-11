package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/pkg/errorsx/httpx"
	"github.com/slam0504/go-ddd-core/ports/jobs"
)

// These tests are the contract's executable specification: a deliberately
// small in-memory implementation (fakeStore + fakeWorker) demonstrates that
// the API can be implemented correctly and shows the intended delivery
// semantics. They do NOT enforce the public delivery contract on adapters —
// that is the tag-gate's job (adapter intent tests; see .agent/decisions.md).

// fakeClock is the fake's single scheduling clock: ProcessAt eligibility and
// lease expiry are both judged on it, so timing tests advance it instead of
// sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_000_000, 0).UTC()} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type jobState int

const (
	statePending jobState = iota
	stateActive
	stateCompleted
)

type storedJob struct {
	id         string
	typ        string
	payload    []byte
	processAt  time.Time
	state      jobState
	leaseUntil time.Time
	attempts   int
}

// fakeStore implements jobs.Enqueuer and is the shared backing store for any
// number of fakeWorker instances (restart tests construct a NEW worker over
// the same store, honouring Run-once-per-instance).
type fakeStore struct {
	clock    *fakeClock
	leaseTTL time.Duration
	horizon  time.Duration // 0 = unbounded

	mu            sync.Mutex
	seq           int
	jobs          map[string]*storedJob
	unavailable   bool
	acceptButFail bool // accepted-but-ack-lost fault: job stored, Enqueue errors
}

func newFakeStore(clock *fakeClock) *fakeStore {
	return &fakeStore{clock: clock, leaseTTL: time.Minute, jobs: map[string]*storedJob{}}
}

func (s *fakeStore) Enqueue(ctx context.Context, job jobs.Job) (jobs.JobInfo, error) {
	// Class 1 runs before observing ctx or backend state.
	if job.Type == "" {
		return jobs.JobInfo{}, errorsx.New(errorsx.CodeInvalidArgument, "jobs: empty job type")
	}
	if s.horizon > 0 && !job.ProcessAt.IsZero() && job.ProcessAt.After(s.clock.Now().Add(s.horizon)) {
		return jobs.JobInfo{}, errorsx.New(errorsx.CodeInvalidArgument, "jobs: ProcessAt beyond scheduling horizon")
	}
	// snapshot-before-submit: the copy exists before any backend interaction,
	// so even an accepted-but-ack-lost job is isolated from caller mutation.
	payload := append([]byte(nil), job.Payload...)
	// Class 2a entry check precedes any backend contact.
	if err := ctx.Err(); err != nil {
		return jobs.JobInfo{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return jobs.JobInfo{}, errorsx.New(errorsx.CodeUnavailable, "jobs: backend unavailable")
	}
	s.seq++
	id := strconv.Itoa(s.seq)
	s.jobs[id] = &storedJob{id: id, typ: job.Type, payload: payload, processAt: job.ProcessAt, state: statePending}
	if s.acceptButFail {
		return jobs.JobInfo{}, errorsx.New(errorsx.CodeUnavailable, "jobs: backend accepted but the enqueue ack was lost")
	}
	return jobs.JobInfo{ID: id}, nil
}

// dequeue reclaims expired leases, then leases out one eligible pending job.
func (s *fakeStore) dequeue() *storedJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	for _, j := range s.jobs {
		if j.state == stateActive && now.After(j.leaseUntil) {
			j.state = statePending
		}
	}
	for _, j := range s.jobs {
		if j.state == statePending && (j.processAt.IsZero() || !j.processAt.After(now)) {
			j.state = stateActive
			j.leaseUntil = now.Add(s.leaseTTL)
			j.attempts++
			return j
		}
	}
	return nil
}

func (s *fakeStore) complete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.state = stateCompleted
	}
}

func (s *fakeStore) requeue(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok && j.state == stateActive {
		j.state = statePending
	}
}

func (s *fakeStore) jobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

func (s *fakeStore) stateOf(id string) (jobState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return 0, false
	}
	return j.state, true
}

func (s *fakeStore) onlyJobID(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.jobs) != 1 {
		t.Fatalf("store holds %d jobs, want exactly 1", len(s.jobs))
	}
	for id := range s.jobs {
		return id
	}
	return ""
}

func (s *fakeStore) setUnavailable(v bool) {
	s.mu.Lock()
	s.unavailable = v
	s.mu.Unlock()
}

// fakeWorker implements jobs.Worker over a shared fakeStore. Each instance is
// Run at most once; restarts construct a new instance over the same store.
type fakeWorker struct {
	store *fakeStore
	// shutdownWithin is the fake's DECLARED shutdown bound: Run must return
	// within it after its ctx is cancelled (mirrors ReclaimWithin's
	// adapter-declared-bound pattern; tests t.Fatal on a non-positive value).
	shutdownWithin  time.Duration
	fatalStartup    error // non-nil: Run surfaces this coded fatal (endpoint B)
	teardownFail    bool  // shutdown-path backend error: recorded, never returned
	dequeueDisabled bool  // simulates a worker that stops before any dequeue

	running chan struct{} // closed once Run's dispatch loop has started

	mu           sync.Mutex
	handlers     map[string]jobs.Handler
	teardownErrs []string // the adapter-level channel (would be logs/metrics)
}

func newFakeWorker(store *fakeStore) *fakeWorker {
	return &fakeWorker{
		store:          store,
		shutdownWithin: 2 * time.Second,
		handlers:       map[string]jobs.Handler{},
		running:        make(chan struct{}),
	}
}

func (w *fakeWorker) Register(jobType string, h jobs.Handler) error {
	// Argument validation first, only then the duplicate check.
	if jobType == "" {
		return errorsx.New(errorsx.CodeInvalidArgument, "jobs: empty job type")
	}
	if h == nil {
		return errorsx.New(errorsx.CodeInvalidArgument, "jobs: nil handler")
	}
	if _, ok := w.handlers[jobType]; ok {
		return errorsx.New(errorsx.CodeAlreadyExists, "jobs: handler already registered for type "+jobType)
	}
	w.handlers[jobType] = h
	return nil
}

func (w *fakeWorker) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		// Contract endpoint (A): cancellation is the expected stop signal, so a
		// pre-cancelled ctx returns nil without starting.
		return nil //nolint:nilerr
	}
	if w.fatalStartup != nil {
		return w.fatalStartup
	}
	hctx, hcancel := context.WithCancel(context.Background())
	var inflight sync.WaitGroup
	close(w.running)
	for {
		select {
		case <-ctx.Done():
			hcancel() // handler ctx is cancelled when shutdown begins
			drained := make(chan struct{})
			go func() { inflight.Wait(); close(drained) }()
			select { // bounded best-effort drain: a stuck handler never blocks Run
			case <-drained:
			case <-time.After(100 * time.Millisecond):
			}
			if w.teardownFail {
				// A shutdown-path backend error goes to the adapter-level
				// channel; it never changes the nil return.
				w.mu.Lock()
				w.teardownErrs = append(w.teardownErrs, "teardown: backend error during requeue")
				w.mu.Unlock()
			}
			return nil
		default:
		}
		var j *storedJob
		if !w.dequeueDisabled {
			j = w.store.dequeue()
		}
		if j == nil {
			time.Sleep(time.Millisecond)
			continue
		}
		inflight.Add(1)
		go func(j *storedJob) {
			defer inflight.Done()
			w.mu.Lock()
			h := w.handlers[j.typ] // exact match only — no prefix routing
			w.mu.Unlock()
			if h == nil {
				// Unhandled type: never acked as success. Fake policy: leave it
				// leased; lease expiry makes it pending again.
				return
			}
			// Every delivery hands the handler its own private copy.
			payload := append([]byte(nil), j.payload...)
			if err := h.Handle(hctx, jobs.Task{ID: j.id, Type: j.typ, Payload: payload}); err != nil {
				w.store.requeue(j.id) // any non-nil result is a failed attempt
			} else {
				w.store.complete(j.id) // nil acks; late acks land here too
			}
		}(j)
	}
}

// --- helpers ---

func startWorker(t *testing.T, w *fakeWorker) (cancel func(), done chan error) {
	t.Helper()
	if w.shutdownWithin <= 0 {
		t.Fatalf("declared ShutdownWithin must be positive, got %v", w.shutdownWithin)
	}
	ctx, c := context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return c, done
}

func stopWorker(t *testing.T, w *fakeWorker, cancel func(), done chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v after cancel, want nil", err)
		}
	case <-time.After(w.shutdownWithin):
		t.Fatalf("Run did not return within its declared ShutdownWithin %v", w.shutdownWithin)
	}
}

func expectDelivery(t *testing.T, ch <-chan jobs.Task, what string) jobs.Task {
	t.Helper()
	select {
	case task := <-ch:
		return task
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: no delivery within 5s", what)
		return jobs.Task{}
	}
}

func expectNoDelivery(t *testing.T, ch <-chan jobs.Task, window time.Duration, what string) {
	t.Helper()
	select {
	case task := <-ch:
		t.Fatalf("%s: unexpected delivery of task %q", what, task.ID)
	case <-time.After(window):
	}
}

// --- Enqueue surface (delivery-independent, fault-injected) ---

func TestMalformedEnqueueWritesNothing(t *testing.T) {
	store := newFakeStore(newFakeClock())
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: ""}); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
		t.Fatalf("err = %v, want CodeInvalidArgument", err)
	}
	if n := store.jobCount(); n != 0 {
		t.Fatalf("malformed Enqueue wrote %d jobs into the backend, want 0", n)
	}
}

func TestBackendUnavailable(t *testing.T) {
	store := newFakeStore(newFakeClock())
	store.setUnavailable(true)
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: []byte("p")})
	if errorsx.CodeOf(err) != errorsx.CodeUnavailable {
		t.Fatalf("err = %v, want CodeUnavailable", err)
	}
	if info.ID != "" {
		t.Fatalf("error return carried JobInfo.ID %q, want zero value", info.ID)
	}
}

// TestErrorCodesTranslateToHTTP is the package's single httpx importer: it
// pins the numeric mapping of every code this contract fixes.
func TestErrorCodesTranslateToHTTP(t *testing.T) {
	for _, tc := range []struct {
		code errorsx.Code
		want int
	}{
		{errorsx.CodeInvalidArgument, 400},
		{errorsx.CodeAlreadyExists, 409},
		{errorsx.CodeUnavailable, 503},
	} {
		if _, status := httpx.Translate(errorsx.New(tc.code, "boom")); status != tc.want {
			t.Errorf("Translate(%v) = %d, want %d", tc.code, status, tc.want)
		}
	}
}

func TestEnqueueCtxCancellationPrecedesBackendError(t *testing.T) {
	store := newFakeStore(newFakeClock())
	store.setUnavailable(true) // backend is down AND ctx is already cancelled

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	info, err := store.Enqueue(ctx, jobs.Job{Type: "t"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled — (2a) ctx precedes (2b) backend", err)
	}
	if info.ID != "" {
		t.Fatalf("error return carried JobInfo.ID %q", info.ID)
	}

	dctx, dcancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer dcancel()
	info, err = store.Enqueue(dctx, jobs.Job{Type: "t"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded — both ctx error kinds share the entry check", err)
	}
	if info.ID != "" {
		t.Fatalf("error return carried JobInfo.ID %q", info.ID)
	}
}

func TestEnqueueRejectsOutOfHorizonProcessAt(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	store.horizon = time.Hour
	tooFar := clock.Now().Add(2 * time.Hour)

	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", ProcessAt: tooFar})
	if errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
		t.Fatalf("out-of-horizon ProcessAt: err = %v, want CodeInvalidArgument — reject at Enqueue, never accept-then-drop", err)
	}
	if info.ID != "" {
		t.Fatalf("error return carried JobInfo.ID %q", info.ID)
	}
	if n := store.jobCount(); n != 0 {
		t.Fatalf("rejected job was written anyway (%d jobs)", n)
	}

	// Class 1 precedes ctx: even a cancelled ctx yields the validation code.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Enqueue(ctx, jobs.Job{Type: "t", ProcessAt: tooFar}); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
		t.Fatalf("out-of-horizon + cancelled ctx: err = %v, want CodeInvalidArgument", err)
	}
}

func TestEnqueuePayloadSnapshotOnIndeterminateError(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	store.acceptButFail = true

	payload := []byte("original")
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: payload})
	if err == nil {
		t.Fatal("expected the injected accepted-but-ack-lost error")
	}
	// The indeterminate error must follow class-2 rules: a ctx error or a
	// coded errorsx that is never CodeUnknown.
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		errorsx.CodeOf(err) == errorsx.CodeUnknown {
		t.Fatalf("indeterminate enqueue error %v is neither a ctx error nor a coded errorsx", err)
	}
	if info.ID != "" {
		t.Fatalf("error return carried JobInfo.ID %q, want zero value", info.ID)
	}

	// The job WAS accepted; mutating the caller's slice afterwards must not
	// corrupt it (snapshot happened before backend I/O).
	copy(payload, "XXXXXXXX")
	store.acceptButFail = false
	id := store.onlyJobID(t)

	got := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	task := expectDelivery(t, got, "accepted-but-ack-lost job")
	if string(task.Payload) != "original" {
		t.Fatalf("delivered payload %q, want %q — snapshot-before-submit must isolate accepted jobs on error returns", task.Payload, "original")
	}
	if task.ID != id {
		t.Fatalf("Task.ID = %q, want stored id %q", task.ID, id)
	}
	stopWorker(t, w, cancel, done)
}

// --- delivery semantics (running the fake worker) ---

func TestDeliversCorrectTask(t *testing.T) {
	store := newFakeStore(newFakeClock())
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: []byte("payload")})
	if err != nil {
		t.Fatal(err)
	}

	got := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	task := expectDelivery(t, got, "enqueued job")
	if task.Type != "t" || string(task.Payload) != "payload" {
		t.Fatalf("delivered Task = %+v, want Type t / payload %q", task, "payload")
	}
	if task.ID == "" || task.ID != info.ID {
		t.Fatalf("Task.ID = %q, want non-empty JobInfo.ID %q", task.ID, info.ID)
	}
	stopWorker(t, w, cancel, done)
}

func TestEnqueuePayloadSnapshot(t *testing.T) {
	store := newFakeStore(newFakeClock())
	payload := []byte("original")
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	copy(payload, "XXXXXXXX") // caller mutates after Enqueue returned

	got := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	if task := expectDelivery(t, got, "snapshot job"); string(task.Payload) != "original" {
		t.Fatalf("delivered payload %q, want %q — Enqueue must snapshot the caller's bytes", task.Payload, "original")
	}
	stopWorker(t, w, cancel, done)
}

func TestNilAndEmptyPayloadDeliverZeroLength(t *testing.T) {
	store := newFakeStore(newFakeClock())
	for _, payload := range [][]byte{nil, {}} {
		if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: payload}); err != nil {
			t.Fatalf("Enqueue(%v): %v", payload, err)
		}
	}

	got := make(chan jobs.Task, 2)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	for i := 0; i < 2; i++ {
		// nil and empty are equivalent; only len==0 is observable, nil-ness
		// is deliberately unspecified.
		if task := expectDelivery(t, got, "zero-length payload job"); len(task.Payload) != 0 {
			t.Fatalf("delivered payload %q, want zero length", task.Payload)
		}
	}
	stopWorker(t, w, cancel, done)
}

func TestHandlerFuncAdapts(t *testing.T) {
	called := false
	var h jobs.Handler = jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		called = true
		return nil
	})
	if err := h.Handle(context.Background(), jobs.Task{}); err != nil || !called {
		t.Fatalf("HandlerFunc adaption failed: err=%v called=%v", err, called)
	}
}

func TestExactTypeMatchNoPrefix(t *testing.T) {
	store := newFakeStore(newFakeClock())
	emailed := make(chan jobs.Task, 1)
	wrong := make(chan jobs.Task, 1)

	w := newFakeWorker(store)
	if err := w.Register("email", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		emailed <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := w.Register("email:send", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		wrong <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "email"}); err != nil {
		t.Fatal(err)
	}

	cancel, done := startWorker(t, w)
	expectDelivery(t, emailed, `"email" job`)
	expectNoDelivery(t, wrong, 50*time.Millisecond, `"email:send" handler (prefix routing would hit it)`)
	stopWorker(t, w, cancel, done)
}

func TestDuplicateRegisterKeepsOriginalHandler(t *testing.T) {
	store := newFakeStore(newFakeClock())
	original := make(chan jobs.Task, 1)
	usurper := make(chan jobs.Task, 1)

	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		original <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if code := errorsx.CodeOf(w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		usurper <- task
		return nil
	}))); code != errorsx.CodeAlreadyExists {
		t.Fatalf("duplicate Register code = %v, want CodeAlreadyExists", code)
	}
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatal(err)
	}

	cancel, done := startWorker(t, w)
	expectDelivery(t, original, "original handler")
	expectNoDelivery(t, usurper, 50*time.Millisecond, "second handler (silent replace would route here)")
	stopWorker(t, w, cancel, done)
}

func TestUnhandledTypeNotAckedAsSuccess(t *testing.T) {
	store := newFakeStore(newFakeClock())
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "unregistered"})
	if err != nil {
		t.Fatal(err)
	}

	w := newFakeWorker(store) // nothing registered
	cancel, done := startWorker(t, w)
	time.Sleep(20 * time.Millisecond) // let the worker dequeue it
	stopWorker(t, w, cancel, done)

	if state, ok := store.stateOf(info.ID); !ok || state == stateCompleted {
		t.Fatalf("unhandled job state = %v ok=%v; it must never be acked as success", state, ok)
	}
}

// --- timing semantics (injected clock — zero real waiting on the schedule) ---

func TestDelayedNotBeforeProcessAt(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", ProcessAt: clock.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	got := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	// The injected clock IS the backend's scheduling clock (the single clock
	// the "no earlier than" MUST is judged on).
	expectNoDelivery(t, got, 30*time.Millisecond, "job before its ProcessAt")
	clock.Advance(time.Hour + time.Second)
	expectDelivery(t, got, "job after its ProcessAt")
	stopWorker(t, w, cancel, done)
}

func TestPastProcessAtIsImmediatelyEligible(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", ProcessAt: clock.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatalf("a past ProcessAt is never an error, got %v", err)
	}

	got := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	if task := expectDelivery(t, got, "past-ProcessAt job"); task.ID != info.ID {
		t.Fatalf("Task.ID = %q, want %q", task.ID, info.ID)
	}
	stopWorker(t, w, cancel, done)
}

func TestScheduledJobRetainedPastProcessAtWithoutWorker(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", ProcessAt: clock.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// ProcessAt passes while NO worker exists; the job must not expire.
	clock.Advance(2 * time.Hour)

	got := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	expectDelivery(t, got, "job whose ProcessAt passed with no worker available")
	stopWorker(t, w, cancel, done)
}

func TestJobRetainedWhenNoWorkerThenDeliveredOnRestart(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatal(err)
	}

	// Variant: a worker runs but stops before dequeuing anything — that only
	// suspends prerequisite (2); the job stays pending.
	w1 := newFakeWorker(store)
	w1.dequeueDisabled = true
	if err := w1.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error { return nil })); err != nil {
		t.Fatal(err)
	}
	cancel1, done1 := startWorker(t, w1)
	time.Sleep(10 * time.Millisecond)
	stopWorker(t, w1, cancel1, done1)

	// A brand-new Worker instance over the SAME store (Run is once per
	// instance) picks the retained job up.
	got := make(chan jobs.Task, 1)
	w2 := newFakeWorker(store)
	if err := w2.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		got <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel2, done2 := startWorker(t, w2)
	expectDelivery(t, got, "job retained across the worker gap")
	stopWorker(t, w2, cancel2, done2)
}

// --- redelivery semantics ---

func TestAtLeastOnceRedelivery(t *testing.T) {
	store := newFakeStore(newFakeClock())
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"})
	if err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	ids := make(chan string, 2)
	donec := make(chan struct{}, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		ids <- task.ID
		if attempts.Add(1) == 1 {
			return fmt.Errorf("transient failure") // first attempt fails
		}
		donec <- struct{}{}
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	select {
	case <-donec:
	case <-time.After(5 * time.Second):
		t.Fatal("job was not redelivered after a failed attempt")
	}
	if n := attempts.Load(); n < 2 {
		t.Fatalf("attempts = %d, want >= 2", n)
	}
	for i := 0; i < 2; i++ {
		if id := <-ids; id != info.ID {
			t.Fatalf("delivery %d carried Task.ID %q, want stable JobInfo.ID %q", i, id, info.ID)
		}
	}
	stopWorker(t, w, cancel, done)
}

func TestHandlerMutationIsolation(t *testing.T) {
	store := newFakeStore(newFakeClock())
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t", Payload: []byte("original")}); err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	second := make(chan jobs.Task, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(_ context.Context, task jobs.Task) error {
		if attempts.Add(1) == 1 {
			copy(task.Payload, "XXXXXXXX") // mutate the delivered copy, then fail
			return fmt.Errorf("fail to force a redelivery")
		}
		second <- task
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	if task := expectDelivery(t, second, "redelivery after handler mutation"); string(task.Payload) != "original" {
		t.Fatalf("redelivered payload %q, want %q — Task.Payload is a private copy per delivery", task.Payload, "original")
	}
	stopWorker(t, w, cancel, done)
}

func TestHandlerResultRecognized(t *testing.T) {
	store := newFakeStore(newFakeClock())
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"})
	if err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	acked := make(chan struct{}, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		if attempts.Add(1) == 1 {
			return fmt.Errorf("not done yet") // non-nil is NOT treated as success
		}
		acked <- struct{}{}
		return nil // nil is recognized as completion
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	select {
	case <-acked:
	case <-time.After(5 * time.Second):
		t.Fatal("nil result was not recognized as completion")
	}
	stopWorker(t, w, cancel, done)
	if state, _ := store.stateOf(info.ID); state != stateCompleted {
		t.Fatalf("job state = %v after nil result, want completed", state)
	}
}

func TestHandlerCtxErrorIsFailedAttempt(t *testing.T) {
	store := newFakeStore(newFakeClock())
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	redelivered := make(chan struct{}, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		if attempts.Add(1) == 1 {
			// A handler returning a wrapped ctx error is an ORDINARY failed
			// attempt — the contract does not special-case it.
			return fmt.Errorf("aborted: %w", context.Canceled)
		}
		redelivered <- struct{}{}
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	select {
	case <-redelivered:
	case <-time.After(5 * time.Second):
		t.Fatal("task whose handler returned a ctx error was not redelivered")
	}
	if n := attempts.Load(); n < 2 {
		t.Fatalf("attempts = %d, want >= 2", n)
	}
	stopWorker(t, w, cancel, done)
}

// --- Run lifecycle ---

func TestRunReturnsNilOnCancel(t *testing.T) {
	store := newFakeStore(newFakeClock())
	w := newFakeWorker(store)
	cancel, done := startWorker(t, w)
	stopWorker(t, w, cancel, done) // asserts nil within declared ShutdownWithin

	// Already-cancelled ctx: Run returns nil without starting.
	w2 := newFakeWorker(store)
	ctx, c := context.WithCancel(context.Background())
	c()
	if err := w2.Run(ctx); err != nil {
		t.Fatalf("Run with pre-cancelled ctx returned %v, want nil", err)
	}
}

func TestRunTwoEndpoints(t *testing.T) {
	store := newFakeStore(newFakeClock())

	// Endpoint (A): clean cancellation → nil (covered via stopWorker).
	wA := newFakeWorker(store)
	cancel, done := startWorker(t, wA)
	stopWorker(t, wA, cancel, done)

	// Endpoint (B): an independent fatal → a coded error, never a ctx error.
	wB := newFakeWorker(store)
	wB.fatalStartup = errorsx.New(errorsx.CodeUnavailable, "jobs: backend unreachable at startup")
	err := wB.Run(context.Background())
	if err == nil {
		t.Fatal("Run with a fatal returned nil")
	}
	if code := errorsx.CodeOf(err); code == errorsx.CodeUnknown {
		t.Fatalf("fatal error code = CodeUnknown (err=%v); fatals must carry a real code", err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fatal error %v satisfies a ctx error; the coded-vs-ctx split must stay observable", err)
	}
}

func TestRunReturnsCodedErrorOnFatalStartup(t *testing.T) {
	store := newFakeStore(newFakeClock())
	w := newFakeWorker(store)
	w.fatalStartup = errorsx.New(errorsx.CodeInvalidArgument, "jobs: misconfigured worker")
	err := w.Run(context.Background())
	if errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
		t.Fatalf("err = %v, want the declared coded fatal", err)
	}
}

func TestRunReturnsNilEvenIfTeardownFails(t *testing.T) {
	store := newFakeStore(newFakeClock())
	w := newFakeWorker(store)
	w.teardownFail = true
	cancel, done := startWorker(t, w)
	<-w.running                    // ensure shutdown goes through the teardown path, not the pre-start fast path
	stopWorker(t, w, cancel, done) // nil within ShutdownWithin despite the teardown error

	w.mu.Lock()
	recorded := len(w.teardownErrs)
	w.mu.Unlock()
	if recorded == 0 {
		t.Fatal("teardown error was not reported through the adapter-level channel")
	}
}

func TestHandlerCtxCancelledOnShutdown(t *testing.T) {
	store := newFakeStore(newFakeClock())
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, 1)
	observed := make(chan struct{}, 1)
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(ctx context.Context, _ jobs.Task) error {
		started <- struct{}{}
		<-ctx.Done() // a well-behaved long handler waits on its ctx
		observed <- struct{}{}
		return ctx.Err()
	})); err != nil {
		t.Fatal(err)
	}
	cancel, done := startWorker(t, w)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started")
	}
	cancel()
	select {
	case <-observed:
	case <-time.After(w.shutdownWithin):
		t.Fatal("handler ctx was not cancelled when the worker began shutting down")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(w.shutdownWithin):
		t.Fatal("Run did not return within its declared ShutdownWithin")
	}
}

func TestStuckHandlerDoesNotBlockRunAndTaskRedelivered(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	if _, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"}); err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	started := make(chan struct{}, 1)
	redelivered := make(chan struct{}, 1)
	release := make(chan struct{})
	w := newFakeWorker(store)
	if err := w.Register("t", jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		if attempts.Add(1) == 1 {
			started <- struct{}{}
			<-release // ignores its cancelled ctx: the stuck straggler
			return nil
		}
		redelivered <- struct{}{}
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { close(release) })

	cancel, done := startWorker(t, w)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first delivery did not happen")
	}
	// Expire the stuck attempt's lease (the fake redelivers while the first
	// attempt is still running — the allowed concurrent-duplicate case).
	clock.Advance(store.leaseTTL + time.Second)
	select {
	case <-redelivered:
	case <-time.After(5 * time.Second):
		t.Fatal("un-acked task was not redelivered after lease expiry")
	}
	// Liveness: the still-stuck first handler must not keep Run from returning.
	stopWorker(t, w, cancel, done)
}

// --- shutdown recoverability: the two legal endings, separately ---

func TestInFlightUnackedAtShutdown_LateAckCompletes(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"})
	if err != nil {
		t.Fatal(err)
	}

	var deliveries atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	handler := jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		deliveries.Add(1)
		started <- struct{}{}
		<-release
		return nil // the late ack
	})

	w1 := newFakeWorker(store)
	if err := w1.Register("t", handler); err != nil {
		t.Fatal(err)
	}
	cancel1, done1 := startWorker(t, w1)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first delivery did not happen")
	}
	stopWorker(t, w1, cancel1, done1) // Run returns while the handler is still blocked

	// NOW release the straggler: its late ack lands atomically.
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if state, _ := store.stateOf(info.ID); state == stateCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("late ack did not complete the job")
		}
		time.Sleep(time.Millisecond)
	}

	// Completed is a legal terminal: a fresh worker must NOT redeliver it,
	// even after the old lease would have expired.
	clock.Advance(store.leaseTTL + time.Second)
	w2 := newFakeWorker(store)
	if err := w2.Register("t", handler); err != nil {
		t.Fatal(err)
	}
	cancel2, done2 := startWorker(t, w2)
	time.Sleep(50 * time.Millisecond)
	stopWorker(t, w2, cancel2, done2)
	if n := deliveries.Load(); n != 1 {
		t.Fatalf("deliveries = %d, want exactly 1 — a completed job must not be redelivered", n)
	}
}

func TestInFlightUnackedAtShutdown_NoAckIsRedelivered(t *testing.T) {
	clock := newFakeClock()
	store := newFakeStore(clock)
	info, err := store.Enqueue(context.Background(), jobs.Job{Type: "t"})
	if err != nil {
		t.Fatal(err)
	}

	var deliveries atomic.Int32
	started := make(chan struct{}, 1)
	redelivered := make(chan struct{}, 1)
	release := make(chan struct{})
	handler := jobs.HandlerFunc(func(context.Context, jobs.Task) error {
		if deliveries.Add(1) == 1 {
			started <- struct{}{}
			<-release // never acks during the test's first phase
			return nil
		}
		redelivered <- struct{}{}
		return nil
	})
	t.Cleanup(func() { close(release) })

	w1 := newFakeWorker(store)
	if err := w1.Register("t", handler); err != nil {
		t.Fatal(err)
	}
	cancel1, done1 := startWorker(t, w1)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first delivery did not happen")
	}
	stopWorker(t, w1, cancel1, done1)

	// The un-acked job is leased (the transient state); it must NOT be lost.
	if state, ok := store.stateOf(info.ID); !ok || state == stateCompleted {
		t.Fatalf("job state = %v ok=%v right after shutdown; want a recoverable state", state, ok)
	}
	// Lease expiry resolves the transient state to retryable; a brand-new
	// Worker instance over the same store actually redelivers.
	clock.Advance(store.leaseTTL + time.Second)
	w2 := newFakeWorker(store)
	if err := w2.Register("t", handler); err != nil {
		t.Fatal(err)
	}
	cancel2, done2 := startWorker(t, w2)
	select {
	case <-redelivered:
	case <-time.After(5 * time.Second):
		t.Fatal("un-acked in-flight task was not redelivered by the new worker")
	}
	stopWorker(t, w2, cancel2, done2)
}
