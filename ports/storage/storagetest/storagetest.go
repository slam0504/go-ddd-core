// Package storagetest is the exported conformance suite for
// ports/storage.ObjectStorage. Any adapter runs RunContract against its own
// implementation to prove it honours the object-storage contract. The suite
// is deterministic: it asserts invariants visible through the interface,
// compares List results as SETS (ordering is backend-defined), and never
// asserts ETag behaviour beyond "empty allowed". Missing-object assertions
// use errors.Is(err, storage.ErrNotFound), deliberately NEVER
// errorsx.CodeOf (ErrNotFound is an uncoded domain sentinel). Imports only
// testing + the contract + errorsx so adapters inherit no transport
// dependency. PresignedURLer is deliberately out of scope: an in-memory
// reference cannot sign meaningful URLs; adapters prove presign against a
// real backend.
package storagetest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/errorsx"
	"github.com/slam0504/go-ddd-core/ports/storage"
)

// Factory returns a fresh, keyspace-isolated storage.ObjectStorage for one
// subtest. It receives the subtest's *testing.T so an adapter can key
// isolation on t.Name() and register t.Cleanup (matches cachetest /
// inboxtest). Each call MUST return an implementation whose keyspace does
// not overlap any other call's (e.g. a new in-memory map, or a fresh
// bucket per call).
type Factory func(t *testing.T) storage.ObjectStorage

// RunContract runs the full deterministic ObjectStorage conformance suite.
func RunContract(t *testing.T, newStorage Factory) {
	t.Helper()
	ctx := context.Background()

	put := func(t *testing.T, s storage.ObjectStorage, key, body string, opts storage.PutOptions) {
		t.Helper()
		if err := s.Put(ctx, key, strings.NewReader(body), int64(len(body)), opts); err != nil {
			t.Fatalf("Put(%q): %v", key, err)
		}
	}

	t.Run("PutGetRoundtrip", func(t *testing.T) {
		s := newStorage(t)
		put(t, s, "k", "hello", storage.PutOptions{ContentType: "text/plain"})
		rc, info, err := s.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer func() { _ = rc.Close() }()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if !bytes.Equal(got, []byte("hello")) {
			t.Fatalf("body = %q, want %q", got, "hello")
		}
		if info.Key != "k" || info.Size != 5 || info.ContentType != "text/plain" {
			t.Fatalf("info = %+v, want Key=k Size=5 ContentType=text/plain", info)
		}
	})

	t.Run("PutOverwriteWins", func(t *testing.T) {
		s := newStorage(t)
		put(t, s, "k", "first", storage.PutOptions{ContentType: "text/plain"})
		put(t, s, "k", "second-longer", storage.PutOptions{ContentType: "application/json"})
		rc, info, err := s.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get after overwrite: %v", err)
		}
		defer func() { _ = rc.Close() }()
		got, _ := io.ReadAll(rc)
		if string(got) != "second-longer" {
			t.Fatalf("body = %q, want the second write (last-writer-wins)", got)
		}
		if info.Size != int64(len("second-longer")) || info.ContentType != "application/json" {
			t.Fatalf("info = %+v: overwrite must fully replace size and content type", info)
		}
		// Deliberately NO ETag assertion: ETag is an opaque validator.
	})

	t.Run("PutNegativeSizeInvalidArgument", func(t *testing.T) {
		s := newStorage(t)
		err := s.Put(ctx, "k", strings.NewReader("x"), -1, storage.PutOptions{})
		if errorsx.CodeOf(err) != errorsx.CodeInvalidArgument {
			t.Fatalf("Put(size=-1) code = %v, want CodeInvalidArgument", errorsx.CodeOf(err))
		}
	})

	t.Run("MetadataRoundtripLowercaseKeys", func(t *testing.T) {
		s := newStorage(t)
		md := map[string]string{"request-id": "r-42", "owner": "orders"}
		put(t, s, "k", "v", storage.PutOptions{ContentType: "text/plain", Metadata: md})
		info, err := s.Stat(ctx, "k")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		for wantK, wantV := range md {
			if info.Metadata[wantK] != wantV {
				t.Fatalf("Stat Metadata[%q] = %q, want %q (full: %v)", wantK, info.Metadata[wantK], wantV, info.Metadata)
			}
		}
		rc, ginfo, err := s.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		_ = rc.Close()
		for wantK, wantV := range md {
			if ginfo.Metadata[wantK] != wantV {
				t.Fatalf("Get Metadata[%q] = %q, want %q", wantK, ginfo.Metadata[wantK], wantV)
			}
		}
	})

	t.Run("GetMissingReturnsErrNotFound", func(t *testing.T) {
		s := newStorage(t)
		_, _, err := s.Get(ctx, "absent")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Get(absent) err = %v, want errors.Is(_, storage.ErrNotFound)", err)
		}
		// Deliberately NO CodeOf assertion: ErrNotFound is an uncoded sentinel.
	})

	t.Run("StatMissingReturnsErrNotFound", func(t *testing.T) {
		s := newStorage(t)
		_, err := s.Stat(ctx, "absent")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Stat(absent) err = %v, want errors.Is(_, storage.ErrNotFound)", err)
		}
	})

	t.Run("DeleteIsIdempotent", func(t *testing.T) {
		s := newStorage(t)
		if err := s.Delete(ctx, "absent"); err != nil {
			t.Fatalf("Delete(absent) = %v, want nil (idempotent delete)", err)
		}
		put(t, s, "k", "v", storage.PutOptions{})
		if err := s.Delete(ctx, "k"); err != nil {
			t.Fatalf("Delete(existing): %v", err)
		}
		if _, err := s.Stat(ctx, "k"); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("Stat after Delete err = %v, want ErrNotFound", err)
		}
		if err := s.Delete(ctx, "k"); err != nil {
			t.Fatalf("second Delete = %v, want nil", err)
		}
	})

	t.Run("EmptyKeyPrecedesCancelledCtx", func(t *testing.T) {
		s := newStorage(t)
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if code := errorsx.CodeOf(s.Put(cancelled, "", strings.NewReader(""), 0, storage.PutOptions{})); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Put empty key code = %v, want CodeInvalidArgument before ctx", code)
		}
		_, _, err := s.Get(cancelled, "")
		if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Get empty key code = %v, want CodeInvalidArgument before ctx", code)
		}
		_, serr := s.Stat(cancelled, "")
		if code := errorsx.CodeOf(serr); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Stat empty key code = %v, want CodeInvalidArgument before ctx", code)
		}
		if code := errorsx.CodeOf(s.Delete(cancelled, "")); code != errorsx.CodeInvalidArgument {
			t.Fatalf("Delete empty key code = %v, want CodeInvalidArgument before ctx", code)
		}
	})

	t.Run("ListLimitNonPositivePrecedesCancelledCtx", func(t *testing.T) {
		s := newStorage(t)
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		for _, limit := range []int{0, -1} {
			_, err := s.List(cancelled, storage.ListOptions{Limit: limit})
			if code := errorsx.CodeOf(err); code != errorsx.CodeInvalidArgument {
				t.Fatalf("List(Limit=%d) code = %v, want CodeInvalidArgument before ctx", limit, code)
			}
		}
	})

	t.Run("ListTokenWalkCompleteNoDupes", func(t *testing.T) {
		s := newStorage(t)
		want := map[string]bool{}
		for _, k := range []string{"w/a", "w/b", "w/c", "w/d", "w/e", "w/f", "w/g"} {
			put(t, s, k, "v", storage.PutOptions{})
			want[k] = true
		}
		got := map[string]bool{}
		token := ""
		for pages := 0; ; pages++ {
			if pages > 20 {
				t.Fatal("walk did not terminate within 20 pages")
			}
			page, err := s.List(ctx, storage.ListOptions{Prefix: "w/", Token: token, Limit: 3})
			if err != nil {
				t.Fatalf("List page %d: %v", pages, err)
			}
			if len(page.Objects) > 3 {
				t.Fatalf("page %d has %d objects, exceeds Limit 3", pages, len(page.Objects))
			}
			for _, o := range page.Objects {
				if got[o.Key] {
					t.Fatalf("duplicate key %q in walk", o.Key)
				}
				got[o.Key] = true
			}
			if page.NextToken == "" {
				break
			}
			token = page.NextToken
		}
		if len(got) != len(want) {
			t.Fatalf("walk yielded %d keys, want %d: got %v", len(got), len(want), got)
		}
		for k := range want {
			if !got[k] {
				t.Fatalf("walk omitted key %q", k)
			}
		}
	})

	t.Run("ListPrefixFilters", func(t *testing.T) {
		s := newStorage(t)
		for _, k := range []string{"a/1", "a/2", "b/1"} {
			put(t, s, k, "v", storage.PutOptions{})
		}
		page, err := s.List(ctx, storage.ListOptions{Prefix: "a/", Limit: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got := map[string]bool{}
		for _, o := range page.Objects {
			got[o.Key] = true
		}
		if page.NextToken != "" || len(got) != 2 || !got["a/1"] || !got["a/2"] {
			t.Fatalf("List(a/) = %v (next %q), want exactly {a/1, a/2}", got, page.NextToken)
		}
	})

	t.Run("ListEmptyPrefixListsAll", func(t *testing.T) {
		s := newStorage(t)
		for _, k := range []string{"a/1", "b/1", "c"} {
			put(t, s, k, "v", storage.PutOptions{})
		}
		page, err := s.List(ctx, storage.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page.Objects) != 3 || page.NextToken != "" {
			t.Fatalf("List(\"\") returned %d objects (next %q), want all 3", len(page.Objects), page.NextToken)
		}
	})

	t.Run("ListNoMatchReturnsEmptyPage", func(t *testing.T) {
		s := newStorage(t)
		put(t, s, "a/1", "v", storage.PutOptions{})
		page, err := s.List(ctx, storage.ListOptions{Prefix: "zzz/", Limit: 5})
		if err != nil {
			t.Fatalf("List(no match) err = %v, want nil", err)
		}
		if len(page.Objects) != 0 || page.NextToken != "" {
			t.Fatalf("List(no match) = %+v, want empty page with empty NextToken", page)
		}
	})
}
