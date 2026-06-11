# Roadmap

Forward-looking investment plan for `go-ddd-core` and `go-ddd-adapters`.

This document complements [`anti-patterns.md`](anti-patterns.md): that
catalogue records **what we deliberately refuse** (shapes proven to hurt
in production); this document records **where we plan to invest next**.
The two are mirror images and should not be merged.

## How we audit coverage

When asking "what's missing for a typical web service?", split items
into **four quadrants** before discussing implementations. The
quadrant determines _where_ the work belongs, not just _whether_ it
should happen.

| Quadrant | Where the work lives | Question that puts an item here |
|---|---|---|
| **A. Core contract gap** | `go-ddd-core/ports/<name>` | Multiple adapters or services need to agree on a shape — no port exists yet. |
| **B. Adapter selection** | `go-ddd-adapters/<area>/<driver>` | Port already defined in core, just need a concrete driver implementation. |
| **C. Deliberately omitted** | Neither (boundary is documented) | A default would either pick a vendor or lose meaning — see anti-patterns "Design boundaries". |
| **D. Service-specific** | The service repo | The choice is dictated by business or vendor lock-in, not framework concerns. |

Audits framed as "we don't have X yet" without this split tend to
collapse adapter work, contract work, and out-of-scope work into a
single "missing features" list. Keep them separated.

## Current quadrant map (post-v0.4.0)

### A. Core contract gaps (need ports in core first)

| Item | Why core, not adapter | Sketch |
|---|---|---|
| **Health checks** | Multiple adapter packages (`database`, `cache`, `storage`, `httpclient`) want to publish a probe; without a shared contract each invents its own. `ports/database.Pinger` already exists but only as a one-shot. | `ports/health.Check { Name() string; Check(ctx) error }` plus optional `CheckFunc` helper. **No registry / HTTP handler in core** — those live in the HTTP adapter, otherwise core grows orchestration opinions it does not need. |
| **AuthN identity** | Inbound HTTP / gRPC / GraphQL all need to attach an identity to ctx; downstream use cases need a uniform read shape. JWT / opaque / session / API-key verifiers differ, but the verifier _contract_ should be one shape. | `ports/auth.Identity` + `ports/auth.TokenVerifier`. Issuer is out of scope (it is a service feature, not a framework concern). |
| **AuthZ policy** | RBAC / ABAC / OPA / Casbin differ, but the call site question — "may this identity perform this action on this resource?" — is universal. | `ports/auth.Authorizer.Allow(ctx, subject, action, resource) error`. Specific policy language belongs in adapters. |
| **Background jobs** ✅ contract shipped (unreleased, tag-gate) | Delayed / queue-worker contracts touch persistence, scheduling, and observability — service code should not be coupled to Asynq, River, or Temporal directly. | `ports/jobs.Enqueuer` + `ports/jobs.Worker` + `ports/jobs.Job`/`Task` plus the synchronous-only `jobstest` suite. **Done in core**, awaiting first adapter consumer to cut the tag. Cron/recurring scheduling deliberately excluded (no consumer evidence to define schedule identity / overlap / missed-run semantics); retry/backoff/dead-letter are adapter policy. |
| **Inbound idempotency-key** ✅ contract shipped (unreleased, tag-gate) | Distinct from `eventbus.Inbox` (which is broker-side dedup). HTTP POST retry protection needs an atomic claim + lease lifecycle, not a plain `ports/cache` get/set (which has a TOCTOU race and cannot express reservation ownership) — hence a dedicated contract. | `ports/idempotency.Store` (`Begin`/`Finish`/`Cancel` + `Reservation`/`Status`) + `idempotencytest` conformance suite. **Done in core**, awaiting first adapter consumer to cut the tag; enforcement middleware lives in the HTTP adapter. |
| **Rate limiting** | Inbound throttling and outbound quota share the same "may this call proceed?" shape. Token-bucket vs leaky-bucket vs sliding-window is an adapter concern. | `ports/ratelimit.Limiter.Allow(ctx, key) (bool, error)`. |

### B. Adapter selection (port exists, just need drivers)

| Port | Adapters wanted | Notes |
|---|---|---|
| `transport/http.Server` | `transport/http/stdlib` first (Go 1.22+ ServeMux), then `chi`, `echo` as siblings | Driver in the path makes it visible the default is not a framework choice. |
| `transport/grpc.Server` | `transport/grpc/grpc` (wraps `google.golang.org/grpc`) | core uses `any` to keep grpc library out of its imports; adapter casts back. |
| `transport/graphql` | `transport/graphql/gqlgen` | Cursor + FilterInput helpers in core; server adapter is the missing piece. |
| `ports/cache.Cache` | `cache/redis`, optionally `cache/ristretto` for in-process L1 | go-redis is the obvious first driver. |
| `ports/storage.ObjectStorage` | `storage/s3`, `storage/gcs`, `storage/minio`, `storage/fs` | aws-sdk-go-v2 for s3. |
| `ports/httpclient.Client` | `httpclient/std` with timeout / retry / OTel tracing / optional breaker bundled | Resilience policy is folded **into** this adapter rather than getting its own port — see "Decisions" below. |
| `ports/database.TxManager` | already `database/pgx`; `database/sql`, `database/mongo` as future drivers | Each driver brings its own ctx-injected handle convention. |
| `eventsourcing.EventStore` / `SnapshotStore` | (no default planned — see C) | If a service repo wants ES, it builds its own adapter using core's contracts. |

### C. Deliberately omitted (boundary kept)

These all have rationale recorded in [`anti-patterns.md`](anti-patterns.md)
"Design boundaries" — link there, do not re-litigate here.

- `eventbus.Relay` default — codec / header conventions are adapter-bound.
- `eventsourcing.EventStore` default — concurrency / snapshot / projection
  policies vary per service.
- Generic CRUD `Store[T, ID]` — would either be too narrow or too wide.

### D. Service-specific (not core, not adapters)

- Business middleware (audit log shape, tenant extraction rules, user
  impersonation policy).
- JWT **issuer** / OAuth server / login flow — issuing tokens is a
  service feature; only the verifier shape belongs in core.
- Feature flag clients (LaunchDarkly, Unleash, Statsig) — vendor lock-in
  + flag policy is service-level.

## Planned cycles

### v0.5.0 — Inbound HTTP + Health

**Core changes** (minimal, low-risk):

- New package `ports/health` with the two-method `Check` interface
  plus a `CheckFunc` adapter. No registry, no aggregation, no HTTP
  handler in core.

**Adapters changes**:

- New package `transport/http/stdlib` (package name `stdlib`,
  importers typically alias as `stdhttp`). First adapter for the
  `transport/http.Server` contract, built on Go 1.22+ `net/http`
  ServeMux (method-pattern routing).
- Graceful shutdown wiring (`Shutdown` deadline honoured against
  `bootstrap.Lifecycle`).
- `health` sub-package providing the `/healthz` + `/readyz` handlers
  plus a small in-adapter registry. Database / cache / storage
  adapters can each export a `health.Check` and the HTTP adapter
  composes them.

**Why this cycle scope**: every service repeats HTTP boot + graceful
shutdown + health endpoints. Putting these out first removes the most
common "yet another bespoke implementation" burden without committing
to controversial choices (no framework default, no auth model, no
session strategy).

**Why stdlib first, not chi/echo**: the first adapter sets the
default behaviour even if it is not labelled "default". Picking chi
or echo would smuggle a framework choice through the side door; the
stdlib adapter is a real, low-dependency baseline that proves the
contract without locking the ecosystem.

### v0.5.x — Cache + Outbound HTTP client

**Adapters only** (no core changes expected):

- `cache/redis` — implements `ports/cache.Cache` over go-redis.
  Translate redis nil to `cache.ErrMiss`. Optional TTL semantics
  documented to match the core port (zero = no expiry).
- `httpclient/std` — implements `ports/httpclient.Client` with:
  - context-aware timeout defaults,
  - retry policy (backoff + max attempts + retryable status codes),
  - OTel tracing,
  - optional circuit breaker via `WithBreaker(...)`.

  Resilience policy is bundled here rather than extracted into a
  separate `ports/resilience.Policy`. Rationale: only one consumer
  needs it today (outbound HTTP). When a second consumer
  (e.g. gRPC client adapter, external API client) wants the same
  policy, refactor it into a port at that point. Premature
  abstraction would lock the policy shape before we know what the
  second consumer actually wants.

### v0.6.0 — AuthN contract

**Core changes**:

- New package `ports/auth` with `Identity` (struct: Subject,
  TenantID, Roles, Claims) and `TokenVerifier` interface. Possibly
  a ctx key helper (`auth.IdentityFromContext` / `WithIdentity`).
- Error sentinels for token-missing / token-invalid / token-expired
  so middleware can map cleanly to HTTP 401/403.

**Adapters changes**:

- `auth/jwt` adapter implementing `TokenVerifier` against a JWKS
  endpoint or static keys.
- HTTP middleware in `transport/http/stdlib` (and any other HTTP
  adapter) that pulls a token, calls the configured verifier, and
  populates the ctx.

**Why a separate cycle**: AuthN design space is large (JWT vs opaque
vs session vs API-key vs mTLS, plus tenant extraction, plus the
verifier-error-to-HTTP-status mapping). A rushed v0.5.0 contract
would force everyone to copy the first shape we shipped. Independent
cycle gives room to brainstorm before committing.

**Explicitly out of scope for v0.6.0**: AuthZ (`ports/auth.Authorizer`),
session management, token issuer, multi-tenant resolution rules.
These follow in v0.6.x or v0.7.0 once the AuthN shape settles.

## Not on the near-term roadmap

- **EventStore implementation** — anti-patterns already records the
  rationale for no default; revisit only if a real service repo needs
  ES and is willing to drive the design.
- **gRPC / GraphQL server adapters** — port exists, but the demand
  signal is weaker than HTTP. Defer until a service actually requests
  one.
- **Generic CRUD repository helper** — keeps the aggregate-repository
  boundary clean; do not add convenience that erodes it.

## Decision log shorthand (2026-05-21 audit)

These are the resolved decisions from the v0.4.0-close audit so future
sessions do not re-relitigate them:

1. Roadmap doc lives at `docs/roadmap.md`, not appended to
   `anti-patterns.md`. Anti-patterns is "what we refuse"; roadmap is
   "what we invest in next". Different purposes.
2. Health `Check` contract lives in `ports/health`, not in
   `bootstrap`. Health is a capability, bootstrap is lifecycle. The
   reverse direction (adapters depending on bootstrap to publish a
   probe) would create a cycle.
3. HTTP adapter path is `transport/http/stdlib`, not
   `transport/http`. Driver-in-path keeps the door open for chi /
   echo siblings without retroactively renaming the first one.
4. Resilience policy is folded into the httpclient adapter for the
   first cycle, not extracted into `ports/resilience`. Refactor when
   a second consumer materialises.
5. AuthN is its own cycle (v0.6.0), not bundled into v0.5.0.
   AuthZ further deferred.
