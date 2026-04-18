package domain

import "errors"

// Sentinel domain errors. Infrastructure adapters should wrap these when
// translating technology-specific errors so that application code can
// pattern-match on domain semantics only.
var (
	ErrNotFound            = errors.New("domain: aggregate not found")
	ErrConcurrencyConflict = errors.New("domain: concurrency conflict")
	ErrInvalidArgument     = errors.New("domain: invalid argument")
	ErrRuleViolation       = errors.New("domain: business rule violation")
)

// RuleViolation is a domain-level error that carries a machine-readable code
// alongside the human message. Aggregates return this when invariants fail.
type RuleViolation struct {
	Code    string
	Message string
}

func (e *RuleViolation) Error() string { return e.Code + ": " + e.Message }

func (e *RuleViolation) Is(target error) bool { return target == ErrRuleViolation }

// NewRuleViolation constructs a RuleViolation with the given code and message.
func NewRuleViolation(code, message string) *RuleViolation {
	return &RuleViolation{Code: code, Message: message}
}
