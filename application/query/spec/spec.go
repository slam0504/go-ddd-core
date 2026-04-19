// Package spec implements the Specification pattern: composable, type-safe
// predicates that read like business rules and can optionally translate
// themselves into a query language.
//
// The core Specification[T] interface is pure domain — it answers
// IsSatisfiedBy(T) bool. SQL or other backend translations are exposed via the
// opt-in SQLTranslatable side-interface so adapters can type-assert without
// every spec being forced to know about persistence.
package spec

// Specification is a composable predicate over T. Implementations should be
// immutable so And/Or/Not return fresh values without mutating the receiver.
type Specification[T any] interface {
	IsSatisfiedBy(T) bool
	And(Specification[T]) Specification[T]
	Or(Specification[T]) Specification[T]
	Not() Specification[T]
}

// SQLTranslatable is an opt-in side interface. A Specification that knows how
// to express itself as a SQL fragment implements ToSQL; adapters perform a
// type assertion to discover this capability.
//
// IMPORTANT: args MUST be passed via parameter placeholders ($1, ?). Never
// concatenate user input into clause; doing so reintroduces SQL injection.
type SQLTranslatable interface {
	ToSQL() (clause string, args []any)
}

// Predicate adapts a plain function to Specification[T]. Useful for ad-hoc
// rules and tests; production specs typically declare a named struct so the
// rule shows up in stack traces and logs.
func Predicate[T any](pred func(T) bool) Specification[T] {
	return predicateSpec[T]{pred: pred}
}

type predicateSpec[T any] struct {
	pred func(T) bool
}

func (s predicateSpec[T]) IsSatisfiedBy(t T) bool                  { return s.pred(t) }
func (s predicateSpec[T]) And(o Specification[T]) Specification[T] { return And(s, o) }
func (s predicateSpec[T]) Or(o Specification[T]) Specification[T]  { return Or(s, o) }
func (s predicateSpec[T]) Not() Specification[T]                   { return Not(s) }

// And returns a Specification that is satisfied iff both left and right are.
// Composed specs do NOT auto-implement SQLTranslatable; adapters compose SQL
// at translation time.
func And[T any](left, right Specification[T]) Specification[T] {
	return andSpec[T]{left: left, right: right}
}

type andSpec[T any] struct{ left, right Specification[T] }

func (s andSpec[T]) IsSatisfiedBy(t T) bool {
	return s.left.IsSatisfiedBy(t) && s.right.IsSatisfiedBy(t)
}
func (s andSpec[T]) And(o Specification[T]) Specification[T] { return And(s, o) }
func (s andSpec[T]) Or(o Specification[T]) Specification[T]  { return Or(s, o) }
func (s andSpec[T]) Not() Specification[T]                   { return Not(s) }

// Left and Right expose the operands so adapter-side translators can walk the
// tree without reflection.
func (s andSpec[T]) Left() Specification[T]  { return s.left }
func (s andSpec[T]) Right() Specification[T] { return s.right }

// Or returns a Specification that is satisfied iff either left or right is.
func Or[T any](left, right Specification[T]) Specification[T] {
	return orSpec[T]{left: left, right: right}
}

type orSpec[T any] struct{ left, right Specification[T] }

func (s orSpec[T]) IsSatisfiedBy(t T) bool {
	return s.left.IsSatisfiedBy(t) || s.right.IsSatisfiedBy(t)
}
func (s orSpec[T]) And(o Specification[T]) Specification[T] { return And(s, o) }
func (s orSpec[T]) Or(o Specification[T]) Specification[T]  { return Or(s, o) }
func (s orSpec[T]) Not() Specification[T]                   { return Not(s) }
func (s orSpec[T]) Left() Specification[T]                  { return s.left }
func (s orSpec[T]) Right() Specification[T]                 { return s.right }

// Not returns the logical negation of inner.
func Not[T any](inner Specification[T]) Specification[T] {
	return notSpec[T]{inner: inner}
}

type notSpec[T any] struct{ inner Specification[T] }

func (s notSpec[T]) IsSatisfiedBy(t T) bool                  { return !s.inner.IsSatisfiedBy(t) }
func (s notSpec[T]) And(o Specification[T]) Specification[T] { return And(s, o) }
func (s notSpec[T]) Or(o Specification[T]) Specification[T]  { return Or(s, o) }
func (s notSpec[T]) Not() Specification[T]                   { return s.inner }
func (s notSpec[T]) Inner() Specification[T]                 { return s.inner }

// Composite identifies the binary combinators (And, Or) so adapter-side
// translators can pattern-match them with a single type switch instead of
// matching each combinator individually.
type Composite[T any] interface {
	Specification[T]
	Left() Specification[T]
	Right() Specification[T]
}

// Negation identifies Not so translators can recover the inner spec.
type Negation[T any] interface {
	Specification[T]
	Inner() Specification[T]
}
