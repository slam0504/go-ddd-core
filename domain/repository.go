package domain

import "context"

// Repository is the contract for persisting and retrieving aggregates. Concrete
// repositories belong to the infrastructure layer; domain code depends only on
// this interface.
type Repository[T AggregateRoot[ID], ID comparable] interface {
	FindByID(ctx context.Context, id ID) (T, error)
	Save(ctx context.Context, aggregate T) error
	Delete(ctx context.Context, id ID) error
}

// ReadOnlyRepository is a narrowed contract for read paths (CQRS queries).
type ReadOnlyRepository[T any, ID comparable] interface {
	FindByID(ctx context.Context, id ID) (T, error)
}
