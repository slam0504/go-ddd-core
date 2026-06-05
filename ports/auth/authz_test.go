package auth_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/pkg/errorsx/httpx"
	"github.com/slam0504/go-ddd-core/ports/auth"
)

func TestAuthorizerFunc_AllowsAndDenies(t *testing.T) {
	allow := auth.AuthorizerFunc(func(context.Context, auth.Identity, string, auth.Resource) error {
		return nil
	})
	if err := allow.Allow(context.Background(), auth.Identity{Subject: "u"}, "read", auth.Resource{Type: "order"}); err != nil {
		t.Errorf("Allow() = %v, want nil (allowed)", err)
	}

	deny := auth.AuthorizerFunc(func(context.Context, auth.Identity, string, auth.Resource) error {
		return auth.ErrForbidden
	})
	err := deny.Allow(context.Background(), auth.Identity{Subject: "u"}, "delete", auth.Resource{Type: "order", ID: "1"})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("Allow() error = %v, want ErrForbidden (via errors.Is)", err)
	}
}

func TestAuthorizerFunc_PassesArgs(t *testing.T) {
	type ctxKey struct{}
	const marker = "marker-value"
	wantCaller := auth.Identity{Subject: "user-1", TenantID: "acme", Roles: []string{"admin"}}
	wantAction := "export"
	wantResource := auth.Resource{Type: "order", ID: "42"}

	a := auth.AuthorizerFunc(func(ctx context.Context, caller auth.Identity, action string, resource auth.Resource) error {
		if got, _ := ctx.Value(ctxKey{}).(string); got != marker {
			return errors.New("context not propagated")
		}
		if caller.Subject != wantCaller.Subject || caller.TenantID != wantCaller.TenantID {
			return fmt.Errorf("caller not propagated: %+v", caller)
		}
		if action != wantAction {
			return fmt.Errorf("action not propagated: %q", action)
		}
		if resource != wantResource {
			return fmt.Errorf("resource not propagated: %+v", resource)
		}
		return nil
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, marker)
	if err := a.Allow(ctx, wantCaller, wantAction, wantResource); err != nil {
		t.Errorf("Allow() did not propagate args: %v", err)
	}
}

// TestErrForbidden_MapsToHTTP403 is the primary acceptance for this contract:
// ErrForbidden must translate to HTTP 403 through pkg/errorsx/httpx without any
// per-adapter mapping table. Asserting only CodeOf would prove the code but not
// the status, so the httpx.Translate status check is mandatory.
func TestErrForbidden_MapsToHTTP403(t *testing.T) {
	if got := errorsx.CodeOf(auth.ErrForbidden); got != errorsx.CodeForbidden {
		t.Errorf("CodeOf(ErrForbidden) = %q, want %q", got, errorsx.CodeForbidden)
	}
	if _, status := httpx.Translate(auth.ErrForbidden); status != http.StatusForbidden {
		t.Errorf("Translate(ErrForbidden) status = %d, want %d", status, http.StatusForbidden)
	}
}

// TestErrInvalidAuthorizationRequest_MapsToHTTP400 pins the malformed-input
// sentinel to 400, keeping caller bugs distinct from policy denials (403).
func TestErrInvalidAuthorizationRequest_MapsToHTTP400(t *testing.T) {
	if got := errorsx.CodeOf(auth.ErrInvalidAuthorizationRequest); got != errorsx.CodeInvalidArgument {
		t.Errorf("CodeOf(ErrInvalidAuthorizationRequest) = %q, want %q", got, errorsx.CodeInvalidArgument)
	}
	if _, status := httpx.Translate(auth.ErrInvalidAuthorizationRequest); status != http.StatusBadRequest {
		t.Errorf("Translate(ErrInvalidAuthorizationRequest) status = %d, want %d", status, http.StatusBadRequest)
	}
}

// TestAuthZSentinels_Immutable guards the tamper-proof contract for both AuthZ
// sentinels: a caller that reaches the coded value via errors.As and mutates its
// exported fields must not corrupt the shared sentinel's code or status mapping.
func TestAuthZSentinels_Immutable(t *testing.T) {
	cases := []struct {
		name       string
		sentinel   error
		wantCode   errorsx.Code
		wantStatus int
	}{
		{"ErrForbidden", auth.ErrForbidden, errorsx.CodeForbidden, http.StatusForbidden},
		{"ErrInvalidAuthorizationRequest", auth.ErrInvalidAuthorizationRequest, errorsx.CodeInvalidArgument, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ex *errorsx.Error
			if !errors.As(tc.sentinel, &ex) {
				t.Fatalf("%s does not unwrap to *errorsx.Error", tc.name)
			}
			ex.Code = errorsx.CodeInternal
			ex.Message = "tampered"

			if got := errorsx.CodeOf(tc.sentinel); got != tc.wantCode {
				t.Errorf("sentinel code mutated to %q, want %q", got, tc.wantCode)
			}
			if _, status := httpx.Translate(tc.sentinel); status != tc.wantStatus {
				t.Errorf("sentinel status mutated to %d, want %d", status, tc.wantStatus)
			}
		})
	}
}

// TestErrForbidden_WrappedStillMaps proves an adapter may add context with %w
// without breaking either the errors.Is match or the 403 mapping — the common
// case of a policy adapter annotating the denial.
func TestErrForbidden_WrappedStillMaps(t *testing.T) {
	wrapped := fmt.Errorf("rbac: subject lacks role: %w", auth.ErrForbidden)

	if !errors.Is(wrapped, auth.ErrForbidden) {
		t.Errorf("errors.Is(wrapped, ErrForbidden) = false, want true")
	}
	if _, status := httpx.Translate(wrapped); status != http.StatusForbidden {
		t.Errorf("Translate(wrapped) status = %d, want %d", status, http.StatusForbidden)
	}
}

// staticAuthorizer confirms the contract is open to direct implementation, not
// only the AuthorizerFunc convenience path.
type staticAuthorizer struct {
	err error
}

func (s staticAuthorizer) Allow(context.Context, auth.Identity, string, auth.Resource) error {
	return s.err
}

func TestAuthorizer_DirectImplementation(t *testing.T) {
	var a auth.Authorizer = staticAuthorizer{err: auth.ErrForbidden}

	err := a.Allow(context.Background(), auth.Identity{Subject: "svc"}, "read", auth.Resource{Type: "order"})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Errorf("Allow() error = %v, want ErrForbidden", err)
	}
}
