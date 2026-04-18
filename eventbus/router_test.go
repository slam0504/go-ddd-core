package eventbus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	wm "github.com/ThreeDotsLabs/watermill/message"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/eventbus"
)

type fakeSub struct {
	ch chan eventbus.Envelope
}

func (f *fakeSub) Subscribe(_ context.Context, _ string) (<-chan eventbus.Envelope, error) {
	return f.ch, nil
}

func (f *fakeSub) Close() error { close(f.ch); return nil }

type ping struct {
	domain.BaseEvent
	Value int
}

func TestRouter_DispatchesTypedHandler(t *testing.T) {
	sub := &fakeSub{ch: make(chan eventbus.Envelope, 1)}
	r := eventbus.NewRouter(sub)

	gotCh := make(chan int, 1)
	eventbus.Register[ping](r, "ping.fired", eventbus.HandlerFunc[ping](func(_ context.Context, e ping) error {
		gotCh <- e.Value
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, "test") }()

	sub.ch <- eventbus.Envelope{
		Event: ping{BaseEvent: domain.NewBaseEvent("e1", "ping.fired", "a1", "pinger", 1), Value: 42},
		Name:  "ping.fired",
		Raw:   wm.NewMessage("e1", nil),
	}

	select {
	case got := <-gotCh:
		if got != 42 {
			t.Fatalf("handler saw %d, want 42", got)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for handler")
	}

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("router run err: %v", err)
	}
}

func TestRouter_MiddlewareWrapsHandlers(t *testing.T) {
	sub := &fakeSub{ch: make(chan eventbus.Envelope, 1)}
	callsCh := make(chan string, 3)
	mw := func(next eventbus.DispatchFunc) eventbus.DispatchFunc {
		return func(ctx context.Context, e domain.DomainEvent) error {
			callsCh <- "before"
			err := next(ctx, e)
			callsCh <- "after"
			return err
		}
	}
	r := eventbus.NewRouter(sub, mw)
	eventbus.Register[ping](r, "ping.fired", eventbus.HandlerFunc[ping](func(_ context.Context, _ ping) error {
		callsCh <- "handler"
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, "test") }()

	sub.ch <- eventbus.Envelope{
		Event: ping{BaseEvent: domain.NewBaseEvent("e1", "ping.fired", "a1", "pinger", 1)},
		Name:  "ping.fired",
		Raw:   wm.NewMessage("e1", nil),
	}

	want := []string{"before", "handler", "after"}
	got := make([]string, 0, 3)
	for range want {
		select {
		case c := <-callsCh:
			got = append(got, c)
		case <-ctx.Done():
			t.Fatalf("timeout; got so far: %v", got)
		}
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("calls[%d] = %q, want %q (%v)", i, got[i], v, got)
		}
	}
	cancel()
	<-done
}
