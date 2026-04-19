// Package pagination defines reusable types for paged queries shared across
// repositories, query handlers, and transport adapters. The package is a thin
// data-only layer with no infrastructure dependencies.
package pagination

// PageRequest is a sealed interface implemented by Limit and Cursor. Callers
// must pick exactly one paging style; mixing them is intentionally not
// representable.
type PageRequest interface {
	isPageRequest()
}

// Limit selects an offset-based slice. Size is the page size; Offset is the
// number of items to skip. Both must be non-negative; Size of 0 means "use
// adapter default".
type Limit struct {
	Size   int
	Offset int
}

func (Limit) isPageRequest() {}

// Cursor selects a forward slice anchored at an opaque token. Token of "" means
// "first page". Size of 0 means "use adapter default".
type Cursor struct {
	Token string
	Size  int
}

func (Cursor) isPageRequest() {}

// Page is the result envelope for a paged query. Total reports the full match
// count; -1 signals the count was not computed (typical for cursor pagination).
// NextCursor is the opaque token for the next call; "" means no further pages.
type Page[T any] struct {
	Items      []T
	Total      int64
	NextCursor string
}

// HasNext reports whether p has a non-empty NextCursor.
func (p Page[T]) HasNext() bool { return p.NextCursor != "" }
