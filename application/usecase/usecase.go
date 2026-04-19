// Package usecase defines a lightweight application service contract for
// modules that want a typed Execute call without going through a Bus.
//
// Use UseCase[D, R] when the caller already holds a reference to the service
// and dispatch infrastructure (middleware chain, audit logging, transaction
// wrapping) is not needed. When those concerns matter, prefer
// application/command.Handler[C, R] together with a Bus.
//
// # What a UseCase is for
//
// A UseCase orchestrates domain objects to express a business intent: it
// loads aggregates, calls their methods, dispatches the resulting events,
// and coordinates multiple repositories within a unit of work. It is the
// thin layer between a transport handler and the domain model — not a
// place to put business rules.
//
// Anti-pattern: a UseCase that merely shuttles fields between an HTTP
// request and a repository call (the "anemic application service") is a
// sign that the surrounding aggregate is anemic. The rule belongs on the
// aggregate, not on the use case. See docs/anti-patterns.md (anti-pattern
// #3) and docs/aggregate-design.md for the alternative.
package usecase

import (
	"context"

	"github.com/slam0504/go-ddd-core/application/command"
	"github.com/slam0504/go-ddd-core/application/query"
)

// UseCase executes an application service with a typed input and result.
// Implementations belong to the application layer and orchestrate domain
// objects; they should not depend on transport or persistence specifics.
type UseCase[D any, R any] interface {
	Execute(ctx context.Context, input D) (R, error)
}

// Func adapts a plain function to the UseCase interface, mirroring
// http.HandlerFunc. It removes the boilerplate of declaring an empty struct
// when the use case is small enough to fit in a single function.
type Func[D any, R any] func(ctx context.Context, input D) (R, error)

// Execute satisfies UseCase[D, R].
func (f Func[D, R]) Execute(ctx context.Context, input D) (R, error) {
	return f(ctx, input)
}

// AsCommandHandler lifts a UseCase into a command.Handler so callers can
// register it on a command.Bus and benefit from middleware, audit hooks, and
// transactional wrapping. Useful when a use case grows from "called directly"
// to "needs cross-cutting concerns" without rewriting the implementation.
func AsCommandHandler[D any, R any](uc UseCase[D, R]) command.Handler[D, R] {
	return command.HandlerFunc[D, R](uc.Execute)
}

// AsQueryHandler is the read-side counterpart of AsCommandHandler.
func AsQueryHandler[D any, R any](uc UseCase[D, R]) query.Handler[D, R] {
	return query.HandlerFunc[D, R](uc.Execute)
}
