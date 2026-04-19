package grpc

// CombineInterceptorProviders flattens the unary and stream interceptors of
// every provider into two ordered slices. The result preserves provider order
// (provider 0's interceptors run outermost) which matches the conventional
// gRPC chain expectation.
//
// Returned values stay typed as []any to keep this package free of
// google.golang.org/grpc; adapters cast each entry to grpc.UnaryServerInterceptor
// or grpc.StreamServerInterceptor at registration time.
func CombineInterceptorProviders(providers ...InterceptorProvider) (unary, stream []any) {
	for _, p := range providers {
		if p == nil {
			continue
		}
		unary = append(unary, p.UnaryInterceptors()...)
		stream = append(stream, p.StreamInterceptors()...)
	}
	return unary, stream
}

// CollectServiceProviders returns providers in the order they were supplied,
// dropping nil entries. Adapters typically iterate the result and call
// p.RegisterServices(registrar) for each.
func CollectServiceProviders(providers ...ServiceProvider) []ServiceProvider {
	out := make([]ServiceProvider, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}
