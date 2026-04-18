package command

import "errors"

var (
	// ErrHandlerNotFound is returned when no handler is registered for a command.
	ErrHandlerNotFound = errors.New("command: handler not found")
	// ErrCommandMismatch is returned when a dispatched command's concrete type
	// does not match the type the registered handler expects.
	ErrCommandMismatch = errors.New("command: command type mismatch")
)
