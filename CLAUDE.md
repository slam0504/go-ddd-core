# Claude Notes for go-ddd-core

Read `AGENTS.md` first. It defines the startup protocol and points at
the repo-local `.agent/` memory files (`state.md`, `decisions.md`,
`review-log.md`).

For cross-repo work with `go-ddd-adapters`, sync state through each
repo's own `.agent/state.md`; there is no shared file outside the
repos.
