package idempotency_test

import (
	"net/http"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/pkg/errorsx/httpx"
	"github.com/slam0504/go-ddd-core/ports/idempotency"
)

func TestStatus_String(t *testing.T) {
	cases := []struct {
		status idempotency.Status
		want   string
	}{
		{idempotency.StatusUnknown, "unknown"},
		{idempotency.StatusNew, "new"},
		{idempotency.StatusInProgress, "in_progress"},
		{idempotency.StatusCompleted, "completed"},
		{idempotency.StatusMismatch, "mismatch"},
		{idempotency.Status(99), "unknown"}, // out-of-range renders as unknown, never panics
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// TestStatusUnknown_IsZeroValue guards the deliberate choice that the zero value
// is the undefined state, so a bug returning a zero Reservation{} fails closed
// instead of masquerading as a successful new reservation.
func TestStatusUnknown_IsZeroValue(t *testing.T) {
	var s idempotency.Status
	if s != idempotency.StatusUnknown {
		t.Errorf("zero Status = %v, want StatusUnknown", s)
	}
	if got := (idempotency.Reservation{}).Status; got != idempotency.StatusUnknown {
		t.Errorf("zero Reservation.Status = %v, want StatusUnknown", got)
	}
}

// TestErrorCodes_MapToHTTP proves the contract's three coded errors already map
// to the right HTTP status through the existing httpx bridge — the contract adds
// no new mapping. This is the only test that imports httpx; the transport-neutral
// conformance suite asserts errorsx.Code directly.
func TestErrorCodes_MapToHTTP(t *testing.T) {
	cases := []struct {
		code errorsx.Code
		want int
	}{
		{errorsx.CodeInvalidArgument, http.StatusBadRequest},     // 400 — malformed scope/key/fingerprint/reservation
		{errorsx.CodeConflict, http.StatusConflict},              // 409 — stale/expired/terminal reservation
		{errorsx.CodeUnavailable, http.StatusServiceUnavailable}, // 503 — store could not determine state
	}
	for _, tc := range cases {
		if _, status := httpx.Translate(errorsx.New(tc.code, "boom")); status != tc.want {
			t.Errorf("Translate(%s) status = %d, want %d", tc.code, status, tc.want)
		}
	}
}
