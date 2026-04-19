package command_test

import (
	"context"
	"testing"

	"github.com/slam0504/go-ddd-core/application/command"
)

type explicitName struct{ X int }

func (explicitName) CommandName() string { return "explicit_name" }

type implicitName struct{ Y int }

type pointerExplicit struct{ Z int }

func (*pointerExplicit) CommandName() string { return "pointer_explicit" }

func TestNameOf_PrefersExplicitMethod(t *testing.T) {
	if got := command.NameOf(explicitName{}); got != "explicit_name" {
		t.Fatalf("got %q, want explicit_name", got)
	}
}

func TestNameOf_FallsBackToTypeName(t *testing.T) {
	if got := command.NameOf(implicitName{}); got != "implicitName" {
		t.Fatalf("got %q, want implicitName", got)
	}
}

func TestNameOf_StripsPointerLevels(t *testing.T) {
	v := &implicitName{}
	if got := command.NameOf(v); got != "implicitName" {
		t.Fatalf("got %q, want implicitName", got)
	}
}

func TestNameOf_NilReturnsEmpty(t *testing.T) {
	if got := command.NameOf(nil); got != "" {
		t.Fatalf("nil should yield empty name, got %q", got)
	}
}

func TestNameOf_PointerReceiverCommandNameWorks(t *testing.T) {
	if got := command.NameOf(&pointerExplicit{}); got != "pointer_explicit" {
		t.Fatalf("got %q, want pointer_explicit", got)
	}
}

// Register-side: verify nameOfType picks the right key both with and without
// CommandName(). We exercise this end-to-end by dispatching against
// InMemoryBus rather than calling the unexported helper directly.

type implicitHandler struct{}

func (implicitHandler) Handle(_ context.Context, c implicitName) (int, error) {
	return c.Y * 10, nil
}

func TestRegister_TypeOnlyHandlerIsRouted(t *testing.T) {
	bus := command.NewInMemoryBus()
	command.Register[implicitName, int](bus, implicitHandler{})

	res, err := bus.Dispatch(context.Background(), implicitName{Y: 4})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got, _ := res.(int); got != 40 {
		t.Fatalf("got %v, want 40", res)
	}
}
