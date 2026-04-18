package query

import "errors"

var (
	ErrHandlerNotFound = errors.New("query: handler not found")
	ErrQueryMismatch   = errors.New("query: query type mismatch")
)
