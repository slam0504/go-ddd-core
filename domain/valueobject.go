package domain

// ValueObject is identified by the equality of its attributes, not by an ID.
// Implementations should be immutable.
type ValueObject interface {
	Equal(other ValueObject) bool
}
