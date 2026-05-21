# go-ddd-core State

Last verified: 2026-05-20 Asia/Taipei (post core deletion commit on `release/v0.4.0-prep`)
Source: verified via `git log`, `git tag`, cross-repo inspection of
`go-ddd-adapters` `main` (HEAD `d438c0a`) + tags (`v0.4.0` at
`bc9b041`) + `.agent/state.md`, the 2026-05-20 gating-status
update recorded below, and local execution of `gofmt -l .`,
`go vet`, `go build`, `go test ./...` against branch tip `ef0e535`.

## v0.3.0 Release Cycle: CLOSED

- core PR #3 merged at `d9c8e5c` (2026-05-15T06:24:25Z)
- core v0.3.0 tagged (annotated `4e58a65` → `d9c8e5c`); GitHub Release
  published as Latest: https://github.com/slam0504/go-ddd-core/releases/tag/v0.3.0
- adapters PR #5 (`chore(deps): bump go-ddd-core to v0.3.0`) merged at
  `ab92ea3` (2026-05-15T07:10:18Z); adapters main now on v0.3.0.

## Current Branch / Heads

- core `main` head: `d9c8e5c`
- adapters `main` head: `ab92ea3`
- Merged feature branches deleted from origin and locally:
  - `release/v0.3.0-prep` (core)
  - `release/v0.3.0-bump` (adapters)

## Worktree

- `.bat-worktrees/fc48c152`
  is on `bat/worktree-fc48c152`, fast-forwarded to `d9c8e5c` (origin/main).
  Ready for the next work cycle.

## v0.4.0 Release Cycle: IN PROGRESS

**Deletion executed locally 2026-05-20.** Branch
`release/v0.4.0-prep` at commit `ef0e535` (off `main` at
`d0fe9cd`) carries:

- `D eventbus/inbox/memory.go` (was 116 lines)
- `D eventbus/inbox/memory_test.go` (was 129 lines)
- `M CHANGELOG.md` — new `[0.4.0] - 2026-05-20` section with
  `### Removed (BREAKING)` entry and migration snippet
- `M docs/anti-patterns.md` — Substitute section now points at
  adapters unambiguously; "Note on `eventbus/inbox/memory.go`"
  renamed to "Historical note: the v0.4.0 `eventbus/inbox` removal"
  and rewritten in past-tense

Verification on the branch tip:

- `gofmt -l .` clean
- `go vet ./...` clean
- `go build ./...` clean
- `go test ./...` PASS (all packages, including the `eventbus`
  contract tests — the `Inbox` interface in `eventbus/inbox.go`
  was never under the deleted sub-package)

**Awaiting (all reversible until push, increasingly hard to reverse
after each step):**

1. `git push origin main` to publish the two docs sync commits
   (`c942cd0`, `d0fe9cd`) on which this branch is based.
2. `git push -u origin release/v0.4.0-prep` to publish the branch.
3. Open release PR → `main`.
4. Merge PR (after any final review).
5. Cut annotated tag `v0.4.0` on the merge commit.
6. Publish GitHub Release marked Latest.
7. Branch cleanup (`release/v0.4.0-prep` local + remote).

Steps 1–4 are conventionally reversible (revert merge, rewrite
branch); steps 5–6 are not (tag deletion is technically possible
but Go's module proxy caches the version permanently — once
`v0.4.0` is on proxy.golang.org, downstream `go get` calls can
fetch it forever).

**Adapters-side status (verified 2026-05-20):** the relocation target
has been shipped **and the overlap window is closed**.

- `go-ddd-adapters v0.3.0` (annotated tag at `3ce2a23`, 2026-05-19)
  first relocated `Memory` to `eventbus/inbox` with `WithTTL` +
  `WithClock` options. CHANGELOG promised "Core retains its copy ...
  for one more release cycle".
- `go-ddd-adapters v0.4.0` (annotated tag at `bc9b041`, 2026-05-20)
  shipped the pgx Outbox + pgx TxManager and bumped CI to Go 1.25.
  PRs #14–#17 merged; cycle branches cleaned up; `main` HEAD
  `d438c0a`. **This advance satisfies the "one more release cycle"
  promise** — adapters has moved past v0.3.0, so removing the core
  copy no longer breaks any published guarantee.

Call sites stay identical — only the import path changes:

- before: `github.com/slam0504/go-ddd-core/eventbus/inbox`
- after:  `github.com/slam0504/go-ddd-adapters/eventbus/inbox`

Secondary items (accumulate in core `CHANGELOG.md` `[Unreleased]`):

- TBD; revisit `docs/anti-patterns.md` "Design boundaries" section to
  see if any documented gap has shifted to "should be addressed".

## Open Items

- **v0.4.0 deletion gating** — physically removing
  `eventbus/inbox/memory.go` was originally gated on **two**
  conditions; **only (a) remains open** as of 2026-05-20:
  1. ~~**Downstream services have migrated their import path** from
     `go-ddd-core/eventbus/inbox` to `go-ddd-adapters/eventbus/inbox`~~
     — **SATISFIED 2026-05-20** by user-supplied formal answer:
     `consumer inventory = []` (personal repo, no external
     consumers). Recorded as a discipline-preserving closure rather
     than a process shortcut: the same gating framework will apply
     unchanged if this repo ever gains external consumers, but for
     the current state the inventory legitimately resolves empty.
  2. ~~`go-ddd-adapters` has cut its next release after v0.3.0~~ —
     **SATISFIED 2026-05-20** by `v0.4.0` annotated tag `bc9b041`
     (pgx Outbox + pgx TxManager cycle). The "one more release
     cycle" overlap window in adapters CHANGELOG has now been
     burned; removing the core copy no longer breaks the published
     guarantee.

  Net: **both gates closed 2026-05-20**. Core v0.4.0 deletion is
  unblocked. The cross-repo dependency that previously coupled
  core's release cadence to adapters' release cadence is now
  resolved, and the empty downstream inventory was recorded as a
  formal answer (not bypassed).

## Verification (post-cycle)

- core `main` @ `d9c8e5c`: `go test ./...` last passed during PR #3
  local verification.
- adapters `main` @ `ab92ea3`: `go test ./...` last passed during PR #5
  local verification (both modules, plus `go vet -tags=integration`).
