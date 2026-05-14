// Package application ties together CQRS primitives and transaction semantics.
package application

import (
	"context"

	"github.com/slam0504/go-ddd-core/ports/database"
)

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

// UnitOfWorkFromTxManager adapts a database-level TxManager into an
// application-level UnitOfWork. Use cases depend on UnitOfWork to stay
// decoupled from the database driver; adapters implement TxManager and pass
// it through this bridge during wiring.
//
// The bridge is intentionally thin: the TxManager owns transaction lifecycle
// and driver-specific context propagation (e.g. injecting *sql.Tx or *gorm.DB
// into ctx). UnitOfWork.Do just forwards the contextualised closure.
func UnitOfWorkFromTxManager(tm database.TxManager) UnitOfWork {
	return UnitOfWorkFunc(func(ctx context.Context, fn func(ctx context.Context) error) error {
		return tm.WithinTx(ctx, database.TxFunc(fn))
	})
}
