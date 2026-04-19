package graphql_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/slam0504/go-ddd-core/application/query/spec"
	"github.com/slam0504/go-ddd-core/transport/graphql"
)

type user struct {
	Name string
	Age  int
}

// nameLeaf and ageLeaf model adapter-defined leaf clauses. A real adapter
// would translate them into SQL via spec.SQLTranslatable.
type nameLeaf struct{ Prefix string }
type ageLeaf struct{ Min int }

func leafBuilder(leaf any) (spec.Specification[user], error) {
	switch l := leaf.(type) {
	case nameLeaf:
		return spec.Predicate(func(u user) bool { return strings.HasPrefix(u.Name, l.Prefix) }), nil
	case ageLeaf:
		return spec.Predicate(func(u user) bool { return u.Age >= l.Min }), nil
	default:
		return nil, errors.New("unknown leaf type")
	}
}

func TestBuildSpecification_LeafOnly(t *testing.T) {
	in := graphql.FilterInput{Leaf: ageLeaf{Min: 18}}
	s, err := graphql.BuildSpecification(in, leafBuilder)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !s.IsSatisfiedBy(user{Age: 30}) {
		t.Fatalf("expected adult to satisfy")
	}
	if s.IsSatisfiedBy(user{Age: 10}) {
		t.Fatalf("child should not satisfy age>=18")
	}
}

func TestBuildSpecification_AndOfTwo(t *testing.T) {
	in := graphql.FilterInput{
		And: []graphql.FilterInput{
			{Leaf: ageLeaf{Min: 18}},
			{Leaf: nameLeaf{Prefix: "A"}},
		},
	}
	s, _ := graphql.BuildSpecification(in, leafBuilder)
	if !s.IsSatisfiedBy(user{Name: "Alice", Age: 30}) {
		t.Fatalf("Alice/30 should satisfy")
	}
	if s.IsSatisfiedBy(user{Name: "Bob", Age: 30}) {
		t.Fatalf("Bob/30 should fail prefix")
	}
}

func TestBuildSpecification_OrOfTwo(t *testing.T) {
	in := graphql.FilterInput{
		Or: []graphql.FilterInput{
			{Leaf: ageLeaf{Min: 65}},
			{Leaf: nameLeaf{Prefix: "VIP_"}},
		},
	}
	s, _ := graphql.BuildSpecification(in, leafBuilder)
	if !s.IsSatisfiedBy(user{Name: "VIP_Bob", Age: 30}) {
		t.Fatalf("VIP_Bob should satisfy via name")
	}
	if !s.IsSatisfiedBy(user{Name: "Carol", Age: 70}) {
		t.Fatalf("Carol/70 should satisfy via age")
	}
	if s.IsSatisfiedBy(user{Name: "Bob", Age: 30}) {
		t.Fatalf("regular Bob should not satisfy either branch")
	}
}

func TestBuildSpecification_NotInverts(t *testing.T) {
	in := graphql.FilterInput{Not: &graphql.FilterInput{Leaf: ageLeaf{Min: 18}}}
	s, _ := graphql.BuildSpecification(in, leafBuilder)
	if !s.IsSatisfiedBy(user{Age: 10}) {
		t.Fatalf("Not(age>=18) should accept 10")
	}
	if s.IsSatisfiedBy(user{Age: 30}) {
		t.Fatalf("Not(age>=18) should reject 30")
	}
}

func TestBuildSpecification_NestedAndOr(t *testing.T) {
	// (age >= 18) AND ((name startsWith "A") OR (name startsWith "V"))
	in := graphql.FilterInput{
		And: []graphql.FilterInput{
			{Leaf: ageLeaf{Min: 18}},
			{Or: []graphql.FilterInput{
				{Leaf: nameLeaf{Prefix: "A"}},
				{Leaf: nameLeaf{Prefix: "V"}},
			}},
		},
	}
	s, _ := graphql.BuildSpecification(in, leafBuilder)
	if !s.IsSatisfiedBy(user{Name: "Alice", Age: 30}) {
		t.Fatalf("Alice/30 should satisfy")
	}
	if !s.IsSatisfiedBy(user{Name: "Vincent", Age: 30}) {
		t.Fatalf("Vincent/30 should satisfy")
	}
	if s.IsSatisfiedBy(user{Name: "Bob", Age: 30}) {
		t.Fatalf("Bob/30 should fail prefix branch")
	}
	if s.IsSatisfiedBy(user{Name: "Alice", Age: 10}) {
		t.Fatalf("Alice/10 should fail age branch")
	}
}

func TestBuildSpecification_EmptyReturnsNil(t *testing.T) {
	s, err := graphql.BuildSpecification[user](graphql.FilterInput{}, leafBuilder)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s != nil {
		t.Fatalf("empty input should return nil spec")
	}
}

func TestBuildSpecification_LeafErrorPropagates(t *testing.T) {
	in := graphql.FilterInput{
		And: []graphql.FilterInput{
			{Leaf: ageLeaf{Min: 18}},
			{Leaf: "garbage"},
		},
	}
	_, err := graphql.BuildSpecification(in, leafBuilder)
	if err == nil {
		t.Fatalf("expected leaf error to propagate")
	}
}
