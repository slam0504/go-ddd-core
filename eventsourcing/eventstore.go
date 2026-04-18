package eventsourcing

import (
	"context"
	"errors"

	"github.com/slam0504/go-ddd-core/domain"
)

// StreamID identifies an event stream. Adapters are free to encode it as
// "aggregateType-aggregateID" or as separate columns.
type StreamID struct {
	AggregateType string
	AggregateID   string
}

// PersistedEvent is an EventStore record with its global and stream position.
type PersistedEvent struct {
	Event          domain.DomainEvent
	StreamID       StreamID
	StreamVersion  int64
	GlobalPosition int64
}

// ExpectedVersion constants for optimistic concurrency on Append.
const (
	// ExpectedVersionAny skips the concurrency check.
	ExpectedVersionAny int64 = -1
	// ExpectedVersionNew requires the stream to not yet exist.
	ExpectedVersionNew int64 = 0
)

// EventStore is the persistence contract for event-sourced aggregates.
type EventStore interface {
	// Append writes events to the stream, enforcing optimistic concurrency
	// against expectedVersion. On conflict it returns ErrConcurrency.
	Append(ctx context.Context, stream StreamID, expectedVersion int64, events []domain.DomainEvent) error

	// Load returns all events for the stream plus the current version.
	Load(ctx context.Context, stream StreamID) (events []domain.DomainEvent, version int64, err error)

	// LoadFrom returns events from fromVersion (exclusive) onwards. It is
	// used in combination with snapshots to minimise replay work.
	LoadFrom(ctx context.Context, stream StreamID, fromVersion int64) (events []domain.DomainEvent, version int64, err error)
}

// AllStream is an optional capability for stores that expose a global stream,
// typically consumed by projectors and reactors.
type AllStream interface {
	// ReadAll returns events starting at fromPosition up to limit records.
	ReadAll(ctx context.Context, fromPosition int64, limit int) ([]PersistedEvent, error)
	// SubscribeAll streams events in real time from fromPosition.
	SubscribeAll(ctx context.Context, fromPosition int64) (<-chan PersistedEvent, error)
}

// ErrConcurrency is returned on optimistic concurrency violations.
var ErrConcurrency = errors.New("eventsourcing: concurrency conflict")
