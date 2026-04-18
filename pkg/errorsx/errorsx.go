// Package errorsx adds a coded error type on top of the stdlib errors
// package so application and transport layers can translate domain failures
// into stable machine-readable responses.
package errorsx

import (
	"errors"
	"fmt"
	"maps"
)

// Code is a stable identifier for an error category. Transport adapters map
// these to HTTP status codes, gRPC codes, or GraphQL error extensions.
type Code string

const (
	CodeUnknown         Code = "unknown"
	CodeInvalidArgument Code = "invalid_argument"
	CodeNotFound        Code = "not_found"
	CodeAlreadyExists   Code = "already_exists"
	CodeUnauthorized    Code = "unauthorized"
	CodeForbidden       Code = "forbidden"
	CodeConflict        Code = "conflict"
	CodeRateLimited     Code = "rate_limited"
	CodeInternal        Code = "internal"
	CodeUnavailable     Code = "unavailable"
)

// Error is a coded error with optional cause and structured details.
type Error struct {
	Code    Code
	Message string
	Details map[string]any
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause so errors.Is / errors.As continue to work.
func (e *Error) Unwrap() error { return e.cause }

// New builds an Error without a cause.
func New(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

// Wrap attaches a coded surface to an existing error.
func Wrap(code Code, msg string, cause error) *Error {
	return &Error{Code: code, Message: msg, cause: cause}
}

// WithDetail returns a shallow copy of e with the given detail attached.
// The Details map is cloned so callers sharing a base *Error (for example a
// package-level sentinel) do not pollute each other's details.
func (e *Error) WithDetail(key string, value any) *Error {
	cp := *e
	if e.Details != nil {
		cp.Details = maps.Clone(e.Details)
	} else {
		cp.Details = map[string]any{}
	}
	cp.Details[key] = value
	return &cp
}

// CodeOf returns the Code of err or CodeUnknown if err is not an *Error.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeUnknown
}
