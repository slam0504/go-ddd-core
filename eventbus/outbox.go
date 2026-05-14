package eventbus

import (
	"context"
	"time"

	"github.com/slam0504/go-ddd-core/domain"
)

// OutboxRecord is a serialised DomainEvent staged for reliable delivery.
// Concrete outbox storage (SQL table, KV store, etc.) is an adapter concern.
//
// ID and EventID are distinct on purpose. ID identifies the outbox row,
// chosen by the OutboxStore — typically a database sequence or UUID — so
// retries and ack bookkeeping address a specific staging entry. EventID is
// the domain event's identity carried by DomainEvent.EventID(); brokers use
// it as message-id / idempotency key and downstream Inboxes use it for
// dedup. Fan-out to multiple topics produces multiple rows with distinct
// IDs but a shared EventID.
type OutboxRecord struct {
	ID            string
	EventID       string
	Topic         string
	EventName     string
	AggregateID   string
	AggregateType string
	Payload       []byte
	Headers       map[string]string
	CreatedAt     time.Time
	AvailableAt   time.Time
	Attempts      int
}

// Outbox atomically stages events as part of the same transaction that
// persisted the aggregate, decoupling domain persistence from broker writes.
// Implementations must participate in the caller's transaction (typically by
// reading a tx handle from ctx).
type Outbox interface {
	Stage(ctx context.Context, topic string, events ...domain.DomainEvent) error
}

// OutboxStore is the read/ack side used by a Relay. It exposes the staging
// table for a background worker to drain.
type OutboxStore interface {
	// Fetch returns up to limit records that are due for delivery.
	Fetch(ctx context.Context, limit int) ([]OutboxRecord, error)
	// MarkSent records a successful publish.
	MarkSent(ctx context.Context, id string) error
	// MarkFailed bumps attempt count and defers the next attempt.
	MarkFailed(ctx context.Context, id string, reason string, nextAttemptAt time.Time) error
}

// Relay drains an OutboxStore and publishes records via a Publisher. It runs
// until ctx is cancelled.
type Relay interface {
	Run(ctx context.Context) error
}
