package bootstrap_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/bootstrap"
)

func TestLifecycle_StartAndStopOrder(t *testing.T) {
	var order []string
	lc := &bootstrap.Lifecycle{}

	lc.Append(
		bootstrap.Hook{Name: "a", Run: func(_ context.Context) error { order = append(order, "start:a"); return nil }},
		bootstrap.Hook{Name: "a", Run: func(_ context.Context) error { order = append(order, "stop:a"); return nil }},
	)
	lc.Append(
		bootstrap.Hook{Name: "b", Run: func(_ context.Context) error { order = append(order, "start:b"); return nil }},
		bootstrap.Hook{Name: "b", Run: func(_ context.Context) error { order = append(order, "stop:b"); return nil }},
	)

	if err := lc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := lc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	want := []string{"start:a", "start:b", "stop:b", "stop:a"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i, v := range want {
		if order[i] != v {
			t.Fatalf("order[%d] = %q, want %q (%v)", i, order[i], v, order)
		}
	}
}

func TestLifecycle_StartStopsOnFirstError(t *testing.T) {
	var calls int
	lc := &bootstrap.Lifecycle{}
	wantErr := errors.New("boom")

	lc.Append(bootstrap.Hook{Name: "ok", Run: func(_ context.Context) error { calls++; return nil }}, bootstrap.Hook{})
	lc.Append(bootstrap.Hook{Name: "bad", Run: func(_ context.Context) error { calls++; return wantErr }}, bootstrap.Hook{})
	lc.Append(bootstrap.Hook{Name: "never", Run: func(_ context.Context) error { calls++; return nil }}, bootstrap.Hook{})

	if err := lc.Start(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Start err = %v, want %v", err, wantErr)
	}
	if calls != 2 {
		t.Fatalf("hook calls = %d, want 2", calls)
	}
}

func TestLifecycle_StopAccumulatesErrors(t *testing.T) {
	lc := &bootstrap.Lifecycle{}
	e1 := errors.New("e1")
	e2 := errors.New("e2")

	lc.Append(bootstrap.Hook{}, bootstrap.Hook{Name: "x", Run: func(_ context.Context) error { return e1 }})
	lc.Append(bootstrap.Hook{}, bootstrap.Hook{Name: "y", Run: func(_ context.Context) error { return e2 }})

	err := lc.Stop(context.Background())
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Fatalf("Stop err = %v, want both e1 and e2", err)
	}
}
