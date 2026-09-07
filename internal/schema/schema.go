// Package schema compiles a parsed schema expression into a normalized,
// order-preserving description of members and their constraints, and validates
// records against it.
//
// LAYOUT. One concern per file, and one file per IO TYPE — the same shape
// io-js2 uses in src/schema/types/. A type's memberdef schema (what a
// `{string, minLen: 2}` typedef must satisfy) sits next to the validator that
// enforces it, so everything about `string` is in type-string.go and nothing
// else is.
//
//	schema.go      the compiled Schema
//	member-def.go  one member's compiled definition
//	family.go      type families, registered names
//	typedef.go     a typedef IS a record; each type declares its schema
//	compile.go     parsed expression -> compiled schema
//	validate.go    the record/object algorithm
//	type-*.go      one per type: its memberdef schema and its validator
package schema

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Schema is one compiled schema: ordered member names, their definitions, and
// the open state.
type Schema struct {
	Names []string
	Defs  map[string]*MemberDef
	// Index maps a declared name to its position in Names, so validation can
	// address members by slice index instead of allocating per-record maps
	// (ADR 0006 P2). Built once, here, at compile time.
	Index map[string]int
	// Open: nil = closed; OpenAny = any additional members; a *MemberDef =
	// additional members must match it.
	Open any
}

// OpenAny marks a schema opened by a bare `*`.
type openAny struct{}

var OpenAny = openAny{}

func addMember(s *Schema, md *MemberDef) {
	if _, dup := s.Defs[md.Name]; dup {
		fail(errs.DuplicateMember)
	}
	if s.Index == nil {
		s.Index = map[string]int{}
	}
	s.Index[md.Name] = len(s.Names)
	s.Names = append(s.Names, md.Name)
	s.Defs[md.Name] = md
}
