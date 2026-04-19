# gRPC Integration Guide

`go-ddd-core` provides gRPC **contracts and helpers**, not a server. The
`google.golang.org/grpc` package is intentionally not a core dependency so
services can pick the gRPC version (and any complementary packages — health,
reflection, gateway, mTLS) that suit their environment.

This document explains how the helpers in `transport/grpc` glue gRPC
adapters to the rest of the core (`pkg/errorsx`, `bootstrap`).

## Why no Server in core?

The same reasoning as `transport/http` and `transport/graphql`: server
construction couples to a specific library version, observability stack, and
authentication strategy. By keeping core abstract, services can compose their
own server in a few dozen lines using their preferred deps.

## Helpers at a glance

| Helper                                | Purpose                                       | Source                                |
|---------------------------------------|-----------------------------------------------|---------------------------------------|
| `ServiceProvider`                     | Modules contribute gRPC services              | `transport/grpc/grpc.go` (v0.1.0)     |
| `InterceptorProvider`                 | Modules contribute unary / stream interceptors | `transport/grpc/grpc.go` (v0.1.0)     |
| `CombineInterceptorProviders`         | Flatten interceptors from many providers      | `transport/grpc/interceptor.go` (v0.2.0) |
| `CollectServiceProviders`             | Drop nil providers, preserve order            | `transport/grpc/interceptor.go` (v0.2.0) |
| `Status*` constants                   | gRPC status codes as `uint32` (no grpc dep)   | `transport/grpc/errorx.go` (v0.2.0)   |
| `CodeFromErrorsx(errorsx.Code)`       | `errorsx.Code` → gRPC status code             | `transport/grpc/errorx.go` (v0.2.0)   |

## Standard wiring

### 1. Build your adapter Server

A typical adapter `Server` wraps `*grpc.Server` and satisfies
`transport/grpc.Server`:

```go
package grpcadapter

import (
    "context"
    "net"

    "google.golang.org/grpc"

    txgrpc "github.com/slam0504/go-ddd-core/transport/grpc"
)

type Server struct {
    addr   string
    server *grpc.Server
    lis    net.Listener
}

func New(addr string, providers ...txgrpc.InterceptorProvider) *Server {
    unary, stream := txgrpc.CombineInterceptorProviders(providers...)
    s := grpc.NewServer(
        grpc.ChainUnaryInterceptor(castUnary(unary)...),
        grpc.ChainStreamInterceptor(castStream(stream)...),
    )
    return &Server{addr: addr, server: s}
}

// txgrpc.ServiceRegistrar — adapters cast to grpc.ServiceDesc on register.
func (s *Server) RegisterService(desc any, impl any) {
    s.server.RegisterService(desc.(*grpc.ServiceDesc), impl)
}

func (s *Server) Start(ctx context.Context) error {
    lis, err := net.Listen("tcp", s.addr)
    if err != nil {
        return err
    }
    s.lis = lis
    go func() { _ = s.server.Serve(lis) }()
    return nil
}

func (s *Server) Stop(ctx context.Context) error {
    done := make(chan struct{})
    go func() { s.server.GracefulStop(); close(done) }()
    select {
    case <-done:
        return nil
    case <-ctx.Done():
        s.server.Stop()
        return ctx.Err()
    }
}

func (s *Server) Addr() string { return s.lis.Addr().String() }

func castUnary(in []any) []grpc.UnaryServerInterceptor {
    out := make([]grpc.UnaryServerInterceptor, len(in))
    for i, v := range in {
        out[i] = v.(grpc.UnaryServerInterceptor)
    }
    return out
}

func castStream(in []any) []grpc.StreamServerInterceptor {
    out := make([]grpc.StreamServerInterceptor, len(in))
    for i, v := range in {
        out[i] = v.(grpc.StreamServerInterceptor)
    }
    return out
}
```

The cast is constrained to one place — modules contributing interceptors
supply real `grpc.UnaryServerInterceptor` / `grpc.StreamServerInterceptor`
values stored as `any`.

### 2. Map errorsx → gRPC status

In your error-translation interceptor:

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

    "github.com/slam0504/go-ddd-core/pkg/errorsx"
    txgrpc "github.com/slam0504/go-ddd-core/transport/grpc"
)

func errorMapping() grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
        resp, err := h(ctx, req)
        if err == nil {
            return resp, nil
        }
        c := errorsx.CodeOf(err)                       // pkg/errorsx
        return nil, status.Error(codes.Code(txgrpc.CodeFromErrorsx(c)), err.Error())
    }
}
```

`errorsx.CodeOf` walks the error chain (errors.As) so wrapped errorsx values
still surface their code; `CodeFromErrorsx` returns the numeric gRPC code.

A `domain.RuleViolation` returned from an aggregate is best lifted via
`pkg/errorsx/httpx.FromRuleViolation` (which produces `*errorsx.Error` with
`InvalidArgument` and the rule code in details). The same value is then
translated correctly by `errorMapping` above.

### 3. Module wiring with `bootstrap`

Modules implement `ServiceProvider` / `InterceptorProvider` and hand them to
the adapter at startup:

```go
adapter := grpcadapter.New(
    cfg.Addr,
    moduleA, moduleB, // both implement InterceptorProvider
)

for _, p := range txgrpc.CollectServiceProviders(moduleA, moduleB, ordersModule) {
    p.RegisterServices(adapter)
}

app.Lifecycle.AppendStart(adapter.Start)
app.Lifecycle.AppendStop(adapter.Stop)
```

`CollectServiceProviders` quietly drops nil providers so optional modules
behave nicely under feature flags.

## What core deliberately does NOT do

- **No gRPC version pin** — the library moves quickly; pinning here would
  drag every consumer into the same upgrade path.
- **No reflection / health / channelz** — these are application-level
  decisions and trivial to add in the adapter.
- **No interceptor implementations** (logging, tracing, recovery) — too
  opinionated; the helper just composes `[]any` so adapters wire whatever
  they prefer.
