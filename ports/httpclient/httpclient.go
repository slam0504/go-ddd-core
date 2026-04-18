// Package httpclient defines the outbound HTTP client contract. The
// interface matches net/http.Client.Do so adapters can wrap the stdlib
// client, resty, retryablehttp, or a traced variant.
package httpclient

import (
	"context"
	"net/http"
)

// Client sends HTTP requests and returns responses. Implementations must
// honour context cancellation and close the response body is the caller's
// responsibility.
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// ContextualClient is an optional richer interface for adapters that want to
// expose a context-first API (instead of relying on Request.WithContext).
type ContextualClient interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}
