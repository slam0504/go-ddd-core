package domain

import "reflect"

// ValueObject is identified by the equality of its attributes, not by an ID.
// Implementations must be immutable: never expose pointer fields, never define
// methods that mutate the receiver, and prefer value-receiver methods.
type ValueObject interface {
	Equal(other ValueObject) bool
}

// EqualValues reports whether two values of the same comparable type are
// identical. Provided as a helper for ValueObject.Equal implementations whose
// fields are all comparable (no slices, maps, or function values).
//
// Example:
//
//	func (m Money) Equal(other ValueObject) bool {
//	    o, ok := other.(Money)
//	    return ok && domain.EqualValues(m, o)
//	}
func EqualValues[T comparable](a, b T) bool { return a == b }

// DeepEqualValues reports whether two values of the same type are equal by
// reflect.DeepEqual. Use when the value object contains non-comparable fields
// (slices, maps). Hot paths should hand-write Equal instead, as DeepEqual is
// allocation-heavy.
func DeepEqualValues[T any](a, b T) bool { return reflect.DeepEqual(a, b) }
