package spec_test

import (
	"strings"
	"testing"

	"github.com/slam0504/go-ddd-core/application/query/spec"
)

type user struct {
	Name string
	Age  int
}

func ageAtLeast(min int) spec.Specification[user] {
	return spec.Predicate(func(u user) bool { return u.Age >= min })
}

func nameStartsWith(prefix string) spec.Specification[user] {
	return spec.Predicate(func(u user) bool { return strings.HasPrefix(u.Name, prefix) })
}

func TestPredicateBasic(t *testing.T) {
	s := ageAtLeast(18)
	if !s.IsSatisfiedBy(user{Age: 21}) {
		t.Fatalf("expected 21 to satisfy age>=18")
	}
	if s.IsSatisfiedBy(user{Age: 17}) {
		t.Fatalf("expected 17 not to satisfy age>=18")
	}
}

func TestAndCombinator(t *testing.T) {
	s := ageAtLeast(18).And(nameStartsWith("A"))
	if !s.IsSatisfiedBy(user{Name: "Alice", Age: 30}) {
		t.Fatalf("Alice/30 should satisfy")
	}
	if s.IsSatisfiedBy(user{Name: "Bob", Age: 30}) {
		t.Fatalf("Bob/30 should fail name prefix")
	}
	if s.IsSatisfiedBy(user{Name: "Alice", Age: 10}) {
		t.Fatalf("Alice/10 should fail age")
	}
}

func TestOrCombinator(t *testing.T) {
	s := ageAtLeast(65).Or(nameStartsWith("VIP_"))
	if !s.IsSatisfiedBy(user{Name: "Alice", Age: 70}) {
		t.Fatalf("70yo should satisfy via age branch")
	}
	if !s.IsSatisfiedBy(user{Name: "VIP_Bob", Age: 30}) {
		t.Fatalf("VIP_Bob should satisfy via name branch")
	}
	if s.IsSatisfiedBy(user{Name: "Bob", Age: 30}) {
		t.Fatalf("regular Bob should not satisfy either branch")
	}
}

func TestNotCombinator(t *testing.T) {
	s := ageAtLeast(18).Not()
	if !s.IsSatisfiedBy(user{Age: 10}) {
		t.Fatalf("Not(age>=18) should accept 10")
	}
	if s.IsSatisfiedBy(user{Age: 30}) {
		t.Fatalf("Not(age>=18) should reject 30")
	}
}

func TestDoubleNotCollapses(t *testing.T) {
	inner := ageAtLeast(18)
	got := inner.Not().Not()
	// Double-Not returns the original inner spec, so behaviour matches.
	if got.IsSatisfiedBy(user{Age: 10}) {
		t.Fatalf("double-Not should behave like original")
	}
	if !got.IsSatisfiedBy(user{Age: 30}) {
		t.Fatalf("double-Not should behave like original")
	}
}

func TestCompositeExposesOperands(t *testing.T) {
	left := ageAtLeast(18)
	right := nameStartsWith("A")
	combined := spec.And(left, right)
	c, ok := combined.(spec.Composite[user])
	if !ok {
		t.Fatalf("And result should implement Composite[T]")
	}
	if !c.Left().IsSatisfiedBy(user{Age: 30}) {
		t.Fatalf("Left() did not return age spec")
	}
	if !c.Right().IsSatisfiedBy(user{Name: "Alice"}) {
		t.Fatalf("Right() did not return name spec")
	}
}

func TestNegationExposesInner(t *testing.T) {
	inner := ageAtLeast(18)
	n, ok := spec.Not(inner).(spec.Negation[user])
	if !ok {
		t.Fatalf("Not result should implement Negation[T]")
	}
	if !n.Inner().IsSatisfiedBy(user{Age: 30}) {
		t.Fatalf("Inner() did not return age spec")
	}
}

// sqlSpec demonstrates how an adapter-defined spec opts in to SQL translation.
type sqlAgeAtLeast struct{ min int }

func (s sqlAgeAtLeast) IsSatisfiedBy(u user) bool { return u.Age >= s.min }
func (s sqlAgeAtLeast) And(o spec.Specification[user]) spec.Specification[user] {
	return spec.And(s, o)
}
func (s sqlAgeAtLeast) Or(o spec.Specification[user]) spec.Specification[user] { return spec.Or(s, o) }
func (s sqlAgeAtLeast) Not() spec.Specification[user]                          { return spec.Not(s) }
func (s sqlAgeAtLeast) ToSQL() (string, []any)                                 { return "age >= ?", []any{s.min} }

func TestSQLTranslatableOptIn(t *testing.T) {
	var s spec.Specification[user] = sqlAgeAtLeast{min: 18}
	sql, ok := s.(spec.SQLTranslatable)
	if !ok {
		t.Fatalf("expected sqlAgeAtLeast to satisfy SQLTranslatable")
	}
	clause, args := sql.ToSQL()
	if clause != "age >= ?" {
		t.Fatalf("unexpected clause: %q", clause)
	}
	if len(args) != 1 || args[0] != 18 {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestPredicateSpecDoesNotImplementSQLTranslatable(t *testing.T) {
	if _, ok := ageAtLeast(18).(spec.SQLTranslatable); ok {
		t.Fatalf("Predicate-built specs should not opt into SQL translation")
	}
}
