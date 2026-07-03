package storagetest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/storage"
)

// memStorage is the test-only reference implementation the suite is proven
// against. It exists so RunContract is demonstrably satisfiable and
// non-vacuous; it is not exported for production use.
type memStorage struct {
	mu      sync.Mutex
	objects map[string]memObject
}

type memObject struct {
	data        []byte
	contentType string
	metadata    map[string]string
	updatedAt   time.Time
}

func newMemStorage() *memStorage {
	return &memStorage{objects: map[string]memObject{}}
}

func errEmptyKey() error {
	return errorsx.New(errorsx.CodeInvalidArgument, "storagetest: empty key")
}

func (m *memStorage) Put(ctx context.Context, key string, body io.Reader, size int64, opts storage.PutOptions) error {
	if key == "" {
		return errEmptyKey()
	}
	if size < 0 {
		return errorsx.New(errorsx.CodeInvalidArgument, "storagetest: negative size")
	}
	if err := ctx.Err(); err != nil {
		return errorsx.Wrap(errorsx.CodeUnavailable, "storagetest: context done", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return errorsx.Wrap(errorsx.CodeUnavailable, "storagetest: read body", err)
	}
	if int64(len(data)) != size {
		return errorsx.New(errorsx.CodeInvalidArgument,
			fmt.Sprintf("storagetest: declared size %d != actual %d", size, len(data)))
	}
	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memObject{
		data:        data,
		contentType: ct,
		metadata:    maps.Clone(opts.Metadata),
		updatedAt:   time.Now(),
	}
	return nil
}

func (m *memStorage) Get(ctx context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	info, err := m.Stat(ctx, key)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	m.mu.Lock()
	data := bytes.Clone(m.objects[key].data)
	m.mu.Unlock()
	return io.NopCloser(bytes.NewReader(data)), info, nil
}

func (m *memStorage) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	if key == "" {
		return storage.ObjectInfo{}, errEmptyKey()
	}
	if err := ctx.Err(); err != nil {
		return storage.ObjectInfo{}, errorsx.Wrap(errorsx.CodeUnavailable, "storagetest: context done", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, ok := m.objects[key]
	if !ok {
		return storage.ObjectInfo{}, fmt.Errorf("storagetest: %q: %w", key, storage.ErrNotFound)
	}
	return storage.ObjectInfo{
		Key:         key,
		Size:        int64(len(obj.data)),
		ContentType: obj.contentType,
		ETag:        "", // empty ETag is contractually allowed
		UpdatedAt:   obj.updatedAt,
		Metadata:    maps.Clone(obj.metadata),
	}, nil
}

func (m *memStorage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return errEmptyKey()
	}
	if err := ctx.Err(); err != nil {
		return errorsx.Wrap(errorsx.CodeUnavailable, "storagetest: context done", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key) // idempotent: absent key is a no-op success
	return nil
}

func (m *memStorage) List(ctx context.Context, opts storage.ListOptions) (storage.ListPage, error) {
	if opts.Limit <= 0 {
		return storage.ListPage{}, errorsx.New(errorsx.CodeInvalidArgument, "storagetest: non-positive limit")
	}
	if err := ctx.Err(); err != nil {
		return storage.ListPage{}, errorsx.Wrap(errorsx.CodeUnavailable, "storagetest: context done", err)
	}
	m.mu.Lock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		if strings.HasPrefix(k, opts.Prefix) {
			keys = append(keys, k)
		}
	}
	m.mu.Unlock()
	sort.Strings(keys) // lexical order = this backend's stable walk order

	page := storage.ListPage{}
	for _, k := range keys {
		if opts.Token != "" && k <= opts.Token {
			continue // resume strictly after the cursor
		}
		if len(page.Objects) == opts.Limit {
			page.NextToken = page.Objects[opts.Limit-1].Key
			return page, nil
		}
		info, err := m.Stat(ctx, k)
		if err != nil {
			return storage.ListPage{}, err
		}
		info.Metadata = nil // List is not required to populate Metadata
		page.Objects = append(page.Objects, info)
	}
	return page, nil
}
