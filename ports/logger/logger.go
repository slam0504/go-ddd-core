// Package logger defines the logging contract used across the core. The
// interface is intentionally small and slog-compatible so adapters can wrap
// any backend (slog, zap, zerolog, etc.).
package logger

import "context"

// Level represents logging severity.
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// Attr is a structured logging field.
type Attr struct {
	Key   string
	Value any
}

// F is a shorthand constructor for Attr.
func F(key string, value any) Attr { return Attr{Key: key, Value: value} }

// Logger emits structured, contextual log records. Implementations must be
// safe for concurrent use.
type Logger interface {
	Log(ctx context.Context, level Level, msg string, attrs ...Attr)
	With(attrs ...Attr) Logger
	Enabled(ctx context.Context, level Level) bool
}

// Convenience wrappers — not part of the interface so adapters don't need to
// implement them.

func Debug(ctx context.Context, l Logger, msg string, attrs ...Attr) {
	l.Log(ctx, LevelDebug, msg, attrs...)
}
func Info(ctx context.Context, l Logger, msg string, attrs ...Attr) {
	l.Log(ctx, LevelInfo, msg, attrs...)
}
func Warn(ctx context.Context, l Logger, msg string, attrs ...Attr) {
	l.Log(ctx, LevelWarn, msg, attrs...)
}
func Error(ctx context.Context, l Logger, msg string, attrs ...Attr) {
	l.Log(ctx, LevelError, msg, attrs...)
}
