package graphql

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/slam0504/go-ddd-core/pkg/pagination"
)

// EncodeCursor turns an opaque payload (typically the last seen key plus its
// type tag) into a Relay Connection cursor string. The encoding is
// URL-safe base64 with a "v1:" prefix so future format changes can be
// detected and routed without breaking older clients in flight.
func EncodeCursor(payload string) string {
	return cursorPrefix + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// DecodeCursor extracts the opaque payload from an EncodeCursor result.
// Returns ErrInvalidCursor for missing prefix, malformed base64, or any
// other parsing failure so callers can reject the request with a single
// branch.
func DecodeCursor(cursor string) (string, error) {
	if !strings.HasPrefix(cursor, cursorPrefix) {
		return "", ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor[len(cursorPrefix):])
	if err != nil {
		return "", ErrInvalidCursor
	}
	return string(raw), nil
}

// ConnectionArgs is the standard Relay Connection input on the wire. Callers
// translate this into a pagination.PageRequest via ToPageRequest.
type ConnectionArgs struct {
	First  *int
	After  *string
	Last   *int
	Before *string
}

// ToPageRequest converts Relay Connection args into a pagination.Cursor. Only
// forward pagination (First / After) is supported here; backward pagination
// (Last / Before) returns ErrUnsupportedDirection because most read models
// only index the forward direction efficiently.
//
// Backward pagination remains representable on the wire so resolvers that
// DO support it can reject explicitly rather than silently ignore the args.
func ToPageRequest(args ConnectionArgs) (pagination.Cursor, error) {
	if args.Last != nil || args.Before != nil {
		return pagination.Cursor{}, ErrUnsupportedDirection
	}
	c := pagination.Cursor{}
	if args.First != nil {
		c.Size = *args.First
	}
	if args.After != nil {
		payload, err := DecodeCursor(*args.After)
		if err != nil {
			return pagination.Cursor{}, err
		}
		c.Token = payload
	}
	return c, nil
}

// Connection is the standard Relay Connection wire shape. Resolvers build it
// from a pagination.Page[T] using BuildConnection.
type Connection[T any] struct {
	Edges    []Edge[T]
	PageInfo PageInfo
}

// Edge wraps a node with its cursor.
type Edge[T any] struct {
	Node   T
	Cursor string
}

// PageInfo carries pagination metadata Relay clients expect.
type PageInfo struct {
	HasNextPage     bool
	HasPreviousPage bool
	StartCursor     string
	EndCursor       string
}

// BuildConnection assembles a Relay Connection from a pagination.Page and a
// per-item cursor extractor. cursorOf receives each item and returns the
// payload (e.g. the item's ID) that EncodeCursor will wrap.
//
// HasPreviousPage is left false: cursor pagination cannot answer it cheaply
// without an extra round-trip; resolvers needing the flag should compute it
// from the request side and override the result.
func BuildConnection[T any](page pagination.Page[T], cursorOf func(T) string) Connection[T] {
	edges := make([]Edge[T], len(page.Items))
	for i, item := range page.Items {
		edges[i] = Edge[T]{Node: item, Cursor: EncodeCursor(cursorOf(item))}
	}

	info := PageInfo{HasNextPage: page.HasNext()}
	if len(edges) > 0 {
		info.StartCursor = edges[0].Cursor
		info.EndCursor = edges[len(edges)-1].Cursor
	}
	if info.HasNextPage && page.NextCursor != "" {
		info.EndCursor = EncodeCursor(page.NextCursor)
	}
	return Connection[T]{Edges: edges, PageInfo: info}
}

const cursorPrefix = "v1:"

// ErrInvalidCursor is returned when DecodeCursor cannot interpret the input.
var ErrInvalidCursor = errors.New("graphql: invalid cursor")

// ErrUnsupportedDirection is returned by ToPageRequest when Last/Before are
// supplied but only forward pagination is implemented.
var ErrUnsupportedDirection = errors.New("graphql: backward pagination not supported")
