# GraphQL Integration Guide

`go-ddd-core` provides GraphQL **contracts and helpers**, not a server. The
schema-execution library (gqlgen, graphql-go, 99designs, etc.) is your choice;
this document explains how the helpers in `transport/graphql` compose with the
rest of the core (`pagination`, `query/spec`, `errorsx`) so each adapter does
not reinvent these wiring points.

## Why no Server in core?

`transport/graphql` deliberately omits a `Server` implementation. GraphQL
servers are tightly coupled to the chosen schema library (each library defines
its own resolver signatures, directives, subscription transport, error
extensions). A core-supplied server would force every downstream service into
one library; instead, core defines the wiring shape and lets adapters plug in
the library they prefer.

## Helpers at a glance

| Helper                    | Purpose                                            | Source                              |
|---------------------------|----------------------------------------------------|-------------------------------------|
| `Loader[K, V]`            | Batched / deduped lookup contract for resolvers   | `transport/graphql/loader.go`       |
| `EncodeCursor` / `DecodeCursor` | Opaque versioned cursor codec               | `transport/graphql/cursor.go`       |
| `ConnectionArgs` + `ToPageRequest` | Relay arg → `pagination.Cursor`        | `transport/graphql/cursor.go`       |
| `Connection[T]` + `BuildConnection` | `pagination.Page[T]` → wire shape     | `transport/graphql/cursor.go`       |
| `FilterInput` + `BuildSpecification` | Filter input tree → `spec.Specification[T]` | `transport/graphql/spec_filter.go` |

## Standard wiring

### 1. Pagination — Relay Connection ↔ `pkg/pagination`

Resolvers receive `ConnectionArgs` from the schema, translate to a
`pagination.Cursor`, hand it to a query handler / data store, and assemble a
`Connection[T]` from the returned `Page[T]`.

```go
func (r *userResolver) Users(ctx context.Context, args graphql.ConnectionArgs) (graphql.Connection[User], error) {
    req, err := graphql.ToPageRequest(args)
    if err != nil {
        return graphql.Connection[User]{}, err  // ErrInvalidCursor / ErrUnsupportedDirection
    }
    page, err := r.userQuery.List(ctx, req)
    if err != nil {
        return graphql.Connection[User]{}, err
    }
    return graphql.BuildConnection(page, func(u User) string {
        return "user:" + u.ID
    }), nil
}
```

`ToPageRequest` only supports forward pagination (`first`/`after`). Backward
pagination (`last`/`before`) returns `ErrUnsupportedDirection` so resolvers
can reject it explicitly rather than silently misinterpret the request.

### 2. Filter — GraphQL filter input ↔ `application/query/spec`

Define schema-level leaf types per field; map each leaf to a Specification in
your `LeafBuilder`. Adapters that translate Specifications to SQL implement
`spec.SQLTranslatable` on each leaf.

```go
type userFilterLeaf struct {
    NameStartsWith *string
    AgeAtLeast     *int
}

func leafBuilder(leaf any) (spec.Specification[User], error) {
    l, ok := leaf.(userFilterLeaf)
    if !ok {
        return nil, fmt.Errorf("unknown leaf %T", leaf)
    }
    switch {
    case l.NameStartsWith != nil:
        return userNameSpec{Prefix: *l.NameStartsWith}, nil
    case l.AgeAtLeast != nil:
        return userAgeSpec{Min: *l.AgeAtLeast}, nil
    default:
        return nil, errors.New("filter leaf has no field set")
    }
}

s, err := graphql.BuildSpecification(args.Filter, leafBuilder)
```

The recursive walker handles `And` / `Or` / `Not` automatically and reuses
`spec.And` / `spec.Or` / `spec.Not`, so the resulting tree is interchangeable
with hand-written specs in tests and other call sites.

### 3. DataLoader — `Loader[K, V]` contract

`Loader[K, V]` is intentionally narrow (`Load`, `LoadMany`). Adapters wrap
`graph-gophers/dataloader`, `vektah/dataloaden`, or hand-written batchers and
satisfy this interface. Resolvers depend on the interface, not the concrete
loader, so swapping loader libraries does not ripple into resolver code.

### 4. Errors — `pkg/errorsx` and `pkg/errorsx/httpx`

Resolvers should return `*errorsx.Error` so the adapter can lift the
`errorsx.Code` into the GraphQL error `extensions` payload. The conversion
table mirrors `pkg/errorsx/httpx.StatusFromCode`:

```text
errorsx.CodeInvalidArgument → BAD_USER_INPUT  (Apollo conventions)
errorsx.CodeNotFound        → NOT_FOUND
errorsx.CodeForbidden       → FORBIDDEN
errorsx.CodeUnauthorized    → UNAUTHENTICATED
errorsx.CodeInternal        → INTERNAL_SERVER_ERROR
…
```

A `domain.RuleViolation` returned from an aggregate should be lifted via
`httpx.FromRuleViolation` (which works for any transport, not just HTTP) to
get a coded `*errorsx.Error` carrying the rule code in the details map.

## Wiring with `bootstrap`

GraphQL modules implement `graphql.SchemaProvider` and `graphql.ResolverProvider`,
collected by an adapter at startup:

```go
adapter.RegisterModules(
    userModule,    // implements SchemaProvider, ResolverProvider
    orderModule,
)
adapter.MountOnto(httpServer)  // returns the http.Handler at /graphql
```

The HTTP server itself remains the `transport/http.Server` contract; the
GraphQL adapter simply registers a route handler on it. This keeps the same
HTTP middleware (logging, tracing, auth) wrapping both REST and GraphQL.

## What core deliberately does NOT do

- **No schema definition language** — declare schemas via your library of choice (gqlgen `.graphql` files, code-first builders, etc.)
- **No subscription transport** — WebSocket / SSE handling is library-specific
- **No depth/complexity limits** — these belong to the schema library; configure them at adapter level
- **No persisted query store** — adapter-level concern with infrastructure dependencies
