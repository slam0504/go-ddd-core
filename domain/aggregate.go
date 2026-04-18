package domain

// AggregateRoot is an Entity that owns a consistency boundary and records
// DomainEvents produced by state transitions.
type AggregateRoot[ID comparable] interface {
	Entity[ID]
	DomainEvents() []DomainEvent
	ClearEvents()
	Version() int64
}

// BaseAggregate is an embeddable struct that provides event recording and
// versioning. Concrete aggregates embed it and call Record on state changes.
type BaseAggregate[ID comparable] struct {
	BaseEntity[ID]
	events  []DomainEvent
	version int64
}

// NewBaseAggregate constructs a BaseAggregate at version 0.
func NewBaseAggregate[ID comparable](id ID) BaseAggregate[ID] {
	return BaseAggregate[ID]{BaseEntity: NewBaseEntity(id)}
}

// Record appends a DomainEvent to be published after the aggregate is persisted.
func (a *BaseAggregate[ID]) Record(event DomainEvent) {
	a.events = append(a.events, event)
}

// DomainEvents returns a copy of pending events.
func (a *BaseAggregate[ID]) DomainEvents() []DomainEvent {
	out := make([]DomainEvent, len(a.events))
	copy(out, a.events)
	return out
}

// ClearEvents drops all pending events. Callers should invoke this after
// the events have been safely dispatched or written to an outbox.
func (a *BaseAggregate[ID]) ClearEvents() { a.events = nil }

// Version returns the current aggregate version.
func (a *BaseAggregate[ID]) Version() int64 { return a.version }

// SetVersion overrides the version; typically used after load/save.
func (a *BaseAggregate[ID]) SetVersion(v int64) { a.version = v }

// IncrementVersion bumps the version by one.
func (a *BaseAggregate[ID]) IncrementVersion() { a.version++ }
