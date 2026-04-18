package domain_test

import (
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
)

func TestRuleViolation_IsMatchesSentinel(t *testing.T) {
	err := domain.NewRuleViolation("ORDER_EMPTY", "must have items")

	if !errors.Is(err, domain.ErrRuleViolation) {
		t.Fatal("errors.Is(err, ErrRuleViolation) = false")
	}
	if err.Code != "ORDER_EMPTY" || err.Message != "must have items" {
		t.Fatalf("fields wrong: %+v", err)
	}
	if got := err.Error(); got != "ORDER_EMPTY: must have items" {
		t.Fatalf("Error() = %q", got)
	}
}
