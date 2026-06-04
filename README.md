# go-ddd-core

Go service core for building DDD + Clean Architecture + Event-Driven services.

The core defines **interfaces (ports) and domain primitives** only. It contains
**no infra client code** (no Kafka client, no DB driver, no HTTP framework).
Downstream services pick concrete adapters and wire them via config.

## Principles

- **Ports & Adapters** — core only defines contracts
- **Dependency inversion** — domain/application never depend on infra
- **Config-driven wiring** — bootstrap assembles adapters from config
- **Zero business logic** — core is a skeleton, not a framework opinion

## Layout

```
domain/                   DDD base types (Entity, AggregateRoot, DomainEvent,
                          ValueObject + helpers, Repository)
application/
  command/                Command bus, Handler, Middleware (NameOf since v0.2.0)
  query/                  Query bus mirror (NameOf since v0.2.0)
  query/spec/             Specification pattern (And/Or/Not + opt-in SQLTranslatable) [v0.2.0]
  usecase/                Lightweight UseCase[D, R] + AsCommandHandler / AsQueryHandler [v0.2.0]
eventsourcing/            EventStore, SnapshotStore, Projector
eventbus/                 Publisher/Subscriber (watermill contract), Outbox, Inbox
ports/                    Infra interfaces (logger, cache, database, storage, httpclient, observability, health, auth)
  health/                 Liveness/readiness probe contract [v0.5.0]
  auth/                   AuthN contract: Identity + TokenVerifier [unreleased]
transport/                Server contracts (http, grpc, graphql) — no library deps
  grpc/                   + interceptor combinators + errorsx → grpc Status mapping [v0.2.0]
  graphql/                + Loader contract, Relay cursor codec, FilterInput → Specification [v0.2.0]
config/                   Config Provider (viper-backed), shared schema
bootstrap/                App container, Module, AdapterRegistry, lifecycle
pkg/
  contextx                Standard context keys
  errorsx                 Coded Error + WithDetail
    httpx/                errorsx.Code → HTTP status + JSON writer + RuleViolation bridge [v0.2.0]
  idgen                   Generator contract (UUIDv4 / UUIDv7)
  pagination              Limit / Cursor / Page[T] for paged queries [v0.2.0]
  vo/uuid                 UUID value object (v7, JSON-friendly) [v0.2.0]
examples/                 Minimal runnable example
docs/
  grpc.md                 gRPC integration guide [v0.2.0]
  graphql.md              GraphQL integration guide [v0.2.0]
  anti-patterns.md        Anti-patterns this core helps avoid [v0.2.1]
  aggregate-design.md     Rich aggregate design + worked example [v0.2.1]
```

## Requirements

- Go 1.24+

## Dependencies (core only)

- `github.com/ThreeDotsLabs/watermill` — messaging contract
- `github.com/spf13/viper` — config contract
- `go.opentelemetry.io/otel` — observability contract
- `github.com/google/uuid` — ID utilities

No infra client libraries are imported by the core.

## Why no CRUD-style Store in core?

`go-ddd-core` defines `domain.Repository[T, ID]` for **aggregate persistence**
(`FindByID` / `Save` / `Delete`). It deliberately does *not* ship a generic
CRUD store (`List` / `Search` / `Patch` / `Update`).

Aggregate repositories enforce a consistency boundary — every load and save
moves a complete aggregate through its invariants. Generic CRUD stores serve
read models, projections, and table mappers; their right shape (filter syntax,
patch semantics, soft-delete behaviour, audit columns) varies per project. A
core-imposed `Store[T, ID]` would either be too narrow (missing your projection's
needs) or too wide (forcing every adapter to implement methods it does not
need).

### Building a Store in `go-ddd-adapters`

If your service needs a CRUD store, build it in `go-ddd-adapters` (or a
service-local package) reusing core types instead of inventing parallel ones:

| Need                  | Reuse from core                                    |
|-----------------------|----------------------------------------------------|
| Paging arguments      | `pkg/pagination.Limit` / `pkg/pagination.Cursor`  |
| Paging result         | `pkg/pagination.Page[T]`                           |
| Typed query filter    | `application/query/spec.Specification[T]`          |
| SQL translation hook  | `application/query/spec.SQLTranslatable`           |
| Coded errors          | `pkg/errorsx.Error` + `pkg/errorsx/httpx`         |

A typical adapter signature looks like:

```go
type Store[T any, ID comparable] interface {
    Get(ctx context.Context, id ID) (T, error)
    List(ctx context.Context, req pagination.PageRequest) (pagination.Page[T], error)
    Search(ctx context.Context, s spec.Specification[T], req pagination.PageRequest) (pagination.Page[T], error)
    Create(ctx context.Context, value T) error
    Update(ctx context.Context, value T) error
    Delete(ctx context.Context, id ID) error
}
```

**Anti-patterns to avoid in your Store:**

- `Patch(ctx, id, map[string]any)` — loses type safety; prefer typed
  partial-update structs (REST: PATCH body schema, GraphQL: input type).
- `Search(ctx, map[string]any)` — fight the typing system. Use a
  Specification tree built via `application/query/spec` so SQL translation
  can opt in via the side-interface.

### GraphQL services

The GraphQL helpers in `transport/graphql` (Relay Connection cursor codec,
`FilterInput` → `Specification` walker) are designed to plug into the same
`pagination` and `spec` types. See [`docs/graphql.md`](docs/graphql.md) for
the full integration guide.

## Design discipline

Several APIs in this core exist *because* a specific anti-pattern hurt in
production. The full list, with concrete substitutes, lives in
[`docs/anti-patterns.md`](docs/anti-patterns.md): service locator inside
use cases, static facades over infrastructure, anemic aggregates,
map-typed query/patch, dual error wrapping, half-implemented at-least-once
delivery, ORM tags inside domain entities.

For the positive counterpart — what a rich aggregate looks like and how
to enforce invariants on it — see
[`docs/aggregate-design.md`](docs/aggregate-design.md).
