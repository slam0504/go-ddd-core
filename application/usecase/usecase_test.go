package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/application/command"
	"github.com/slam0504/go-ddd-core/application/query"
	"github.com/slam0504/go-ddd-core/application/usecase"
)

type addInput struct{ A, B int }

type addUseCase struct{}

func (addUseCase) Execute(_ context.Context, in addInput) (int, error) {
	return in.A + in.B, nil
}

func TestUseCaseExecute(t *testing.T) {
	var uc usecase.UseCase[addInput, int] = addUseCase{}
	got, err := uc.Execute(context.Background(), addInput{A: 2, B: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestFuncAdaptsToUseCase(t *testing.T) {
	uc := usecase.Func[addInput, int](func(_ context.Context, in addInput) (int, error) {
		return in.A * in.B, nil
	})
	got, err := uc.Execute(context.Background(), addInput{A: 4, B: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 20 {
		t.Fatalf("got %d, want 20", got)
	}
}

func TestFuncPropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	uc := usecase.Func[addInput, int](func(_ context.Context, _ addInput) (int, error) {
		return 0, sentinel
	})
	_, err := uc.Execute(context.Background(), addInput{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestAsCommandHandler_DispatchableOnBus(t *testing.T) {
	bus := command.NewInMemoryBus()
	command.Register[addInput, int](bus, usecase.AsCommandHandler[addInput, int](addUseCase{}))

	res, err := bus.Dispatch(context.Background(), addInput{A: 7, B: 8})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got, _ := res.(int); got != 15 {
		t.Fatalf("got %v, want 15", res)
	}
}

func TestAsQueryHandler_DispatchableOnBus(t *testing.T) {
	bus := query.NewInMemoryBus()
	query.Register[addInput, int](bus, usecase.AsQueryHandler[addInput, int](addUseCase{}))

	res, err := bus.Dispatch(context.Background(), addInput{A: 9, B: 1})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got, _ := res.(int); got != 10 {
		t.Fatalf("got %v, want 10", res)
	}
}
