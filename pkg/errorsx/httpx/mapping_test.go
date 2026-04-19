package httpx_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/pkg/errorsx/httpx"
)

func TestStatusFromCode_KnownCodes(t *testing.T) {
	cases := map[errorsx.Code]int{
		errorsx.CodeInvalidArgument: 400,
		errorsx.CodeUnauthorized:    401,
		errorsx.CodeForbidden:       403,
		errorsx.CodeNotFound:        404,
		errorsx.CodeAlreadyExists:   409,
		errorsx.CodeConflict:        409,
		errorsx.CodeRateLimited:     429,
		errorsx.CodeInternal:        500,
		errorsx.CodeUnknown:         500,
		errorsx.CodeUnavailable:     503,
	}
	for code, want := range cases {
		if got := httpx.StatusFromCode(code); got != want {
			t.Errorf("StatusFromCode(%q)=%d, want %d", code, got, want)
		}
	}
}

func TestStatusFromCode_UnknownFallsTo500(t *testing.T) {
	if got := httpx.StatusFromCode(errorsx.Code("never_registered")); got != http.StatusInternalServerError {
		t.Fatalf("unknown code should map to 500, got %d", got)
	}
}

func TestTranslate_ErrorsxError(t *testing.T) {
	err := errorsx.New(errorsx.CodeNotFound, "user gone").WithDetail("id", "u-1")
	body, status := httpx.Translate(err)
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
	if body.Code != errorsx.CodeNotFound || body.Message != "user gone" {
		t.Fatalf("body = %+v", body)
	}
	if body.Details["id"] != "u-1" {
		t.Fatalf("expected details to round-trip, got %v", body.Details)
	}
}

func TestTranslate_WrappedErrorsxError(t *testing.T) {
	base := errorsx.New(errorsx.CodeForbidden, "no access")
	wrapped := fmt.Errorf("on path /x: %w", base)
	body, status := httpx.Translate(wrapped)
	if status != 403 {
		t.Fatalf("wrapped errorsx should still resolve via errors.As, got %d", status)
	}
	if body.Code != errorsx.CodeForbidden {
		t.Fatalf("body.Code = %s", body.Code)
	}
}

func TestTranslate_RuleViolationLiftsToInvalidArgument(t *testing.T) {
	rv := domain.NewRuleViolation("MIN_AGE", "must be 18+")
	body, status := httpx.Translate(rv)
	if status != 400 {
		t.Fatalf("RuleViolation should map to 400, got %d", status)
	}
	if body.Code != errorsx.CodeInvalidArgument {
		t.Fatalf("body.Code = %s", body.Code)
	}
	if body.Details["rule"] != "MIN_AGE" {
		t.Fatalf("rule code should appear in details, got %v", body.Details)
	}
}

func TestTranslate_PlainErrorIsUnknown500(t *testing.T) {
	body, status := httpx.Translate(errors.New("disk on fire"))
	if status != 500 {
		t.Fatalf("plain error should be 500, got %d", status)
	}
	if body.Code != errorsx.CodeUnknown {
		t.Fatalf("body.Code = %s, want unknown", body.Code)
	}
	if body.Message != "disk on fire" {
		t.Fatalf("body.Message = %q", body.Message)
	}
}

func TestWriteJSONShapeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteJSON(rec, errorsx.New(errorsx.CodeInvalidArgument, "bad"))

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}

	var body httpx.Body
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not valid json: %v", err)
	}
	if body.Code != errorsx.CodeInvalidArgument {
		t.Fatalf("decoded code = %s", body.Code)
	}
}

func TestFromRuleViolation_NilSafe(t *testing.T) {
	got := httpx.FromRuleViolation(nil)
	if got == nil || got.Code != errorsx.CodeInvalidArgument {
		t.Fatalf("nil RV should still return InvalidArgument errorsx, got %+v", got)
	}
}
