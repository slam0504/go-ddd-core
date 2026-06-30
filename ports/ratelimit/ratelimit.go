// Package ratelimit defines the inbound request-throttling contract. A Limiter
// answers a single question — "may this key proceed right now?" — and returns
// the decision as data (Result.Allowed), never as an error. It also carries
// advisory quota metadata (limit / remaining / reset) that an HTTP transport
// adapter MAY surface as rate-limit response hints — Retry-After plus whatever
// quota headers the service's HTTP convention uses. Core fixes neither the
// throttling algorithm (token-bucket, sliding-window, GCRA, …), the
// rate/burst/window configuration, nor the storage backend — all live in
// adapters.
//
// Every ctx-taking method requires a non-nil ctx (stdlib convention).
package ratelimit

import (
	"context"
	"time"
)

// UnknownCount marks Limit/Remaining as absent — the limiter cannot honestly
// produce the value (e.g. a token-bucket has no discrete window, or an upstream
// quota exceeds what an int can hold). It is distinct from a real 0. Mirrors
// pkg/pagination.Page.Total's -1 "not computed" sentinel.
const UnknownCount = -1

// Result is the outcome of one Allow call. Allowed is the decision; the rest is
// advisory metadata for response headers (see field docs for each obligation).
type Result struct {
	// Allowed MUST be exact — it is the decision itself.
	Allowed bool
	// RetryAfter MUST be present. When Allowed it MUST be 0. When !Allowed it is
	// a conservative wait hint: it MUST NOT be less than the limiter's known
	// earliest-retry time (under-estimating is a bug), and MAY be larger (a
	// conservative over-estimate is fine — the client just waits longer).
	// Retrying before it elapses carries NO success guarantee; it is NOT a
	// guarantee of denial. Per IETF draft-ietf-httpapi-ratelimit-headers-11,
	// reset/retry timing is a hint (a server MAY alter quota between requests),
	// not a hard "denied until then" promise. Reports on the safe side like jobs
	// Job.ProcessAt's "no earlier than", but without ProcessAt's hard guarantee.
	RetryAfter time.Duration
	// Limit / Remaining / ResetAt are accurate-or-absent advisory metadata:
	// each is either a value the limiter genuinely computed (carrying that
	// algorithm's inherent precision; a distributed limiter's stale value
	// counts) or a defined "absent" sentinel — NEVER a fabricated placeholder.
	// They are advisory-only: a consumer MAY surface them as headers but MUST
	// NOT use them to make its own allow/deny decision (that is Allowed's job;
	// Remaining is TOCTOU-stale the instant it is read). A consumer MUST omit
	// the corresponding header when the value is absent (see HasLimit /
	// HasRemaining / ResetAt.IsZero) — it MUST NOT serialise UnknownCount.
	//
	// Multi-policy projection: a limiter MAY enforce several quota policies at
	// once (IETF draft-11 RateLimit-Policy is a list, e.g. 50/60s AND
	// 1000/3600s). Result carries at most ONE policy's metadata, so the adapter
	// MUST project the policy that BOUND this decision — on a denial, the policy
	// that denied; on an allow, the most-constraining policy it can honestly
	// represent. If no single policy is a faithful representative, the fields
	// MUST be absent rather than a fabricated blend. Surfacing every policy is an
	// adapter-layer concern, outside this single-Result contract.
	Limit     int       // UnknownCount means absent.
	Remaining int       // UnknownCount means absent.
	ResetAt   time.Time // IsZero means absent.
}

// HasLimit reports whether Limit carries a real value (not absent).
func (r Result) HasLimit() bool { return r.Limit >= 0 }

// HasRemaining reports whether Remaining carries a real value (not absent).
func (r Result) HasRemaining() bool { return r.Remaining >= 0 }

// Limiter decides whether a key may proceed. Implementations MUST be safe for
// concurrent use — middleware calls Allow on every inbound request.
type Limiter interface {
	// Allow reports whether key may proceed right now, as Result data. Ordinary
	// quota exhaustion is NOT an error: it MUST return Result{Allowed:false}
	// (with a RetryAfter wait hint), nil. The Limiter NEVER returns
	// errorsx.CodeRateLimited for ordinary denial. Because Allowed==false is data
	// (there is no error value), HTTP middleware SHOULD emit 429 directly from the
	// Allowed==false decision rather than route it through an error-translation
	// pipeline (httpx.Translate takes an error, which this is not); only if a
	// particular transport pipeline requires an error object should it mint a
	// CodeRateLimited error inside that adapter.
	//
	// A non-nil error means the limiter could not reach a decision, in two
	// classes with fixed precedence (same shape as jobs.Enqueuer):
	//   CLASS 1 — validation: an empty key is malformed input (a missing
	//     partition key, not an "anonymous" caller) → errorsx.CodeInvalidArgument;
	//     deterministic, nothing consumed.
	//   CLASS 2 — ctx / backend: a ctx already cancelled or past its deadline at
	//     entry returns the matching ctx error (errors.Is context.Canceled /
	//     context.DeadlineExceeded) with NO backend contact; otherwise a backend
	//     failure is a coded errorsx whose CodeOf is NOT CodeUnknown
	//     (unreachable → CodeUnavailable; unclassifiable → CodeInternal).
	//   Precedence: empty key → pre-cancelled/expired ctx → backend.
	Allow(ctx context.Context, key string) (Result, error)
}
