// Package jobs defines the contract for background job processing: enqueuing a
// unit of work to be executed asynchronously by a worker, immediately or at a
// scheduled time.
//
// It is distinct from the eventbus contracts. eventbus carries domain events —
// facts that already happened — fanned out to many independent consumers,
// with Outbox/Relay for reliable delivery and Inbox for per-consumer dedup.
// jobs carries tasks — work that should be done — handed to a worker for
// execution, with scheduling (run-at) and an execution lifecycle. A domain
// event MAY trigger a job, but they are different primitives.
//
// Core ships only the Enqueuer and Worker contracts plus the Job/Task value
// types. Concrete backings (Asynq, River, a SQL queue, an in-memory dev queue),
// the retry/backoff schedule, dead-lettering, queue routing, and cron/recurring
// scheduling live in adapters.
//
// Delivery is at-least-once, and the guarantee is precisely bounded. Given ALL
// of: (1) a nil-error Enqueue, (2) a worker running (a SUSTAINED condition, not
// an instant: no running worker, or a worker stopping before it dequeues the
// job, merely makes (2) temporarily false — delivery is suspended and the job
// stays pending until some worker runs again), (3) a Handler registered for the
// job's Type at the time delivery is attempted, (4) the backend reachable (a
// transient outage merely makes (4) temporarily false — delivery is suspended,
// the job is not dropped), (5) the backend has not DURABLY lost the job
// (durable data loss or loss beyond the adapter's declared durability boundary —
// NOT a transient outage, covered by (4), and NOT a worker stopping, covered by
// (2)), and (6) the job's ProcessAt has arrived (zero has already arrived) —
// the job is handed to its Handler at least once, not before ProcessAt, and is
// not discarded or dead-lettered until after at least one delivery attempt.
// "At least once" means no fewer than once — not "eventually succeeds".
//
// A nil-error Enqueue MUST keep the job pending and deliverable until a running
// worker dequeues it (retain-until-dequeue) — there is no routine pre-attempt
// expiry timer; the only pre-dequeue loss is durable data loss (prerequisite
// (5)). ProcessAt is a "no earlier than" floor, not an "expire when it passes"
// ceiling; a requested ProcessAt beyond the adapter's declared scheduling
// horizon MUST be rejected at Enqueue with CodeInvalidArgument, never accepted
// then silently dropped. A job whose Type has no registered Handler when a
// running worker dequeues it is never acked as success and follows the
// adapter's unhandled-job policy (which MAY discard immediately; no minimum
// retention for unhandled jobs). The contract provides NO type-based routing
// across workers: every worker consuming from the same backing-store namespace
// MUST be wired with an identical registration set (a homogeneous worker
// pool) — that is a DEPLOYMENT PRECONDITION, and the unhandled-job policy
// exists to surface its violation loudly, not to route around it. A worker
// dequeuing a Type it has no Handler for never acks it as success, regardless
// of whether some other worker has that Handler. Partitioning job types across
// heterogeneous workers is adapter-level queue/lane configuration, deliberately
// out of this contract. A producer MAY Enqueue
// before registration; late registration takes effect if it lands before some
// running worker dequeues the job. Redelivery (crash, lease expiry, shutdown before ack) means
// a Handler may run more than once for the same Task — possibly concurrently
// with a stalled earlier attempt; the contract does not guarantee mutually
// exclusive execution, so handler effects must be idempotent (ports/idempotency
// is one tool, not a requirement). Whether/when a failed attempt is retried and
// when a job is finally discarded or dead-lettered is adapter policy; no
// max-attempt count, no skip-retry signal.
//
// Every ctx-taking method requires a non-nil ctx (stdlib convention); passing
// nil is a caller bug with unspecified behaviour, no runtime nil-check required.
package jobs

import (
	"context"
	"time"
)

// Job is a unit of work to enqueue.
type Job struct {
	// Type routes the job to a registered Handler. Required; empty Type is
	// malformed input (Enqueue returns errorsx.CodeInvalidArgument → 400).
	Type string
	// Payload is opaque, caller-encoded work input. Enqueue MUST snapshot these
	// bytes BEFORE any backend I/O (snapshot-before-submit): the queue stores and
	// delivers the snapshot verbatim and never parses it. Mutating the caller's
	// slice after Enqueue returns therefore affects neither a successfully
	// enqueued job NOR a job the backend may have accepted on an indeterminate
	// class-2 error return (see Enqueuer.Enqueue). A nil and an empty Payload are
	// equivalent: both are valid and delivered as a zero-length Task.Payload;
	// whether the delivered slice is nil or empty-non-nil is unspecified
	// (serialization through real backends cannot preserve nil-ness), so Handlers
	// MUST NOT distinguish them — len(Task.Payload) == 0 is the observable.
	Payload []byte
	// ProcessAt is the earliest time the job should run; zero means ASAP. It is
	// an absolute UTC instant (time.Time is location-independent) — "no earlier
	// than", not an exact deadline. A past ProcessAt is already eligible, never
	// an error. The only timing MUST is judged on ONE clock — the backend's own
	// scheduling clock: measured on that clock, the dispatch instant is at or
	// after ProcessAt (single-clock comparison, no tolerance margin; quantizing
	// adapters round later). Caller-wall-clock ↔ backend-clock skew is out of
	// contract scope (operational concern). The scheduling horizon is an adapter
	// property (docs/constructor option), not part of this interface; an
	// unrepresentable ProcessAt MUST be rejected at Enqueue with
	// CodeInvalidArgument.
	ProcessAt time.Time
}

// JobInfo identifies an enqueued job for log/trace correlation. The contract is
// fire-and-forget: there is no result to await.
type JobInfo struct {
	// ID is the adapter-assigned identifier. On success it is non-empty, stable
	// across redeliveries (every delivery carries Task.ID == ID), and not reused
	// for a different job while the backing store retains this one. Uniqueness is
	// scoped to a single adapter's namespace.
	ID string
}

// Task is a job delivered to a Handler for one execution. ID equals the JobInfo
// returned at enqueue. Payload is a private copy the Handler may freely mutate —
// affecting neither the stored job nor any other (re)delivery.
type Task struct {
	ID      string
	Type    string
	Payload []byte
}

// Handler processes one execution of a Task. Returning nil acks the task as
// done; non-nil marks THIS attempt failed (retry/discard is adapter policy). A
// Worker may invoke Handle concurrently and delivery is at-least-once, so a
// Handler must be safe under concurrent calls and idempotent in effects.
//
// The ctx passed to Handle is cancelled when the Worker begins shutting down;
// it MAY carry an adapter/deployment deadline, but the contract sets no
// per-task timeout. A non-nil error returned because ctx was cancelled/expired
// is an ORDINARY failed attempt (no special-casing of handler context errors;
// distinct from Worker.Run's OWN ctx cancellation, which returns nil).
type Handler interface {
	Handle(ctx context.Context, task Task) error
}

// HandlerFunc adapts an ordinary function to Handler.
type HandlerFunc func(ctx context.Context, task Task) error

// Handle calls f.
func (f HandlerFunc) Handle(ctx context.Context, task Task) error { return f(ctx, task) }

// Enqueuer submits jobs for asynchronous execution. Implementations MUST be
// safe for concurrent use by multiple goroutines.
type Enqueuer interface {
	// Enqueue snapshots job (before any backend I/O — see Job.Payload) and
	// registers it for execution. On nil error the job IS enqueued: JobInfo.ID is
	// non-empty and stable across redeliveries, and the at-least-once guarantee
	// binds.
	//
	// Non-nil errors form two classes with fixed precedence. CLASS 1 —
	// deterministic validation (empty Type; out-of-horizon ProcessAt) → contract-
	// FIXED errorsx.CodeInvalidArgument; definitely not enqueued, nothing written.
	// CLASS 2 — ctx/backend failures — generally INDETERMINATE: a mid-flight ctx
	// cancellation/expiry (errors.Is against context.Canceled /
	// context.DeadlineExceeded) or a backend error — which MUST be a coded
	// errorsx whose errorsx.CodeOf is NOT CodeUnknown (CodeUnavailable → 503 for
	// an unreachable backend; a failure the adapter cannot classify maps to
	// CodeInternal, never CodeUnknown, so transport adapters can always translate
	// deterministically) — may surface AFTER the backend accepted the job; a
	// retry may duplicate — recovery is handler-side idempotency, no enqueue-side
	// dedup is provided. Precedence: (1a) empty Type → (1b) out-of-horizon
	// ProcessAt → (2a) ctx cancelled/expired → (2b) backend failure. Class 1 is
	// evaluated before observing ctx or backend. Within class 2, Enqueue MUST
	// observe ctx before touching the backend: a ctx already cancelled OR already
	// past its deadline at entry returns the matching ctx error (errors.Is
	// context.Canceled / context.DeadlineExceeded) with NO backend contact — even
	// if the backend is also unreachable. "Definitely not enqueued" holds for
	// class 1 and for the pre-cancelled/pre-expired entry check only.
	//
	// On ANY non-nil error the returned JobInfo is the zero value; callers MUST
	// ignore it. The only thing a non-nil error denies is the at-least-once
	// guarantee for THIS call.
	Enqueue(ctx context.Context, job Job) (JobInfo, error)
}

// Worker dispatches jobs to the Handler registered for each job's Type.
// Lifecycle: single-threaded wiring (all Register calls before Run, one
// goroutine), then Run exactly once. While running, the Worker MAY dispatch
// Handlers concurrently.
type Worker interface {
	// Register routes tasks whose Type == jobType to h, with fixed precedence:
	// argument validation FIRST — empty jobType or interface-level-nil h →
	// errorsx.CodeInvalidArgument (even if jobType is also already registered);
	// only for a well-formed call does the duplicate check apply — re-registering
	// → errorsx.CodeAlreadyExists, the original Handler stays installed (no
	// silent replace). A typed-nil handler is a caller bug the contract does not
	// separately detect.
	Register(jobType string, h Handler) error
	// Run dispatches until ctx is cancelled, then returns nil (cancellation is
	// the expected stop signal; if ctx is already cancelled at call, return nil
	// without starting). Liveness: after cancellation Run MUST return, MUST NOT
	// block indefinitely; conformance is checked against the implementation-
	// DECLARED shutdown bound (test fixture parameter, e.g. ShutdownWithin;
	// timing from the cancel instant, no extra margin; non-positive declared
	// value = conformance failure). A fatal startup/runtime failure returns a
	// non-nil coded errorsx (CodeOf != CodeUnknown, NOT errors.Is ctx errors);
	// which code maps to which fatal is adapter taxonomy (SHOULD:
	// misconfiguration→CodeInvalidArgument, unreachable→CodeUnavailable,
	// unexpected→CodeInternal). The contract fixes two CALLER-OBSERVABLE
	// endpoints. (A) Cancellation with no independent fatal: Run returns nil.
	// Errors that arise FROM the act of shutting down (backend errors while
	// draining or requeuing, failures closing connections) are NOT fatals — they
	// MUST NOT turn a cancelled Run into an error return; they go to adapter-
	// level channels (logs, metrics), and job safety is covered by the
	// recoverable-state model below. (B) An independent fatal with no
	// cancellation: Run returns the coded error. When a fatal and a cancellation
	// OVERLAP (either order, in flight together), Run MAY return either nil or
	// the coded error — the caller initiated shutdown, so it MUST tolerate both;
	// no internal "observed first" ordering is part of the contract. Tests
	// assert only endpoints (A) and (B).
	//
	// Run returning means it has stopped dispatching new tasks and made a bounded
	// best-effort drain — NOT that every Handler finished. Go cannot kill a
	// goroutine: a Handler ignoring its cancelled ctx MAY still run after Run
	// returns. Such a straggler goroutine is ORPHANED — the worker relinquishes
	// management and MUST NOT let it block Run (liveness wins over graceful
	// drain). Post-return resource lifetime: the adapter MAY tear down resources
	// it owns once Run returns; teardown MUST be straggler-safe — a late
	// operation against a torn-down facility fails with an error, it MUST NOT
	// corrupt state or panic the process. Ack safety for a late straggler is
	// binary: its ack either takes effect atomically on the backend (the job
	// becomes completed — sparing redelivery) or does not take effect at all (the
	// job stays un-acked); a partially applied ack that leaves the job neither
	// completed nor recoverable is a contract violation. Hence the recoverable-
	// state model: after Run returns, a task whose ack the worker did not observe
	// is in one of three states — completed (late ack landed), pending/retryable
	// (eligible for redelivery by a new Worker instance), or active/leased (held
	// in-flight until lease expiry, a TRANSIENT state that MUST resolve to one of
	// the other two within the adapter's declared finite bound). "Terminal
	// lost/discarded without a completed ack" is the only illegal outcome. Both
	// legal endings uphold the at-least-once floor; a late straggler racing a
	// redelivery is exactly the concurrent-duplicate case Handlers must already
	// tolerate.
	// A task whose Type has no registered Handler when THIS Worker dequeues it
	// must NOT be acked as success — it follows the adapter's unhandled-job
	// policy. Registering every enqueueable Type during wiring is the caller's
	// responsibility.
	Run(ctx context.Context) error
}
