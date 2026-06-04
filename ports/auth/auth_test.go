package auth_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/pkg/errorsx/httpx"
	"github.com/slam0504/go-ddd-core/ports/auth"
)

func TestTokenVerifierFunc_ReturnsIdentity(t *testing.T) {
	want := auth.Identity{Subject: "user-1", TenantID: "acme", Roles: []string{"admin"}}
	var v auth.TokenVerifier = auth.TokenVerifierFunc(func(context.Context, string) (auth.Identity, error) {
		return want, nil
	})

	got, err := v.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if got.Subject != want.Subject || got.TenantID != want.TenantID {
		t.Errorf("Verify() = %+v, want %+v", got, want)
	}
}

func TestTokenVerifierFunc_PropagatesError(t *testing.T) {
	v := auth.TokenVerifierFunc(func(context.Context, string) (auth.Identity, error) {
		return auth.Identity{}, auth.ErrTokenExpired
	})

	_, err := v.Verify(context.Background(), "tok")
	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Errorf("Verify() error = %v, want ErrTokenExpired (via errors.Is)", err)
	}
}

func TestTokenVerifierFunc_PassesArgs(t *testing.T) {
	type ctxKey struct{}
	const marker = "marker-value"
	const wantTok = "header.payload.sig"

	v := auth.TokenVerifierFunc(func(ctx context.Context, token string) (auth.Identity, error) {
		if got, _ := ctx.Value(ctxKey{}).(string); got != marker {
			return auth.Identity{}, errors.New("context not propagated")
		}
		if token != wantTok {
			return auth.Identity{}, errors.New("token not propagated")
		}
		return auth.Identity{Subject: "ok"}, nil
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, marker)
	if _, err := v.Verify(ctx, wantTok); err != nil {
		t.Errorf("Verify() did not propagate ctx/token: %v", err)
	}
}

// TestSentinels_MapToHTTP401 is the primary acceptance for this contract: the
// auth sentinels must translate to HTTP 401 through pkg/errorsx/httpx without
// any per-adapter mapping table. Asserting only CodeOf would prove the code but
// not the status, so the httpx.Translate status check is mandatory.
func TestSentinels_MapToHTTP401(t *testing.T) {
	for _, err := range []error{auth.ErrTokenMissing, auth.ErrTokenInvalid, auth.ErrTokenExpired} {
		if got := errorsx.CodeOf(err); got != errorsx.CodeUnauthorized {
			t.Errorf("CodeOf(%v) = %q, want %q", err, got, errorsx.CodeUnauthorized)
		}
		if _, status := httpx.Translate(err); status != http.StatusUnauthorized {
			t.Errorf("Translate(%v) status = %d, want %d", err, status, http.StatusUnauthorized)
		}
	}
}

// TestSentinels_Immutable guards the tamper-proof contract: a caller that
// reaches the coded value via errors.As and mutates its exported fields must
// not corrupt the shared sentinel's code or its 401 mapping. The sentinel's
// Unwrap mints a fresh *errorsx.Error each call, so the extracted value is a
// throwaway copy. This test fails if the sentinels are plain *errorsx.Error.
func TestSentinels_Immutable(t *testing.T) {
	var ex *errorsx.Error
	if !errors.As(auth.ErrTokenMissing, &ex) {
		t.Fatal("ErrTokenMissing does not unwrap to *errorsx.Error")
	}
	ex.Code = errorsx.CodeInternal
	ex.Message = "tampered"

	if got := errorsx.CodeOf(auth.ErrTokenMissing); got != errorsx.CodeUnauthorized {
		t.Errorf("sentinel code mutated to %q, want %q", got, errorsx.CodeUnauthorized)
	}
	if _, status := httpx.Translate(auth.ErrTokenMissing); status != http.StatusUnauthorized {
		t.Errorf("sentinel status mutated to %d, want %d", status, http.StatusUnauthorized)
	}
}

func TestWithIdentity_RoundTrip(t *testing.T) {
	want := auth.Identity{
		Subject:  "user-1",
		TenantID: "acme",
		Roles:    []string{"admin", "billing"},
		Claims:   map[string]any{"scope": "read"},
	}
	ctx := auth.WithIdentity(context.Background(), want)

	got, ok := auth.IdentityFromContext(ctx)
	if !ok {
		t.Fatal("IdentityFromContext() ok = false, want true")
	}
	if got.Subject != want.Subject || got.TenantID != want.TenantID {
		t.Errorf("identity scalars = %+v, want %+v", got, want)
	}
	if !slices.Equal(got.Roles, want.Roles) {
		t.Errorf("Roles = %v, want %v", got.Roles, want.Roles)
	}
	if got.Claims["scope"] != "read" {
		t.Errorf("Claims[scope] = %v, want read", got.Claims["scope"])
	}
}

func TestIdentityFromContext_Absent(t *testing.T) {
	got, ok := auth.IdentityFromContext(context.Background())
	if ok {
		t.Error("IdentityFromContext() ok = true, want false")
	}
	if got.Subject != "" || got.TenantID != "" || got.Roles != nil || got.Claims != nil {
		t.Errorf("IdentityFromContext() = %+v, want zero Identity", got)
	}
}

// TestWithIdentity_IsolatesMutation guards the slice/map isolation contract:
// neither the caller's originals nor a returned copy may mutate the value
// stored in the context.
func TestWithIdentity_IsolatesMutation(t *testing.T) {
	roles := []string{"admin"}
	claims := map[string]any{"scope": "read"}
	ctx := auth.WithIdentity(context.Background(), auth.Identity{
		Subject: "user-1",
		Roles:   roles,
		Claims:  claims,
	})

	// Mutating the caller's originals must not reach the stored identity.
	roles[0] = "tampered"
	claims["scope"] = "tampered"

	got, ok := auth.IdentityFromContext(ctx)
	if !ok {
		t.Fatal("IdentityFromContext() ok = false, want true")
	}
	if got.Roles[0] != "admin" {
		t.Errorf("caller mutation leaked into stored Roles: %v", got.Roles)
	}
	if got.Claims["scope"] != "read" {
		t.Errorf("caller mutation leaked into stored Claims: %v", got.Claims)
	}

	// Mutating a returned copy must not affect a subsequent read.
	got.Roles[0] = "tampered-again"
	got.Claims["scope"] = "tampered-again"

	again, _ := auth.IdentityFromContext(ctx)
	if again.Roles[0] != "admin" {
		t.Errorf("reader mutation leaked into stored Roles: %v", again.Roles)
	}
	if again.Claims["scope"] != "read" {
		t.Errorf("reader mutation leaked into stored Claims: %v", again.Claims)
	}
}

// staticVerifier confirms the contract is open to direct implementation, not
// only the TokenVerifierFunc convenience path.
type staticVerifier struct {
	id  auth.Identity
	err error
}

func (s staticVerifier) Verify(context.Context, string) (auth.Identity, error) {
	return s.id, s.err
}

func TestTokenVerifier_DirectImplementation(t *testing.T) {
	var v auth.TokenVerifier = staticVerifier{id: auth.Identity{Subject: "svc"}}

	got, err := v.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if got.Subject != "svc" {
		t.Errorf("Verify() subject = %q, want svc", got.Subject)
	}
}
