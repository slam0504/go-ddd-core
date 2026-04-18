// Package eventsourcing provides contracts for event-sourced aggregates:
// EventStore, SnapshotStore, Projector, and a BaseESAggregate helper.
package eventsourcing

import (
	"github.com/slam0504/go-ddd-core/domain"
)

// Applier mutates aggregate state for a single DomainEvent. Applier must be
// pure — no side effects beyond in-memory state.
type Applier interface {
	Apply(event domain.DomainEvent) error
}

// ESAggregate is an AggregateRoot that can rebuild its state from events.
type ESAggregate[ID comparable] interface {
	domain.AggregateRoot[ID]
	Applier
}

// BaseESAggregate composes domain.BaseAggregate with an injected apply
// function so concrete aggregates can stay free of framework boilerplate.
//
// Usage:
//
//	type Order struct { eventsourcing.BaseESAggregate[OrderID]; ... }
//	func NewOrder(id OrderID) *Order {
//	    o := &Order{}
//	    o.BaseESAggregate = eventsourcing.NewBaseESAggregate(id, o.apply)
//	    return o
//	}
//	func (o *Order) apply(e domain.DomainEvent) error { ... }
type BaseESAggregate[ID comparable] struct {
	domain.BaseAggregate[ID]
	apply func(domain.DomainEvent) error
}

// NewBaseESAggregate constructs a BaseESAggregate. The apply function
// receives each event — from Raise or from LoadHistory — and should mutate
// in-memory state without emitting further events.
func NewBaseESAggregate[ID comparable](id ID, apply func(domain.DomainEvent) error) BaseESAggregate[ID] {
	return BaseESAggregate[ID]{
		BaseAggregate: domain.NewBaseAggregate(id),
		apply:         apply,
	}
}

// Raise applies an event to the aggregate, records it for publication, and
// bumps the version. Command handlers call this after checking invariants.
func (a *BaseESAggregate[ID]) Raise(event domain.DomainEvent) error {
	if err := a.apply(event); err != nil {
		return err
	}
	a.Record(event)
	a.IncrementVersion()
	return nil
}

// LoadHistory replays persisted events to rebuild state. It does not record
// events for publication and increments version per event.
func (a *BaseESAggregate[ID]) LoadHistory(events []domain.DomainEvent) error {
	for _, e := range events {
		if err := a.apply(e); err != nil {
			return err
		}
		a.IncrementVersion()
	}
	a.ClearEvents()
	return nil
}

// Apply satisfies the Applier interface by delegating to the injected func.
func (a *BaseESAggregate[ID]) Apply(event domain.DomainEvent) error {
	return a.apply(event)
}
