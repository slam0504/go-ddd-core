// Package idempotencytest provides a transport-neutral conformance suite that
// any idempotency.Store implementation can run against itself. It is an exported
// test helper (a contract deliverable), not a production implementation: it
// imports testing and pkg/errorsx only, never pkg/errorsx/httpx, so adapter
// tests do not pick up a transport dependency.
//
// RunStoreContract enforces the deterministic state machine — reservation
// outcomes, lease ownership, fingerprint mismatch, copy semantics, malformed
// input, and true-concurrency atomic claim — without advancing any clock.
// Time-dependent guarantees (expiry timing, completed-record retention) are the
// adapter's policy and are out of scope here. The one liveness guarantee that
// the contract still mandates — an unfinished reservation must eventually become
// reservable again — is verified by the separate, opt-in RunReclaimContract,
// which waits for the adapter-declared ReclaimOptions.ReclaimWithin rather than
// deriving timing from LeaseTTL or injecting a clock.
//
// Error assertions use errorsx.CodeOf, never an HTTP status: httpx.Translate
// maps both CodeConflict and CodeAlreadyExists to 409, so asserting "status ==
// 409" would not prove the implementation returned the contract-specified
// CodeConflict. The errorsx.Code → HTTP status mapping is verified once, in
// core's own local unit tests.
package idempotencytest

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/idempotency"
)

const testFingerprint = "fp-v1"

// StoreFactory builds a fresh, isolated Store for a single subtest, so an
// adapter can do per-test setup/teardown (e.g. a clean Redis keyspace). The
// returned Store must start empty.
type StoreFactory func(t *testing.T) idempotency.Store

// RunStoreContract drives newStore through the deterministic part of the Store
// contract. An adapter passes its own constructor to inherit the same state
// machine guarantees. Run it under -race so the concurrency case exercises the
// atomic-claim requirement.
func RunStoreContract(t *testing.T, newStore StoreFactory) {
	t.Helper()

	t.Run("FirstBeginReservesNew", func(t *testing.T) {
		store := newStore(t)
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, res, idempotency.StatusNew)
		if res.LeaseToken == "" {
			t.Fatal("StatusNew must carry a non-empty LeaseToken")
		}
	})

	t.Run("CrossScopeIsolation", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		const key = "k"
		resA := beginChecked(t, store, "tenantA:create", key, testFingerprint)
		resB := beginChecked(t, store, "tenantB:create", key, testFingerprint)
		requireStatus(t, resA, idempotency.StatusNew)
		requireStatus(t, resB, idempotency.StatusNew)
		if resA.LeaseToken == resB.LeaseToken {
			t.Fatal("reservations in different scopes must hold distinct lease tokens")
		}

		payloadA := []byte("response-A")
		if err := store.Finish(ctx, resA, payloadA); err != nil {
			t.Fatalf("Finish(A): unexpected error: %v", err)
		}
		// Finishing A must not complete B.
		doneA := beginChecked(t, store, "tenantA:create", key, testFingerprint)
		requireStatus(t, doneA, idempotency.StatusCompleted)
		requireResponse(t, doneA, payloadA)
		stillB := beginChecked(t, store, "tenantB:create", key, testFingerprint)
		requireStatus(t, stillB, idempotency.StatusInProgress)

		payloadB := []byte("response-B")
		if err := store.Finish(ctx, resB, payloadB); err != nil {
			t.Fatalf("Finish(B): unexpected error: %v", err)
		}
		doneB := beginChecked(t, store, "tenantB:create", key, testFingerprint)
		requireStatus(t, doneB, idempotency.StatusCompleted)
		requireResponse(t, doneB, payloadB)
	})

	t.Run("TupleSeparationSafety", func(t *testing.T) {
		// ("tenant:op","k") and ("tenant","op:k") must be independent records:
		// a naive scope+":"+key flattening would collide them.
		store := newStore(t)
		ctx := context.Background()
		res1 := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, res1, idempotency.StatusNew)
		if err := store.Finish(ctx, res1, []byte("first")); err != nil {
			t.Fatalf("Finish(res1): unexpected error: %v", err)
		}
		// The second tuple is seen for the first time → StatusNew, not a
		// collision with the now-completed first tuple.
		res2 := beginChecked(t, store, "tenant", "op:k", testFingerprint)
		requireStatus(t, res2, idempotency.StatusNew)
		if res1.LeaseToken == res2.LeaseToken {
			t.Fatal("colliding tuples must not share a lease token")
		}
		// Confirm both records resolve to their own state.
		b1 := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, b1, idempotency.StatusCompleted)
		b2 := beginChecked(t, store, "tenant", "op:k", testFingerprint)
		requireStatus(t, b2, idempotency.StatusInProgress)
	})

	t.Run("LeaseTokenUniqueness", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		seen := map[string]bool{}
		addToken := func(tok string) {
			if tok == "" {
				t.Fatal("StatusNew lease token must be non-empty")
			}
			if seen[tok] {
				t.Fatalf("lease token %q reused across reservations", tok)
			}
			seen[tok] = true
		}
		for _, key := range []string{"a", "b", "c", "d", "e"} {
			res := beginChecked(t, store, "tenant:op", key, testFingerprint)
			addToken(res.LeaseToken)
		}
		// Reusing a (scope,key) after Cancel must mint a fresh, distinct token.
		r1 := beginChecked(t, store, "tenant:op", "reuse", testFingerprint)
		addToken(r1.LeaseToken)
		if err := store.Cancel(ctx, r1); err != nil {
			t.Fatalf("Cancel(r1): unexpected error: %v", err)
		}
		r2 := beginChecked(t, store, "tenant:op", "reuse", testFingerprint)
		addToken(r2.LeaseToken)
		// The cancelled token must no longer own anything.
		requireCode(t, store.Finish(ctx, r1, []byte("x")), errorsx.CodeConflict, "Finish(stale r1)")
		requireCode(t, store.Cancel(ctx, r1), errorsx.CodeConflict, "Cancel(stale r1)")
		// r2 is unaffected.
		if err := store.Finish(ctx, r2, []byte("y")); err != nil {
			t.Fatalf("Finish(r2): unexpected error: %v", err)
		}
	})

	t.Run("InProgressOnDuplicateBegin", func(t *testing.T) {
		store := newStore(t)
		beginChecked(t, store, "tenant:op", "k", testFingerprint)
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, res, idempotency.StatusInProgress)
		if res.LeaseToken != "" || res.LeaseTTL != 0 {
			t.Fatalf("StatusInProgress must carry no lease: token=%q ttl=%v", res.LeaseToken, res.LeaseTTL)
		}
	})

	t.Run("CompletedReplaysResponse", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		payload := []byte("the-stored-response")
		if err := store.Finish(ctx, res, payload); err != nil {
			t.Fatalf("Finish: unexpected error: %v", err)
		}
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, got, idempotency.StatusCompleted)
		requireResponse(t, got, payload)
	})

	t.Run("CompletedEmptyResponseIsValid", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		if err := store.Finish(ctx, res, nil); err != nil {
			t.Fatalf("Finish(nil response): unexpected error: %v", err)
		}
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, got, idempotency.StatusCompleted)
		if len(got.Response) != 0 {
			t.Fatalf("empty completed response must replay empty, got %q", got.Response)
		}
	})

	t.Run("FingerprintMismatchInProgress", func(t *testing.T) {
		store := newStore(t)
		beginChecked(t, store, "tenant:op", "k", "fp-A")
		got := beginChecked(t, store, "tenant:op", "k", "fp-B")
		requireStatus(t, got, idempotency.StatusMismatch)
		if got.Response != nil {
			t.Fatal("StatusMismatch must not leak any payload")
		}
	})

	t.Run("FingerprintMismatchCompleted", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", "fp-A")
		if err := store.Finish(ctx, res, []byte("secret-A")); err != nil {
			t.Fatalf("Finish: unexpected error: %v", err)
		}
		got := beginChecked(t, store, "tenant:op", "k", "fp-B")
		requireStatus(t, got, idempotency.StatusMismatch)
		if got.Response != nil {
			t.Fatal("StatusMismatch on a completed key must not leak the stored payload")
		}
	})

	t.Run("CancelThenBeginIsNew", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		if err := store.Cancel(ctx, res); err != nil {
			t.Fatalf("Cancel: unexpected error: %v", err)
		}
		again := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, again, idempotency.StatusNew)
	})

	t.Run("CancelDoesNotDeleteCompleted", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		payload := []byte("kept")
		if err := store.Finish(ctx, res, payload); err != nil {
			t.Fatalf("Finish: unexpected error: %v", err)
		}
		requireCode(t, store.Cancel(ctx, res), errorsx.CodeConflict, "Cancel(completed)")
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, got, idempotency.StatusCompleted)
		requireResponse(t, got, payload)
	})

	t.Run("ForgedFinishDoesNotOverwrite", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		forged := res
		forged.LeaseToken = res.LeaseToken + "-forged"
		requireCode(t, store.Finish(ctx, forged, []byte("evil")), errorsx.CodeConflict, "Finish(forged)")
		// The real reservation is untouched: still in-progress, still finishable.
		mid := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, mid, idempotency.StatusInProgress)
		if err := store.Finish(ctx, res, []byte("real")); err != nil {
			t.Fatalf("Finish(real): unexpected error: %v", err)
		}
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireResponse(t, got, []byte("real"))
	})

	t.Run("ForgedCancelDoesNotRelease", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		forged := res
		forged.LeaseToken = res.LeaseToken + "-forged"
		requireCode(t, store.Cancel(ctx, forged), errorsx.CodeConflict, "Cancel(forged)")
		// The real in-progress reservation still stands.
		mid := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, mid, idempotency.StatusInProgress)
		if err := store.Cancel(ctx, res); err != nil {
			t.Fatalf("Cancel(real): unexpected error: %v", err)
		}
	})

	t.Run("MalformedReservationRejected", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		// Establish a real reservation that must survive the malformed calls.
		live := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		cases := []struct {
			name string
			res  idempotency.Reservation
		}{
			{"empty", idempotency.Reservation{}},
			{"missing token", idempotency.Reservation{Scope: "tenant:op", Key: "k"}},
			{"missing scope", idempotency.Reservation{Key: "k", LeaseToken: "tok"}},
			{"missing key", idempotency.Reservation{Scope: "tenant:op", LeaseToken: "tok"}},
		}
		for _, tc := range cases {
			requireCode(t, store.Finish(ctx, tc.res, []byte("x")), errorsx.CodeInvalidArgument, "Finish("+tc.name+")")
			requireCode(t, store.Cancel(ctx, tc.res), errorsx.CodeInvalidArgument, "Cancel("+tc.name+")")
		}
		// The live reservation is still finishable.
		if err := store.Finish(ctx, live, []byte("ok")); err != nil {
			t.Fatalf("Finish(live) after malformed calls: unexpected error: %v", err)
		}
	})

	t.Run("MutableFieldsAreNotTrusted", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		// Tamper with every caller-mutable field except Scope/Key/LeaseToken.
		tampered := res
		tampered.LeaseTTL = res.LeaseTTL + time.Hour
		tampered.Status = idempotency.StatusCompleted
		tampered.Response = []byte("garbage")
		payload := []byte("authoritative")
		if err := store.Finish(ctx, tampered, payload); err != nil {
			t.Fatalf("Finish must ignore tampered LeaseTTL/Status/Response, got error: %v", err)
		}
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireStatus(t, got, idempotency.StatusCompleted)
		requireResponse(t, got, payload)

		// Control: only forging LeaseToken flips the decision to a conflict.
		ctl := newStore(t)
		valid := beginChecked(t, ctl, "tenant:op", "k", testFingerprint)
		forged := valid
		forged.LeaseToken = valid.LeaseToken + "-forged"
		requireCode(t, ctl.Finish(ctx, forged, payload), errorsx.CodeConflict, "Finish(forged token)")
	})

	t.Run("PerStatusFieldInvariants", func(t *testing.T) {
		// beginChecked already asserts the invariant table on every call; this
		// case simply drives all four non-error statuses through it.
		store := newStore(t)
		ctx := context.Background()
		newRes := beginChecked(t, store, "tenant:op", "k", testFingerprint) // StatusNew
		requireStatus(t, newRes, idempotency.StatusNew)
		inprog := beginChecked(t, store, "tenant:op", "k", testFingerprint) // StatusInProgress
		requireStatus(t, inprog, idempotency.StatusInProgress)
		mismatch := beginChecked(t, store, "tenant:op", "k", "other-fp") // StatusMismatch
		requireStatus(t, mismatch, idempotency.StatusMismatch)
		if err := store.Finish(ctx, newRes, []byte("done")); err != nil {
			t.Fatalf("Finish: unexpected error: %v", err)
		}
		completed := beginChecked(t, store, "tenant:op", "k", testFingerprint) // StatusCompleted
		requireStatus(t, completed, idempotency.StatusCompleted)
	})

	t.Run("DuplicateFinishDoesNotOverwrite", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		first := []byte("first")
		if err := store.Finish(ctx, res, first); err != nil {
			t.Fatalf("Finish(first): unexpected error: %v", err)
		}
		requireCode(t, store.Finish(ctx, res, []byte("second")), errorsx.CodeConflict, "Finish(duplicate)")
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireResponse(t, got, first)
	})

	t.Run("CopySemantics", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		// (b) Finish must copy the response buffer so later mutation is invisible.
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		buf := []byte("original")
		if err := store.Finish(ctx, res, buf); err != nil {
			t.Fatalf("Finish: unexpected error: %v", err)
		}
		buf[0] = 'X'
		got := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireResponse(t, got, []byte("original"))
		// (a) Begin must return a copy so caller mutation does not corrupt the store.
		got.Response[0] = 'Y'
		again := beginChecked(t, store, "tenant:op", "k", testFingerprint)
		requireResponse(t, again, []byte("original"))
	})

	t.Run("MalformedBeginRejected", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		requireCode(t, beginErr(ctx, store, "", "k", testFingerprint), errorsx.CodeInvalidArgument, "Begin(empty scope)")
		requireCode(t, beginErr(ctx, store, "tenant:op", "", testFingerprint), errorsx.CodeInvalidArgument, "Begin(empty key)")
		requireCode(t, beginErr(ctx, store, "tenant:op", "k", ""), errorsx.CodeInvalidArgument, "Begin(empty fingerprint)")
	})

	t.Run("ConcurrentBeginClaimsOnce", func(t *testing.T) {
		store := newStore(t)
		const n = 8
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]idempotency.Reservation, n)
		errs := make([]error, n)
		for i := range n {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				results[i], errs[i] = store.Begin(context.Background(), "tenant:op", "k", testFingerprint)
			}(i)
		}
		close(start) // release all goroutines at once
		wg.Wait()

		newCount := 0
		var newToken string
		for i := range n {
			if errs[i] != nil {
				t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
			}
			switch results[i].Status {
			case idempotency.StatusNew:
				newCount++
				newToken = results[i].LeaseToken
			case idempotency.StatusInProgress:
			default:
				t.Fatalf("goroutine %d: unexpected status %v", i, results[i].Status)
			}
		}
		if newCount != 1 {
			t.Fatalf("got %d StatusNew, want exactly 1 (atomic claim)", newCount)
		}
		if newToken == "" {
			t.Fatal("the single StatusNew must carry a non-empty lease token")
		}
	})

	t.Run("ContextCancellationIsDeterministic", func(t *testing.T) {
		store := newStore(t)
		// A live reservation so Finish/Cancel have a real target to validate
		// against — proving the ctx check fires before token validation.
		res := beginChecked(t, store, "tenant:op", "k", testFingerprint)

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.Begin(cancelled, "tenant:op", "other", testFingerprint); !errors.Is(err, context.Canceled) {
			t.Errorf("Begin(cancelled): err=%v, want errors.Is context.Canceled", err)
		}
		if err := store.Finish(cancelled, res, []byte("x")); !errors.Is(err, context.Canceled) {
			t.Errorf("Finish(cancelled): err=%v, want errors.Is context.Canceled", err)
		}
		if err := store.Cancel(cancelled, res); !errors.Is(err, context.Canceled) {
			t.Errorf("Cancel(cancelled): err=%v, want errors.Is context.Canceled", err)
		}

		past, cancelPast := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
		defer cancelPast()
		if _, err := store.Begin(past, "tenant:op", "other", testFingerprint); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Begin(past deadline): err=%v, want errors.Is context.DeadlineExceeded", err)
		}
	})
}

// ReclaimOptions carries the adapter-declared bound the reclaim suite waits on.
type ReclaimOptions struct {
	// ReclaimWithin is the adapter's promise that an unfinished in-progress
	// reservation becomes re-Begin-able within this duration. The suite waits
	// (and polls) up to this bound rather than deriving timing from LeaseTTL or
	// injecting a clock. Zero skips the time-based reclaim suite.
	ReclaimWithin time.Duration
}

// RunReclaimContract verifies the liveness MUST: an unfinished reservation is
// eventually reclaimed so the same (scope, key) can be reserved again. It is
// opt-in and time-based — kept out of RunStoreContract so the deterministic
// suite stays zero-wait. An adapter declares its own ReclaimWithin; the suite
// trusts that declaration instead of inspecting the store's clock. When
// ReclaimWithin is zero the suite skips, but a production adapter must still
// document a reclaim policy.
func RunReclaimContract(t *testing.T, newStore StoreFactory, opts ReclaimOptions) {
	t.Helper()
	if opts.ReclaimWithin <= 0 {
		t.Skip("idempotencytest: ReclaimWithin not declared; skipping time-based reclaim contract")
	}

	t.Run("UnfinishedReservationEventuallyReclaimable", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		const scope, key = "tenant:op", "k"

		res1 := beginChecked(t, store, scope, key, testFingerprint)
		requireStatus(t, res1, idempotency.StatusNew)
		// Simulate a crash: never Finish or Cancel res1.

		// Generous bound so scheduling jitter does not flake the test.
		deadline := time.Now().Add(opts.ReclaimWithin*2 + 500*time.Millisecond)
		poll := max(opts.ReclaimWithin/10, 2*time.Millisecond)

		var res2 idempotency.Reservation
		for {
			r := beginChecked(t, store, scope, key, testFingerprint)
			if r.Status == idempotency.StatusNew {
				res2 = r
				break
			}
			if r.Status != idempotency.StatusInProgress {
				t.Fatalf("before reclaim, Begin must report StatusInProgress, got %v", r.Status)
			}
			if time.Now().After(deadline) {
				t.Fatalf("reservation not reclaimed within %v", opts.ReclaimWithin)
			}
			time.Sleep(poll)
		}
		if res2.LeaseToken == res1.LeaseToken {
			t.Fatal("a reclaimed reservation must mint a fresh, distinct lease token")
		}

		// The stale reservation must be a decided rejection, not indeterminate.
		requireCode(t, store.Finish(ctx, res1, []byte("x")), errorsx.CodeConflict, "Finish(reclaimed res1)")
		requireCode(t, store.Cancel(ctx, res1), errorsx.CodeConflict, "Cancel(reclaimed res1)")
		// The freshly reclaimed reservation is unaffected.
		if err := store.Finish(ctx, res2, []byte("y")); err != nil {
			t.Fatalf("Finish(res2) after reclaim: unexpected error: %v", err)
		}
	})
}

// beginChecked calls Begin with a background context, fails on error, and
// asserts the per-Status field invariants every implementation must honour.
func beginChecked(t *testing.T, store idempotency.Store, scope, key, fingerprint string) idempotency.Reservation {
	t.Helper()
	res, err := store.Begin(context.Background(), scope, key, fingerprint)
	if err != nil {
		t.Fatalf("Begin(%q,%q,%q): unexpected error: %v", scope, key, fingerprint, err)
	}
	if res.Scope != scope {
		t.Errorf("Begin(%q,%q,%q): Scope = %q, want %q", scope, key, fingerprint, res.Scope, scope)
	}
	if res.Key != key {
		t.Errorf("Begin(%q,%q,%q): Key = %q, want %q", scope, key, fingerprint, res.Key, key)
	}
	switch res.Status {
	case idempotency.StatusNew:
		if res.Response != nil {
			t.Errorf("StatusNew must carry no Response, got %q", res.Response)
		}
	case idempotency.StatusInProgress, idempotency.StatusMismatch:
		if res.LeaseToken != "" {
			t.Errorf("%v must carry no LeaseToken, got %q", res.Status, res.LeaseToken)
		}
		if res.LeaseTTL != 0 {
			t.Errorf("%v must carry LeaseTTL 0, got %v", res.Status, res.LeaseTTL)
		}
		if res.Response != nil {
			t.Errorf("%v must carry no Response, got %q", res.Status, res.Response)
		}
	case idempotency.StatusCompleted:
		if res.LeaseToken != "" {
			t.Errorf("StatusCompleted must carry no LeaseToken, got %q", res.LeaseToken)
		}
		if res.LeaseTTL != 0 {
			t.Errorf("StatusCompleted must carry LeaseTTL 0, got %v", res.LeaseTTL)
		}
	default:
		t.Errorf("Begin returned an undefined Status %v with a nil error", res.Status)
	}
	return res
}

func beginErr(ctx context.Context, store idempotency.Store, scope, key, fingerprint string) error {
	_, err := store.Begin(ctx, scope, key, fingerprint)
	return err
}

func requireStatus(t *testing.T, res idempotency.Reservation, want idempotency.Status) {
	t.Helper()
	if res.Status != want {
		t.Fatalf("Status = %v, want %v", res.Status, want)
	}
}

func requireResponse(t *testing.T, res idempotency.Reservation, want []byte) {
	t.Helper()
	if !bytes.Equal(res.Response, want) {
		t.Fatalf("Response = %q, want %q", res.Response, want)
	}
}

func requireCode(t *testing.T, err error, want errorsx.Code, op string) {
	t.Helper()
	if got := errorsx.CodeOf(err); got != want {
		t.Fatalf("%s: code = %q (err=%v), want %q", op, got, err, want)
	}
}
