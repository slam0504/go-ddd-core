# go-ddd-core State

Last verified: 2026-06-05 Asia/Taipei (v0.7.0 AuthZ core tag shipped; adapter
dep-bump + tag delegated to the adapter session).

v0.7.0 (AuthZ): annotated tag `v0.7.0` (object `c4a4dc1` → merge commit
`3729add`, release-prep PR #15) pushed to origin; `gh api
repos/.../releases/latest` returns `v0.7.0`, GitHub Release published as Latest
at https://github.com/slam0504/go-ddd-core/releases/tag/v0.7.0. Local `gofmt -l`/
`go vet`/`go build`/`go test ./...` clean before merge. **Adapter side NOT done
by this session** — `go-ddd-adapters` still pins core at pseudo-version
`v0.6.1-0.20260605060735-47e02fa632a8` and must bump to `v0.7.0` (root +
`examples/orders` go.mod, `go mod tidy`), then cut its own tag/Release; handed to
the adapter session per spec §10 steps 3–4. Until then the two repos are NOT on
matching tags.

Prior — v0.6.0 (AuthN cycle CLOSED — tag shipped).
`v0.6.0` annotated tag (object `fd596cd` → merge commit `86b1e15`) pushed to
origin; `gh api repos/.../releases/latest` returns `v0.6.0`, GitHub Release
published as Latest at
https://github.com/slam0504/go-ddd-core/releases/tag/v0.6.0. Release-prep
PR #10 merged `86b1e15` after CI green; local `gofmt -l`/`go build`/`go test`
clean at the pre-merge tip `2c4d0a8`. Adapter side synced 2026-06-05 via
read-only `gh`: `go-ddd-adapters` PR #24 dep-bump (`1b0f3ae`) on `go-ddd-core
v0.6.0`, adapters `v0.6.0` tag (`a9d4bfb`) + Release Latest — both repos on
matching `v0.6.0`.
Source: core verified via `git log` on `main` @ `e2ee2bb` (merge of PR #7
release/v0.5.0-prep), `git ls-remote --tags origin v0.5.0` returning
`543cbf3 refs/tags/v0.5.0`, `gh release view v0.5.0` confirming Latest,
and local `gofmt -l .` (clean), `go vet ./...`, `go build ./...`,
`go test ./...` (all packages PASS) on 2026-05-25. Adapter side
synchronised 2026-05-26 from the adapter session: PR #22 dep-bump
merged at `45274dd` (2026-05-26 00:03:52 Asia/Taipei, CI 5/5 on
workflow `26409070511`), adapters `v0.5.0` annotated tag (object
`a02f6d4` → `45274dd`) pushed, GitHub Release published as Latest
(2026-05-26 00:08:32 Asia/Taipei) at
https://github.com/slam0504/go-ddd-adapters/releases/tag/v0.5.0.

## AuthZ Contract Cycle: CORE TAG SHIPPED (v0.7.0) — adapter side pending

Contract merged 2026-06-05 via PR #13 (`feat/ports-authz`, CI green + Codex no
findings), merge commit `64b6bf6` on top of `ef08343`; state sync PR #14
(`47e02fa`). Core adds the authorization counterpart to the v0.6.0 AuthN
contract, in the **same `ports/auth` package**.

Tag gate **satisfied** 2026-06-05 by the first consumer — `go-ddd-adapters`
PR #25 `auth/casbin` Authorizer adapter (Phase A), merged pinning core at
pseudo-version `47e02fa`. Version resolved to **v0.7.0** (AuthZ adds exported
API → semver minor from v0.6.0; independent of the closed v0.6.0 AuthN cycle).
Release-prep PR #15 (merge `3729add`) moved the CHANGELOG entry from
`[Unreleased]` to `[0.7.0] - 2026-06-05`, updated the README `auth/` row to
`[v0.7.0]`, and marked the decisions.md tag gate satisfied. **Core tag shipped**:
`v0.7.0` annotated tag (object `c4a4dc1` → `3729add`) pushed, GitHub Release
published as Latest.

**Remaining (delegated to the adapter session, NOT this one)** — spec §10
steps 3–4: bump `go-ddd-adapters` core dep from pseudo-version
`v0.6.1-0.20260605060735-47e02fa632a8` to `v0.7.0` (root + `examples/orders`
go.mod/go.sum, `go mod tidy`); CI green merge; then annotate + push the adapters
`v0.7.0` tag (new `auth/casbin` package → semver minor) and open its GitHub
Release. The cross-repo cycle is CLOSED only once that lands and both repos read
matching `v0.7.0`.

Shipped scope (now on `main`):

- `A ports/auth/authz.go` — `Resource{Type, ID}`, `Authorizer.Allow(ctx, caller
  Identity, action string, resource Resource) error`, `AuthorizerFunc`, sentinels
  `ErrForbidden` (`CodeForbidden` → 403) and `ErrInvalidAuthorizationRequest`
  (`CodeInvalidArgument` → 400, splits malformed input from policy denial).
- `M ports/auth/auth.go` — private `tokenError{msg}` generalised to shared
  `codedError{code, msg}` (AuthN sentinels now carry `CodeUnauthorized`
  explicitly); private `clone()` promoted to public `Identity.Clone()`; package
  doc updated (AuthZ no longer "out of scope").
- `A ports/auth/authz_test.go` — 7 contract tests (func adapter allow/deny +
  arg propagation, mandatory `httpx.Translate` 403 + 400 acceptance, both
  sentinels tamper-proof, `%w`-wrapped still maps, direct implementation).
- `M ports/auth/auth_test.go` — `TestIdentity_Clone_Isolates` for the public
  `Clone`.
- `M CHANGELOG.md` — new `[Unreleased]` section with the AuthZ `### Added` entry.

Local verification on `feat/ports-authz`: `gofmt -l .` clean, `go vet ./...`,
`go build ./...` clean, `go test ./...` PASS (all packages; AuthN
`TestSentinels_*` still green after the `codedError`/`Clone` refactor).

Design rationale recorded in `.agent/decisions.md` "AuthZ Contract".

## v0.6.0 AuthN Cycle: CLOSED

Release shipped 2026-06-05. Core ships the `ports/auth` AuthN contract only;
the tag gate (first adapter consumer) was satisfied by `go-ddd-adapters`
PR #23 (`auth/jwt` verifier + HTTP bearer-token middleware, merged `ae76f78`).

Sequence:

- **Contract** — PR #8 (`feat/ports-auth` → `main`) merged `6885ddf`
  2026-06-04. Two commits: `d4811f1` (contract) + `c3243aa` (review fix:
  sentinels made tamper-proof via the `tokenError` wrapper). Codex review
  passed; CI green on `c3243aa`. Branch deleted.
- **Release prep** — PR #10 (`release/v0.6.0-prep` → `main`) merged `86b1e15`
  2026-06-05: CHANGELOG `[Unreleased]` → `[0.6.0] - 2026-06-05`, README
  `auth/` row → `[v0.6.0]`, `.agent/decisions.md` tag-gate marked satisfied.
  CI green on `2c4d0a8`. Branch deleted.
- **Tag** — `v0.6.0` annotated tag (object `fd596cd` → `86b1e15`) pushed to
  origin; GitHub Release published as **Latest** at
  https://github.com/slam0504/go-ddd-core/releases/tag/v0.6.0.

Shipped scope on core (now on `main`):

- `A ports/auth/auth.go` — `Identity` (Subject, TenantID, Roles, Claims),
  `TokenVerifier` + `TokenVerifierFunc`, sentinels `ErrTokenMissing` /
  `ErrTokenInvalid` / `ErrTokenExpired` (all `CodeUnauthorized` → 401,
  tamper-proof via an auth-private `tokenError` wrapper whose `Unwrap()` mints a
  fresh `*errorsx.Error` per call — `errors.As` callers get a throwaway copy,
  `errors.Is` still matches the sentinel), `WithIdentity` /
  `IdentityFromContext` with slice/map clone isolation.
- `A ports/auth/auth_test.go` — 9 contract tests incl. mandatory
  `httpx.Translate` → 401, sentinel tamper-proofing (`errors.As` mutation does
  not corrupt the 401 mapping), slice/map mutation isolation, and
  direct-implementation cases.
- `M CHANGELOG.md` — `[0.6.0] - 2026-06-05` section with cycle narrative.
- `M README.md` — `ports/` parenthetical + `auth/` sub-row `[v0.6.0]`.

Design rationale recorded in `.agent/decisions.md` "AuthN Contract (v0.6.0)".

Out of scope (deferred to v0.6.x): authorization (role/permission checks,
403) and token issuance.

The `v0.6.0` tag is on proxy.golang.org once `go get` fetches it. Treat it as
a permanent published version.

Cross-repo close — **DONE 2026-06-05** (synced from the adapter session via
read-only `gh`): `go-ddd-adapters` PR #24 (`chore(release): bump go-ddd-core to
v0.6.0`) merged `1b0f3ae`; adapters `go.mod` now pins `go-ddd-core v0.6.0`
(was the `core@main` pseudo-version used by the PR #23 consumer). Adapters
`v0.6.0` tag (`a9d4bfb` → `1b0f3ae`) pushed and GitHub Release published as
**Latest** 2026-06-05 04:17Z. Same 2-step cross-repo close used for v0.5.0;
both repos now on matching `v0.6.0` tags.

## v0.3.0 Release Cycle: CLOSED

- core PR #3 merged at `d9c8e5c` (2026-05-15T06:24:25Z)
- core v0.3.0 tagged (annotated `4e58a65` → `d9c8e5c`); GitHub Release
  published as Latest: https://github.com/slam0504/go-ddd-core/releases/tag/v0.3.0
- adapters PR #5 (`chore(deps): bump go-ddd-core to v0.3.0`) merged at
  `ab92ea3` (2026-05-15T07:10:18Z); adapters main now on v0.3.0.

## Current Branch / Heads

- core `main` head: `3729add` `Merge pull request #15 from slam0504/release/v0.7.0-prep`
  (= the `v0.7.0` tag target; this `chore/record-v070-shipped` bookkeeping commit
  advances `main` by one merge on top — the unavoidable ±1 self-reference lag)
- core working branch: `chore/record-v070-shipped` (record v0.7.0 tag shipped)
- core latest tag: `v0.7.0` at `3729add` (annotated tag object `c4a4dc1`); prior `v0.6.0` at `86b1e15` (object `fd596cd`); `v0.5.0` at `e2ee2bb` (object `543cbf3`)
- adapters `main` head: `1b0f3ae` `Merge pull request #24 from slam0504/chore/bump-core-v0.6.0`
  (dep-bump to `go-ddd-core v0.6.0`; consumer landed earlier in PR #23 `ae76f78`)
- adapters latest tag: `v0.6.0` at `a9d4bfb` (→ `1b0f3ae`); prior `v0.5.0` at `45274dd` (object `a02f6d4`)
- Merged feature branches deleted from origin and locally:
  - `release/v0.3.0-prep` (core)
  - `release/v0.3.0-bump` (adapters)
  - `release/v0.4.0-prep` (core, deleted 2026-05-21 post-tag)
  - `docs/roadmap` (core, deleted 2026-05-21 post-merge)
  - `feat/ports-health` (core, deleted 2026-05-21 post-merge)
  - `release/v0.5.0-prep` (core, deleted 2026-05-25 post-tag)
  - `feat/ports-auth` (core, deleted 2026-06-04 post-merge of PR #8)
  - `release/v0.6.0-prep` (core, deleted 2026-06-05 post-tag)

## Worktree

- `.bat-worktrees/fc48c152`
  is on `bat/worktree-fc48c152`, fast-forwarded to `d9c8e5c` (origin/main).
  Ready for the next work cycle.

## v0.5.0 Release Cycle: CLOSED

Release shipped 2026-05-25. Core feature merged 2026-05-21 (PR #6 at
`dce1154`); release-prep PR #7 (`release/v0.5.0-prep` → `main`) merged
at `e2ee2bb`; **v0.5.0** annotated tag pushed (tag object `543cbf3` →
merge commit `e2ee2bb`); GitHub Release published as **Latest** at
https://github.com/slam0504/go-ddd-core/releases/tag/v0.5.0.
Branch `release/v0.5.0-prep` deleted local + remote.

Cycle scope matched `docs/roadmap.md` "Planned cycles" → v0.5.0
exactly (no decision drift). Split:

- **core** — new `ports/health` package: `Check` interface +
  `NewCheck(name, fn)` helper. Registries, aggregation policy, and
  HTTP handler shape deliberately stay out of core.
- **adapters** — `transport/http/stdlib` package (Go 1.22+ ServeMux
  method-pattern routing) with graceful shutdown wired against
  `bootstrap.Lifecycle`, plus a `health` sub-package providing
  `/healthz` + `/readyz` handlers and an in-adapter registry that
  composes per-driver `health.Check` values. Shipped on
  `go-ddd-adapters` main as PR #21 at `d9c7324` (merged 2026-05-25,
  CI 5/5 green, workflow `26404116334`).

Shipped scope on core:

- `A ports/health/health.go` — `Check` interface + `NewCheck` helper + unexported `checkFunc` (PR #6).
- `A ports/health/health_test.go` — 4 contract tests, incl. direct-implementation case (PR #6).
- `M CHANGELOG.md` — `[0.5.0] - 2026-05-25` section with `### Added` ports/health entry + cycle narrative (PR #7).
- `M README.md` — Layout: `ports/` row now lists health; new `health/` sub-row marked `[v0.5.0]`. Drive-by removal of the stale `inbox/  Default in-memory Inbox implementation [v0.2.0]` sub-row that PR #4 (v0.4.0) missed (PR #6).

Tag-gate (per the adapter agent's 4-step memory) — all four steps closed:

1. ✅ adapter implementation merged on adapters `main` (`d9c7324`,
   2026-05-25).
2. ✅ core annotates `v0.5.0` at `e2ee2bb` (tag object `543cbf3`),
   tag pushed to origin (2026-05-25).
3. ✅ adapter dep-bump PR #22 (`core@main` pseudo-version → tagged
   `v0.5.0`) merged at `45274dd` 2026-05-26 00:03:52 Asia/Taipei;
   CI 5/5 on workflow `26409070511`.
4. ✅ adapters `v0.5.0` annotated tag (object `a02f6d4` → `45274dd`)
   pushed; GitHub Release published as Latest 2026-05-26 00:08:32
   Asia/Taipei at
   https://github.com/slam0504/go-ddd-adapters/releases/tag/v0.5.0.

Verification on core `main` @ `e2ee2bb`:

- `gofmt -l .` clean
- `go vet ./...` clean
- `go build ./...` clean
- `go test ./...` PASS (all packages)

The `v0.5.0` tag is on proxy.golang.org once `go get` fetches it.
Treat it as a permanent published version.

## v0.4.0 Release Cycle: CLOSED

Release shipped 2026-05-21. PR #4 (`release/v0.4.0-prep` →
`main`) merged at `aadde89`; v0.4.0 annotated tag pushed
(`aadde89`); GitHub Release published as **Latest** at
https://github.com/slam0504/go-ddd-core/releases/tag/v0.4.0.
Branch `release/v0.4.0-prep` deleted local + remote.

Shipped scope:

- `D eventbus/inbox/memory.go` (was 116 lines)
- `D eventbus/inbox/memory_test.go` (was 129 lines)
- `M CHANGELOG.md` — `[0.4.0] - 2026-05-20` section with
  `### Removed (BREAKING)` entry and migration snippet
- `M docs/anti-patterns.md` — Substitute section now points at
  adapters unambiguously; previous "Note on
  `eventbus/inbox/memory.go`" callout rewritten as past-tense
  "Historical note: the v0.4.0 `eventbus/inbox` removal"
- `A .agent/`, `A AGENTS.md`, `A CLAUDE.md` (in a separate
  `chore(agent)` commit on `main` ahead of the release PR) —
  agent protocol files brought into version control with
  paths normalised to repo-relative

Verification on `main` @ `aadde89`:

- `gofmt -l .` clean
- `go vet ./...` clean
- `go build ./...` clean
- `go test ./...` PASS (all packages, including the `eventbus`
  contract tests — the `Inbox` interface in `eventbus/inbox.go`
  was never under the deleted sub-package)

The `v0.4.0` tag is on proxy.golang.org once `go get` fetches
it. Treat it as a permanent published version.

Call sites stay identical — only the import path changes:

- before: `github.com/slam0504/go-ddd-core/eventbus/inbox`
- after:  `github.com/slam0504/go-ddd-adapters/eventbus/inbox`

Secondary items (accumulate in core `CHANGELOG.md` `[Unreleased]`):

- TBD; revisit `docs/anti-patterns.md` "Design boundaries" section to
  see if any documented gap has shifted to "should be addressed".

## Open Items

- ~~**v0.4.0 deletion gating**~~ ✅ resolved 2026-05-20 (both
  gates closed) and shipped 2026-05-21:
  1. Downstream consumer migration — `consumer inventory = []`
     (personal repo, no external consumers).
  2. Adapters past v0.3.0 — `v0.4.0` annotated tag `bc9b041`
     burned the "one more release cycle" overlap window.

  The same gating framework still applies if this repo ever
  gains external consumers — record any new consumer in
  cross-repo memory before a future breaking removal.

- ~~**Cross-repo memory file missing**~~ ✅ resolved 2026-05-25 via
  option (b): dropped the dangling references in `AGENTS.md` and
  `CLAUDE.md`. Rationale: the parent dir is not a git repo, so any
  shared markdown there has no version control, no review, and forces
  every checkout to recreate a non-repo layout. Cross-repo
  coordination with `go-ddd-adapters` now flows exclusively through
  each repo's own `.agent/state.md`, synchronised by the operator
  across sessions. If a future scale-up needs a versioned shared
  memory, create a dedicated `go-ddd-meta` repo rather than dropping
  loose files in the parent.

## Verification (post-cycle)

- core `main` @ `e2ee2bb` (= `v0.5.0` tag target): `go test ./...`
  PASS 2026-05-25 (full repo, all testable packages); `gofmt -l .`,
  `go vet ./...`, `go build ./...` also clean. GitHub Actions CI run
  `26406409023` green on PR #7 prior to merge.
- core `main` @ `dce1154`: `go test ./...` PASS 2026-05-25 (full repo,
  all 21 testable packages); `gofmt -l .`, `go vet ./...`,
  `go build ./...` also clean.
- core `main` @ `aadde89`: `go test ./...` last passed 2026-05-21
  pre-tag (post-merge tip); `gofmt -l .`, `go vet ./...`,
  `go build ./...` also clean.
- adapters `main` @ `d9c7324` (= v0.5.0 adapter implementation):
  CI 5/5 green on workflow `26404116334` per the adapter agent's
  memory; local verification not re-run from this repo.
- adapters `main` @ `d438c0a`: `go test ./...` last passed during
  the v0.4.0 cycle (both modules, plus `go vet -tags=integration`).
