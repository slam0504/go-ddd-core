// Package jobstest provides an exported, synchronous-only conformance suite
// for ports/jobs implementations.
//
// RunContract verifies only the contract surface that is visible through
// interface return values without running a worker: Enqueue validation codes
// and their fixed precedence, non-empty JobInfo.ID, context cancellation and
// deadline handling at Enqueue, and Register validation semantics. It never
// calls Worker.Run, sets no timers, and waits on nothing, so it cannot flake.
//
// Delivery-side invariants (payload snapshot isolation, exact-type dispatch,
// at-least-once redelivery, shutdown recoverability) need a running worker
// and backend-specific observation, so they are NOT in this suite: adapters
// must cover them with their own intent tests (see the tag-gate acceptance
// criteria recorded in the repository's .agent/decisions.md), and core
// demonstrates them against a local fake in ports/jobs's own tests.
package jobstest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/jobs"
)

// Backend bundles the Enqueuer and Worker views of ONE shared backing store:
// jobs enqueued through Enqueuer are the jobs Worker would dispatch.
// RunContract itself never calls Worker.Run; Worker is included so
// registration invariants are testable and so future delivery-half suites can
// reuse the same fixture.
type Backend struct {
	Enqueuer jobs.Enqueuer
	Worker   jobs.Worker
}

// Factory returns a fresh Backend for one subtest. Each call MUST return a
// backend whose state is isolated from every other call (no shared jobs, no
// shared registrations). The factory registers any cleanup it needs via
// t.Cleanup — RunContract imposes no additional teardown protocol.
type Factory func(t *testing.T) Backend

// RunContract runs the synchronous-only contract suite. It calls factory once
// per subtest, asserts only on interface return values (errorsx.CodeOf /
// errors.Is / JobInfo), and never calls Worker.Run.
func RunContract(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("MalformedEnqueue", func(t *testing.T) {
		b := factory(t)
		info, err := b.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: ""})
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Enqueue with empty Type: code = %v, want CodeInvalidArgument (err=%v)", code, err)
		}
		if info.ID != "" {
			t.Fatalf("Enqueue with empty Type returned non-zero JobInfo.ID %q; callers must get the zero value on error", info.ID)
		}
	})

	t.Run("MalformedEnqueuePrecedence", func(t *testing.T) {
		b := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		info, err := b.Enqueuer.Enqueue(ctx, jobs.Job{Type: ""})
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("empty Type + cancelled ctx: code = %v, want CodeInvalidArgument — class-1 validation precedes ctx observation (err=%v)", code, err)
		}
		if errors.Is(err, context.Canceled) {
			t.Fatal("empty Type + cancelled ctx returned the ctx error; the fixed precedence is (1a) empty Type first")
		}
		if info.ID != "" {
			t.Fatalf("error return carried non-zero JobInfo.ID %q", info.ID)
		}
	})

	t.Run("MalformedEnqueuePrecedenceDeadline", func(t *testing.T) {
		b := factory(t)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		info, err := b.Enqueuer.Enqueue(ctx, jobs.Job{Type: ""})
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("empty Type + expired deadline: code = %v, want CodeInvalidArgument — class-1 validation precedes ctx observation (err=%v)", code, err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("empty Type + expired deadline returned the ctx error; the fixed precedence is (1a) empty Type first")
		}
		if info.ID != "" {
			t.Fatalf("error return carried non-zero JobInfo.ID %q", info.ID)
		}
	})

	t.Run("EnqueueReturnsNonEmptyID", func(t *testing.T) {
		b := factory(t)
		info, err := b.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "jobstest:ok", Payload: []byte("p")})
		if err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if info.ID == "" {
			t.Fatal("successful Enqueue returned empty JobInfo.ID; producers need it for log/trace correlation")
		}
	})

	t.Run("EnqueueNilPayloadAccepted", func(t *testing.T) {
		b := factory(t)
		info, err := b.Enqueuer.Enqueue(context.Background(), jobs.Job{Type: "jobstest:nil-payload", Payload: nil})
		if err != nil {
			t.Fatalf("Enqueue with nil Payload: %v — nil and empty payloads are equivalent valid input", err)
		}
		if info.ID == "" {
			t.Fatal("successful Enqueue returned empty JobInfo.ID")
		}
	})

	t.Run("EnqueueCtxCancellation", func(t *testing.T) {
		b := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		info, err := b.Enqueuer.Enqueue(ctx, jobs.Job{Type: "jobstest:cancelled", Payload: []byte("p")})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-cancelled ctx: err = %v, want errors.Is(context.Canceled) — Enqueue must observe ctx before touching the backend", err)
		}
		if info.ID != "" {
			t.Fatalf("error return carried non-zero JobInfo.ID %q", info.ID)
		}
	})

	t.Run("EnqueueDeadlineExceeded", func(t *testing.T) {
		b := factory(t)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		info, err := b.Enqueuer.Enqueue(ctx, jobs.Job{Type: "jobstest:expired", Payload: []byte("p")})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("pre-expired deadline: err = %v, want errors.Is(context.DeadlineExceeded) — both ctx error kinds get the same entry check", err)
		}
		if info.ID != "" {
			t.Fatalf("error return carried non-zero JobInfo.ID %q", info.ID)
		}
	})

	t.Run("RegisterValidation", func(t *testing.T) {
		b := factory(t)
		h := jobs.HandlerFunc(func(context.Context, jobs.Task) error { return nil })
		if err := b.Worker.Register("jobstest:reg", h); err != nil {
			t.Fatalf("first Register: %v, want nil", err)
		}
		if code := errorsx.CodeOf(b.Worker.Register("", h)); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Register with empty type: code = %v, want CodeInvalidArgument", code)
		}
		if code := errorsx.CodeOf(b.Worker.Register("jobstest:nil-handler", nil)); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Register with nil handler: code = %v, want CodeInvalidArgument", code)
		}
		if code := errorsx.CodeOf(b.Worker.Register("jobstest:reg", h)); code != errorsx.CodeAlreadyExists {
			t.Fatalf("duplicate Register: code = %v, want CodeAlreadyExists — silent replacement would hide a wiring bug", code)
		}
	})

	t.Run("RegisterValidationPrecedence", func(t *testing.T) {
		b := factory(t)
		h := jobs.HandlerFunc(func(context.Context, jobs.Task) error { return nil })
		if err := b.Worker.Register("jobstest:prec", h); err != nil {
			t.Fatalf("first Register: %v, want nil", err)
		}
		if code := errorsx.CodeOf(b.Worker.Register("jobstest:prec", nil)); code != errorsx.CodeInvalidArgument {
			t.Fatalf("registered type + nil handler: code = %v, want CodeInvalidArgument — argument validation precedes the duplicate check", code)
		}
	})
}
