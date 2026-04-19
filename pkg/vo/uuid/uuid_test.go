package uuid_test

import (
	"encoding/json"
	"testing"

	"github.com/slam0504/go-ddd-core/domain"
	"github.com/slam0504/go-ddd-core/pkg/vo/uuid"
)

func TestNewIsUniqueAndNonNil(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	if a.IsNil() {
		t.Fatalf("New() returned Nil")
	}
	if a.Equal(b) {
		t.Fatalf("two New() results should differ")
	}
}

func TestParseRoundTrip(t *testing.T) {
	a := uuid.New()
	b, err := uuid.Parse(a.String())
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !a.Equal(b) {
		t.Fatalf("round-trip lost equality")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := uuid.Parse("not-a-uuid"); err == nil {
		t.Fatalf("expected parse error for garbage input")
	}
}

func TestMustParsePanicsOnGarbage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("MustParse should panic on garbage input")
		}
	}()
	_ = uuid.MustParse("xxx")
}

func TestNilSentinel(t *testing.T) {
	if !uuid.Nil.IsNil() {
		t.Fatalf("Nil.IsNil() should be true")
	}
	if uuid.New().IsNil() {
		t.Fatalf("New() should never produce Nil")
	}
}

type otherVO struct{}

func (otherVO) Equal(_ domain.ValueObject) bool { return true }

func TestEqualRejectsOtherValueObjectTypes(t *testing.T) {
	if uuid.New().Equal(otherVO{}) {
		t.Fatalf("Equal across types should be false")
	}
}

func TestJSONMarshalRoundTrip(t *testing.T) {
	a := uuid.New()
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var b uuid.UUID
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !a.Equal(b) {
		t.Fatalf("JSON round-trip lost equality")
	}
}

func TestUnmarshalTextRejectsGarbage(t *testing.T) {
	var u uuid.UUID
	if err := u.UnmarshalText([]byte("not-a-uuid")); err == nil {
		t.Fatalf("expected unmarshal error")
	}
}
