package command

// Middleware wraps a DispatchFunc with cross-cutting concerns such as logging,
// tracing, validation, or transaction boundaries.
type Middleware func(next DispatchFunc) DispatchFunc

// Chain composes middlewares so the first one in the list is the outermost.
func Chain(mws ...Middleware) Middleware {
	return func(next DispatchFunc) DispatchFunc {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// Apply wraps a single DispatchFunc with the given middlewares.
func Apply(fn DispatchFunc, mws ...Middleware) DispatchFunc {
	return Chain(mws...)(fn)
}
