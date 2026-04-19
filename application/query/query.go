// Package query defines the read side of CQRS: Query, Handler, and Bus.
// Queries must be side-effect free.
//
// Naming: in v0.2.0 Query no longer requires QueryName() string. The bus
// derives the routing name via NameOf(q), which prefers an explicit
// QueryName() method and falls back to the Go type name. Cross-service
// contracts should declare QueryName() explicitly so renaming the Go type
// does not change the wire key.
package query

import (
	"context"
	"reflect"
)

// Query is the input to a read-side use case. It is a marker interface:
// any type qualifies. Implementations may opt in to a stable routing name by
// defining `QueryName() string`.
type Query interface{}

// Handler executes a Query and returns a typed result.
type Handler[Q Query, R any] interface {
	Handle(ctx context.Context, q Q) (R, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc[Q Query, R any] func(ctx context.Context, q Q) (R, error)

func (f HandlerFunc[Q, R]) Handle(ctx context.Context, q Q) (R, error) {
	return f(ctx, q)
}

// DispatchFunc is the type-erased form of a query handler.
type DispatchFunc func(ctx context.Context, q Query) (any, error)

// Bus dispatches queries to their registered handler.
type Bus interface {
	Dispatch(ctx context.Context, q Query) (any, error)
	RegisterHandler(name string, fn DispatchFunc)
}

// Register binds a typed Handler to the bus.
func Register[Q Query, R any](bus Bus, h Handler[Q, R]) {
	bus.RegisterHandler(nameOfType[Q](), func(ctx context.Context, q Query) (any, error) {
		typed, ok := q.(Q)
		if !ok {
			return nil, ErrQueryMismatch
		}
		return h.Handle(ctx, typed)
	})
}

// NameOf returns the routing name for q. It prefers an explicit
// `QueryName() string` method, falling back to the Go type name. Returns ""
// for nil. See package doc for cross-service contract guidance.
func NameOf(q Query) string {
	if q == nil {
		return ""
	}
	if n, ok := q.(interface{ QueryName() string }); ok {
		if s := n.QueryName(); s != "" {
			return s
		}
	}
	t := reflect.TypeOf(q)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func nameOfType[Q any]() string {
	t := reflect.TypeOf((*Q)(nil)).Elem()
	isPtr := t.Kind() == reflect.Ptr
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if !isPtr {
		var zero Q
		if n, ok := any(zero).(interface{ QueryName() string }); ok {
			if s := n.QueryName(); s != "" {
				return s
			}
		}
	}
	return t.Name()
}
