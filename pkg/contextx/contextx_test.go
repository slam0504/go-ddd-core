package contextx_test

import (
	"context"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/contextx"
)

func TestContextxRoundTrip(t *testing.T) {
	ctx := context.Background()
	ctx = contextx.WithTraceID(ctx, "t1")
	ctx = contextx.WithRequestID(ctx, "r1")
	ctx = contextx.WithCorrelationID(ctx, "co1")
	ctx = contextx.WithCausationID(ctx, "ca1")
	ctx = contextx.WithTenantID(ctx, "tn1")
	ctx = contextx.WithUserID(ctx, "u1")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"trace", contextx.TraceID(ctx), "t1"},
		{"request", contextx.RequestID(ctx), "r1"},
		{"correlation", contextx.CorrelationID(ctx), "co1"},
		{"causation", contextx.CausationID(ctx), "ca1"},
		{"tenant", contextx.TenantID(ctx), "tn1"},
		{"user", contextx.UserID(ctx), "u1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestContextx_MissingReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	if got := contextx.TraceID(ctx); got != "" {
		t.Fatalf("TraceID on empty ctx = %q, want empty", got)
	}
}
