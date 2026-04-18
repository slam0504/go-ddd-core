package eventbus

import (
	"context"
	"time"

	"github.com/slam0504/go-ddd-core/domain"
)

// OutboxRecord is a serialised DomainEvent staged for reliable delivery.
// Concrete outbox storage (SQL table, KV store, etc.) is an adapter concern.
type OutboxRecord struct {
	ID            string
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
