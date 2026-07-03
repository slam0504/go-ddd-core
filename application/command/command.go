// Package command defines the write side of CQRS: Command, Handler, and Bus.
// The package contains no infrastructure — transports, persistence, and
// messaging live in adapters selected by downstream services.
//
// Naming: in v0.2.0 Command no longer requires CommandName() string. The bus
// derives the routing name via NameOf(c), which prefers an explicitly declared
// CommandName() method and falls back to the Go type name. Cross-service
// contracts (Kafka topics, RPC method names) should still implement
// CommandName() explicitly so renaming the Go type does not change the wire
// key.
package command

import (
	"context"
	"reflect"
)

// Command is the input to a write-side use case. It is a marker interface:
// any type qualifies. Implementations may opt in to a stable routing name by
// defining `CommandName() string`.
type Command interface{}

// Handler executes a Command and returns a typed result.
type Handler[C Command, R any] interface {
	Handle(ctx context.Context, cmd C) (R, error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc[C Command, R any] func(ctx context.Context, cmd C) (R, error)

func (f HandlerFunc[C, R]) Handle(ctx context.Context, cmd C) (R, error) {
	return f(ctx, cmd)
}

// DispatchFunc is the type-erased form of a handler. Bus implementations
// store dispatchers of this type keyed by command name.
type DispatchFunc func(ctx context.Context, cmd Command) (any, error)

// Bus dispatches commands to their registered handler.
type Bus interface {
	Dispatch(ctx context.Context, cmd Command) (any, error)
	RegisterHandler(name string, fn DispatchFunc)
}

// Register binds a typed Handler to the bus. It is a free function because
// interface methods cannot introduce their own type parameters.
func Register[C Command, R any](bus Bus, h Handler[C, R]) {
	bus.RegisterHandler(nameOfType[C](), func(ctx context.Context, cmd Command) (any, error) {
		typed, ok := cmd.(C)
		if !ok {
			return nil, ErrCommandMismatch
		}
		return h.Handle(ctx, typed)
	})
}

// NameOf returns the routing name for cmd. The lookup order is:
//
//  1. If cmd implements `CommandName() string` and the result is non-empty,
//     that value is used.
//  2. Otherwise the Go type name is used (with leading pointer indirections
//     stripped).
//
// Returns "" when cmd is nil. Cross-service contracts should declare
// CommandName() explicitly so renaming the Go type does not silently move the
// routing key.
func NameOf(cmd Command) string {
	if cmd == nil {
		return ""
	}
	if n, ok := cmd.(interface{ CommandName() string }); ok {
		if s := n.CommandName(); s != "" {
			return s
		}
	}
	t := reflect.TypeOf(cmd)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// nameOfType resolves the routing name from the type parameter, used by
// Register where no value is in hand. For pointer types it probes via a
// non-nil *T allocated by reflect.New so CommandName() methods on pointer
// receivers are honoured without risking nil-deref. This keeps Register-side
// name resolution symmetric with the Dispatch-side NameOf, which receives a
// concrete value.
func nameOfType[C any]() string {
	t := reflect.TypeOf((*C)(nil)).Elem()
	var probe any
	if t.Kind() == reflect.Pointer {
		probe = reflect.New(t.Elem()).Interface()
	} else {
		var zero C
		probe = any(zero)
	}
	if n, ok := probe.(interface{ CommandName() string }); ok {
		if s := n.CommandName(); s != "" {
			return s
		}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}
