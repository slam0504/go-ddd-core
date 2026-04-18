# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/slam0504/go-ddd-core/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/slam0504/go-ddd-core/releases/tag/v0.1.0
