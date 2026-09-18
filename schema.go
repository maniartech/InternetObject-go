package internetobject

import (
	"reflect"
	"slices"
	"sync"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Schema is a compiled schema definition.
type Schema struct {
	s *schema.Schema
	// The document header this schema writes, rendered at most once.
	//
	// MarshalWith used to re-render it on EVERY call — ~45% of the allocations
	// of a single-record marshal, for text that cannot change, since a compiled
	// schema is read-only (ADR 0009 D1).
	//
	// Lazily, because Schema() and SchemaOf() mint a wrapper per call and must
	// not pay for a header nobody marshals; and through a sync.Once, because a
	// *Schema is explicitly meant to be compiled once and shared across
	// goroutines. That is the distinction the pattern-regexp race turned on:
	// the bug there was an UNSYNCHRONISED write to shared state, not a lazy one.
	headerOnce sync.Once
	headerText string

	// fast is what MarshalWith's direct encoder needs for each struct type
	// written against this schema (reflect.Type → *withPlan), worked out once.
	// It lives here rather than in a global cache so it is freed with the
	// schema: a cache keyed by schema would pin every schema ever used.
	fast sync.Map
}

// withPlan is the direct encoder's plan for one struct type against one schema.
type withPlan struct {
	ok     bool         // the type can be written directly against the schema
	header string       // the schema's header and separator
	checks []fieldCheck // what to ask the validator per field
}

// fastFor returns the direct encoder's plan for writing t against s. It holds
// when t's fields are s's members in s's order — MarshalWith emits them
// positionally in the schema's order — and every field passes fastCheck
// against s's definitions. An open schema is no obstacle: a struct has no
// members beyond its fields, so the tree writes it the same way. That now
// includes the TYPED `*: T` form, which since ADR 0012 is not in Names and so
// no longer fails the length check below — a region this path used to decline.
func (s *Schema) fastFor(t reflect.Type, plan *structPlan) *withPlan {
	if w, ok := s.fast.Load(t); ok {
		return w.(*withPlan)
	}
	w := &withPlan{}
	sameOrder := slices.EqualFunc(s.s.Names, plan.fields, func(name string, f fieldPlan) bool {
		return name == f.name
	})
	if plan.shapeOK && sameOrder {
		if w.checks, w.ok = fastChecks(t, plan, s.s); w.ok {
			w.header = "---\n"
			if h := s.header(); h != "" {
				w.header = h + "\n---\n"
			}
		}
	}
	actual, _ := s.fast.LoadOrStore(t, w)
	return actual.(*withPlan)
}

// header renders the schema's document header at most once per Schema.
func (s *Schema) header() string {
	s.headerOnce.Do(func() { s.headerText = document.SchemaHeaderText(s.s) })
	return s.headerText
}

// newSchema wraps a compiled schema. Every *Schema the package hands out is
// built here, so the header cache cannot be missed.
func newSchema(s *schema.Schema) *Schema { return &Schema{s: s} }

// ParseSchema compiles a schema definition string, e.g.
// "name: string, age: {int, min: 0}". Compilation fails fast: the error is
// an ErrorList holding the one designated fault.
func ParseSchema(def string) (*Schema, error) {
	s, cerr := document.ParseSchema(def)
	if cerr != nil {
		return nil, ErrorList{toError(*cerr)}
	}
	return newSchema(s), nil
}

// MemberNames returns the schema's member names in declaration order.
func (s *Schema) MemberNames() []string {
	// The wildcard is openness, not a member, and is not in Names (ADR 0012).
	// A schema with no members returns an EMPTY slice, never nil.
	names := make([]string, len(s.s.Names))
	copy(names, s.s.Names)
	return names
}

// Open reports whether the schema accepts undeclared members.
func (s *Schema) Open() bool { return s.s.Open != nil }
