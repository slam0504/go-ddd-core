// Package storage defines the object storage contract for blob data (S3,
// GCS, MinIO, local filesystem, etc.).
//
// # Validation precedence
//
// On every method, a malformed argument — an empty key, or List with
// Limit <= 0 — is rejected with an errorsx.CodeInvalidArgument error BEFORE
// the context is consulted and BEFORE the backend is contacted. A cancelled
// or expired context is reported next; only then do backend errors surface.
//
// # Sentinel vs coded errors
//
// A missing object on Get or Stat is reported as an error wrapping
// ErrNotFound. ErrNotFound is a domain sentinel, never a coded error:
// errorsx.CodeOf on such an error is NOT guaranteed to be CodeNotFound (it
// may be CodeUnknown). Callers detect missing objects with errors.Is, never
// with CodeOf. Any OTHER non-nil error (backend, configuration, transport)
// MUST carry an errorsx code and is never CodeUnknown.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ObjectInfo describes a stored object.
//
// ETag is an opaque backend validator: it may be empty (some backends have
// none), and when non-empty the contract promises no format — it is not
// required to be a content hash, nor to change on overwrite. Metadata holds
// user metadata with keys canonicalized to lowercase ASCII by the adapter;
// portable callers use lowercase ASCII keys only (see PutOptions). Metadata
// is populated by Get and Stat; List implementations are not required to
// populate it.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
	UpdatedAt   time.Time
	Metadata    map[string]string
}

// PutOptions configures a Put operation.
//
// ContentType left empty lets the adapter or backend apply a default (for
// example application/octet-stream); empty-string round-trip is not
// promised — ObjectInfo.ContentType reports the backend's effective value.
// Portable Metadata keys are lowercase ASCII: S3-class backends
// canonicalize metadata keys, so mixed-case keys are not guaranteed to
// round-trip byte-for-byte.
type PutOptions struct {
	ContentType string
	Metadata    map[string]string
}

// ListOptions configures a List call.
//
// Token is opaque and adapter-owned: callers must not parse or construct
// it — only pass back the previous page's NextToken (or "" for the first
// page). Limit is the maximum page size and must be positive: Limit <= 0
// is rejected with CodeInvalidArgument (there is no unbounded default).
// Backends may cap the effective page size below Limit; adapters document
// their cap.
type ListOptions struct {
	Prefix string
	Token  string
	Limit  int
}

// ListPage is one page of a List walk.
//
// NextToken == "" is the ONLY termination signal: a page may hold fewer
// than Limit objects while NextToken is non-empty, and callers must not
// treat a short page as terminal.
type ListPage struct {
	Objects   []ObjectInfo
	NextToken string
}

// ObjectStorage stores and retrieves opaque byte streams keyed by string.
type ObjectStorage interface {
	// Put stores body under key, unconditionally overwriting any existing
	// object (last-writer-wins). size is the exact byte length of body and
	// must be >= 0: a negative size is rejected with CodeInvalidArgument
	// (unknown-length streaming is not part of this contract — callers
	// buffer first). The declared size is handed to the backend as-is; if
	// the backend or the reader reports a mismatch the implementation
	// surfaces that error, and it must never intentionally truncate or pad
	// silently. Implementations are not required to pre-validate the
	// reader's length.
	Put(ctx context.Context, key string, body io.Reader, size int64, opts PutOptions) error

	// Get returns the object's content and its ObjectInfo. The caller owns
	// the returned io.ReadCloser and MUST close it. The returned ObjectInfo
	// is consistent with what Stat reports for the same object. A missing
	// key returns an error wrapping ErrNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)

	// Stat returns the object's ObjectInfo without its content. A missing
	// key returns an error wrapping ErrNotFound.
	Stat(ctx context.Context, key string) (ObjectInfo, error)

	// Delete removes the object. Delete is idempotent: deleting an absent
	// key succeeds and never returns ErrNotFound.
	Delete(ctx context.Context, key string) error

	// List returns one page of objects whose keys start with opts.Prefix
	// (an empty Prefix lists everything). Ordering is backend-defined and
	// not promised globally; the only requirement is a stable walk: under
	// no concurrent mutation, walking pages via NextToken until it is ""
	// yields every matching object exactly once. An invalid Token returns
	// a coded error, never CodeUnknown: a malformed adapter-owned token
	// may be CodeInvalidArgument, while a backend-rejected token surfaces
	// as a backend-class coded error.
	List(ctx context.Context, opts ListOptions) (ListPage, error)
}

// PresignedURLer is an optional capability for adapters that support
// pre-signed URLs (S3-compatible services, GCS, etc.). expires must be
// positive: expires <= 0 is rejected with CodeInvalidArgument. A presigned
// URL grants unauthenticated access until it expires — callers treat it as
// a secret.
type PresignedURLer interface {
	PresignGet(ctx context.Context, key string, expires time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, expires time.Duration) (string, error)
}

// ErrNotFound is returned (wrapped) by Get and Stat when a key does not
// exist. It is a domain sentinel: match with errors.Is. It is deliberately
// NOT an errorsx-coded error — see the package doc.
var ErrNotFound = errors.New("storage: object not found")
