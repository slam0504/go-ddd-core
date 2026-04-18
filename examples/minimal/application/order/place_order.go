// Package order contains the write-side use cases for the Order aggregate.
package order

import (
	"context"

	"github.com/slam0504/go-ddd-core/eventbus"
	"github.com/slam0504/go-ddd-core/pkg/idgen"

	orderdom "github.com/slam0504/go-ddd-core/examples/minimal/domain/order"
)

// PlaceOrderCommand places a new Order for a customer.
type PlaceOrderCommand struct {
	OrderID    string
	CustomerID string
	Items      []orderdom.Item
}

// CommandName identifies the command for the bus.
func (PlaceOrderCommand) CommandName() string { return "order.place" }

// PlaceOrderResult returns the placed order id and total.
type PlaceOrderResult struct {
	OrderID    string
	TotalCents int64
}

// PlaceOrderHandler executes PlaceOrderCommand.
type PlaceOrderHandler struct {
	repo      orderdom.Repository
	publisher eventbus.Publisher
	topic     string
	ids       idgen.Generator
}

// NewPlaceOrderHandler constructs the handler. publisher may be nil in
// tests; when nil, domain events are kept on the aggregate but not
// dispatched to a broker.
func NewPlaceOrderHandler(repo orderdom.Repository, publisher eventbus.Publisher, topic string, ids idgen.Generator) *PlaceOrderHandler {
	return &PlaceOrderHandler{repo: repo, publisher: publisher, topic: topic, ids: ids}
}

// Handle runs the command.
func (h *PlaceOrderHandler) Handle(ctx context.Context, cmd PlaceOrderCommand) (PlaceOrderResult, error) {
	o := orderdom.New(orderdom.ID(cmd.OrderID), cmd.CustomerID)
	if err := o.Place(cmd.Items, h.ids.New()); err != nil {
		return PlaceOrderResult{}, err
	}
	if err := h.repo.Save(ctx, o); err != nil {
		return PlaceOrderResult{}, err
	}
	if h.publisher != nil {
		if err := h.publisher.Publish(ctx, h.topic, o.DomainEvents()...); err != nil {
			return PlaceOrderResult{}, err
		}
	}
	o.ClearEvents()
	return PlaceOrderResult{OrderID: cmd.OrderID, TotalCents: o.TotalCents()}, nil
}
