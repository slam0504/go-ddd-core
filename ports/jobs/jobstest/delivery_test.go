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
	store       *refStore
	mu          sync.Mutex
	handlers    map[string]jobs.Handler
	retryDelay  time.Duration
	poll        time.Duration
	dropOnError bool // test knob: if true, a failed handler is dropped (no retry)
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
				if err := h.Handle(ctx, task); err != nil && !w.dropOnError {
					w.store.retry(c.id, w.retryDelay, time.Now())
				} else {
					w.store.complete(c.id) // success, or (dropOnError) a dropped failure
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

// noRetryFixture is newRefFixture with a Worker whose dropOnError knob is set:
// a failed handler attempt is DROPPED instead of redelivered. This must make
// FailedAttemptRedelivered (and the other retry-dependent subtests) FAIL,
// proving the suite is non-vacuous. Reuses refWorker via the knob — no
// duplicated Run loop.
func noRetryFixture(t *testing.T) jobstest.DeliveryFixture {
	t.Helper()
	fx := newRefFixture(t)
	store := fx.Enqueuer.(*refEnqueuer).store
	fx.NewWorker = func() jobs.Worker {
		return &refWorker{
			store:       store,
			handlers:    map[string]jobs.Handler{},
			retryDelay:  20 * time.Millisecond,
			poll:        10 * time.Millisecond,
			dropOnError: true, // broken variant: no redelivery
		}
	}
	return fx
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

func TestReferenceBackendSatisfiesDeliveryContract(t *testing.T) {
	jobstest.RunDeliveryContract(t, newRefFixture)
}
