package eventbus

import (
	"context"
	"errors"
)

// InboxKey scopes idempotency to a specific consumer plus event. Dedup must
// be per-consumer because the same event is often delivered to several
// independent consumers — a projector, a reactor, a downstream saga — and
// each must record its own progress. Sharing a single global eventID-keyed
// inbox would let one consumer's record silently skip another consumer's
// handler.
//
// Consumer is a logical name chosen by the caller: a consumer group, a
// projector name, a handler tag — whatever uniquely identifies the
// processing position. EventID is the domain event id (DomainEvent.EventID()).
type InboxKey struct {
	Consumer string
	EventID  string
}

// ErrAlreadyRecorded is returned by Inbox.Record when the key is already
// recorded. A caller running Record inside the handler's transaction treats
// it as "duplicate delivery": roll the transaction back and acknowledge the
// message without re-running side effects. Implementations may wrap it;
// callers test with errors.Is.
var ErrAlreadyRecorded = errors.New("eventbus: inbox key already recorded")

// Inbox deduplicates incoming messages so at-least-once delivery becomes
// effectively-once per consumer.
//
// Semantics:
//
//   - Record MUST return ErrAlreadyRecorded (possibly wrapped) when key is
//     already recorded — including when the conflict only surfaces at
//     commit time under concurrent duplicate delivery. Duplicate Record is
//     never a silent success: silent success cannot roll the caller's
//     transaction back, which defeats the exactly-once guard.
//   - Seen is an advisory fast-path to skip handler work cheaply; it may be
//     stale under concurrent delivery. Correctness rests on Record running
//     in the same transaction as the handler's side effects and the caller
//     rolling back on ErrAlreadyRecorded.
//   - Validation precedence (both methods): an empty Consumer or EventID is
//     rejected with an errorsx.CodeInvalidArgument-coded error BEFORE the
//     ctx is consulted and BEFORE any backend call; a valid key under a
//     cancelled or expired ctx returns the ctx error verbatim.
type Inbox interface {
	// Seen reports whether key has been recorded. When true, the consumer
	// should ack without re-running the handler. Advisory under
	// concurrency — see the interface comment.
	Seen(ctx context.Context, key InboxKey) (bool, error)
	// Record marks key as processed, typically within the same transaction
	// as the handler's side effects. Returns ErrAlreadyRecorded (possibly
	// wrapped) when key was already recorded.
	Record(ctx context.Context, key InboxKey) error
}
