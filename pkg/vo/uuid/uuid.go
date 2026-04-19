// Package uuid provides a UUID value object suitable for embedding in
// aggregates and value objects. It wraps github.com/google/uuid with a
// narrower, value-object-shaped API: New produces a v7 UUID (time-ordered for
// index locality), Parse rejects malformed strings, and Equal participates in
// domain.ValueObject.
//
// Database serialisation (BINARY(16), BYTEA, etc.) is intentionally omitted;
// adapters in go-ddd-adapters handle storage encoding so this package stays
// dependency-free at the domain layer.
package uuid

import (
	"encoding"

	gguid "github.com/google/uuid"

	"github.com/slam0504/go-ddd-core/domain"
)

// UUID is a 128-bit identifier wrapped as a value object.
type UUID struct {
	v gguid.UUID
}

// Nil is the zero UUID, useful as a sentinel for "not yet assigned".
var Nil = UUID{}

// New returns a freshly generated v7 UUID. v7 embeds a unix-millisecond
// timestamp as the leading bits, giving database B-tree indexes locality
// without leaking ordering across processes.
func New() UUID {
	return UUID{v: gguid.Must(gguid.NewV7())}
}

// Parse decodes a string into a UUID. It accepts the canonical 8-4-4-4-12
// hex form with or without surrounding braces.
func Parse(s string) (UUID, error) {
	v, err := gguid.Parse(s)
	if err != nil {
		return UUID{}, err
	}
	return UUID{v: v}, nil
}

// MustParse is the panicking variant of Parse, suited for package-level
// constants and tests where the input is statically known.
func MustParse(s string) UUID {
	u, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// String returns the canonical 8-4-4-4-12 hex form.
func (u UUID) String() string { return u.v.String() }

// IsNil reports whether u is the zero UUID.
func (u UUID) IsNil() bool { return u.v == gguid.Nil }

// Equal satisfies domain.ValueObject. Two UUIDs are equal iff their 128-bit
// payload matches.
func (u UUID) Equal(other domain.ValueObject) bool {
	o, ok := other.(UUID)
	return ok && u.v == o.v
}

// MarshalText supports JSON / TOML / form encoding by emitting the canonical
// string form.
func (u UUID) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

// UnmarshalText accepts the canonical string form produced by MarshalText.
func (u *UUID) UnmarshalText(text []byte) error {
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

// Compile-time guards.
var (
	_ domain.ValueObject       = UUID{}
	_ encoding.TextMarshaler   = UUID{}
	_ encoding.TextUnmarshaler = (*UUID)(nil)
)
