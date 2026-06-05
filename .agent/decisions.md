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
  (Resolved in the AuthZ cycle: the func adapter is named `AuthorizerFunc` to
  mirror the `Authorizer` interface, so the reserved `VerifierFunc` spelling
  ended up unused — the qualification still paid off by keeping the two adapter
  names unambiguous.)
- **`ErrTokenMissing` ownership**: transport middleware may return it before
  calling `Verify` (e.g. no Authorization header); a verifier invoked with an
  empty token should also return it, so behaviour stays consistent across
  adapters.
- **Tag gate** (satisfied 2026-06-05): the contract shipped unreleased until
  the first adapter consumer landed — `auth/jwt` + HTTP middleware in
  `go-ddd-adapters` PR #23 (merged `ae76f78`). `v0.6.0` was then cut, the same
  discipline used for v0.5.0.

## AuthZ Contract (`ports/auth`, post-v0.6.0)

Added on branch `feat/ports-authz`. Scope is AuthZ only — whether a caller may
perform an action on a resource. It is the counterpart to the v0.6.0 AuthN
contract: AuthN says who you are (`TokenVerifier`, `auth.go`), AuthZ says what
you may do (`Authorizer`, `authz.go`). Same package — `Authorizer` reuses
`Identity` directly and a back-dependency would otherwise be needed. Concrete
policy engines (RBAC / ABAC / OPA / Casbin), enforcement middleware, and token
issuance stay out of core.

- **Caller is the full `Identity`, not a `subject string`.** RBAC needs `Roles`,
  ABAC needs `Claims`/`TenantID`; a bare subject would force every policy
  adapter to re-fetch them. This refines the `docs/roadmap.md:36` shorthand
  `Allow(ctx, subject, action, resource)` — `subject` is read as the caller
  principal, typed as the now-shipped `Identity`.
- **`Resource` is a 2-field struct (`Type`, `ID`), not a `string` or `any`.**
  A string invites ad-hoc `"order:123"` formats; `any` is too loose and pushes a
  type switch into every adapter. `Type`+`ID` is the minimal portable shape that
  RBAC/ABAC/Casbin/OPA-style adapters can all consume. `ID == ""` means
  collection/type-level (e.g. `Resource{Type:"order"}`); an empty `Type` is
  malformed input.
- **`Allow` returns a bare `error`, four meanings.** `nil` = allowed;
  `ErrForbidden` (incl. `%w` wrap) = policy evaluated then denied → 403; any
  other error = the decision could not be made (return a coded `errorsx` value,
  e.g. `CodeUnavailable` for 503; uncoded → 500). No `(bool, error)` (double
  semantics when `allowed=false, err!=nil`) and no `Decision` struct (no
  consumer needs reason/obligations/audit metadata yet — YAGNI).
- **Malformed input is split from denial via `ErrInvalidAuthorizationRequest`**
  (coded `errorsx.CodeInvalidArgument` → 400). Empty `action`, empty
  `Resource.Type`, and a zero `Identity` are caller contract bugs; folding them
  into `ErrForbidden` would mask a 4xx-client bug as a policy 403 and hurt
  observability. Fail-loud over fail-closed here.
- **A zero `Identity` is malformed, not anonymous.** Callers that mean anonymous
  must say so explicitly (`Identity{Subject:"anonymous"}`); an `auth.Anonymous`
  helper/constant is deferred until a consumer needs it.
- **`Allow` borrows `caller` read-only; no auto-clone.** AuthZ is a high-frequency
  check (middleware / handler / use case), so cloning `Roles`/`Claims` on every
  call is an unwanted hot-path cost. Auto-cloning inside `AuthorizerFunc` would
  also only protect the func path, not direct implementations — misleading
  half-isolation. Instead the previously private `Identity.clone()` is promoted
  to public **`Identity.Clone()`**; an implementation that retains the caller
  beyond the call (async audit, queued decision, separate goroutine) clones
  explicitly. This differs from AuthN's `WithIdentity`/`IdentityFromContext`,
  which auto-clone because they store the identity in a context across time.
- **Both AuthZ sentinels reuse the AuthN tamper-proof wrapper.** The private
  `tokenError{msg}` was generalised to `codedError{code, msg}`; the three AuthN
  sentinels now carry `CodeUnauthorized` explicitly and the two AuthZ sentinels
  carry their own codes. With five coded sentinels across the two concerns, one
  shared wrapper avoids duplicating a security-sensitive mechanism whose `Unwrap`
  mints a throwaway copy per call. Existing `TestSentinels_MapToHTTP401` /
  `TestSentinels_Immutable` guard the AuthN side against regression.
- **Core ships the contract only** — `Authorizer`, `AuthorizerFunc`, `Resource`,
  `ErrForbidden`, `ErrInvalidAuthorizationRequest`, and the now-public
  `Identity.Clone()`. No enforcement helper / middleware (those smuggle in
  transport/use-case opinions and have no consumer evidence yet), matching the
  v0.6.0 minimalism.
- **Tag gate** (satisfied 2026-06-05): the contract shipped unreleased until the
  first adapter consumer landed — the `auth/casbin` Authorizer adapter (Phase A)
  in `go-ddd-adapters` PR #25 (merged, pinning core at pseudo-version `47e02fa`).
  Version resolved to **v0.7.0** at release-prep: AuthZ adds exported API → semver
  minor bump from v0.6.0, and v0.7.0 stays independent of the closed v0.6.0 AuthN
  cycle (v0.6.x reserved for AuthN/JWT/Bearer fixes). `v0.7.0` was then cut, the
  same discipline used for v0.5.0 / v0.6.0.
