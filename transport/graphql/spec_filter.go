package graphql

import "github.com/slam0504/go-ddd-core/application/query/spec"

// FilterInput is the canonical wire shape for a Relay-style GraphQL filter
// expression: a tree of And / Or / Not nodes culminating in adapter-specific
// leaf clauses. The Leaf field is left as any so adapters can model their
// own field comparators (StringEq, IntGt, BetweenTime, etc.) without core
// learning about them.
type FilterInput struct {
	And  []FilterInput
	Or   []FilterInput
	Not  *FilterInput
	Leaf any
}

// LeafBuilder converts an adapter-specific Leaf into a Specification[T].
// Returning a non-nil error from the builder aborts the whole tree
// translation so callers can surface the bad input as a single GraphQL
// validation error.
type LeafBuilder[T any] func(leaf any) (spec.Specification[T], error)

// BuildSpecification recursively translates a FilterInput tree into a
// composed Specification[T]. The walk uses spec.And / spec.Or / spec.Not
// from application/query/spec, which means the resulting tree is
// directly composable with hand-written specs and opt-in SQLTranslatable
// leaves.
//
// An empty input (no And, no Or, no Not, no Leaf) returns nil, nil. The
// caller decides whether nil means "match everything" or "client supplied
// nothing"; modeling that distinction in core would impose an unwanted
// universal-spec abstraction on every adapter.
func BuildSpecification[T any](in FilterInput, leaf LeafBuilder[T]) (spec.Specification[T], error) {
	if in.Leaf != nil {
		return leaf(in.Leaf)
	}

	switch {
	case len(in.And) > 0:
		var combined spec.Specification[T]
		for _, sub := range in.And {
			s, err := BuildSpecification(sub, leaf)
			if err != nil {
				return nil, err
			}
			if s == nil {
				continue
			}
			if combined == nil {
				combined = s
			} else {
				combined = spec.And(combined, s)
			}
		}
		return combined, nil

	case len(in.Or) > 0:
		var combined spec.Specification[T]
		for _, sub := range in.Or {
			s, err := BuildSpecification(sub, leaf)
			if err != nil {
				return nil, err
			}
			if s == nil {
				continue
			}
			if combined == nil {
				combined = s
			} else {
				combined = spec.Or(combined, s)
			}
		}
		return combined, nil

	case in.Not != nil:
		s, err := BuildSpecification(*in.Not, leaf)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, nil
		}
		return spec.Not(s), nil
	}

	return nil, nil
}
