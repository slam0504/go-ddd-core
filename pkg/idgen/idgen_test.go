package idgen_test

import (
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/idgen"
)

func TestUUIDv4_Unique(t *testing.T) {
	g := idgen.UUIDv4()
	a, b := g.New(), g.New()
	if a == "" || b == "" {
		t.Fatalf("empty id: %q %q", a, b)
	}
	if a == b {
		t.Fatalf("expected unique ids, got %q twice", a)
	}
}

func TestUUIDv7_Sortable(t *testing.T) {
	g := idgen.UUIDv7()
	a := g.New()
	b := g.New()
	if a == "" || b == "" {
		t.Fatal("empty id")
	}
	if a == b {
		t.Fatalf("expected different ids, got %q", a)
	}
}

func TestGeneratorFunc_Adapts(t *testing.T) {
	g := idgen.GeneratorFunc(func() string { return "fixed" })
	if g.New() != "fixed" {
		t.Fatal("adapter did not call the function")
	}
}
