package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/application"
	"github.com/slam0504/go-ddd-core/ports/database"
)

// stubTxManager records what it received and forwards to fn, so the bridge
// can be verified without a real database driver.
type stubTxManager struct {
	calls int
	got   context.Context
}

func (s *stubTxManager) WithinTx(ctx context.Context, fn database.TxFunc) error {
	s.calls++
	s.got = ctx
	return fn(ctx)
}

func TestUnitOfWorkFromTxManager_ForwardsCtxAndFn(t *testing.T) {
	tm := &stubTxManager{}
	uow := application.UnitOfWorkFromTxManager(tm)

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")

	var seen any
	err := uow.Do(ctx, func(inner context.Context) error {
		seen = inner.Value(ctxKey{})
		return nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if tm.calls != 1 {
		t.Fatalf("TxManager calls = %d, want 1", tm.calls)
	}
	if seen != "marker" {
		t.Fatalf("ctx value not propagated, got %v", seen)
	}
}

func TestUnitOfWorkFromTxManager_PropagatesError(t *testing.T) {
	tm := &stubTxManager{}
	uow := application.UnitOfWorkFromTxManager(tm)

	want := errors.New("boom")
	got := uow.Do(context.Background(), func(_ context.Context) error { return want })
	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
}
