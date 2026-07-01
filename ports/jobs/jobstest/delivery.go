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
