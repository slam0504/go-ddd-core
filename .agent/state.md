# go-ddd-core State

Last verified: 2026-05-25 Asia/Taipei (v0.5.0 core half shipped; awaiting adapters)
Source: verified via `git log` on `main` @ `dce1154` (merge of PR #6
feat/ports-health), `git tag v0.4.0` push at `aadde89`, `gh release view
v0.4.0` confirming Latest marker, and local `gofmt -l .` (clean),
`go vet ./...`, `go build ./...`, `go test ./...` (all packages PASS)
against `main` HEAD on 2026-05-25.

## v0.3.0 Release Cycle: CLOSED

- core PR #3 merged at `d9c8e5c` (2026-05-15T06:24:25Z)
- core v0.3.0 tagged (annotated `4e58a65` → `d9c8e5c`); GitHub Release
  published as Latest: https://github.com/slam0504/go-ddd-core/releases/tag/v0.3.0
- adapters PR #5 (`chore(deps): bump go-ddd-core to v0.3.0`) merged at
  `ab92ea3` (2026-05-15T07:10:18Z); adapters main now on v0.3.0.

## Current Branch / Heads

- core `main` head: `dce1154` `Merge pull request #6 from slam0504/feat/ports-health`
- core working branch: *(none — back on `main`)*
- core latest tag: `v0.4.0` at `aadde89` (post-tag bookkeeping,
  roadmap merge, and v0.5.0 core-half merge are ahead but deliberately
  not retagged; v0.5.0 tag waits for the adapters half)
- adapters `main` head: `b9696f6` (PR #18: core dep bumped v0.3.0 → v0.4.0)
- Merged feature branches deleted from origin and locally:
  - `release/v0.3.0-prep` (core)
  - `release/v0.3.0-bump` (adapters)
  - `release/v0.4.0-prep` (core, deleted 2026-05-21 post-tag)
  - `docs/roadmap` (core, deleted 2026-05-21 post-merge)
  - `feat/ports-health` (core, deleted 2026-05-21 post-merge)

## Worktree

- `.bat-worktrees/fc48c152`
  is on `bat/worktree-fc48c152`, fast-forwarded to `d9c8e5c` (origin/main).
  Ready for the next work cycle.

## v0.5.0 Release Cycle: IN PROGRESS — core half SHIPPED

Cycle scope is documented in `docs/roadmap.md` "Planned cycles" → v0.5.0
(implementation matched the plan; no decision drift):

- **core** (shipped 2026-05-21): new `ports/health` package with the
  `Check` interface + `NewCheck(name, fn)` helper. Registries and HTTP
  handlers stay out of core.
- **adapters** (separate cycle, separate repo, **not yet shipped**):
  `transport/http/stdlib` adapter with graceful shutdown + a `health`
  sub-package that wires `/healthz` + `/readyz` against the core contract.

Core half — shipped:

- PR #6 (`feat/ports-health` → `main`) merged at `dce1154` (2026-05-21).
- Branch `feat/ports-health` deleted on origin and locally.
- Delivered files:
  - `A ports/health/health.go` — `Check` interface + `NewCheck` helper + unexported `checkFunc`.
  - `A ports/health/health_test.go` — 4 contract tests (incl. direct-implementation case).
  - `M CHANGELOG.md` — `[Unreleased]` → `### Added` ports/health entry.
  - `M README.md` — Layout: `ports/` row now lists health, new
    `health/` sub-row marked `[v0.5.0]`; drive-by removal of the
    stale `inbox/  Default in-memory Inbox implementation [v0.2.0]`
    sub-row that PR #4 (v0.4.0) missed.

Adapters half — outstanding (in `go-ddd-adapters`):

- `transport/http/stdlib` package (`net/http` ServeMux, method-pattern routing).
- Graceful shutdown wired against `bootstrap.Lifecycle` deadlines.
- `health` sub-package providing `/healthz` + `/readyz` handlers + a
  small in-adapter registry that composes per-driver `health.Check`s.
- On ship: open adapters PR, bump `go-ddd-core` if any further core
  fixups are needed (none expected), then tag **core v0.5.0** and
  cut the adapters companion release.

v0.5.0 does **not** require a tag yet — the cycle finishes only when
the adapters-side ships. Tag at the last piece of the cycle, not the
first.

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

- **Cross-repo memory file missing**: `AGENTS.md` and `CLAUDE.md`
  reference `../AGENTS.md` and `../.agent-memory/go-ddd.md` as the
  shared Claude/Codex protocol + cross-repo coordination notes.
  Verified 2026-05-25 that neither file exists in this checkout
  (`/Users/eason/playground/project/` only holds `arkham-docs-server`,
  `go-ddd-adapters`, `go-ddd-core`). Decide whether to (a) create the
  pair so adapter-coordination state lives outside the per-repo
  `.agent/`, or (b) drop the references from the two protocol docs.
  Until resolved, v0.5.0 adapter-half coordination notes stay in this
  file's v0.5.0 section above.

## Verification (post-cycle)

- core `main` @ `dce1154`: `go test ./...` PASS 2026-05-25 (full repo,
  all 21 testable packages); `gofmt -l .`, `go vet ./...`,
  `go build ./...` also clean.
- core `main` @ `aadde89`: `go test ./...` last passed 2026-05-21
  pre-tag (post-merge tip); `gofmt -l .`, `go vet ./...`,
  `go build ./...` also clean.
- adapters `main` @ `d438c0a`: `go test ./...` last passed during
  the v0.4.0 cycle (both modules, plus `go vet -tags=integration`).
