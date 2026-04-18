package order

import (
	"context"

	orderdom "github.com/slam0504/go-ddd-core/examples/minimal/domain/order"
)

// GetOrderQuery reads a single Order by id.
type GetOrderQuery struct {
	OrderID string
}

// QueryName identifies the query for the bus.
func (GetOrderQuery) QueryName() string { return "order.get" }

// OrderView is the read-side DTO returned to callers.
type OrderView struct {
	OrderID    string
	CustomerID string
	Status     string
	TotalCents int64
	Version    int64
}

// GetOrderHandler executes GetOrderQuery. In a larger system this would
// read from a projected read model, not the write-side repository.
type GetOrderHandler struct {
	repo orderdom.Repository
}

// NewGetOrderHandler constructs the handler.
func NewGetOrderHandler(repo orderdom.Repository) *GetOrderHandler {
	return &GetOrderHandler{repo: repo}
}

// Handle runs the query.
func (h *GetOrderHandler) Handle(ctx context.Context, q GetOrderQuery) (OrderView, error) {
	o, err := h.repo.FindByID(ctx, orderdom.ID(q.OrderID))
	if err != nil {
		return OrderView{}, err
	}
	return OrderView{
		OrderID:    string(o.ID()),
		CustomerID: o.CustomerID(),
		Status:     string(o.Status()),
		TotalCents: o.TotalCents(),
		Version:    o.Version(),
	}, nil
}
