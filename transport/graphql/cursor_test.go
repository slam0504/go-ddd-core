package graphql_test

import (
	"errors"
	"testing"

	"github.com/slam0504/go-ddd-core/pkg/pagination"
	"github.com/slam0504/go-ddd-core/transport/graphql"
)

func TestEncodeDecodeCursor_RoundTrip(t *testing.T) {
	encoded := graphql.EncodeCursor("user:42")
	if encoded == "" {
		t.Fatalf("EncodeCursor returned empty")
	}
	got, err := graphql.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if got != "user:42" {
		t.Fatalf("got %q, want user:42", got)
	}
}

func TestDecodeCursor_RejectsMissingPrefix(t *testing.T) {
	if _, err := graphql.DecodeCursor("not-a-cursor"); !errors.Is(err, graphql.ErrInvalidCursor) {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeCursor_RejectsBadBase64(t *testing.T) {
	if _, err := graphql.DecodeCursor("v1:not_base64!!"); !errors.Is(err, graphql.ErrInvalidCursor) {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

func TestToPageRequest_ForwardOnly(t *testing.T) {
	first := 20
	after := graphql.EncodeCursor("u:7")
	req, err := graphql.ToPageRequest(graphql.ConnectionArgs{First: &first, After: &after})
	if err != nil {
		t.Fatalf("ToPageRequest: %v", err)
	}
	if req.Size != 20 {
		t.Fatalf("Size = %d, want 20", req.Size)
	}
	if req.Token != "u:7" {
		t.Fatalf("Token = %q, want u:7", req.Token)
	}
}

func TestToPageRequest_BackwardRejected(t *testing.T) {
	last := 10
	_, err := graphql.ToPageRequest(graphql.ConnectionArgs{Last: &last})
	if !errors.Is(err, graphql.ErrUnsupportedDirection) {
		t.Fatalf("err = %v, want ErrUnsupportedDirection", err)
	}
}

func TestToPageRequest_BadAfterPropagatesError(t *testing.T) {
	bad := "not-a-cursor"
	_, err := graphql.ToPageRequest(graphql.ConnectionArgs{After: &bad})
	if !errors.Is(err, graphql.ErrInvalidCursor) {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

func TestBuildConnection_BasicShape(t *testing.T) {
	page := pagination.Page[string]{
		Items:      []string{"alice", "bob", "carol"},
		Total:      -1,
		NextCursor: "carol",
	}
	conn := graphql.BuildConnection(page, func(s string) string { return s })
	if len(conn.Edges) != 3 {
		t.Fatalf("Edges = %d, want 3", len(conn.Edges))
	}
	if conn.Edges[0].Node != "alice" || conn.Edges[0].Cursor == "" {
		t.Fatalf("first edge wrong: %+v", conn.Edges[0])
	}
	if !conn.PageInfo.HasNextPage {
		t.Fatalf("HasNextPage should be true when NextCursor is set")
	}
	if conn.PageInfo.StartCursor == "" || conn.PageInfo.EndCursor == "" {
		t.Fatalf("StartCursor/EndCursor must be set when edges exist")
	}
	// EndCursor should reflect the page-level NextCursor when present.
	wantEnd := graphql.EncodeCursor("carol")
	if conn.PageInfo.EndCursor != wantEnd {
		t.Fatalf("EndCursor = %q, want %q", conn.PageInfo.EndCursor, wantEnd)
	}
}

func TestBuildConnection_EmptyPage(t *testing.T) {
	conn := graphql.BuildConnection(pagination.Page[string]{}, func(s string) string { return s })
	if len(conn.Edges) != 0 {
		t.Fatalf("Edges = %d, want 0", len(conn.Edges))
	}
	if conn.PageInfo.HasNextPage {
		t.Fatalf("empty page should not HasNextPage")
	}
	if conn.PageInfo.StartCursor != "" || conn.PageInfo.EndCursor != "" {
		t.Fatalf("empty page should leave cursors empty")
	}
}
