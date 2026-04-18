package eventsourcing_test

import (
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/eventsourcing"
)

func TestErrConcurrency_IsDomainErrConcurrencyConflict(t *testing.T) {
	if !errors.Is(eventsourcing.ErrConcurrency, domain.ErrConcurrencyConflict) {
		t.Fatal("eventsourcing.ErrConcurrency must unwrap to domain.ErrConcurrencyConflict")
	}
}
