// Package observability defines the tracing and metrics contract using
// OpenTelemetry as the standard surface. Adapters wire the Provider with a
// concrete SDK (OTLP, Prometheus exporter, etc.).
package observability

import (
	"context"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Provider returns the OpenTelemetry primitives the core needs. It also
// exposes Shutdown for graceful teardown during application stop.
type Provider interface {
	Tracer(name string, opts ...trace.TracerOption) trace.Tracer
	Meter(name string, opts ...metric.MeterOption) metric.Meter
	TextMapPropagator() propagation.TextMapPropagator
	Shutdown(ctx context.Context) error
}
