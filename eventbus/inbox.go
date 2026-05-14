package eventbus

import "context"

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

// Inbox deduplicates incoming messages so at-least-once delivery becomes
// effectively-once per consumer. Implementations typically persist the
// processed (consumer, event_id) pair in a store that is queried before
// handler invocation.
type Inbox interface {
	// Seen reports whether key has been recorded. When true, the consumer
	// should ack without re-running the handler.
	Seen(ctx context.Context, key InboxKey) (bool, error)
	// Record marks key as processed, typically within the same transaction
	// as the handler's side effects.
	Record(ctx context.Context, key InboxKey) error
}
