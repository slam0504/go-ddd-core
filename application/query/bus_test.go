package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/application/query"
)

type lookup struct {
	Key string
}

func (lookup) QueryName() string { return "lookup" }

type lookupHandler struct {
	data map[string]string
}

func (h lookupHandler) Handle(_ context.Context, q lookup) (string, error) {
	return h.data[q.Key], nil
}

func TestInMemoryBus_DispatchRegistered(t *testing.T) {
	bus := query.NewInMemoryBus()
	query.Register[lookup, string](bus, lookupHandler{data: map[string]string{"a": "alpha"}})

	res, err := bus.Dispatch(context.Background(), lookup{Key: "a"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got, _ := res.(string); got != "alpha" {
		t.Fatalf("result = %v, want alpha", res)
	}
}

func TestInMemoryBus_UnknownReturnsNotFound(t *testing.T) {
	bus := query.NewInMemoryBus()

	_, err := bus.Dispatch(context.Background(), lookup{})
	if !errors.Is(err, query.ErrHandlerNotFound) {
		t.Fatalf("err = %v, want ErrHandlerNotFound", err)
	}
}
