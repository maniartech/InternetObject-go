package internetobject

import (
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
		return nil, ErrorList{{Code: cerr.Code, Line: int(cerr.Line), Col: int(cerr.Col)}}
	}
	return newSchema(s), nil
}

// MemberNames returns the schema's member names in declaration order.
func (s *Schema) MemberNames() []string {
	names := make([]string, 0, len(s.s.Names))
	for _, n := range s.s.Names {
		if n != "*" {
			names = append(names, n)
		}
	}
	return names
}

// Open reports whether the schema accepts undeclared members.
func (s *Schema) Open() bool { return s.s.Open != nil }
