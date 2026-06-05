package auth

import (
	"context"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
)

// Resource identifies the target of an authorization check.
//
//	ID == ""   collection / type-level target (valid), e.g. Resource{Type: "order"}
//	           means the order collection rather than one instance.
//	Type == "" malformed input — there is no resource kind to authorize against;
//	           implementations should return ErrInvalidAuthorizationRequest.
type Resource struct {
	Type string // resource kind, e.g. "order", "invoice"
	ID   string // instance id; "" addresses the whole Type
}

// Authorizer reports whether caller may perform action on resource. It is the
// AuthZ counterpart to TokenVerifier: the verifier says who the caller is, the
// Authorizer says what they may do. Concrete policy engines (RBAC / ABAC / OPA /
// Casbin) live in adapters; this contract only fixes the call shape and the
// error vocabulary.
//
// Return semantics:
//
//	nil                            allowed.
//	ErrForbidden (incl. %w wrap)   policy evaluated, access denied — maps to 403.
//	ErrInvalidAuthorizationRequest malformed input (empty action, empty
//	                               Resource.Type, or a zero Identity) — maps to
//	                               400. Kept distinct from ErrForbidden so a
//	                               caller bug is not masked as a policy denial.
//	any other error                the decision could not be made (policy engine
//	                               or I/O failure). Return a coded errorsx value
//	                               (e.g. errorsx.New(errorsx.CodeUnavailable, ...)
//	                               for 503); an uncoded error maps to 500.
//
// Implementations must honour ctx cancellation, be safe for concurrent use, and
// must not mutate caller. Allow borrows caller read-only; an implementation that
// retains it beyond the call must Clone it first. A zero Identity is treated as
// malformed input, not as an anonymous principal — callers that mean "anonymous"
// must say so explicitly (e.g. Identity{Subject: "anonymous"}).
type Authorizer interface {
	Allow(ctx context.Context, caller Identity, action string, resource Resource) error
}

// AuthorizerFunc adapts a plain function to the Authorizer interface, mirroring
// TokenVerifierFunc and command.HandlerFunc. It performs no cloning — the
// read-only-borrow contract on Allow applies unchanged.
type AuthorizerFunc func(ctx context.Context, caller Identity, action string, resource Resource) error

func (f AuthorizerFunc) Allow(ctx context.Context, caller Identity, action string, resource Resource) error {
	return f(ctx, caller, action, resource)
}

// Authorization sentinels. Both are tamper-proof through codedError (see
// auth.go) and map through pkg/errorsx/httpx without any per-adapter table:
// ErrForbidden -> 403, ErrInvalidAuthorizationRequest -> 400.
var (
	ErrForbidden                   error = &codedError{code: errorsx.CodeForbidden, msg: "auth: forbidden"}
	ErrInvalidAuthorizationRequest error = &codedError{code: errorsx.CodeInvalidArgument, msg: "auth: invalid authorization request"}
)
