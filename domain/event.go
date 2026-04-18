package domain

import "time"

// DomainEvent represents something that happened in the domain. Events are
// immutable facts and should be named in the past tense (e.g. OrderPlaced).
type DomainEvent interface {
	EventID() string
	EventName() string
	AggregateID() string
	AggregateType() string
	OccurredAt() time.Time
	Version() int64
}

// BaseEvent is an embeddable struct implementing DomainEvent metadata.
// Concrete events embed it and add their own payload fields.
type BaseEvent struct {
	ID            string    `json:"event_id"`
	Name          string    `json:"event_name"`
	AggregateKey  string    `json:"aggregate_id"`
	AggregateKind string    `json:"aggregate_type"`
	At            time.Time `json:"occurred_at"`
	Ver           int64     `json:"version"`
}

// NewBaseEvent constructs a BaseEvent. The caller is responsible for supplying
// a unique event ID; core does not bind to a specific ID generator.
func NewBaseEvent(id, name, aggregateID, aggregateType string, version int64) BaseEvent {
	return BaseEvent{
		ID:            id,
		Name:          name,
		AggregateKey:  aggregateID,
		AggregateKind: aggregateType,
		At:            time.Now().UTC(),
		Ver:           version,
	}
}

func (e BaseEvent) EventID() string       { return e.ID }
func (e BaseEvent) EventName() string     { return e.Name }
func (e BaseEvent) AggregateID() string   { return e.AggregateKey }
func (e BaseEvent) AggregateType() string { return e.AggregateKind }
func (e BaseEvent) OccurredAt() time.Time { return e.At }
func (e BaseEvent) Version() int64        { return e.Ver }
