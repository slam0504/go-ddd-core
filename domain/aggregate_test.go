package domain_test

import (
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
)

type testAgg struct {
	domain.BaseAggregate[string]
}

func TestBaseAggregate_RecordAndClear(t *testing.T) {
	a := &testAgg{BaseAggregate: domain.NewBaseAggregate[string]("id-1")}

	if got := a.ID(); got != "id-1" {
		t.Fatalf("ID = %q, want id-1", got)
	}
	if v := a.Version(); v != 0 {
		t.Fatalf("initial version = %d, want 0", v)
	}
	if got := len(a.DomainEvents()); got != 0 {
		t.Fatalf("initial events = %d, want 0", got)
	}

	e1 := domain.NewBaseEvent("evt-1", "test.happened", "id-1", "testAgg", 1)
	e2 := domain.NewBaseEvent("evt-2", "test.happened", "id-1", "testAgg", 2)
	a.Record(e1)
	a.Record(e2)
	a.IncrementVersion()
	a.IncrementVersion()

	events := a.DomainEvents()
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].EventID() != "evt-1" || events[1].EventID() != "evt-2" {
		t.Fatalf("event order unexpected: %v, %v", events[0].EventID(), events[1].EventID())
	}
	if v := a.Version(); v != 2 {
		t.Fatalf("version after increments = %d, want 2", v)
	}

	a.ClearEvents()
	if got := len(a.DomainEvents()); got != 0 {
		t.Fatalf("events after ClearEvents = %d, want 0", got)
	}
}

func TestBaseAggregate_DomainEventsReturnsCopy(t *testing.T) {
	a := &testAgg{BaseAggregate: domain.NewBaseAggregate[string]("id-2")}
	a.Record(domain.NewBaseEvent("e", "x", "id-2", "t", 1))

	events := a.DomainEvents()
	events[0] = domain.NewBaseEvent("tampered", "x", "id-2", "t", 1)

	if got := a.DomainEvents()[0].EventID(); got != "e" {
		t.Fatalf("aggregate mutated via returned slice: got %q", got)
	}
}

func TestBaseEntity_Equal(t *testing.T) {
	a := domain.NewBaseEntity[string]("x")
	b := domain.NewBaseEntity[string]("x")
	c := domain.NewBaseEntity[string]("y")

	if !a.Equal(b) {
		t.Fatal("a.Equal(b) = false, want true")
	}
	if a.Equal(c) {
		t.Fatal("a.Equal(c) = true, want false")
	}
	if a.Equal(nil) {
		t.Fatal("a.Equal(nil) = true, want false")
	}
}
