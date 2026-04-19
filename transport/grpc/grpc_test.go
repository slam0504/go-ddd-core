package grpc_test

import (
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/transport/grpc"
)

type fakeIP struct {
	unary, stream []any
}

func (f fakeIP) UnaryInterceptors() []any  { return f.unary }
func (f fakeIP) StreamInterceptors() []any { return f.stream }

func TestCombineInterceptorProviders_PreservesOrder(t *testing.T) {
	a := fakeIP{unary: []any{"a1", "a2"}, stream: []any{"as1"}}
	b := fakeIP{unary: []any{"b1"}, stream: []any{"bs1", "bs2"}}

	unary, stream := grpc.CombineInterceptorProviders(a, b)
	if len(unary) != 3 || unary[0] != "a1" || unary[2] != "b1" {
		t.Fatalf("unary order wrong: %v", unary)
	}
	if len(stream) != 3 || stream[0] != "as1" || stream[2] != "bs2" {
		t.Fatalf("stream order wrong: %v", stream)
	}
}

func TestCombineInterceptorProviders_SkipsNil(t *testing.T) {
	unary, stream := grpc.CombineInterceptorProviders(nil, fakeIP{unary: []any{"x"}}, nil)
	if len(unary) != 1 || unary[0] != "x" {
		t.Fatalf("unary should drop nil providers, got %v", unary)
	}
	if len(stream) != 0 {
		t.Fatalf("stream should be empty, got %v", stream)
	}
}

type fakeSP struct{ tag string }

func (fakeSP) RegisterService(_ any, _ any) {}

func TestCollectServiceProviders_DropsNil(t *testing.T) {
	a, b := fakeSP{tag: "a"}, fakeSP{tag: "b"}
	got := grpc.CollectServiceProviders(a, nil, b)
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
}

func (fakeSP) RegisterServices(_ grpc.ServiceRegistrar) {}

func TestCodeFromErrorsx_KnownCodes(t *testing.T) {
	cases := map[errorsx.Code]uint32{
		errorsx.CodeUnknown:         grpc.StatusUnknown,
		errorsx.CodeInvalidArgument: grpc.StatusInvalidArgument,
		errorsx.CodeNotFound:        grpc.StatusNotFound,
		errorsx.CodeAlreadyExists:   grpc.StatusAlreadyExists,
		errorsx.CodeUnauthorized:    grpc.StatusUnauthenticated,
		errorsx.CodeForbidden:       grpc.StatusPermissionDenied,
		errorsx.CodeConflict:        grpc.StatusAborted,
		errorsx.CodeRateLimited:     grpc.StatusResourceExhausted,
		errorsx.CodeInternal:        grpc.StatusInternal,
		errorsx.CodeUnavailable:     grpc.StatusUnavailable,
	}
	for in, want := range cases {
		if got := grpc.CodeFromErrorsx(in); got != want {
			t.Errorf("CodeFromErrorsx(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestCodeFromErrorsx_UnknownFallsToUnknown(t *testing.T) {
	if got := grpc.CodeFromErrorsx(errorsx.Code("never_registered")); got != grpc.StatusUnknown {
		t.Fatalf("unknown code should map to StatusUnknown, got %d", got)
	}
}
