# go-ddd-core Decisions

Last verified: 2026-05-15 Asia/Taipei

## DDD Contract Boundaries

- Core is **infrastructure-client-free**, not literally interface-only.
  It ships in-process defaults (CQRS in-memory buses, viper-backed config
  provider, the historical in-memory inbox), but deliberately omits any
  code that talks to a network broker, database driver, or HTTP framework.
  Full rationale lives in `docs/anti-patterns.md` "Design boundaries:
  what core deliberately omits".
- Application code should depend on `application.UnitOfWork`. Use cases
  must not import `ports/database.TxManager` directly — the v0.3.0
  `application.UnitOfWorkFromTxManager(tm)` bridge exists for exactly
  this isolation.
- Database adapters expose `ports/database.TxManager`.

## Event Idempotency

- `DomainEvent.EventID()` is the canonical event id.
- `eventbus.OutboxRecord.ID` identifies the outbox row.
- `eventbus.OutboxRecord.EventID` carries the domain event id for broker
  message ids and downstream inbox dedup. Fan-out to multiple topics
  produces multiple rows sharing one `EventID`.
- `eventbus.Inbox` dedup is scoped by `eventbus.InboxKey{Consumer, EventID}`.
  Do not deduplicate by bare event id across all consumers.
- `eventbus.Inbox` is **best-effort dedup**. `Inbox.Record` returns only
  `error`; the `Memory` implementation silently no-ops on duplicate
  (`eventbus/inbox/memory.go:73-75`), so the caller cannot distinguish
  "first write" from "duplicate write". Concurrent redelivery can let
  two workers both pass `Seen`, run the handler, then both `Record`
  returns nil while side effects already duplicated. **Handlers must
  therefore be idempotent**: at-least-once with best-effort dedup is
  not effectively-once.

## Command / Query Routing

- `command.Register` and `query.Register` must resolve the same explicit
  names that `Dispatch` uses, including pointer receiver `CommandName()` /
  `QueryName()` implementations.

## Planned Removals (v0.4.0)

- `eventbus/inbox/memory.go` is slated to move to `go-ddd-adapters` in
  v0.4.0. v0.2.x shipped it as a historical exception before the
  infrastructure-client-free boundary was made explicit. v0.3.0 preserves
  it for backward compatibility and announces the upcoming move via
  CHANGELOG `### Deprecated` and `docs/anti-patterns.md` "Note on
  `eventbus/inbox/memory.go`".
