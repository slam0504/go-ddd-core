package pagination_test

import (
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/pagination"
)

func TestLimitAndCursorImplementPageRequest(t *testing.T) {
	var _ pagination.PageRequest = pagination.Limit{}
	var _ pagination.PageRequest = pagination.Cursor{}
}

func TestPageHasNext(t *testing.T) {
	cases := []struct {
		name string
		page pagination.Page[int]
		want bool
	}{
		{"empty cursor → no next", pagination.Page[int]{NextCursor: ""}, false},
		{"non-empty cursor → has next", pagination.Page[int]{NextCursor: "abc"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.page.HasNext(); got != tc.want {
				t.Fatalf("HasNext() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPageZeroValue(t *testing.T) {
	var p pagination.Page[string]
	if len(p.Items) != 0 {
		t.Fatalf("zero Page should have empty Items, got len=%d", len(p.Items))
	}
	if p.Total != 0 {
		t.Fatalf("zero Page should have Total=0, got %d", p.Total)
	}
	if p.HasNext() {
		t.Fatalf("zero Page should not HasNext")
	}
}

func TestTotalUnknownSentinel(t *testing.T) {
	p := pagination.Page[int]{Total: -1}
	if p.Total != -1 {
		t.Fatalf("Total=-1 should round-trip; got %d", p.Total)
	}
}
