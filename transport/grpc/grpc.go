// Package grpc defines the inbound gRPC server contract. Core stays free of
// google.golang.org/grpc; concrete types are erased behind any so adapters
// can pass real gRPC objects without core depending on the library.
package grpc

import (
	"context"
)

// Server runs a gRPC listener. Start must not block; Stop should perform a
// graceful shutdown, draining in-flight RPCs.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Addr() string
}

// ServiceRegistrar registers a gRPC service implementation against the
// underlying server. Adapters typically downcast raw/impl to the concrete
// grpc.ServiceDesc + service struct. Keeping them as any prevents core
// from importing google.golang.org/grpc.
type ServiceRegistrar interface {
	RegisterService(desc any, impl any)
}

// ServiceProvider is implemented by components that contribute gRPC
// services during bootstrap.
type ServiceProvider interface {
	RegisterServices(r ServiceRegistrar)
}

// InterceptorProvider lets modules contribute unary/stream interceptors.
// The interceptor values themselves are typed any so core does not depend
// on the gRPC package. Adapters cast them to the expected types.
type InterceptorProvider interface {
	UnaryInterceptors() []any
	StreamInterceptors() []any
}
