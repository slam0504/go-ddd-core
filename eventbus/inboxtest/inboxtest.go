// Package inboxtest provides a deterministic conformance suite for
// implementations of eventbus.Inbox. Adapters call RunContract from their
// own tests; every subtest is deterministic (no sleeps, no containers).
//
// The suite pins the matured contract: duplicate Record returns
// ErrAlreadyRecorded (never silent success), Seen is an advisory fast-path,
// and both methods validate the key (CodeInvalidArgument) before consulting
// ctx and before any backend call. Concurrency-race behavior (blocking
// INSERT, loser rollback) is deliberately NOT here — it depends on backend
// locking and lives in adapter-specific integration tests.
package inboxtest

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/eventbus"
	"github.com/slam0504/go-ddd-core/pkg/errorsx"
)

// Factory returns a fresh, empty Inbox for one subtest. Implementations
// backed by shared state must isolate each call (fresh keyspace or store).
type Factory func(t *testing.T) eventbus.Inbox

// RunContract runs the Inbox conformance suite against newInbox.
func RunContract(t *testing.T, newInbox Factory) {
	t.Helper()
	ctx := context.Background()
	key := eventbus.InboxKey{Consumer: "consumer-a", EventID: "evt-1"}

	t.Run("SeenUnrecordedReturnsFalse", func(t *testing.T) {
		ibx := newInbox(t)
		seen, err := ibx.Seen(ctx, key)
		if err != nil {
			t.Fatalf("Seen: %v", err)
		}
		if seen {
			t.Fatal("Seen on unrecorded key = true, want false")
		}
	})

	t.Run("RecordThenSeenReturnsTrue", func(t *testing.T) {
		ibx := newInbox(t)
		if err := ibx.Record(ctx, key); err != nil {
			t.Fatalf("Record: %v", err)
		}
		seen, err := ibx.Seen(ctx, key)
		if err != nil {
			t.Fatalf("Seen: %v", err)
		}
		if !seen {
			t.Fatal("Seen after Record = false, want true")
		}
	})

	t.Run("DuplicateRecordReturnsErrAlreadyRecorded", func(t *testing.T) {
		ibx := newInbox(t)
		if err := ibx.Record(ctx, key); err != nil {
			t.Fatalf("first Record: %v", err)
		}
		err := ibx.Record(ctx, key)
		if !errors.Is(err, eventbus.ErrAlreadyRecorded) {
			t.Fatalf("duplicate Record err = %v, want ErrAlreadyRecorded", err)
		}
	})

	t.Run("PerConsumerIsolation", func(t *testing.T) {
		ibx := newInbox(t)
		a := eventbus.InboxKey{Consumer: "consumer-a", EventID: "evt-shared"}
		b := eventbus.InboxKey{Consumer: "consumer-b", EventID: "evt-shared"}
		if err := ibx.Record(ctx, a); err != nil {
			t.Fatalf("Record a: %v", err)
		}
		if err := ibx.Record(ctx, b); err != nil {
			t.Fatalf("Record b (same event, different consumer): %v", err)
		}
		seenB, err := ibx.Seen(ctx, b)
		if err != nil || !seenB {
			t.Fatalf("Seen b = (%v, %v), want (true, nil)", seenB, err)
		}
	})

	t.Run("PerEventIsolation", func(t *testing.T) {
		ibx := newInbox(t)
		e1 := eventbus.InboxKey{Consumer: "consumer-a", EventID: "evt-1"}
		e2 := eventbus.InboxKey{Consumer: "consumer-a", EventID: "evt-2"}
		if err := ibx.Record(ctx, e1); err != nil {
			t.Fatalf("Record e1: %v", err)
		}
		if err := ibx.Record(ctx, e2); err != nil {
			t.Fatalf("Record e2 (same consumer, different event): %v", err)
		}
	})

	t.Run("EmptyConsumerInvalidArgument", func(t *testing.T) {
		ibx := newInbox(t)
		bad := eventbus.InboxKey{Consumer: "", EventID: "evt-1"}
		if _, err := ibx.Seen(ctx, bad); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("Seen empty consumer: CodeOf = %v, want CodeInvalidArgument", errorsx.CodeOf(err))
		}
		if err := ibx.Record(ctx, bad); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("Record empty consumer: CodeOf = %v, want CodeInvalidArgument", errorsx.CodeOf(err))
		}
	})

	t.Run("EmptyEventIDInvalidArgument", func(t *testing.T) {
		ibx := newInbox(t)
		bad := eventbus.InboxKey{Consumer: "consumer-a", EventID: ""}
		if _, err := ibx.Seen(ctx, bad); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("Seen empty event id: CodeOf = %v, want CodeInvalidArgument", errorsx.CodeOf(err))
		}
		if err := ibx.Record(ctx, bad); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("Record empty event id: CodeOf = %v, want CodeInvalidArgument", errorsx.CodeOf(err))
		}
	})

	t.Run("EmptyKeyPrecedesCancelledCtx", func(t *testing.T) {
		ibx := newInbox(t)
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		bad := eventbus.InboxKey{Consumer: "", EventID: "evt-1"}
		if err := ibx.Record(cancelled, bad); errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("empty key + cancelled ctx: CodeOf = %v, want CodeInvalidArgument (key precedes ctx)", errorsx.CodeOf(err))
		}
	})

	t.Run("ValidKeyCancelledCtxReturnsCtxError", func(t *testing.T) {
		ibx := newInbox(t)
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := ibx.Seen(cancelled, key); !errors.Is(err, context.Canceled) {
			t.Fatalf("Seen cancelled ctx err = %v, want context.Canceled", err)
		}
		if err := ibx.Record(cancelled, key); !errors.Is(err, context.Canceled) {
			t.Fatalf("Record cancelled ctx err = %v, want context.Canceled", err)
		}
	})
}
