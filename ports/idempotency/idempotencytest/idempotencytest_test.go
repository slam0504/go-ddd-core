package idempotencytest_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/idempotency"
	"github.com/slam0504/go-ddd-core/ports/idempotency/idempotencytest"
)

// TestFakeStore_StoreContract proves the deterministic suite is satisfiable by a
// correct implementation. The fake never reclaims (reclaimWithin == 0) so the
// zero-wait state machine is exercised without timing interference.
func TestFakeStore_StoreContract(t *testing.T) {
	idempotencytest.RunStoreContract(t, func(t *testing.T) idempotency.Store {
		return newFakeStore(0)
	})
}

// TestFakeStore_ReclaimContract proves the opt-in liveness suite is satisfiable.
// The fake reclaims an unfinished reservation once reclaimWithin elapses, and
// the suite is told the same bound so its real-wait poll matches the fake.
func TestFakeStore_ReclaimContract(t *testing.T) {
	const reclaimWithin = 40 * time.Millisecond
	idempotencytest.RunReclaimContract(t, func(t *testing.T) idempotency.Store {
		return newFakeStore(reclaimWithin)
	}, idempotencytest.ReclaimOptions{ReclaimWithin: reclaimWithin})
}

// fakeKey is a struct composite key, NOT a flattened scope+":"+key string, so
// the tuple-separation case in RunStoreContract passes by construction.
type fakeKey struct {
	scope string
	key   string
}

type fakeEntry struct {
	fingerprint string
	leaseToken  string             // owner credential while in-progress; cleared on completion
	status      idempotency.Status // StatusInProgress or StatusCompleted only
	response    []byte
	reservedAt  time.Time
}

// fakeStore is a mutex-backed reference Store living only in _test.go. It is the
// minimum correct implementation: atomic claim under a lock, lease-token
// ownership, fingerprint mismatch, completed retention, and (when reclaimWithin
// > 0) eventual reclaim of an unfinished in-progress reservation.
type fakeStore struct {
	mu            sync.Mutex
	entries       map[fakeKey]*fakeEntry
	seq           int
	reclaimWithin time.Duration // 0 = never reclaim (deterministic suite)
}

func newFakeStore(reclaimWithin time.Duration) *fakeStore {
	return &fakeStore{
		entries:       make(map[fakeKey]*fakeEntry),
		reclaimWithin: reclaimWithin,
	}
}

// reclaimable reports whether an unfinished in-progress entry has outlived the
// declared reclaim window. Completed records are retained regardless.
func (s *fakeStore) reclaimable(e *fakeEntry) bool {
	if s.reclaimWithin <= 0 || e.status != idempotency.StatusInProgress {
		return false
	}
	return time.Since(e.reservedAt) >= s.reclaimWithin
}

// lookup returns the live entry for fk, dropping it first if it is reclaimable.
// Callers hold s.mu.
func (s *fakeStore) lookup(fk fakeKey) *fakeEntry {
	e := s.entries[fk]
	if e != nil && s.reclaimable(e) {
		delete(s.entries, fk)
		return nil
	}
	return e
}

func (s *fakeStore) Begin(ctx context.Context, scope, key, fingerprint string) (idempotency.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Reservation{}, fmt.Errorf("idempotencytest: %w", err)
	}
	if scope == "" || key == "" || fingerprint == "" {
		return idempotency.Reservation{}, errorsx.New(errorsx.CodeInvalidArgument, "idempotencytest: empty scope, key, or fingerprint")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fk := fakeKey{scope, key}
	e := s.lookup(fk)
	if e == nil {
		s.seq++
		tok := "lease-" + strconv.Itoa(s.seq)
		s.entries[fk] = &fakeEntry{
			fingerprint: fingerprint,
			leaseToken:  tok,
			status:      idempotency.StatusInProgress,
			reservedAt:  time.Now(),
		}
		return idempotency.Reservation{
			Scope:      scope,
			Key:        key,
			LeaseToken: tok,
			LeaseTTL:   time.Minute, // advisory positive budget
			Status:     idempotency.StatusNew,
		}, nil
	}

	if e.fingerprint != fingerprint {
		return idempotency.Reservation{Scope: scope, Key: key, Status: idempotency.StatusMismatch}, nil
	}
	switch e.status {
	case idempotency.StatusCompleted:
		return idempotency.Reservation{
			Scope:    scope,
			Key:      key,
			Status:   idempotency.StatusCompleted,
			Response: cloneBytes(e.response),
		}, nil
	default: // StatusInProgress
		return idempotency.Reservation{Scope: scope, Key: key, Status: idempotency.StatusInProgress}, nil
	}
}

func (s *fakeStore) Finish(ctx context.Context, res idempotency.Reservation, response []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("idempotencytest: %w", err)
	}
	if res.Scope == "" || res.Key == "" || res.LeaseToken == "" {
		return errorsx.New(errorsx.CodeInvalidArgument, "idempotencytest: malformed reservation")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e := s.lookup(fakeKey{res.Scope, res.Key})
	if e == nil || e.status != idempotency.StatusInProgress || e.leaseToken != res.LeaseToken {
		return errorsx.New(errorsx.CodeConflict, "idempotencytest: stale, reclaimed, or terminal reservation")
	}
	e.status = idempotency.StatusCompleted
	e.response = cloneBytes(response)
	e.leaseToken = "" // completed records have no owner
	return nil
}

func (s *fakeStore) Cancel(ctx context.Context, res idempotency.Reservation) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("idempotencytest: %w", err)
	}
	if res.Scope == "" || res.Key == "" || res.LeaseToken == "" {
		return errorsx.New(errorsx.CodeInvalidArgument, "idempotencytest: malformed reservation")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fk := fakeKey{res.Scope, res.Key}
	e := s.lookup(fk)
	if e == nil || e.status != idempotency.StatusInProgress || e.leaseToken != res.LeaseToken {
		return errorsx.New(errorsx.CodeConflict, "idempotencytest: stale, reclaimed, or terminal reservation")
	}
	delete(s.entries, fk)
	return nil
}

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
