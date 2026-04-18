// Package graphql defines the inbound GraphQL contract. GraphQL is typically
// served over HTTP, so this package focuses on producing the http.Handler
// that a transport/http Server (or any http mux) can mount. Schema
// construction and resolver wiring are the adapter's concern (gqlgen,
// graphql-go, 99designs, etc.).
package graphql

import (
	"context"
	"net/http"
)

// Server runs a GraphQL endpoint. When used alongside transport/http, the
// adapter usually returns a Server that delegates to the shared HTTP server
// and simply registers the GraphQL handler on a path.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	// Handler returns the http.Handler that serves GraphQL requests so it
	// can be mounted onto an existing HTTP router.
	Handler() http.Handler
}

// SchemaProvider contributes GraphQL schema fragments (types, resolvers).
// The exec value is passed as any so core does not depend on a specific
// GraphQL library — adapters know the concrete type.
type SchemaProvider interface {
	Schema() any
}

// ResolverRegistrar allows multiple modules to contribute resolvers to a
// single schema. Adapters translate the opaque values into their library's
// resolver registration APIs.
type ResolverRegistrar interface {
	RegisterResolver(name string, resolver any)
}

// ResolverProvider is implemented by modules that want to contribute
// resolvers during bootstrap.
type ResolverProvider interface {
	RegisterResolvers(r ResolverRegistrar)
}
