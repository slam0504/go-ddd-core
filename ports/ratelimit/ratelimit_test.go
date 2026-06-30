package ratelimit_test

import (
	"testing"
	"time"

	"github.com/slam0504/go-ddd-core/ports/ratelimit"
)

func TestUnknownCountIsNegativeOne(t *testing.T) {
	if ratelimit.UnknownCount != -1 {
		t.Fatalf("UnknownCount = %d, want -1 (mirrors pagination.Page.Total's absent sentinel)", ratelimit.UnknownCount)
	}
}

func TestHasLimit(t *testing.T) {
	cases := []struct {
		limit int
		want  bool
	}{
		{ratelimit.UnknownCount, false}, // -1 is absent
		{0, true},                       // a real limit of 0 is KNOWN, not absent
		{5, true},
	}
	for _, c := range cases {
		if got := (ratelimit.Result{Limit: c.limit}).HasLimit(); got != c.want {
			t.Fatalf("Result{Limit:%d}.HasLimit() = %v, want %v", c.limit, got, c.want)
		}
	}
}

func TestHasRemaining(t *testing.T) {
	cases := []struct {
		remaining int
		want      bool
	}{
		{ratelimit.UnknownCount, false}, // -1 is absent
		{0, true},                       // window exhausted: Remaining 0 is KNOWN, not absent
		{3, true},
	}
	for _, c := range cases {
		if got := (ratelimit.Result{Remaining: c.remaining}).HasRemaining(); got != c.want {
			t.Fatalf("Result{Remaining:%d}.HasRemaining() = %v, want %v", c.remaining, got, c.want)
		}
	}
}

func TestZeroValueResultTreatsCountsAsKnownZero(t *testing.T) {
	// A zero-value Result has Limit==0 / Remaining==0, which HasLimit/HasRemaining
	// report as KNOWN (not absent). Adapters that cannot honestly produce a count
	// MUST set it to UnknownCount explicitly; they cannot rely on the zero value
	// to mean absent. This pins the trap so it is documented, not discovered.
	var r ratelimit.Result
	if !r.HasLimit() {
		t.Fatal("zero-value Result.HasLimit() = false; want true (Limit==0 is known real 0; absent must be UnknownCount)")
	}
	if !r.HasRemaining() {
		t.Fatal("zero-value Result.HasRemaining() = false; want true (Remaining==0 is known)")
	}
}

func TestResultIsComparableValue(t *testing.T) {
	// Result is a flat value struct (no pointers); equal field values compare equal.
	a := ratelimit.Result{Allowed: true, RetryAfter: time.Second, Limit: 10, Remaining: 9}
	b := ratelimit.Result{Allowed: true, RetryAfter: time.Second, Limit: 10, Remaining: 9}
	if a != b {
		t.Fatal("identical Result values compare unequal; Result must be a comparable value type")
	}
}
