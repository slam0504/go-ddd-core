# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- New `ports/auth` package: the authentication (AuthN) contract for
  downstream services. Surfaces:
  - `auth.Identity` — the verified principal (Subject, TenantID, Roles,
    Claims).
  - `auth.TokenVerifier` interface (`Verify(ctx, token) (Identity, error)`)
    plus an `auth.TokenVerifierFunc` adapter for plain functions.
  - Sentinels `ErrTokenMissing` / `ErrTokenInvalid` / `ErrTokenExpired`, all
    coded `errorsx.CodeUnauthorized` so `pkg/errorsx/httpx` maps them to HTTP
    401 with no per-adapter table. Declared as `error` (not `*errorsx.Error`)
    so the shared coded value cannot be mutated by callers.
  - `auth.WithIdentity` / `auth.IdentityFromContext` context helpers; both
    clone `Roles`/`Claims` so an identity stored in a context cannot be
    mutated through an alias.

  Authorization (role/permission checks, 403), token issuance, and the JWT
  verifier adapter are out of scope here (see `docs/roadmap.md` v0.6.0). Core
  ships only the contract; the first consumer (an `auth/jwt` adapter + HTTP
  middleware in `go-ddd-adapters`) gates the release tag.

## [0.5.0] - 2026-05-25

Inbound HTTP + health cycle. Core ships the shared probe contract;
the first consumer — `transport/http/stdlib` with `/healthz` +
`/readyz` handlers and graceful shutdown — lives in
`go-ddd-adapters` (`go-ddd-adapters` PR #21, merged at `d9c7324`).
Splitting the work this way keeps registries, aggregation policy,
and HTTP handler shape out of core, where they would smuggle in
framework opinions; adapters compose per-driver `health.Check`s
behind their own registry.

### Added

- New `ports/health` package providing the shared probe shape
  for liveness/readiness endpoints. Two surfaces:
  - `health.Check` interface (`Name() string; Check(ctx context.Context) error`).
    Implementations must be safe for concurrent use and respect ctx
    cancellation. Returning nil signals healthy; a non-nil error
    becomes the operator-facing failure reason.
  - `health.NewCheck(name, fn)` convenience constructor for wrapping
    a plain `func(ctx) error` (typically a driver's `Ping`) without
    defining a new type.

  The core deliberately ships only the probe shape: registries, HTTP
  handlers, and aggregation policy live in the transport adapter
  that exposes the probes (see `docs/roadmap.md` v0.5.0 cycle).

## [0.4.0] - 2026-05-20

Deprecation-cycle close. The single change in this release is the
physical removal of the in-process `Memory` Inbox implementation
from core, completing the deprecation that was pre-announced in
v0.3.0's `### Deprecated` section. The relocation target has been
available at `github.com/slam0504/go-ddd-adapters/eventbus/inbox`
since `go-ddd-adapters v0.3.0` (2026-05-19), and the "one more
release cycle" overlap window committed to in that release was
honoured by `go-ddd-adapters v0.4.0` (2026-05-20). Both gating
conditions for the removal are now formally satisfied; downstream
consumer inventory is empty (recorded 2026-05-20).

The public `eventbus.Inbox` interface and `eventbus.InboxKey` value
type are **unchanged** — they live in `eventbus/inbox.go` under
`package eventbus`, not under the removed `package inbox`.

### Removed (BREAKING)

- `eventbus/inbox/memory.go` and `eventbus/inbox/memory_test.go`
  physically removed. The entire `eventbus/inbox/` sub-package is
  gone; there is no `package inbox` in core anymore. The in-process
  `Memory` Inbox now lives at
  `github.com/slam0504/go-ddd-adapters/eventbus/inbox` (since
  `go-ddd-adapters v0.3.0`, with added `WithTTL` / `WithClock`
  options). Downstream services migrate by changing the import path;
  call sites stay identical:

  ```go
  // before
  import "github.com/slam0504/go-ddd-core/eventbus/inbox"

  // after
  import "github.com/slam0504/go-ddd-adapters/eventbus/inbox"
  // inbox.NewMemory(...) unchanged
  ```

  The `eventbus.Inbox` interface and `eventbus.InboxKey` struct
  remain in `eventbus/inbox.go` under `package eventbus` and require
  no caller changes.

### Documentation

- `docs/anti-patterns.md` updated: the Inbox / Outbox anti-pattern
  Substitute section now points at `go-ddd-adapters/eventbus/inbox`
  unambiguously (no more transitional dual-path framing), and the
  "Note on `eventbus/inbox/memory.go`" section is rewritten as a
  historical note documenting the v0.2.0 → adapters journey for
  future readers tracing the boundary decisions.

## [0.3.0] - 2026-05-15

Contract alignment release. Two breaking changes tighten the boundary
between domain transactions and consumer-side idempotency:

- Use cases now depend on `application.UnitOfWork` exclusively. The new
  `application.UnitOfWorkFromTxManager(tm)` bridge turns any
  `ports/database.TxManager` into a `UnitOfWork`, so adapters keep the
  driver-specific transaction handle while use cases only see the
  application contract. Direct `TxManager.WithinTx` calls at the use
  case layer become an anti-pattern (see `docs/anti-patterns.md`).
- `eventbus.Inbox` is now scoped per-consumer via `InboxKey{Consumer,
  EventID}`. A single domain event can now reach a projector, a reactor,
  and a saga without one consumer's recorded "seen" silently
  short-circuiting another consumer's handler. `OutboxRecord` gains a
  dedicated `EventID` field distinct from the storage row `ID`, so
  fan-out to multiple topics keeps a stable cross-topic event identity
  while each row stays addressable by its own primary key.

See [`docs/anti-patterns.md`](docs/anti-patterns.md) "Design boundaries:
what core deliberately omits" for the rationale on which contracts ship
implementations and which do not.

### Changed (BREAKING)

- `eventbus.Inbox` is now scoped to `(consumer, eventID)` via a new
  `eventbus.InboxKey{Consumer, EventID}` value type. `Seen` and `Record`
  take an `InboxKey` instead of a bare `eventID string`. Per-consumer
  scoping prevents one consumer's recording from silently suppressing
  another consumer's handler — a real risk when the same event reaches a
  projector, a reactor, and a saga.
  - Update call sites to `inbox.Seen(ctx, eventbus.InboxKey{Consumer: name, EventID: id})`.
  - `eventbus/inbox.Memory` updated in lockstep.
- `eventbus.OutboxRecord` gains an `EventID` field, distinct from `ID`.
  `ID` identifies the outbox row (assigned by the OutboxStore); `EventID`
  carries `DomainEvent.EventID()` for broker message-id and downstream
  inbox dedup. Fan-out to multiple topics produces multiple rows sharing
  one `EventID`.

### Added

- `application.UnitOfWorkFromTxManager(tm)` bridges a `ports/database.TxManager`
  into an `application.UnitOfWork`, so use cases can depend on the application
  contract while adapters supply the SQL transaction. The bridge is a thin
  passthrough; driver-specific tx-handle propagation (ctx values, `*sql.Tx`,
  etc.) remains the TxManager's responsibility.

### Fixed

- `command.Register[*T, R]` / `query.Register[*T, R]` now honour
  `CommandName()` / `QueryName()` declared on pointer receivers. Previously
  `nameOfType` skipped the method probe for pointer type parameters to
  avoid nil-deref, leaving the handler registered under the Go type name
  while `Dispatch(&T{})` looked it up under the explicit method-returned
  name. The probe now uses `reflect.New(t.Elem()).Interface()` for a
  non-nil zero-value `*T`, so Register and Dispatch resolve identically
  for both value and pointer type parameters.

### Deprecated

- `eventbus/inbox/memory.go` (the in-memory `Memory` Inbox) is preserved
  for backward compatibility in v0.3.0 and **is slated to move to
  `go-ddd-adapters` in v0.4.0**. Services that import it directly should
  plan to migrate once the adapter package lands. Rationale: the v0.2.x
  in-memory inbox predates the infrastructure-client-free boundary, and
  v0.3.0 tightens core to ship only interfaces plus in-process defaults
  for CQRS buses and config. See `docs/anti-patterns.md` "Note on
  `eventbus/inbox/memory.go`" for the long-form explanation.

### Documentation

- `docs/anti-patterns.md` Inbox / Outbox example aligned with the new
  `InboxKey` API and the `UnitOfWork.Do` use-case pattern. The
  documentation gains a new "Design boundaries: what core deliberately
  omits" section covering why no default `eventbus.Relay`,
  `eventsourcing.EventStore`, or `CheckpointStore` ship in core.

## [0.2.1] - 2026-04-19

Documentation-only release that complements v0.2.0. Where v0.2.0 added the
APIs that make good designs easy, v0.2.1 names the anti-patterns those
APIs were built to displace, so reviewers and new readers know which
shortcuts to refuse.

### Added

- `docs/anti-patterns.md` — seven production anti-patterns (service
  locator inside use cases, static facade over infrastructure, anemic
  aggregate, map-typed query/patch, dual error wrapping, at-least-once
  only on the producer side, ORM tags inside domain entities) with the
  concrete core API that replaces each.
- `docs/aggregate-design.md` — rule-of-thumb guide on rich vs anemic
  aggregates, when to record domain events, when to split inner entities
  vs value objects vs separate aggregates, plus a worked `BankAccount`
  example that lives as compilable code in
  `examples/minimal/domain/account/` (21 tests covering every invariant)
  so the documentation cannot drift from the API.
- `examples/minimal/domain/account/` — second worked aggregate alongside
  Order, exercising multi-rule methods (Withdraw checks status, sign,
  overdraft), status-gated mutations (Freeze blocks Withdraw but not
  Deposit), and constructor-time invariant enforcement.
- `application/usecase/usecase.go` package doc — explicit warning that a
  UseCase merely shuttling fields between handler and repository is the
  anemic-application-service anti-pattern, with pointers to the new docs.
- `README.md` — new "Design discipline" section linking the anti-pattern
  catalogue and aggregate guide.

### Changed

Nothing. No code or API changes; safe drop-in upgrade from v0.2.0.

[0.2.1]: https://github.com/slam0504/go-ddd-core/releases/tag/v0.2.1

## [0.2.0] - 2026-04-18

Production-ready augmentation. v0.2.0 fills the "last mile" for CRUD-heavy
services without compromising the zero-opinion contract framework stance: it
adds shared types every adapter would otherwise re-derive (pagination,
specification, value objects, HTTP error mapping) plus contract helpers for
gRPC and GraphQL transports. The core's third-party dependency surface is
unchanged.

### Added

#### Pagination (`pkg/pagination/`) — new
- `PageRequest` sealed interface with `Limit{Size, Offset}` and
  `Cursor{Token, Size}` variants so callers must pick a paging style
  explicitly rather than mixing offset and cursor accidentally.
- `Page[T]{Items, Total, NextCursor}` envelope with `HasNext()`. Total of
  `-1` is the sentinel for "cursor mode, count not computed".

#### Specification (`application/query/spec/`) — new
- `Specification[T]` interface with `IsSatisfiedBy / And / Or / Not`.
- `Predicate[T](func)` adapter for ad-hoc rules, plus standalone `And` /
  `Or` / `Not` functions for tree assembly.
- `SQLTranslatable` opt-in side-interface so adapter-defined leaves can
  expose a `(clause, args)` pair without forcing every spec to know SQL.
- `Composite[T]` and `Negation[T]` interfaces let translators walk the
  tree with a single type switch instead of one per combinator.

#### UseCase (`application/usecase/`) — new
- `UseCase[D, R]` interface for module-internal application services that
  do not need bus dispatching.
- `Func[D, R]` plain-function adapter (mirrors `http.HandlerFunc`).
- `AsCommandHandler` / `AsQueryHandler` upgrade adapters so a use case can
  later be promoted onto the command / query Bus without rewriting the
  implementation.

#### Inbox default implementation (`eventbus/inbox/`) — new
- `Memory` — goroutine-safe in-memory `eventbus.Inbox` for tests,
  examples, and single-instance services. Optional `WithMaxSize` evicts
  the oldest half on overflow; `WithClock` allows deterministic tests.

#### Value object helpers (`domain/`, `pkg/vo/uuid/`)
- `domain.EqualValues[T comparable]` and `domain.DeepEqualValues[T any]`
  helpers for `ValueObject.Equal` implementations (replaces the bare
  `ValueObject` interface with practical building blocks).
- `pkg/vo/uuid.UUID` — UUID v7 value object with `New` / `Parse` /
  `MustParse` / `String` / `IsNil` / `Equal`, implementing
  `domain.ValueObject` and `encoding.TextMarshaler` /
  `encoding.TextUnmarshaler` so it round-trips through JSON. Storage
  encoding (BINARY(16) / BYTEA) intentionally lives in adapters.

#### errorsx HTTP bridge (`pkg/errorsx/httpx/`) — new
- `StatusFromCode(errorsx.Code) int` mapping table covering all 10
  built-in codes; unknown codes fall back to 500.
- `WriteJSON(w, err)` writes a standard `{code, message, details}` body
  with the correct status header.
- `Translate(err)` exposes the mapping logic for non-`net/http`
  transports (echo, gin, fiber).
- `FromRuleViolation(rv)` lifts a `domain.RuleViolation` into a coded
  `*errorsx.Error` with the rule code preserved as a `rule` detail.

#### transport/grpc helpers
- `CombineInterceptorProviders(...)` flattens unary and stream
  interceptors from multiple providers in declaration order.
- `CollectServiceProviders(...)` deduplicates nil providers.
- `Status*` constants mirror `google.golang.org/grpc/codes` numerically
  (so adapters cast directly) without importing the gRPC library.
- `CodeFromErrorsx(errorsx.Code) uint32` translates the 10 built-in
  errorsx codes to gRPC status codes.

#### transport/graphql helpers
- `Loader[K, V]` interface for batched / deduped resolver lookups,
  agnostic to the concrete dataloader library.
- `EncodeCursor` / `DecodeCursor` versioned (`v1:`) opaque cursor codec.
- `ConnectionArgs` + `ToPageRequest` translate Relay `first/after` into
  `pagination.Cursor` (backward `last/before` returns
  `ErrUnsupportedDirection` so resolvers reject explicitly).
- `Connection[T]` + `BuildConnection` assemble a Relay Connection from a
  `pagination.Page[T]` and a per-item cursor extractor.
- `FilterInput` + `BuildSpecification[T]` recursively translate a
  GraphQL filter input tree into a composed `spec.Specification[T]`,
  routing leaves through a user-supplied `LeafBuilder`.

#### Documentation
- New `docs/graphql.md` integration guide covering pagination, filter,
  loader, and error wiring.
- New `docs/grpc.md` integration guide covering adapter assembly,
  errorsx → gRPC code mapping, and bootstrap wiring.
- `README.md` adds a "Why no CRUD-style Store in core?" section with
  reuse table and recommended `Store[T, ID]` shape for adapters.

### Changed (BREAKING)

- `application/command.Command` is now a marker interface
  (`interface{}`); the previously required `CommandName() string` method
  is **optional**. Routing names are resolved via `command.NameOf`, which
  prefers an explicitly declared `CommandName()` and falls back to the Go
  type name. The same change applies to `application/query.Query` /
  `query.NameOf`.
- `command.InMemoryBus.Dispatch` and `query.InMemoryBus.Dispatch` now
  call `NameOf` instead of `cmd.CommandName()` directly.
- `command.Register[C, R]` and `query.Register[Q, R]` derive the
  registration name from the type parameter via reflection rather than
  the value-side `CommandName()` call (still preferring the explicit
  method when present and safe).

### Migration

Existing services that **declared `CommandName()` / `QueryName()` on
their commands and queries** require **no changes** — the explicit method
remains preferred at runtime and over reflection.

Code that **directly invoked `cmd.CommandName()`** on a value typed as
the `Command` interface will fail to compile. Replace such calls with
`command.NameOf(cmd)` (and `query.NameOf(q)` for queries):

```go
// Before
name := cmd.CommandName()

// After
name := command.NameOf(cmd)
```

Cross-service contracts (Kafka topics, RPC method keys) should continue
to declare `CommandName()` explicitly so renaming the Go type does not
silently move the routing key on the wire.

[Unreleased]: https://github.com/slam0504/go-ddd-core/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/slam0504/go-ddd-core/releases/tag/v0.3.0
[0.2.0]: https://github.com/slam0504/go-ddd-core/releases/tag/v0.2.0

## [0.1.0] - 2026-04-18

Initial public release. The core defines interfaces and primitives only —
no infra client code. Downstream services pick concrete adapters and wire
them via config.

### Added

#### Domain primitives (`domain/`)
- `Entity[ID]` / `BaseEntity[ID]` with structural equality by id.
- `AggregateRoot[ID]` / `BaseAggregate[ID]` with event recording, version
  tracking, and a defensive copy on `DomainEvents()`.
- `ValueObject` marker interface.
- `DomainEvent` / `BaseEvent` with stable metadata fields.
- `Repository[T, ID]` and `ReadOnlyRepository[T, ID]` generic contracts.
- Sentinel errors: `ErrNotFound`, `ErrConcurrencyConflict`, `ErrInvalidArgument`,
  `ErrRuleViolation`, plus `RuleViolation` with code + message.

#### CQRS (`application/`)
- `command.Command` / `command.Handler[C,R]` + typed `Register[C,R]` helper.
- `command.Bus` interface + `command.InMemoryBus` default implementation.
- `query.Query` / `query.Handler[Q,R]` + mirrored bus and helpers.
- `Middleware` + `Chain` composition for cross-cutting concerns.
- `UnitOfWork` interface for transaction boundaries.

#### Event sourcing (`eventsourcing/`)
- `Applier` + `ESAggregate[ID]` contract.
- `BaseESAggregate[ID]` helper with `Raise` and `LoadHistory`.
- `EventStore` with `Append` / `Load` / `LoadFrom` + optimistic concurrency.
- Optional `AllStream` capability for global projection feeds.
- `SnapshotStore` + `SnapshotPolicy` (`EveryN` helper).
- `Projector` / `Reactor` / `CheckpointStore` for read models and side effects.
- `ErrConcurrency` wraps `domain.ErrConcurrencyConflict` so downstream code
  can match a single sentinel across layers.

#### Event bus (`eventbus/`)
- `Codec` interface for DomainEvent ↔ transport message translation.
- Standard header conventions (`event_id`, `event_name`, `aggregate_id`, …).
- `Publisher` / `Subscriber` interfaces built on top of watermill.
- `Envelope` with decoded event + raw watermill message + ack/nack
  (nil-safe when `Raw` is absent).
- `EventHandler[E]` + `HandlerFunc[E]` + `DispatchFunc` + `Middleware`.
- `Router` with typed registration, middleware, and an `OnUnknown` hook.
- `Outbox` / `OutboxStore` / `Relay` contracts for transactional delivery.
- `Inbox` contract for consumer-side idempotency.

#### Infra ports (`ports/`)
- `logger`: slog-shaped `Logger` interface + `Level` + `Attr`.
- `observability`: OpenTelemetry-based `Provider` (Tracer, Meter, Propagator,
  Shutdown).
- `cache`: `Cache` + generic `TypedCache[T]` + `ErrMiss`.
- `database`: `TxManager` + `TxFunc` + `Pinger` + `Closer` + `ErrNoRows`.
- `storage`: `ObjectStorage` + optional `PresignedURLer` + `ErrNotFound`.
- `httpclient`: `Client` + `ContextualClient`.

#### Transport contracts (`transport/`)
- `http`: `Server`, `Router`, `Middleware` + `Chain`, `HandlerProvider`.
- `grpc`: `Server`, `ServiceRegistrar`, `ServiceProvider`,
  `InterceptorProvider` (uses `any` to avoid depending on `google.golang.org/grpc`).
- `graphql`: `Server` (returns `http.Handler`), `SchemaProvider`,
  `ResolverRegistrar`, `ResolverProvider`.

#### Config (`config/`)
- `Provider` interface with `Load` / `Get` / `OnChange`.
- `Root` schema covering App, Logger, Observability, HTTP, gRPC, GraphQL,
  Database, Cache, Messaging, Storage.
- `ViperProvider` default implementation supporting yaml + env sources,
  with race-safe `OnChange` (RWMutex + snapshot-then-invoke).

#### Bootstrap (`bootstrap/`)
- `AdapterRegistry` with `Register` / `Resolve` / `Build` by (kind, driver).
- `Module` interface + `ModuleFunc` adapter.
- `Lifecycle` with ordered start, reverse-order stop, `AppendStart` /
  `AppendStop` helpers, `WaitForSignal`, and `ShutdownContext`.
- `App` container with config + logger + registry + lifecycle + named
  value bag + documented Start execution order.

#### Utilities (`pkg/`)
- `contextx`: trace / request / correlation / causation / tenant / user ID
  context keys.
- `errorsx`: coded `Error` type with `Unwrap`, copy-based `WithDetail`
  (maps.Clone), and `CodeOf`.
- `idgen`: `Generator` contract + `UUIDv4` / `UUIDv7` defaults.

#### Tooling
- GitHub Actions CI (`lint` via golangci-lint-action@v7 + `build + test`
  with race detection).
- `.golangci.yml` v2 config with errorlint / gocritic / gosec / misspell /
  nilerr / unconvert / unparam / whitespace plus gofmt + goimports.

#### Example (`examples/minimal/`)
- Order aggregate built on `BaseAggregate`.
- `PlaceOrderHandler` + `GetOrderHandler` wired through the CQRS buses.
- In-memory repository, slog logger adapter, stdlib HTTP server adapter.
- Configurable via `examples/minimal/config.yaml`.
- Runs end-to-end with `go run ./examples/minimal/cmd`.

### Requirements
- Go 1.24+

[0.1.0]: https://github.com/slam0504/go-ddd-core/releases/tag/v0.1.0
