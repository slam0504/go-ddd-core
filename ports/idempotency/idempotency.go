// Package idempotency defines the contract for inbound request idempotency:
// guarding a non-idempotent operation (typically an HTTP POST) against
// duplicate execution when a client retries with the same idempotency key.
//
// It is distinct from eventbus.Inbox, which deduplicates broker-delivered
// domain events per consumer (InboxKey{Consumer, EventID}). This contract
// guards inbound request retries, keyed by an adapter-built scope (isolating at
// least tenant and operation/endpoint) plus a client-supplied idempotency key
// plus a request fingerprint, and tracks reservation ownership via a lease.
//
// Core ships only the Store contract. Enforcement middleware, key/fingerprint
// extraction, response (de)serialization, and concrete backings (in-memory,
// Redis, SQL) live in adapters. TTL/retention, atomic-reservation mechanics,
// lease validation, and cross-node consistency are the adapter's responsibility.
//
// The lease token guards against a stale handler overwriting a newer
// reservation after the key is reused, but it cannot prevent a stale handler
// that has already committed an external side effect. The Store therefore makes
// no exactly-once guarantee for side effects: callers should keep their work
// within Reservation.LeaseTTL (or abandon it), and the operation itself or a
// downstream system must also be idempotent. LeaseTTL is an advisory budget —
// roughly how much processing time the reservation has left — to be tracked by
// the caller's own monotonic clock; whether a Finish still applies is ultimately
// decided by the Store validating the lease token, not by the caller comparing
// clocks against the backend. An adapter MUST eventually reclaim an unfinished
// in-progress reservation (via a bounded lease or equivalent) so the same
// (scope, key) can be reserved again — otherwise a handler crash, dropped
// connection, or ambiguous Cancel would block the key forever. This guarantees
// key liveness only, NOT exactly-once side effects: the handler or a downstream
// system must still be idempotent. The exact reclaim delay and precision, and
// completed-record retention, are the adapter's policy (its own TTL, cleanup
// strategy, and clock) and are therefore not exercised by the generic
// conformance suite — see ports/idempotency/idempotencytest.
//
// Finish and Cancel read only Scope, Key, and LeaseToken from the passed-in
// Reservation (the lookup keys plus the ownership credential); a caller cannot
// extend its lease or forge ownership by mutating the returned struct's
// LeaseTTL, Status, or Response — the Store validates against its own stored
// state.
package idempotency

import (
	"context"
	"time"
)

// Status is the outcome of a Begin lookup — data, not an error. A non-nil error
// from Begin means the state could not be determined (malformed input, store
// unavailable, serialization failure, timeout), which is distinct from any
// Status.
type Status int

const (
	// StatusUnknown is the zero value: an undefined state that should never be
	// returned alongside a nil error. Callers must fail-closed (never execute)
	// so an implementation bug returning a zero Reservation{} is not mistaken
	// for a successful new reservation.
	StatusUnknown Status = iota
	// StatusNew means the (scope, key) was not seen and is now reserved for this
	// caller. LeaseToken is set (LeaseTTL is an advisory budget); execute the
	// operation then Finish, or Cancel only if no observable side effect was
	// committed.
	StatusNew
	// StatusInProgress means another in-flight request holds the (scope, key).
	// Do not execute (HTTP 409).
	StatusInProgress
	// StatusCompleted means the (scope, key) already finished. Replay
	// Reservation.Response without re-executing.
	StatusCompleted
	// StatusMismatch means the (scope, key) exists but the fingerprint differs
	// (the key was reused for a different request). Do not execute and do not
	// replay (HTTP 409).
	StatusMismatch
)

// String renders s for logs and tests. Unknown or out-of-range values render as
// "unknown".
func (s Status) String() string {
	switch s {
	case StatusNew:
		return "new"
	case StatusInProgress:
		return "in_progress"
	case StatusCompleted:
		return "completed"
	case StatusMismatch:
		return "mismatch"
	default:
		return "unknown"
	}
}

// Reservation is the handle Begin returns for a (scope, key). LeaseToken
// identifies the lease and is populated only when Status == StatusNew;
// Finish/Cancel pass the Reservation back so the Store can validate ownership.
// LeaseTTL (also meaningful only when Status == StatusNew) is an advisory
// processing budget: > 0 means the caller has roughly that much time left
// (tracked by its own monotonic clock), <= 0 means the adapter exposes no
// observable budget — it does NOT mean the reservation never expires (the
// adapter still reclaims it eventually). LeaseTTL does not bind the Store —
// expiry timing is the adapter's policy and Finish/Cancel are authorized only by
// the lease token. Response carries the stored payload, populated only when
// Status == StatusCompleted; it is an opaque, complete, replayable encoding the
// consumer produced (for HTTP: status code, the headers needed to replay
// faithfully, and body — not the body alone). The Store stores and returns these
// bytes verbatim and never parses or alters them.
type Reservation struct {
	Scope      string
	Key        string
	LeaseToken string        // opaque lease; set when Status == StatusNew. Core does not interpret it.
	LeaseTTL   time.Duration // advisory budget; meaningful only when Status == StatusNew (<= 0 = none, NOT never-expires).
	Status     Status
	Response   []byte // opaque complete replay encoding; set when Status == StatusCompleted; a copy the caller may freely mutate.
}

// Store coordinates inbound-request idempotency. Begin's reservation must be
// atomic: two concurrent calls with the same (scope, key) must not both observe
// StatusNew. An implementation must also guarantee liveness — an unfinished
// in-progress reservation is eventually reclaimable so the (scope, key) can be
// reserved again (the reclaim delay/precision is the adapter's policy).
// Implementations must be safe for concurrent use and honour ctx.
type Store interface {
	// Begin atomically reserves (scope, key) for a new execution or reports the
	// existing state. scope is a trusted, adapter-built isolation prefix
	// (isolating at least tenant and operation/endpoint) — it is NOT taken from
	// the client; key is the raw client-supplied idempotency key. The same key
	// under a different scope is a fully independent record; fingerprint does not
	// substitute for scope (an identical payload could still replay across
	// tenants). fingerprint is an opaque caller-derived value identifying the
	// request payload; a stored (scope, key) whose fingerprint differs yields
	// StatusMismatch (whether in-progress or completed). On StatusNew the
	// returned Reservation carries a fresh LeaseToken and an advisory LeaseTTL. A
	// non-nil error means the state could not be determined — an empty scope,
	// key, or fingerprint is malformed input (return errorsx.CodeInvalidArgument
	// → 400); other failures should return a coded errorsx value (e.g.
	// CodeUnavailable → 503).
	//
	// Begin is also the recovery path after an INDETERMINATE Finish/Cancel: a
	// re-issued Begin returns the authoritative Status — StatusCompleted means a
	// prior Finish actually landed (replay Response), StatusInProgress means it
	// is still held, StatusNew means a prior Cancel succeeded or the reservation
	// was reclaimed (reprocess), StatusMismatch follows the mismatch policy.
	Begin(ctx context.Context, scope, key, fingerprint string) (Reservation, error)

	// Finish stores response as the completed outcome, but only if reservation's
	// LeaseToken still owns the current in-progress reservation. The Store must
	// copy response (the caller may reuse its buffer). The error has THREE
	// distinct meanings — do not collapse them:
	//
	//	nil                  → applied, for certain.
	//	errorsx.CodeConflict → NOT applied, for certain (stale/expired token, not
	//	                       the current reservation, or already completed — a
	//	                       decided rejection). → 409.
	//	any other error      → INDETERMINATE: the write may or may not have landed
	//	                       (e.g. a remote store committed then the ACK was lost
	//	                       to a timeout/ctx/I/O error). The caller MUST NOT
	//	                       assume "not applied"; recover by calling Begin again
	//	                       (same scope/key/fingerprint) to read the
	//	                       authoritative state (see Begin).
	Finish(ctx context.Context, reservation Reservation, response []byte) error

	// Cancel releases an in-progress reservation owned by reservation's
	// LeaseToken so the key may be retried. Cancel must NOT delete an
	// already-completed payload. Callers must invoke Cancel only when sure the
	// operation committed no observable side effect; on an ambiguous failure
	// leave the reservation standing (let the adapter reclaim it) to avoid a
	// retry re-executing a partially-applied operation. The error carries the
	// same three meanings as Finish: nil = released for certain;
	// errorsx.CodeConflict = not released, for certain (stale/expired/terminal,
	// → 409); any other error = INDETERMINATE, recover via Begin.
	Cancel(ctx context.Context, reservation Reservation) error
}
