// Package contextx provides context helpers for carrying cross-cutting
// identifiers (trace, correlation, tenant, user) that are used by middleware
// and logging across the stack.
package contextx

import "context"

type ctxKey int

const (
	keyTraceID ctxKey = iota
	keyRequestID
	keyCorrelationID
	keyCausationID
	keyTenantID
	keyUserID
)

// WithTraceID returns a copy of ctx carrying traceID.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, keyTraceID, traceID)
}

// TraceID extracts the trace ID, or "" if absent.
func TraceID(ctx context.Context) string { return stringValue(ctx, keyTraceID) }

// WithRequestID returns a copy of ctx carrying requestID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID extracts the request ID, or "" if absent.
func RequestID(ctx context.Context) string { return stringValue(ctx, keyRequestID) }

// WithCorrelationID sets a correlation id spanning a business workflow.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyCorrelationID, id)
}

// CorrelationID extracts the correlation id, or "" if absent.
func CorrelationID(ctx context.Context) string { return stringValue(ctx, keyCorrelationID) }

// WithCausationID sets the id of the message that caused the current action.
func WithCausationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyCausationID, id)
}

// CausationID extracts the causation id, or "" if absent.
func CausationID(ctx context.Context) string { return stringValue(ctx, keyCausationID) }

// WithTenantID returns a copy of ctx carrying tenantID.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, keyTenantID, tenantID)
}

// TenantID extracts the tenant id, or "" if absent.
func TenantID(ctx context.Context) string { return stringValue(ctx, keyTenantID) }

// WithUserID returns a copy of ctx carrying userID.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, keyUserID, userID)
}

// UserID extracts the user id, or "" if absent.
func UserID(ctx context.Context) string { return stringValue(ctx, keyUserID) }

func stringValue(ctx context.Context, k ctxKey) string {
	if v, ok := ctx.Value(k).(string); ok {
		return v
	}
	return ""
}
