package health_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/ports/health"
)

func TestNewCheck_NameAndHealthy(t *testing.T) {
	c := health.NewCheck("postgres", func(context.Context) error { return nil })

	if got, want := c.Name(), "postgres"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

func TestNewCheck_PropagatesError(t *testing.T) {
	want := errors.New("connection refused")
	c := health.NewCheck("redis", func(context.Context) error { return want })

	got := c.Check(context.Background())
	if !errors.Is(got, want) {
		t.Errorf("Check() = %v, want %v (via errors.Is)", got, want)
	}
}

func TestNewCheck_PassesContext(t *testing.T) {
	type ctxKey struct{}
	want := "marker-value"

	c := health.NewCheck("ctx-probe", func(ctx context.Context) error {
		got, ok := ctx.Value(ctxKey{}).(string)
		if !ok || got != want {
			return errors.New("context value not propagated")
		}
		return nil
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, want)
	if err := c.Check(ctx); err != nil {
		t.Errorf("Check propagated wrong context: %v", err)
	}
}

// staticCheck verifies that a hand-written struct can also implement
// the Check interface without going through NewCheck. The contract
// is intentionally open to direct implementation; NewCheck is only
// the convenience path for wrapping plain functions.
type staticCheck struct {
	label string
	err   error
}

func (s staticCheck) Name() string                { return s.label }
func (s staticCheck) Check(context.Context) error { return s.err }

func TestCheck_DirectImplementationSatisfiesInterface(t *testing.T) {
	var c health.Check = staticCheck{label: "billing-api", err: nil}

	if got, want := c.Name(), "billing-api"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}
