# go-ddd-core Review Log

Last verified: 2026-05-15 Asia/Taipei

## Recent Findings

- `command/query.Register` pointer type parameters could register under the
  Go type name while `Dispatch(&T{})` used the explicit method name.
  Status: fixed by `b97822c`.

## codex Review of release/v0.3.0-prep (2026-05-15)

Two rounds of findings on `docs/anti-patterns.md` during PR #3 prep.
All addressed in `9a538b8`.

Round 1:

- `txm.Within(...)` typo in producer example. The actual API is
  `ports/database.TxManager.WithinTx`, and use cases shouldn't depend
  on `TxManager` at all per the v0.3.0 UoW decision. Rewrote to
  `uow.Do(ctx, func(ctx context.Context) error { ... })`.
- "go-ddd-core is a contracts only package" wording was too absolute
  given `config.ViperProvider`, `command.InMemoryBus`,
  `eventbus/inbox/memory.go`. Rewrote as
  "infrastructure-client-free" with an explicit list of the in-process
  defaults that do ship.

Round 2:

- Consumer example claimed an idempotent `Record` (e.g.
  `INSERT ... ON CONFLICT DO NOTHING`) handled concurrent-redelivery
  races. Wrong: `Inbox.Record` returns only `error`, and
  `Memory.Record` silently no-ops on duplicate, so two workers can
  both pass `Seen`, run the handler, then both `Record` returns nil
  while side effects already duplicated. Rewrote the comment to state
  the truth: Inbox is best-effort dedup, handlers must be idempotent.
- Two `err := uow.Do(...)` declarations in the same `go` code fence —
  compile error (`no new variables on left side of :=`). Split into
  separate ```go fences and used scoped
  `if err := uow.Do(...); err != nil { ... }`. Verified by feeding
  both snippets through `gofmt` stdin parser.

## Current Open Review Items

- None recorded. Re-run review after PR #3 merge, tagging, or adapters
  dependency bump.
