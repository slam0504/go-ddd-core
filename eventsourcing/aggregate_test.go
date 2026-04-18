package eventsourcing_test

import (
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/eventsourcing"
)

type counter struct {
	eventsourcing.BaseESAggregate[string]
	value int
}

type incremented struct {
	domain.BaseEvent
	By int
}

func newCounter(id string) *counter {
	c := &counter{}
	c.BaseESAggregate = eventsourcing.NewBaseESAggregate[string](id, c.apply)
	return c
}

func (c *counter) apply(e domain.DomainEvent) error {
	if inc, ok := e.(incremented); ok {
		c.value += inc.By
	}
	return nil
}

func (c *counter) Increment(by int, eventID string) error {
	return c.Raise(incremented{
		BaseEvent: domain.NewBaseEvent(eventID, "counter.inc", c.ID(), "counter", c.Version()+1),
		By:        by,
	})
}

func TestBaseESAggregate_RaiseAppliesAndRecords(t *testing.T) {
	c := newCounter("c-1")
	if err := c.Increment(3, "e1"); err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if err := c.Increment(2, "e2"); err != nil {
		t.Fatalf("Increment: %v", err)
	}

	if c.value != 5 {
		t.Fatalf("value = %d, want 5", c.value)
	}
	if v := c.Version(); v != 2 {
		t.Fatalf("version = %d, want 2", v)
	}
	if got := len(c.DomainEvents()); got != 2 {
		t.Fatalf("recorded events = %d, want 2", got)
	}
}

func TestBaseESAggregate_LoadHistoryReplaysWithoutRecording(t *testing.T) {
	history := []domain.DomainEvent{
		incremented{BaseEvent: domain.NewBaseEvent("e1", "counter.inc", "c-1", "counter", 1), By: 5},
		incremented{BaseEvent: domain.NewBaseEvent("e2", "counter.inc", "c-1", "counter", 2), By: 7},
	}

	c := newCounter("c-1")
	if err := c.LoadHistory(history); err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}

	if c.value != 12 {
		t.Fatalf("value after replay = %d, want 12", c.value)
	}
	if v := c.Version(); v != 2 {
		t.Fatalf("version after replay = %d, want 2", v)
	}
	if got := len(c.DomainEvents()); got != 0 {
		t.Fatalf("events after replay = %d, want 0", got)
	}
}
