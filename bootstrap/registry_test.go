package bootstrap_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/bootstrap"
)

func TestAdapterRegistry_RegisterResolveBuild(t *testing.T) {
	r := bootstrap.NewAdapterRegistry()

	r.Register("cache", "memory", func(_ context.Context, cfg any) (any, error) {
		return "memory:" + cfg.(string), nil
	})

	f, err := r.Resolve("cache", "memory")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if f == nil {
		t.Fatal("factory nil")
	}

	v, err := r.Build(context.Background(), "cache", "memory", "ns1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v != "memory:ns1" {
		t.Fatalf("Build = %v", v)
	}
}

func TestAdapterRegistry_ResolveUnknown(t *testing.T) {
	r := bootstrap.NewAdapterRegistry()

	_, err := r.Resolve("cache", "missing")
	if !errors.Is(err, bootstrap.ErrAdapterNotFound) {
		t.Fatalf("err = %v, want ErrAdapterNotFound", err)
	}

	_, err = r.Build(context.Background(), "cache", "missing", nil)
	if !errors.Is(err, bootstrap.ErrAdapterNotFound) {
		t.Fatalf("Build err = %v, want ErrAdapterNotFound", err)
	}
}
