package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/eventbus/inbox"
)

func TestSeen_FalseBeforeRecord(t *testing.T) {
	in := inbox.NewMemory()
	seen, err := in.Seen(context.Background(), "evt-1")
	if err != nil {
		t.Fatalf("Seen: %v", err)
	}
	if seen {
		t.Fatalf("Seen should be false before Record")
	}
}

func TestRecord_ThenSeenIsTrue(t *testing.T) {
	in := inbox.NewMemory()
	if err := in.Record(context.Background(), "evt-1"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	seen, _ := in.Seen(context.Background(), "evt-1")
	if !seen {
		t.Fatalf("Seen should be true after Record")
	}
}

func TestRecord_IsIdempotent(t *testing.T) {
	in := inbox.NewMemory()
	for i := 0; i < 5; i++ {
		if err := in.Record(context.Background(), "evt-1"); err != nil {
			t.Fatalf("Record #%d: %v", i, err)
		}
	}
	if got := in.Size(); got != 1 {
		t.Fatalf("Size = %d, want 1", got)
	}
}

func TestEviction_TriggersOnceMaxSizeExceeded(t *testing.T) {
	tick := time.Unix(0, 0)
	clock := func() time.Time {
		tick = tick.Add(time.Second)
		return tick
	}
	in := inbox.NewMemory(inbox.WithMaxSize(4), inbox.WithClock(clock))

	for i, id := range []string{"a", "b", "c", "d", "e"} {
		if err := in.Record(context.Background(), id); err != nil {
			t.Fatalf("Record #%d: %v", i, err)
		}
	}
	// After insert of "e" the map exceeded 4 → oldest half (2 entries) evicted.
	if got := in.Size(); got != 3 {
		t.Fatalf("Size after eviction = %d, want 3", got)
	}
	// "a" and "b" are oldest and should be gone; "c", "d", "e" remain.
	for _, id := range []string{"a", "b"} {
		if seen, _ := in.Seen(context.Background(), id); seen {
			t.Fatalf("expected %q evicted", id)
		}
	}
	for _, id := range []string{"c", "d", "e"} {
		if seen, _ := in.Seen(context.Background(), id); !seen {
			t.Fatalf("expected %q retained", id)
		}
	}
}

func TestUnboundedByDefault(t *testing.T) {
	in := inbox.NewMemory()
	for i := 0; i < 1000; i++ {
		_ = in.Record(context.Background(), idString(i))
	}
	if got := in.Size(); got != 1000 {
		t.Fatalf("default should not evict, got Size=%d", got)
	}
}

func idString(i int) string {
	return "evt-" + intToStr(i)
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	return string(s)
}
