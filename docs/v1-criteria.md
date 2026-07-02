# v1.0.0 Stability Criteria

This document defines **when v1.0.0 becomes a conversation** — the entry
criteria, not a schedule. There is no target date, and this is deliberately
not a roadmap: `docs/roadmap.md` records what we invest in next; this page
records what must already be true before a v1 tag is even proposed.

## What v1 means here

For a Go module, `v1.0.0` is a compatibility promise on the import path:
after it, any breaking change to an exported identifier requires a `/v2`
module path. Every exported contract in `go-ddd-core` — ports, conformance
suites, `pkg/` helpers, bootstrap — becomes frozen surface. So the bar is
not "the contracts are designed"; it is "the contracts have stopped
moving for reasons we can demonstrate, not assert".

## Entry criteria

All of the following, evaluated against live state (`.agent/state.md`,
`.agent/decisions.md`, the adapters repository) at proposal time:

1. **Adapter-proven shape.** Every A-quadrant port is proven by at least
   one production adapter in `go-ddd-adapters`; the stateful / high-churn
   contracts (`jobs`, `idempotency`, `ratelimit`, `cache`, `storage`,
   `httpclient`) by one to two independent adapters — or a recorded,
   deliberate acceptance of single-adapter risk for that port. A contract
   whose only consumer is its own reference implementation does not count.

2. **Conformance coverage of high-risk contracts.** Each stateful/timing
   contract ships an exported conformance suite (`idempotencytest`,
   `jobstest` incl. the delivery half, `ratelimittest`, plus suites for
   `cache`/`storage` once their first adapters land), and the production
   adapters actually run those suites in their CI.

3. **Known breaking candidates: zero.** No open contract-shape issues in
   `decisions.md` / review logs. Deferred *additive* items (e.g. an opt-in
   recoverability suite behind a second jobs adapter) do not block; a
   deferred item whose resolution might change an existing signature does.

4. **Deprecation windows closed.** Nothing exported is mid-deprecation;
   anything scheduled for removal has been removed in a prior minor.

5. **Process stability.** The release pipeline (release-prep PR → annotated
   tag → GitHub Release → record) and README/CHANGELOG discipline have run
   unchanged across several consecutive releases without ad-hoc repair.

6. **Soak.** A defined quiet period after the last shape-affecting change —
   during which adapters and downstream services keep building against the
   current minor without surfacing new contract-shape findings. Additive
   releases during the soak do not reset it; shape changes do.

## Non-criteria

Deliberately **not** required for v1: covering every quadrant-B adapter,
new contracts beyond the A-quadrant set, feature completeness of
`examples/`, or any calendar milestone. Scope completeness was the v0.x
goal; v1 is purely a stability claim.
