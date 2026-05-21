# go-ddd-core Agent Memory Rules

Read `../AGENTS.md` first for the shared
Claude/Codex protocol.

## Project Role

`go-ddd-core` defines stable DDD / Clean Architecture contracts and primitives.
Keep this repo infrastructure-light. Concrete Kafka, SQL, Redis, slog, OTel,
and transport adapter implementations belong in `go-ddd-adapters` or service
repos unless the code is explicitly a contract.

## Required Startup

1. Read `.agent/state.md`.
2. Read `.agent/decisions.md`.
3. Read `.agent/review-log.md`.
4. If work affects adapters or release coordination, read
   `../.agent-memory/go-ddd.md`.
5. Run:

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
- `../.agent-memory/go-ddd.md` for
  cross-repo changes
