// Package order is an example aggregate demonstrating how a downstream
// service uses go-ddd-core domain primitives. Order models a simple
// purchase with items, total, and status transitions.
package order

import (
	"github.com/slam0504/go-ddd-core/domain"
)

// ID is the aggregate identifier.
type ID string

// Status represents the order lifecycle state.
type Status string

const (
	StatusDraft  Status = "draft"
	StatusPlaced Status = "placed"
)

// Item is a value object within the Order aggregate.
type Item struct {
	SKU      string
	Quantity int
	PriceCents int64
}

// Order is the aggregate root.
type Order struct {
	domain.BaseAggregate[ID]
	customerID string
	items      []Item
	status     Status
	totalCents int64
}

// New constructs a draft Order.
func New(id ID, customerID string) *Order {
	o := &Order{
		BaseAggregate: domain.NewBaseAggregate(id),
		customerID:    customerID,
		status:        StatusDraft,
	}
	return o
}

// CustomerID returns the customer identifier.
func (o *Order) CustomerID() string { return o.customerID }

// Status returns the current status.
func (o *Order) Status() Status { return o.status }

// Items returns a copy of the order lines.
func (o *Order) Items() []Item {
	out := make([]Item, len(o.items))
	copy(out, o.items)
	return out
}

// TotalCents returns the current total.
func (o *Order) TotalCents() int64 { return o.totalCents }

// Place transitions the order to placed status, enforcing business rules.
func (o *Order) Place(items []Item, eventID string) error {
	if o.status != StatusDraft {
		return domain.NewRuleViolation("ORDER_NOT_DRAFT", "order is not in draft state")
	}
	if len(items) == 0 {
		return domain.NewRuleViolation("ORDER_EMPTY", "order must have at least one item")
	}
	var total int64
	for _, it := range items {
		if it.Quantity <= 0 {
			return domain.NewRuleViolation("ITEM_INVALID_QTY", "item quantity must be positive")
		}
		total += it.PriceCents * int64(it.Quantity)
	}
	o.items = items
	o.totalCents = total
	o.status = StatusPlaced
	o.IncrementVersion()
	o.Record(NewOrderPlaced(eventID, string(o.ID()), o.Version(), o.customerID, total))
	return nil
}
