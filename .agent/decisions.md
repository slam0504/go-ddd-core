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

## AuthN Contract (v0.6.0, `ports/auth`)

Added on branch `feat/ports-auth` 2026-06-04. Scope is AuthN only — who the
caller is. AuthZ (permissions / 403), token issuance, and multi-tenant
resolution are deliberately out (see `docs/roadmap.md` v0.6.0 / v0.6.x).

- **`Identity` = 4 fields** (Subject, TenantID, Roles, Claims). No `ExpiresAt` /
  `IssuedAt` — token lifetime is the verifier's concern; an expired token
  yields `ErrTokenExpired`, not an Identity carrying expiry. Keeps the shape
  shareable across JWT / opaque / session / API-key verifiers.
- **Sentinels are `errorsx`-coded, all `CodeUnauthorized`** (→ HTTP 401 via
  `pkg/errorsx/httpx`), so adapters need no per-error mapping table. They are
  **tamper-proof via an auth-private `tokenError` wrapper**: it carries only an
  unexported message, and its `Unwrap()` mints a fresh `*errorsx.Error` on
  every call. `errorsx.CodeOf` / `httpx.Translate` (both `errors.As`) thus get
  a throwaway copy — mutating it cannot corrupt the shared sentinel's code or
  401 mapping — while `errors.Is` still matches the sentinel pointer itself.
  An earlier attempt declared the sentinels as static type `error` over a bare
  `errorsx.New(...)`; that only blocked direct field writes, since `errors.As`
  still handed callers the live `*errorsx.Error` pointer (review-caught).
  **403 / `CodeForbidden` stays out of `TokenVerifier`** — that is AuthZ.
- **ctx helpers live in `ports/auth`, not `pkg/contextx`.** Identity is a
  struct, so it cannot reuse contextx's string-only `stringValue`; keeping the
  helpers with the contract also avoids a `contextx → ports` back-dependency.
  `WithIdentity` / `IdentityFromContext` clone `Roles` / `Claims` on both write
  and read so a context-stored identity cannot be mutated through an alias;
  nested `Claims` values are documented immutable.
- **`TokenVerifierFunc`**, not a bare `VerifierFunc` — the `Token` qualifier
  leaves the plain name free for a future AuthZ verifier. Mirrors the repo's
  single-method func-adapter pattern (`command.HandlerFunc`, etc.).
- **`ErrTokenMissing` ownership**: transport middleware may return it before
  calling `Verify` (e.g. no Authorization header); a verifier invoked with an
  empty token should also return it, so behaviour stays consistent across
  adapters.
- **Tag gate** (satisfied 2026-06-05): the contract shipped unreleased until
  the first adapter consumer landed — `auth/jwt` + HTTP middleware in
  `go-ddd-adapters` PR #23 (merged `ae76f78`). `v0.6.0` was then cut, the same
  discipline used for v0.5.0.
