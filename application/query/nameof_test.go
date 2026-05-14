package query_test

import (
	"context"
	"testing"

	"github.com/slam0504/go-ddd-core/application/query"
)

type implicitQ struct{ K string }

type implicitHandler struct{}

func (implicitHandler) Handle(_ context.Context, q implicitQ) (string, error) {
	return "implicit:" + q.K, nil
}

func TestNameOf_FallsBackToTypeName(t *testing.T) {
	if got := query.NameOf(implicitQ{}); got != "implicitQ" {
		t.Fatalf("got %q, want implicitQ", got)
	}
}

func TestNameOf_PrefersExplicitMethod(t *testing.T) {
	if got := query.NameOf(lookup{}); got != "lookup" {
		t.Fatalf("got %q, want lookup", got)
	}
}

func TestNameOf_NilIsEmpty(t *testing.T) {
	if got := query.NameOf(nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestRegister_TypeOnlyQueryIsRouted(t *testing.T) {
	bus := query.NewInMemoryBus()
	query.Register[implicitQ, string](bus, implicitHandler{})

	res, err := bus.Dispatch(context.Background(), implicitQ{K: "x"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got, _ := res.(string); got != "implicit:x" {
		t.Fatalf("got %v, want implicit:x", res)
	}
}

type pointerExplicitQ struct{ Tag string }

func (*pointerExplicitQ) QueryName() string { return "pointer.explicit.q" }

type pointerExplicitQHandler struct{}

func (pointerExplicitQHandler) Handle(_ context.Context, q *pointerExplicitQ) (string, error) {
	return "got:" + q.Tag, nil
}

// Guards Register/Dispatch name-resolution symmetry for pointer query types
// whose QueryName() is declared on a pointer receiver. See the matching
// command-side test for the underlying invariant.
func TestRegister_PointerQueryWithExplicitNameIsRouted(t *testing.T) {
	bus := query.NewInMemoryBus()
	query.Register[*pointerExplicitQ, string](bus, pointerExplicitQHandler{})

	res, err := bus.Dispatch(context.Background(), &pointerExplicitQ{Tag: "ok"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got, _ := res.(string); got != "got:ok" {
		t.Fatalf("got %v, want got:ok", res)
	}
}
