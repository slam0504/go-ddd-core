// Package database defines transaction and connectivity contracts. The core
// does not prescribe a query API — repositories are defined in the domain
// layer and implemented by adapters that speak SQL, NoSQL, or anything else.
package database

import (
	"context"
	"errors"
)

// TxFunc runs inside a transactional boundary. Returning a non-nil error
// triggers rollback; returning nil commits.
type TxFunc func(ctx context.Context) error

// TxManager starts a transaction and invokes fn within it. Implementations
// typically inject a driver-specific handle into ctx so repositories can
// pick it up via context keys.
type TxManager interface {
	WithinTx(ctx context.Context, fn TxFunc) error
}

// TxOption configures a single transaction (isolation, read-only, etc.).
// Concrete options live in adapters; the core ships only this opaque type.
type TxOption interface {
	apply(any)
}

// Pinger verifies connectivity. Used by health checks.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Closer releases underlying resources.
type Closer interface {
	Close() error
}

// ErrNoRows is the sentinel returned by adapters when a query expected a row
// but found none. Domain code should prefer checking domain.ErrNotFound from
// the repository layer instead of this error.
var ErrNoRows = errors.New("database: no rows")
