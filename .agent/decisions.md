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

## Inbound Request Idempotency Contract (`ports/idempotency`, post-v0.7.0)

Added on branch `feat/ports-idempotency` 2026-06-08. Scope: guard a non-idempotent
inbound request (typically an HTTP POST) against duplicate execution when a client
retries the same idempotency key. **Distinct from `eventbus.Inbox`** (above, "Event
Idempotency"): Inbox dedups broker-delivered domain events per consumer
(`InboxKey{Consumer, EventID}`); this contract guards inbound request retries keyed
by an adapter-built scope + client key + request fingerprint, with lease-tracked
reservation ownership. Core ships the contract + an exported conformance suite only;
enforcement middleware, key/fingerprint extraction, response (de)serialization, and
concrete backings (in-memory / Redis / SQL) live in adapters.

- **Dedicated `Store` contract, not "just use `ports/cache`".** Correct idempotency
  needs an *atomic claim (reserve) + lease lifecycle*. A `cache.Exists`-then-`cache.Set`
  has a TOCTOU race (two concurrent retries both read "unseen" → both execute) and
  cannot express reservation ownership. `Begin` folds "atomically reserve or report
  existing state" into one call; atomicity + lease validation are the adapter's job
  (Redis SETNX+lease, SQL unique, in-memory mutex). Refines the `docs/roadmap.md`
  shorthand "can reuse `ports/cache` as backing".
- **`Begin(ctx, scope, key, fingerprint)` carries a trusted `scope`.** `scope` is an
  adapter/middleware-built isolation prefix (isolating **at least** tenant and
  operation/endpoint), **never taken from the client**; `key` is the raw client
  idempotency key. Same `key` under a different `scope` is a fully independent record;
  `fingerprint` does **not** substitute for `scope` (an identical payload could
  otherwise replay across tenants). Putting scope in the signature is harder to get
  wrong than asking each caller to pre-concatenate an opaque key, and avoids
  string-concat collisions — guarded by the suite's tuple-separation case
  (`("tenant:op","k")` vs `("tenant","op:k")` must be two records).
- **Normal outcomes are data (`Status` enum), not sentinels.** `Begin` returns
  `(Reservation, error)`: `StatusNew`/`InProgress`/`Completed`/`Mismatch` are the
  caller's switch; only "state undeterminable" travels the `error` channel as a coded
  `errorsx` value. No sentinel, no `codedError` wrapper (honest divergence from auth)
  — `Store` is a 3-method interface so there is also **no `StoreFunc`** (func-adapters
  fit single-method contracts only). `StatusUnknown` is the **zero value** so a bug
  returning `Reservation{}` fails closed (never executes) instead of looking like a
  successful new reservation.
- **Lease is a relative advisory `LeaseTTL time.Duration`, not absolute
  `LeaseExpiresAt time.Time`.** Absolute time would invite the caller to compare its
  local clock against a Redis/SQL/remote backend clock (skew + base unreliable). The
  honest promise is "roughly this much processing budget left", tracked by the caller's
  own monotonic clock. `LeaseTTL <= 0` = adapter exposes no observable budget; it does
  **NOT** mean never-expires. Whether a `Finish` still applies is decided by the Store
  validating the lease token, **not** by the caller comparing TTLs. The lease token
  blocks "stale handler overwrites a newer reservation after key reuse"; it cannot stop
  "stale handler already committed an external side effect" — so **no exactly-once
  guarantee for side effects**, and **no `Renew`/`Extend` in v1** (would drag
  heartbeats / renew-failure handling into core).
- **`Finish`/`Cancel` errors are three-way, not a "non-nil = not applied" flatten.**
  `nil` = applied for certain; `errorsx.CodeConflict` (→409) = NOT applied for certain
  (stale/expired/terminal — a *decided* rejection); **any other error
  (`CodeUnavailable`/timeout/ctx) = INDETERMINATE** (a remote store may have committed
  then lost the ACK). The caller MUST NOT assume "not applied"; it recovers by
  re-issuing `Begin` (same scope/key/fingerprint) to read the authoritative state.
  **No read-only query method** — it would overlap `Begin`'s authoritative read and
  re-introduce TOCTOU (`Begin` is already the atomic "read + reserve-if-needed" entry).
  Stale/expired/terminal **all** return `CodeConflict` (one code, so adapters don't
  diverge on HTTP behaviour — an earlier draft allowed `CodeConflict`/`CodeNotFound`).
- **`Cancel` is narrowed: release only when sure no observable side effect committed.**
  On an ambiguous failure, leave the reservation standing (let the adapter reclaim it)
  rather than `Cancel`, so a retry does not re-run a partially-applied operation.
  `Cancel` must NOT delete an already-completed payload (completed is terminal).
- **`Response` is opaque `[]byte` but must be a complete replay encoding.** The Store
  stores/returns bytes verbatim, never parsing — so the contract works for
  gRPC/GraphQL/command, not just HTTP. The contract *requires* the consumer to encode a
  faithfully replayable result (HTTP: status + the headers needed to replay + body, not
  body alone). **Core defines no transport record schema** (which headers, encoding,
  versioning are an adapter concern). `Response == nil`/empty on `StatusCompleted` is a
  legal empty response (e.g. HTTP 204), not lost data; `StatusMismatch` must never leak
  a stored payload.
- **Reservation field trust boundary (security-critical).** `Finish`/`Cancel` read
  **only** `Scope`, `Key`, `LeaseToken` from the passed-in `Reservation` (lookup keys +
  ownership credential); the mutable `Status`/`LeaseTTL`/`Response` are untrusted. A
  caller cannot extend a lease or forge ownership by mutating the returned struct. This
  is a **hard** adapter requirement, guarded by the suite's forged-token / tampered-field
  cases (tamper `LeaseTTL`/`Status`/`Response` → `Finish` still succeeds; only forging
  `LeaseToken` flips to `CodeConflict`).
- **ctx cancellation is part of the error contract.** All three methods must `%w`-wrap
  the cause so `errors.Is(err, context.Canceled)` / `context.DeadlineExceeded` holds;
  a cancelled `Begin` must not silently return `StatusNew`, a cancelled `Finish` must
  not silently count as applied. This is deterministically testable (pass an
  already-cancelled / already-past-deadline ctx, zero wait) → it lives in the exported
  suite, not local-only.
- **Eventual-reclaim liveness is a core MUST, with enforced verification.** An
  unfinished in-progress reservation must *eventually* become re-`Begin`-able (else a
  handler crash / dropped connection / ambiguous Cancel blocks the key forever). This
  guarantees **key liveness only, NOT side-effect exactly-once**. The exact reclaim
  delay/precision and completed-record retention are adapter policy (backend TTL
  precision, cleanup, clock, latency) — so they stay **out** of the deterministic
  `RunStoreContract` (which would turn flaky). Verification is **not** a verbal MUST:
  an opt-in exported `RunReclaimContract(t, factory, ReclaimOptions{ReclaimWithin})`
  holds the store to **exactly** the adapter-**declared** `ReclaimWithin` (real
  wait/poll, deadline anchored at reservation creation, poll capped so enforcement
  stays tight), *not* a value derived from `LeaseTTL` nor an injected `advance` clock
  (an injected clock is easy to wire to a false green and forces unnatural internal
  hooks). The suite adds **no margin of its own** — the adapter must bake any
  scheduling jitter into the value it declares (e.g. declare 80ms while actually
  reclaiming at 20ms). A **non-positive** `ReclaimWithin` **fails** the suite (an
  adapter with no time-based reclaim simply does not call this helper, but the
  liveness MUST still requires a documented reclaim policy). Earlier draft skipped on
  zero and waited `2x + 500ms`; that let an adapter declaring 40ms but reclaiming at
  500ms pass, defeating "the adapter declares the bound" — removed.
- **Conformance suite ships now, but enforces only the deterministic state machine.**
  This is a stateful (state machine + lease ownership + Cancel + mismatch + replay)
  contract; a `_test.go`-local fake would only prove "our fake matches our test" and
  bind no future Redis/SQL adapter. So core exports
  `idempotencytest.RunStoreContract(t, factory)` — **transport-neutral**: it imports
  only `pkg/errorsx` and asserts `errorsx.CodeOf(err) == CodeConflict`, never an HTTP
  status (because `httpx.Translate` maps both `CodeConflict` and `CodeAlreadyExists` to
  409, so a status assertion couldn't prove the contract code). The errorsx→HTTP
  mapping (400/409/503) is verified once, in core's own local unit test (the only
  `httpx` importer). Time-dependent expiry/retention is out (adapter policy);
  reclaim has its own opt-in helper above.
- **Core ships the contract + conformance helpers only** — no in-memory / Redis / SQL
  Store in core (`idempotencytest`'s `fakeStore` lives in `_test.go`, not a production
  path). Matches the v0.5.0–v0.7.0 minimalism.
- **Tag gate** (satisfied 2026-06-09): the contract shipped unreleased until the
  first adapter consumer landed — a Redis `Store` adapter in `go-ddd-adapters`
  PR #27 (merge commit `5248bd5`), pinning core at pseudo-version
  `v0.7.1-0.20260608093712-0e1292d20462`. The version resolved to **v0.8.0**
  (adds exported API → semver minor; v0.7.x reserved for AuthZ fixes), cut via
  release-prep PR (`release/v0.8.0-prep`). The adapter satisfied the acceptance
  criteria at its release:
  (a) `RunStoreContract` green against the real Store;
  (b) `RunReclaimContract` run with the adapter's declared `ReclaimWithin` + the
  adapter docs declaring its TTL/cleanup policy;
  (c) middleware intent tests — both 409 paths (`StatusInProgress`, `StatusMismatch`),
  `StatusUnknown`→500 (fail-closed), full `StatusCompleted` replay (status + headers +
  body, no handler re-run), `StatusMismatch` non-leak, and the error channel
  (`CodeInvalidArgument`→400 / `CodeUnavailable`→503). These were written into the
  contract docs at contract time so the reclaim MUST and the middleware mapping/replay
  were not left unverified when the adapter landed. Same tag-gate discipline used for
  v0.5.0 / v0.6.0 / v0.7.0.
