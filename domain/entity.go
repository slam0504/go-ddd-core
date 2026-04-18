// Package domain defines DDD tactical building blocks: entities, aggregates,
// value objects, domain events, and repository contracts. It contains no
// business logic and no infrastructure dependencies.
package domain

// Entity identifies an object by its ID rather than its attributes.
type Entity[ID comparable] interface {
	ID() ID
	Equal(other Entity[ID]) bool
}

// BaseEntity is an embeddable struct implementing Entity[ID].
type BaseEntity[ID comparable] struct {
	id ID
}

// NewBaseEntity constructs a BaseEntity with the given id.
func NewBaseEntity[ID comparable](id ID) BaseEntity[ID] {
	return BaseEntity[ID]{id: id}
}

func (e BaseEntity[ID]) ID() ID { return e.id }

func (e BaseEntity[ID]) Equal(other Entity[ID]) bool {
	if other == nil {
		return false
	}
	return e.id == other.ID()
}
