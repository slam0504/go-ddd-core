# go-ddd-core State

Last verified: 2026-05-25 Asia/Taipei (v0.5.0 SHIPPED — Latest release)
Source: verified via `git log` on `main` @ `e2ee2bb` (merge of PR #7
release/v0.5.0-prep), `git tag v0.5.0` push at `e2ee2bb` (tag object
`543cbf3`), `gh release view v0.5.0` confirming Latest marker, and
local `gofmt -l .` (clean), `go vet ./...`, `go build ./...`,
`go test ./...` (all packages PASS) against `main` HEAD on 2026-05-25.

## v0.3.0 Release Cycle: CLOSED

- core PR #3 merged at `d9c8e5c` (2026-05-15T06:24:25Z)
- core v0.3.0 tagged (annotated `4e58a65` → `d9c8e5c`); GitHub Release
  published as Latest: https://github.com/slam0504/go-ddd-core/releases/tag/v0.3.0
- adapters PR #5 (`chore(deps): bump go-ddd-core to v0.3.0`) merged at
  `ab92ea3` (2026-05-15T07:10:18Z); adapters main now on v0.3.0.

## Current Branch / Heads

- core `main` head: `e2ee2bb` `Merge pull request #7 from slam0504/release/v0.5.0-prep`
- core working branch: *(none — back on `main`)*
- core latest tag: `v0.5.0` at `e2ee2bb` (annotated tag object `543cbf3`)
- adapters `main` head: `d9c7324` (PR #21: transport/http/stdlib + health,
  v0.5.0 adapter cycle; pinned to `core@main` pseudo-version pending
  the tiny follow-up dep-bump PR to `core v0.5.0`)
- Merged feature branches deleted from origin and locally:
  - `release/v0.3.0-prep` (core)
  - `release/v0.3.0-bump` (adapters)
  - `release/v0.4.0-prep` (core, deleted 2026-05-21 post-tag)
  - `docs/roadmap` (core, deleted 2026-05-21 post-merge)
  - `feat/ports-health` (core, deleted 2026-05-21 post-merge)
  - `release/v0.5.0-prep` (core, deleted 2026-05-25 post-tag)

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

Tag-gate (per the adapter agent's 4-step memory):

1. ✅ adapter implementation merged on adapters `main` (`d9c7324`).
2. ✅ core annotates `v0.5.0` at `e2ee2bb`, tag pushed to origin.
3. ⏳ tiny follow-up PR on adapters: replace the `core@main`
   pseudo-version in `go.mod` with the tagged `v0.5.0` and re-run CI.
4. ⏳ adapters companion release / Latest marker once step 3 lands.

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
