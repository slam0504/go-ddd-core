// Package httpclient defines the outbound HTTP client contract. The Client
// interface matches net/http.Client.Do so adapters can wrap the stdlib
// client, resty, retryablehttp, or a traced variant.
//
// Responsibility boundary: implementations MUST honour the request context
// (req.Context()) so callers can cancel in-flight requests. Closing a
// non-nil resp.Body after a nil error is the CALLER's responsibility,
// exactly as with net/http.Client.Do. Redirect policy, retries, timeouts,
// and instrumentation are adapter policy — this contract does not
// prescribe them.
package httpclient

import (
	"context"
	"net/http"
)

// Client sends HTTP requests and returns responses. The signature and
// semantics are those of net/http.Client.Do: implementations must honour
// req.Context() cancellation; when the returned error is nil the caller
// owns resp.Body and must close it; when the error is non-nil the
// response may be ignored, per stdlib Do semantics.
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// ContextualClient is an optional context-first variant of Client for
// adapters that prefer an explicit ctx parameter over
// req.WithContext(ctx). Do(ctx, req) must behave exactly like
// Client.Do(req.WithContext(ctx)) — the same cancellation and body-
// ownership rules apply. Note that a single concrete type cannot
// implement both Client and ContextualClient (same method name,
// different signatures); adapters typically expose a small wrapper type
// for this interface.
type ContextualClient interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}
