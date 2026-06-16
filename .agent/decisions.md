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

## Background Jobs Contract (`ports/jobs`, post-v0.8.0)

- **jobs vs eventbus**: eventbus carries domain events (facts, fanned out to many
  independent consumers); jobs carries tasks (work to do, handed to one worker
  pool, with run-at scheduling and an execution lifecycle). A domain event MAY
  trigger a job; they are different primitives. `Relay.Run(ctx)`'s
  blocks-until-cancelled lifecycle shape is reused, not its event coupling.
- **Shape**: `Enqueuer.Enqueue(ctx, Job) (JobInfo, error)`,
  `Worker.Register(type, Handler) error` + `Run(ctx) error`,
  `Job{Type, Payload, ProcessAt}` / `Task{ID, Type, Payload}` / `JobInfo{ID}`,
  `Handler`/`HandlerFunc`. No `Status` enum, no sentinels (`errorsx` codes via
  doc-fixed mapping: `CodeInvalidArgument`→400, `CodeAlreadyExists`→409,
  `CodeUnavailable`→503 — all pre-existing), no func-adapters for the
  multi-method roles.
- **Deliberately excluded (YAGNI until consumer evidence)**: cron/recurring
  scheduling, `MaxRetries`/skip-retry signals (retry/backoff/dead-letter are
  adapter policy), queue/lane routing fields, enqueue-side dedup
  (`ports/idempotency` covers handler-side), result/await (that is
  RPC/workflow), per-job timeout fields, ack/attempt observation hooks.
- **At-least-once floor, six prerequisites**: nil-error Enqueue / sustained
  worker-running / handler registered at delivery / sustained backend
  reachability / no durable loss / ProcessAt arrived. (2) and (4) are sustained
  liveness conditions — outages and worker stops suspend (retain), never drop;
  retain-until-dequeue with durable-loss as the only pre-attempt exception.
- **Two-class Enqueue errors, fixed precedence**: (1a) empty Type → (1b)
  out-of-horizon ProcessAt → (2a) ctx (entry check before backend contact;
  Canceled and DeadlineExceeded both) → (2b) backend (always coded, never
  `CodeUnknown`; unclassifiable → `CodeInternal`). Class 2 is INDETERMINATE
  except the pre-cancelled/pre-expired entry check. snapshot-before-submit:
  payload copy precedes any backend I/O, so accepted-but-ack-lost jobs stay
  isolated from caller mutation.
- **Run endpoints, caller-observable**: (A) cancellation with no independent
  fatal → nil (errors arising FROM shutting down are not fatals: logged, never
  returned); (B) independent fatal, no cancellation → coded errorsx; overlap →
  either, callers tolerate both. Liveness over graceful drain (stuck handler is
  orphaned, never blocks Run); conformance bound is implementation-declared
  (`ShutdownWithin`, mirrors `ReclaimWithin`; non-positive = failure).
  Recoverable-state model after Run returns: completed (late ack, atomic) /
  pending-retryable / transient active-leased resolving within a declared
  bound; terminal loss is the only illegal outcome.
- **Homogeneous worker pool is a deployment precondition**: no type-based
  routing across workers sharing a backing-store namespace; the unhandled-job
  policy surfaces violations loudly (never acked as success).
- **Suite split (owner decision, upheld across reviews)**: exported
  `jobstest.RunContract` covers ONLY the synchronous interface-return surface
  (validation codes + precedence incl. DeadlineExceeded variants, zero/non-zero
  `JobInfo`, Register semantics) — it never runs a worker, so it cannot flake.
  Delivery/timing invariants are demonstrated by the core-local fake
  (executable spec, injected clock + lease) and ENFORCED on adapters by the
  tag-gate intent tests below.

### Compile-tested spike (pre-merge gate — executed 2026-06-11, results)

Throwaway branch `spike/jobs-asynq` in `go-ddd-adapters` (replace-directive on
local core; never merged). All tests `go test -race`; testcontainers Redis
(Docker required — a skip would have counted as gate failure, none occurred).

- **Pinned versions (resolved via `go get`, go.sum verbatim)**:
  - `github.com/hibiken/asynq v0.24.1`
    `h1:+5iIEAyA9K/lcSPvx3qoPtsKJeKI5u9aOIvUmSsazEw=`
  - `github.com/alicebob/miniredis/v2 v2.38.0`
    `h1:nZAzCR+Lj+Vxk4ZXzm2NuKq2O33RXj1XxJ2e2uP9jiw=`
  - `github.com/riverqueue/river v0.39.0`
    `h1:VsoPJ8KTx7SvWQGWtdLjKxw15IjnYHj3xKb0UA+7200=` (+
    `riverdriver/riverpgxv5 v0.39.0`); v0 line as planned, no v1 upstream.
  - `github.com/testcontainers/testcontainers-go v0.42.0` (already pinned by
    the adapters module)
    `h1:He3IhTzTZOygSXLJPMX7n44XtK+qhjat1nI9cneBbUY=`
- **Asynq mapping holds** (build clean + miniredis delivery smoke PASS):
  contract implements naturally; two adapter-layer notes confirmed —
  (1) dispatch MUST use a self-managed exact-match map, not `asynq.ServeMux`
  (longest-prefix matching violates exact-type-match); (2) `srv.Shutdown()`
  returns no error (requeue failures are logged), matching the
  cancellation-commits-to-nil endpoint.
- **Three shutdown-semantics tests (testcontainers Redis 7) all PASS**:
  1. Stuck handler: Run nil within `ShutdownWithin` (2s asynq drain bound,
     10s declared); fresh Worker instance actually redelivered.
  2. Redis stopped mid-shutdown (synchronized: handler dequeued+blocked →
     container stopped → ctx cancelled): Run nil while down; operation-specific
     evidence captured — failed `evalsha` touching
     `asynq:{default}:active`/`:lease`/`:pending` (the requeue script) plus
     asynq log "Could not push task ... back to queue"; after restart the job
     was actually redelivered (retryable branch). Deviations from plan, both
     evidence-strengthening: container stop/start used instead of pause
     (deterministic connection-refused beats hung commands), and the failed
     command's full key set recorded (subsumes the script-SHA/key-prefix
     classification concern — poll false-positives excluded because the
     evidence command carries the requeue key tuple).
  3. Ack/shutdown race ×20 iterations: every job ended completed (Inspector +
     `asynq.Retention` evidence) or retryable-then-redelivered; none lost, none
     stuck active beyond the declared bound. asynq's ack (`Done`) runs as an
     atomic Lua script — consistent with the binary late-ack rule.
  Docker port note: a stopped+started container re-publishes its random host
  port; recovery-phase clients must re-resolve the endpoint (spike test does).
- **River compile-level mapping holds** (build clean, runtime UNVERIFIED —
  needs Postgres; deferred to its own adapter cycle): single envelope
  `JobArgs{Kind() fixed}` + dispatching generic worker; `Insert` +
  `InsertOpts.ScheduledAt` maps ProcessAt; int64 job ID → `strconv` string.

### Tag gate (SATISFIED 2026-06-16 → v0.9.0)

Satisfied by the first production adapter — a `jobs/asynq` Enqueuer/Worker
adapter (`jobsasynq`) over `hibiken/asynq v0.24.1` in `go-ddd-adapters` PR #29
(merge `1f8a685`), which pins core at the pseudo-version
`v0.8.1-0.20260616032638-784ef3ea2bcc` and passes all of `(0)+(a)–(v)` under
`go test -race`. The version resolved to **v0.9.0** (additive minor; the
contract is an exported API addition). Built subagent-driven (14-task
sequence) with two main-session review passes that caught a `nilerr`
false-positive, a panic-to-error guard, and an `(t)` semantic gap (reworked
into a deterministic hook-injected test) and confirmed two implementer
plan-deviations as correct.

The first production adapter must pass **(0)+(a)–(v)**, all mandatory, all
under `go test -race`:

- (0) `ports/jobs/jobstest.RunContract` green.
- (a) at-least-once redelivery incl. concurrent-duplicate tolerance;
- (b) dispatch not before `ProcessAt` on the backend's own scheduling clock
  (control/observe that clock; single clock, no margin);
- (c) Run returns nil within the adapter-declared `ShutdownWithin` (timed from
  cancel, no extra margin, non-positive = `t.Fatalf`), incl. the
  teardown-failure variant (injected shutdown-path backend error → still nil);
- (d) handler payload mutation does not pollute redelivery;
- (e) ID stable across redeliveries;
- (f) unreachable backend → `CodeUnavailable`; generalized variant: ANY
  non-ctx backend failure yields `errorsx.CodeOf != CodeUnknown`
  (unclassifiable → `CodeInternal`);
- (g) malformed Enqueue writes nothing (introspection);
- (h) fatal startup → coded errorsx (non-nil, not a ctx error);
- (i) enqueue payload snapshot;
- (j) handler ctx cancelled on shutdown;
- (k) exact-type-match dispatch rejects prefixes;
- (l) concurrent Enqueue clean under -race + Run's two endpoints (no
  simultaneous-race tie-break assertion);
- (m) out-of-horizon ProcessAt rejected at Enqueue (`CodeInvalidArgument`,
  zero JobInfo, nothing written; precedence over cancelled ctx);
- (n) accepted scheduled job retained past ProcessAt without a worker;
- (o) duplicate Register keeps the original handler (h1 receives, not h2);
- (p) past ProcessAt immediately eligible;
- (q) ctx precedes backend within class 2 — both variants against an
  unavailable backend: pre-cancelled → `Canceled`; pre-expired deadline →
  `DeadlineExceeded`; zero JobInfo + no backend contact;
- (r) worker stopping before dequeue, then a NEW Worker instance over the same
  store delivers (Run once per instance);
- (s) shutdown in-flight recoverability, split deterministic:
  (s1) late-ack branch — release the blocked handler after Run returns; within
  the declared `RecoverWithin` the job reaches completed (or
  pending-retryable, then prove via (s2) step 4);
  (s2) never-ack branch — within `RecoverWithin` the job reaches
  pending-retryable (lost = fail; stuck active/leased past the bound = fail),
  then a NEW Worker instance MUST actually redeliver within the declared
  `RedeliverWithin`.
  Fixture: `RecoverWithin`/`RedeliverWithin` (non-positive = `t.Fatalf`;
  RecoverWithin folds in the lease TTL) + `JobState(ctx, id)` introspection
  classifying completed / pendingRetryable / activeLeased / lostDiscarded;
  introspection errors = `t.Fatalf`; not-found with no completion evidence =
  lostDiscarded (backends that prune completed jobs must give the fixture a
  completion-evidence channel, e.g. test-scoped retention);
- (t) accepted-but-ack-lost fault: Enqueue error follows class-2 rules (ctx
  error or coded non-Unknown) + zero JobInfo; caller mutation after the error
  does not corrupt the accepted job's payload (snapshot-before-submit);
- (u) unhandled type never acked as success and follows the DOCUMENTED
  unhandled-job policy (introspection-verified);
- (v) single-source declared values: the documented scheduling horizon,
  durability boundary, and `ShutdownWithin`/`RecoverWithin`/`RedeliverWithin`
  MUST be the same exported constants/options the intent-test fixtures use.

Plus: adapter docs declare the retry/backoff/dead-letter policy and the
fatal-code taxonomy; after the first adapter lands, distill the delivery/timing
half of `jobstest` from the proven common semantics. The pseudo-version-only
compensating control held until the adapter landed; with PR #29 merged and
`(0)+(a)–(v)` green, core cut **v0.9.0** (release-prep PR + annotated tag +
GitHub Release as Latest), same 2-step cross-repo close as v0.5.0–v0.8.0.
