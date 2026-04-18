package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/application/command"
)

type addCmd struct {
	A, B int
}

func (addCmd) CommandName() string { return "add" }

type addHandler struct{}

func (addHandler) Handle(_ context.Context, c addCmd) (int, error) {
	return c.A + c.B, nil
}

func TestInMemoryBus_DispatchRegistered(t *testing.T) {
	bus := command.NewInMemoryBus()
	command.Register[addCmd, int](bus, addHandler{})

	res, err := bus.Dispatch(context.Background(), addCmd{A: 2, B: 3})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if got, _ := res.(int); got != 5 {
		t.Fatalf("result = %v, want 5", res)
	}
}

func TestInMemoryBus_UnknownReturnsNotFound(t *testing.T) {
	bus := command.NewInMemoryBus()

	_, err := bus.Dispatch(context.Background(), addCmd{})
	if !errors.Is(err, command.ErrHandlerNotFound) {
		t.Fatalf("err = %v, want ErrHandlerNotFound", err)
	}
}

func TestInMemoryBus_MiddlewareWrapsHandler(t *testing.T) {
	var calls []string
	mw := func(next command.DispatchFunc) command.DispatchFunc {
		return func(ctx context.Context, c command.Command) (any, error) {
			calls = append(calls, "before:"+c.CommandName())
			res, err := next(ctx, c)
			calls = append(calls, "after")
			return res, err
		}
	}
	bus := command.NewInMemoryBus(mw)
	command.Register[addCmd, int](bus, addHandler{})

	if _, err := bus.Dispatch(context.Background(), addCmd{A: 1, B: 1}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(calls) != 2 || calls[0] != "before:add" || calls[1] != "after" {
		t.Fatalf("middleware order wrong: %v", calls)
	}
}

func TestChain_OrderIsOutermostFirst(t *testing.T) {
	var order []string
	mkMW := func(tag string) command.Middleware {
		return func(next command.DispatchFunc) command.DispatchFunc {
			return func(ctx context.Context, c command.Command) (any, error) {
				order = append(order, "enter:"+tag)
				res, err := next(ctx, c)
				order = append(order, "exit:"+tag)
				return res, err
			}
		}
	}
	fn := command.Apply(
		func(_ context.Context, _ command.Command) (any, error) {
			order = append(order, "handler")
			return nil, nil
		},
		mkMW("A"), mkMW("B"),
	)

	_, _ = fn(context.Background(), addCmd{})

	want := []string{"enter:A", "enter:B", "handler", "exit:B", "exit:A"}
	if len(order) != len(want) {
		t.Fatalf("order length = %d, want %d (%v)", len(order), len(want), order)
	}
	for i, v := range want {
		if order[i] != v {
			t.Fatalf("order[%d] = %q, want %q (%v)", i, order[i], v, order)
		}
	}
}
