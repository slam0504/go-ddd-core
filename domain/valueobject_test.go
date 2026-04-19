package domain_test

import (
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
)

type money struct {
	Amount   int64
	Currency string
}

func (m money) Equal(other domain.ValueObject) bool {
	o, ok := other.(money)
	return ok && domain.EqualValues(m, o)
}

type tags struct {
	Names []string
}

func (t tags) Equal(other domain.ValueObject) bool {
	o, ok := other.(tags)
	return ok && domain.DeepEqualValues(t, o)
}

func TestEqualValues_Comparable(t *testing.T) {
	a := money{Amount: 100, Currency: "USD"}
	b := money{Amount: 100, Currency: "USD"}
	c := money{Amount: 200, Currency: "USD"}
	if !a.Equal(b) {
		t.Fatalf("expected a == b")
	}
	if a.Equal(c) {
		t.Fatalf("expected a != c")
	}
}

func TestDeepEqualValues_NonComparable(t *testing.T) {
	a := tags{Names: []string{"x", "y"}}
	b := tags{Names: []string{"x", "y"}}
	c := tags{Names: []string{"x", "z"}}
	if !a.Equal(b) {
		t.Fatalf("expected a == b")
	}
	if a.Equal(c) {
		t.Fatalf("expected a != c")
	}
}

func TestEqualRejectsDifferentType(t *testing.T) {
	a := money{Amount: 1, Currency: "USD"}
	b := tags{Names: []string{"x"}}
	if a.Equal(b) {
		t.Fatalf("Equal across different types should be false")
	}
}
