// Package memrepo is an in-memory example adapter implementing the Order
// repository. It exists to keep the example self-contained; real services
// should provide a SQL/NoSQL implementation in their own infra layer.
package memrepo

import (
	"context"
	"sync"

	"github.com/slam0504/go-ddd-core/domain"

	orderdom "github.com/slam0504/go-ddd-core/examples/minimal/domain/order"
)

// OrderRepository satisfies orderdom.Repository using an in-memory map.
type OrderRepository struct {
	mu    sync.RWMutex
	store map[orderdom.ID]*orderdom.Order
}

// NewOrderRepository constructs an empty repository.
func NewOrderRepository() *OrderRepository {
	return &OrderRepository{store: map[orderdom.ID]*orderdom.Order{}}
}

// FindByID retrieves an order by id.
func (r *OrderRepository) FindByID(_ context.Context, id orderdom.ID) (*orderdom.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.store[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return o, nil
}

// Save persists the aggregate.
func (r *OrderRepository) Save(_ context.Context, o *orderdom.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[o.ID()] = o
	return nil
}

// Delete removes the aggregate.
func (r *OrderRepository) Delete(_ context.Context, id orderdom.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	return nil
}
