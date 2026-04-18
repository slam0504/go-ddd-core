package errorsx_test

import (
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
)

func TestNewAndError(t *testing.T) {
	e := errorsx.New(errorsx.CodeNotFound, "missing user")
	if got := e.Error(); got != "not_found: missing user" {
		t.Fatalf("Error() = %q", got)
	}
	if errorsx.CodeOf(e) != errorsx.CodeNotFound {
		t.Fatalf("CodeOf = %v", errorsx.CodeOf(e))
	}
}

func TestWrapPreservesCauseAndIs(t *testing.T) {
	cause := errors.New("db down")
	e := errorsx.Wrap(errorsx.CodeUnavailable, "lookup failed", cause)

	if !errors.Is(e, cause) {
		t.Fatal("errors.Is(e, cause) = false")
	}
	if got := e.Error(); got != "unavailable: lookup failed: db down" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestWithDetailAttaches(t *testing.T) {
	e := errorsx.New(errorsx.CodeInvalidArgument, "bad input").
		WithDetail("field", "email").
		WithDetail("reason", "empty")

	if e.Details["field"] != "email" || e.Details["reason"] != "empty" {
		t.Fatalf("details = %v", e.Details)
	}
}

func TestWithDetail_DoesNotMutateReceiver(t *testing.T) {
	sentinel := errorsx.New(errorsx.CodeNotFound, "not found")

	withA := sentinel.WithDetail("id", "a")
	withB := sentinel.WithDetail("id", "b")

	if len(sentinel.Details) != 0 {
		t.Fatalf("sentinel mutated: %v", sentinel.Details)
	}
	if withA.Details["id"] != "a" || withB.Details["id"] != "b" {
		t.Fatalf("derived errors cross-contaminated: %v / %v", withA.Details, withB.Details)
	}
}

func TestCodeOf_UnknownForPlainError(t *testing.T) {
	if got := errorsx.CodeOf(errors.New("x")); got != errorsx.CodeUnknown {
		t.Fatalf("CodeOf(plain) = %v, want unknown", got)
	}
}
