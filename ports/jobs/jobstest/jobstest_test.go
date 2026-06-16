package jobstest_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/jobs"
	"github.com/slam0504/go-ddd-core/ports/jobs/jobstest"
)

// fakeBackend is a minimal correct implementation of the synchronous contract
// surface: Enqueue/Register run the real validation logic and error
// precedence; Run just blocks until ctx is cancelled (the suite never calls
// it, but the fake honours the endpoint anyway).
type fakeBackend struct {
	mu       sync.Mutex
	seq      int
	jobs     map[string]jobs.Job
	handlers map[string]jobs.Handler
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{jobs: map[string]jobs.Job{}, handlers: map[string]jobs.Handler{}}
}

func (f *fakeBackend) Enqueue(ctx context.Context, job jobs.Job) (jobs.JobInfo, error) {
	if job.Type == "" {
		return jobs.JobInfo{}, errorsx.New(errorsx.CodeInvalidArgument, "jobs: empty job type")
	}
	payload := make([]byte, len(job.Payload)) // snapshot-before-submit
	copy(payload, job.Payload)
	job.Payload = payload
	if err := ctx.Err(); err != nil {
		return jobs.JobInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := strconv.Itoa(f.seq)
	f.jobs[id] = job
	return jobs.JobInfo{ID: id}, nil
}

func (f *fakeBackend) Register(jobType string, h jobs.Handler) error {
	if jobType == "" {
		return errorsx.New(errorsx.CodeInvalidArgument, "jobs: empty job type")
	}
	if h == nil {
		return errorsx.New(errorsx.CodeInvalidArgument, "jobs: nil handler")
	}
	if _, ok := f.handlers[jobType]; ok {
		return errorsx.New(errorsx.CodeAlreadyExists, "jobs: handler already registered for type "+jobType)
	}
	f.handlers[jobType] = h
	return nil
}

func (f *fakeBackend) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// TestRunContract_PassesAgainstCorrectImplementation proves the exported
// suite passes against a known-correct implementation, so an adapter failure
// points at the adapter, not at the suite.
func TestRunContract_PassesAgainstCorrectImplementation(t *testing.T) {
	jobstest.RunContract(t, func(t *testing.T) jobstest.Backend {
		f := newFakeBackend()
		return jobstest.Backend{Enqueuer: f, Worker: f}
	})
}
