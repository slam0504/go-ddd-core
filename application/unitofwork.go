// Package application ties together CQRS primitives and transaction semantics.
package application

import "context"

// UnitOfWork runs fn within a single transactional boundary. The concrete
// transaction mechanism (SQL tx, saga, in-memory) is provided by an adapter.
// Implementations must propagate cancellation and commit when fn returns nil,
// rolling back on any non-nil error or panic.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// UnitOfWorkFunc adapts a function to the UnitOfWork interface.
type UnitOfWorkFunc func(ctx context.Context, fn func(ctx context.Context) error) error

func (f UnitOfWorkFunc) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return f(ctx, fn)
}
