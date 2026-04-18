// Package storage defines the object storage contract for blob data (S3,
// GCS, MinIO, local filesystem, etc.).
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
	UpdatedAt   time.Time
}

// PutOptions configures a Put operation.
type PutOptions struct {
	ContentType string
	Metadata    map[string]string
}

// ObjectStorage stores and retrieves opaque byte streams keyed by string.
type ObjectStorage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, opts PutOptions) error
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// PresignedURLer is an optional capability for adapters that support
// pre-signed URLs (S3-compatible services, GCS, etc.).
type PresignedURLer interface {
	PresignGet(ctx context.Context, key string, expires time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, expires time.Duration) (string, error)
}

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("storage: object not found")
