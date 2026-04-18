package eventbus

import "context"

// Inbox deduplicates incoming messages so at-least-once delivery becomes
// effectively-once at the consumer. Implementations typically persist the
// processed event ID in a store that is queried before handler invocation.
type Inbox interface {
	// Seen reports whether the given event ID has been processed. When
	// true, the consumer should ack without re-running the handler.
	Seen(ctx context.Context, eventID string) (bool, error)
	// Record marks the event ID as processed, typically within the same
	// transaction as the handler's side effects.
	Record(ctx context.Context, eventID string) error
}
