// Package httpx bridges errorsx.Code to HTTP status codes and provides a
// standard JSON error response writer. The mapping is opinionated but
// conservative; services that need different conventions (e.g. 410 Gone in
// place of 404, 422 in place of 400) can build their own writer on top.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/pkg/errorsx"
)

// statusByCode is the default mapping from errorsx.Code to HTTP status code.
// It covers every Code declared in pkg/errorsx; unknown codes fall back to
// 500 Internal Server Error so latent codes never leak as 200.
var statusByCode = map[errorsx.Code]int{
	errorsx.CodeUnknown:         http.StatusInternalServerError,
	errorsx.CodeInvalidArgument: http.StatusBadRequest,
	errorsx.CodeNotFound:        http.StatusNotFound,
	errorsx.CodeAlreadyExists:   http.StatusConflict,
	errorsx.CodeUnauthorized:    http.StatusUnauthorized,
	errorsx.CodeForbidden:       http.StatusForbidden,
	errorsx.CodeConflict:        http.StatusConflict,
	errorsx.CodeRateLimited:     http.StatusTooManyRequests,
	errorsx.CodeInternal:        http.StatusInternalServerError,
	errorsx.CodeUnavailable:     http.StatusServiceUnavailable,
}

// StatusFromCode returns the HTTP status mapped to c, or 500 for unknown
// codes. The mapping is read-only at runtime; services with bespoke conventions
// should compose their own writer rather than mutating this table.
func StatusFromCode(c errorsx.Code) int {
	if s, ok := statusByCode[c]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// Body is the wire shape of the JSON error response. Code is machine-readable;
// Message is human-readable; Details carries structured context attached via
// errorsx.Error.WithDetail.
type Body struct {
	Code    errorsx.Code   `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteJSON serialises err as a Body and writes it with the appropriate HTTP
// status. If err is not an *errorsx.Error, it is reported as
// CodeUnknown / 500 with err.Error() as the message. Domain RuleViolations are
// promoted to InvalidArgument so callers receive 400 instead of 500.
func WriteJSON(w http.ResponseWriter, err error) {
	body, status := Translate(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Translate converts err into the wire body and HTTP status. Exposed
// separately from WriteJSON so callers using a different transport
// (e.g. echo, gin) can reuse the mapping logic.
func Translate(err error) (Body, int) {
	if err == nil {
		return Body{Code: errorsx.CodeUnknown, Message: ""}, http.StatusInternalServerError
	}

	var ex *errorsx.Error
	if errors.As(err, &ex) {
		return Body{Code: ex.Code, Message: ex.Message, Details: ex.Details}, StatusFromCode(ex.Code)
	}

	var rv *domain.RuleViolation
	if errors.As(err, &rv) {
		ex = FromRuleViolation(rv)
		return Body{Code: ex.Code, Message: ex.Message, Details: ex.Details}, StatusFromCode(ex.Code)
	}

	return Body{Code: errorsx.CodeUnknown, Message: err.Error()}, http.StatusInternalServerError
}

// FromRuleViolation lifts a domain RuleViolation into an errorsx.Error coded
// as InvalidArgument. The RuleViolation's Code is preserved as a "rule"
// detail so callers can distinguish business rules without reading the
// message string.
func FromRuleViolation(rv *domain.RuleViolation) *errorsx.Error {
	if rv == nil {
		return errorsx.New(errorsx.CodeInvalidArgument, "")
	}
	return errorsx.New(errorsx.CodeInvalidArgument, rv.Message).
		WithDetail("rule", rv.Code)
}
