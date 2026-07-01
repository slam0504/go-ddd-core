package jobstest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
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

	// ConcurrentEnqueueSmoke is a race/shutdown smoke test, NOT a delivery-count
	// assertion: it proves concurrent Enqueue is safe under -race and that Run
	// shuts down cleanly afterward. It deliberately ignores Enqueue errors and
	// asserts no delivery count (at-least-once coverage lives in the retry/
	// depletion subtests above).
	t.Run("ConcurrentEnqueueSmoke", func(t *testing.T) {
		fx := factory(t)
		fx.Bounds.validate(t)
		w := fx.NewWorker()
		mustRegister(t, w, "l:job", jobs.HandlerFunc(func(context.Context, jobs.Task) error { return nil }))
		cancel, done := runWorker(w)
		var wg sync.WaitGroup
		for range 50 {
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
