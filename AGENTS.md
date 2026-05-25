# go-ddd-core Agent Memory Rules

Durable agent memory for this repo lives entirely under `.agent/`
(state, decisions, review log). Cross-repo coordination with
`go-ddd-adapters` is recorded in each repo's own `.agent/state.md`
and synchronised by the operator across sessions; there is no
shared file outside either repository.

## Project Role

`go-ddd-core` defines stable DDD / Clean Architecture contracts and primitives.
Keep this repo infrastructure-light. Concrete Kafka, SQL, Redis, slog, OTel,
and transport adapter implementations belong in `go-ddd-adapters` or service
repos unless the code is explicitly a contract.

## Required Startup

1. Read `.agent/state.md`.
2. Read `.agent/decisions.md`.
3. Read `.agent/review-log.md`.
4. Run:

   ```sh
   git status --short
   git log --oneline -5
   ```

## Verification

Default verification:

```sh
go test ./...
```

Run targeted tests when touching a package, but run the full command before
release or broad API changes.

## Durable Memory Update

At the end of meaningful work, update:

- `.agent/state.md`
- `.agent/decisions.md` if a design is accepted or changed
- `.agent/review-log.md` if CR findings were added or resolved

For cross-repo changes (release coordination with `go-ddd-adapters`),
mirror the relevant state into the other repo's `.agent/state.md`
during the same session — there is no shared file outside the repos.
