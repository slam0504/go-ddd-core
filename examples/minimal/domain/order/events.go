package order

import "github.com/slam0504/go-ddd-core/domain"

// OrderPlaced is emitted when an Order transitions from draft to placed.
type OrderPlaced struct {
	domain.BaseEvent
	CustomerID string
	TotalCents int64
}

// NewOrderPlaced constructs an OrderPlaced event.
func NewOrderPlaced(eventID, aggregateID string, version int64, customerID string, totalCents int64) OrderPlaced {
	return OrderPlaced{
		BaseEvent:  domain.NewBaseEvent(eventID, "order.placed", aggregateID, "Order", version),
		CustomerID: customerID,
		TotalCents: totalCents,
	}
}
