package graphql

import "context"

// Loader is the minimal contract for batched, deduplicated lookups commonly
// used inside GraphQL resolvers to defeat the N+1 problem. The interface is
// intentionally narrow so adapters can wrap graph-gophers/dataloader,
// vektah/dataloaden, or hand-written batchers without bringing them into
// core.
type Loader[K comparable, V any] interface {
	// Load fetches a single value, joining concurrent calls into a batch
	// transparently to the caller.
	Load(ctx context.Context, key K) (V, error)
	// LoadMany fetches a slice of values, returning a parallel slice of
	// errors so partial failures do not collapse the whole batch.
	LoadMany(ctx context.Context, keys []K) ([]V, []error)
}

// BatchFunc is the user-supplied batch resolver wrapped by a Loader. It
// receives every key requested in the current batch and must return values
// and errors aligned positionally with keys.
type BatchFunc[K comparable, V any] func(ctx context.Context, keys []K) ([]V, []error)
