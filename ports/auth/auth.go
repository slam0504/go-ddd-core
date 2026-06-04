// Package auth defines the authentication contract: who the caller is
// (Identity) and how a bearer token is verified (TokenVerifier). It contains
// no infrastructure — JWT/JWKS, opaque-token introspection, and session lookup
// live in adapters selected by downstream services. Authorization
// (role/permission checks, 403) is intentionally out of scope here.
package auth

import (
	"context"
	"maps"
	"slices"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
)

// Identity is the verified principal extracted from a token. It is a plain data
// carrier: AuthZ decisions (role/permission checks) belong to a separate
// contract and are intentionally absent here.
//
// Roles and Claims are reference types; WithIdentity and IdentityFromContext
// shallow-clone them so a value stored in a context cannot be mutated through
// an alias held by the caller or another reader. Nested values inside Claims
// are not deep-copied and must be treated as immutable by all parties.
type Identity struct {
	Subject  string         // stable principal id (JWT "sub", session user id, ...)
	TenantID string         // owning tenant; "" when single-tenant
	Roles    []string       // coarse role labels carried for downstream AuthZ
	Claims   map[string]any // raw verifier-specific claims (issuer, exp, scopes, ...)
}

// clone returns a copy of id with Roles and Claims shallow-cloned. slices.Clone
// and maps.Clone preserve nil, so a zero Identity round-trips unchanged.
func (id Identity) clone() Identity {
	id.Roles = slices.Clone(id.Roles)
	id.Claims = maps.Clone(id.Claims)
	return id
}

// TokenVerifier turns an opaque bearer token into a verified Identity.
// Implementations must honour ctx cancellation (network calls to JWKS /
// introspection endpoints) and must be safe for concurrent use.
//
// On failure they should return one of the package sentinels so transport
// adapters map cleanly to HTTP 401: ErrTokenInvalid (malformed / bad
// signature), ErrTokenExpired (well-formed but past its lifetime), or
// ErrTokenMissing. Transport middleware may return ErrTokenMissing itself
// before ever calling Verify (e.g. no Authorization header); a verifier
// invoked with an empty token should likewise return ErrTokenMissing so the
// behaviour stays consistent across adapters.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (Identity, error)
}

// TokenVerifierFunc adapts a plain function to the TokenVerifier interface,
// mirroring command.HandlerFunc and the eventbus handler-func adapters. The
// name is qualified ("Token") so a future authorization verifier can take the
// plain VerifierFunc spelling without colliding with this one.
type TokenVerifierFunc func(ctx context.Context, token string) (Identity, error)

func (f TokenVerifierFunc) Verify(ctx context.Context, token string) (Identity, error) {
	return f(ctx, token)
}

// tokenError is an immutable, errorsx-coded sentinel. It holds only an
// unexported message and exposes the CodeUnauthorized surface through Unwrap,
// which mints a fresh *errorsx.Error on every call. A caller reaching the coded
// value via errorsx.CodeOf / httpx.Translate (both errors.As) therefore mutates
// only a throwaway copy — the shared sentinel and its 401 mapping cannot be
// corrupted. Identity comparison still works via errors.Is because that matches
// the sentinel pointer itself before ever unwrapping.
type tokenError struct {
	msg string
}

func (e *tokenError) Error() string {
	return string(errorsx.CodeUnauthorized) + ": " + e.msg
}

func (e *tokenError) Unwrap() error {
	return errorsx.New(errorsx.CodeUnauthorized, e.msg)
}

// Token verification sentinels. All three map to HTTP 401 through
// pkg/errorsx/httpx; see tokenError for why they are tamper-proof.
var (
	ErrTokenMissing error = &tokenError{msg: "auth: token missing"}
	ErrTokenInvalid error = &tokenError{msg: "auth: token invalid"}
	ErrTokenExpired error = &tokenError{msg: "auth: token expired"}
)

// ctxKey is unexported so an Identity stored here cannot collide with another
// package's context keys; mirrors pkg/contextx's convention.
type ctxKey int

const keyIdentity ctxKey = iota

// WithIdentity returns a child context carrying a clone of the verified
// Identity. Transport middleware calls this after a successful Verify so
// handlers downstream can read the principal via IdentityFromContext.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, keyIdentity, id.clone())
}

// IdentityFromContext returns a clone of the Identity placed by WithIdentity.
// The bool is false when no identity is present — needed because a zero
// Identity{} is otherwise indistinguishable from an unauthenticated context.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(keyIdentity).(Identity)
	if !ok {
		return Identity{}, false
	}
	return id.clone(), true
}
