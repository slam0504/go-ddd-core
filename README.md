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
domain/          DDD base types (Entity, AggregateRoot, DomainEvent, Repository)
application/     CQRS primitives (Command/Query bus, UnitOfWork)
eventsourcing/   EventStore, SnapshotStore, Projector
eventbus/        Publisher/Subscriber (watermill contract), Outbox, Inbox
ports/           Infra interfaces (logger, cache, database, storage, httpclient, observability)
transport/       Server contracts (http, grpc, graphql)
config/          Config Provider (viper-backed), shared schema
bootstrap/       App container, Module, AdapterRegistry, lifecycle
pkg/             contextx, errorsx, idgen
examples/        Minimal runnable example
```

## Requirements

- Go 1.24+

## Dependencies (core only)

- `github.com/ThreeDotsLabs/watermill` — messaging contract
- `github.com/spf13/viper` — config contract
- `go.opentelemetry.io/otel` — observability contract
- `github.com/google/uuid` — ID utilities

No infra client libraries are imported by the core.
