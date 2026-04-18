// Package slogger is a logger.Logger adapter backed by log/slog. It exists
// to keep the example self-contained; other adapters (zap, zerolog) are
// downstream concerns and would live in their own repos.
package slogger

import (
	"context"
	"io"
	"log/slog"

	"github.com/slam0504/go-ddd-core/ports/logger"
)

// Logger is the slog-backed implementation of logger.Logger.
type Logger struct {
	inner *slog.Logger
}

// New constructs a Logger writing JSON to w at the given level.
func New(w io.Writer, level slog.Level) *Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return &Logger{inner: slog.New(h)}
}

// Log emits a record at the mapped slog level.
func (l *Logger) Log(ctx context.Context, level logger.Level, msg string, attrs ...logger.Attr) {
	l.inner.LogAttrs(ctx, slog.Level(level), msg, toSlogAttrs(attrs)...)
}

// With returns a Logger derived with the given attrs.
func (l *Logger) With(attrs ...logger.Attr) logger.Logger {
	sl := l.inner.With(toAnySlice(toSlogAttrs(attrs))...)
	return &Logger{inner: sl}
}

// Enabled reports whether the underlying slog handler would emit this level.
func (l *Logger) Enabled(ctx context.Context, level logger.Level) bool {
	return l.inner.Enabled(ctx, slog.Level(level))
}

func toSlogAttrs(in []logger.Attr) []slog.Attr {
	out := make([]slog.Attr, len(in))
	for i, a := range in {
		out[i] = slog.Any(a.Key, a.Value)
	}
	return out
}

func toAnySlice(in []slog.Attr) []any {
	out := make([]any, len(in))
	for i, a := range in {
		out[i] = a
	}
	return out
}
