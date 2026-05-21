# go-ddd-core State

Last verified: 2026-05-21 Asia/Taipei (v0.5.0 cycle open on branch `feat/ports-health`)
Source: verified via `git log` on `main` @ `1bd0a8c` (merge of PR #5
docs/roadmap), `git tag v0.4.0` push at `aadde89`, `gh release view
v0.4.0` confirming Latest marker, and local `gofmt -l ports/health/`
(clean), `go vet ./ports/health/...`, `go test ./ports/health/... -race`
(4 tests PASS) against branch tip on `feat/ports-health`.

## v0.3.0 Release Cycle: CLOSED

- core PR #3 merged at `d9c8e5c` (2026-05-15T06:24:25Z)
- core v0.3.0 tagged (annotated `4e58a65` → `d9c8e5c`); GitHub Release
  published as Latest: https://github.com/slam0504/go-ddd-core/releases/tag/v0.3.0
- adapters PR #5 (`chore(deps): bump go-ddd-core to v0.3.0`) merged at
  `ab92ea3` (2026-05-15T07:10:18Z); adapters main now on v0.3.0.

## Current Branch / Heads

- core `main` head: `1bd0a8c` `Merge pull request #5 from slam0504/docs/roadmap`
- core working branch: `feat/ports-health` (v0.5.0 cycle, off `main`)
- core latest tag: `v0.4.0` at `aadde89` (post-tag bookkeeping +
  roadmap merge are ahead but deliberately not retagged)
- adapters `main` head: `b9696f6` (PR #18: core dep bumped v0.3.0 → v0.4.0)
- Merged feature branches deleted from origin and locally:
  - `release/v0.3.0-prep` (core)
  - `release/v0.3.0-bump` (adapters)
  - `release/v0.4.0-prep` (core, deleted 2026-05-21 post-tag)
  - `docs/roadmap` (core, deleted 2026-05-21 post-merge)

## Worktree

- `.bat-worktrees/fc48c152`
  is on `bat/worktree-fc48c152`, fast-forwarded to `d9c8e5c` (origin/main).
  Ready for the next work cycle.

## v0.5.0 Release Cycle: OPEN

Started 2026-05-21 on branch `feat/ports-health`. Cycle scope is
documented in `docs/roadmap.md` "Planned cycles" → v0.5.0:

- **core** (this branch): new `ports/health` package with the
  `Check` interface + `NewCheck(name, fn)` helper. Registries
  and HTTP handlers stay out of core.
- **adapters** (separate cycle, separate repo): `transport/http/stdlib`
  adapter with graceful shutdown + a `health` sub-package that
  wires `/healthz` + `/readyz` against the core contract.

Branch tip status:

- `A ports/health/health.go` (Check interface + NewCheck helper + unexported checkFunc)
- `A ports/health/health_test.go` (4 contract tests, including direct-implementation case)
- `M CHANGELOG.md` `[Unreleased]` → `### Added` ports/health entry
- `M README.md` Layout section: ports/ row updated to include health,
  new health/ sub-row marked `[v0.5.0]`. Same commit removes the
  stale `inbox/  Default in-memory Inbox implementation [v0.2.0]`
  line that v0.4.0's PR #4 missed (the sub-package was deleted then
  but the README sub-row remained).

Verification on branch tip:

- `gofmt -l ports/health/` clean
- `go vet ./ports/health/...` clean
- `go test ./ports/health/... -race` PASS (4 tests, 1.8s)

Next steps on this branch:

1. Full repo `gofmt -l .` + `go vet ./...` + `go build ./...` +
   `go test ./...` pass before push.
2. Open PR `feat/ports-health` → `main`.
3. Wait for CI green (golangci-lint + build/test).
4. Merge (merge commit), delete branch local + remote.
5. Bookkeeping commit: update `.agent/state.md` to "v0.5.0 core
   half: SHIPPED, waiting on adapters", update `docs/roadmap.md`
   if any cycle decisions shifted during implementation.

v0.5.0 does **not** require a tag yet — the cycle finishes only
when the adapters-side `transport/http/stdlib` ships. Tag at the
last piece of the cycle, not the first.

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

## Verification (post-cycle)

- core `main` @ `aadde89`: `go test ./...` last passed 2026-05-21
  pre-tag (post-merge tip); `gofmt -l .`, `go vet ./...`,
  `go build ./...` also clean.
- adapters `main` @ `d438c0a`: `go test ./...` last passed during
  the v0.4.0 cycle (both modules, plus `go vet -tags=integration`).
