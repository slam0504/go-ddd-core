package query

// Middleware wraps a DispatchFunc with cross-cutting concerns.
type Middleware func(next DispatchFunc) DispatchFunc

// Chain composes middlewares so the first one is outermost.
func Chain(mws ...Middleware) Middleware {
	return func(next DispatchFunc) DispatchFunc {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

func Apply(fn DispatchFunc, mws ...Middleware) DispatchFunc {
	return Chain(mws...)(fn)
}
