package document

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// Framed is a document whose HEADER has been parsed normally — it is small,
// read once, and defines the schema — while its DATA records are only framed
// (ADR 0007 phase 2). Nothing in the data has been decoded or boxed.
type Framed struct {
	Stream *tokenizer.Stream
	Raw    *parser.RawDoc
	Schema *schema.Schema // the schema the data section binds to, or nil
	Defs   schema.Defs    // for @variables and $refs inside records
}

// ParseFramed parses src's header and frames its data, or reports that it
// could not — in which case the caller uses Parse, which is the path that has
// always run.
//
// It declines the shapes framing does not own (see parser.FrameData) and, in
// addition, any document whose header does not resolve cleanly: a header
// fault must be reported by the normal path with its designated code, not
// swallowed here.
func ParseFramed(src string) (*Framed, bool) {
	s := tokenizer.Tokenize(src)

	// The header is everything before the first separator. Re-parsing just
	// that text is O(header), not O(document).
	sepAt := -1
	for i := range s.Tokens {
		if s.Tokens[i].Kind == tokenizer.KindSectionSep {
			sepAt = i
			break
		}
	}
	if sepAt < 0 {
		return nil, false
	}

	// An explicit `--- $Name` selector is not framed yet: the default-schema
	// resolution below covers the common case, and a selector needs the
	// section binding rules the tree path owns.
	if sepAt+1 < len(s.Tokens) {
		if t := s.Tokens[sepAt+1]; t.Kind == tokenizer.KindString &&
			(t.Sub == tokenizer.SubSectionName || t.Sub == tokenizer.SubSectionSchema) {
			return nil, false
		}
	}

	headerSrc := src[:s.Tokens[sepAt].Start]
	hdoc := parser.Parse(headerSrc + "\n---\n")
	if len(hdoc.Errors) > 0 {
		return nil, false // the normal path reports the header's fault
	}

	defs := newDefs(hdoc.Header)
	sec := &parser.Section{Name: "data"}
	sch, cerr := sectionSchema(sec, defs)
	if cerr != nil {
		return nil, false
	}
	if hdoc.Header != nil && len(hdoc.Header.Vars) > 0 {
		return nil, false // @variables resolve during the tree walk
	}

	raw, ok := parser.FrameData(s)
	if !ok {
		return nil, false
	}
	return &Framed{Stream: s, Raw: raw, Schema: sch, Defs: defs}, true
}

// SchemaMemberDefs exposes the compiled member definitions in schema order,
// which is what a binder walks alongside the framed members.
func SchemaMemberDefs(s *schema.Schema) ([]string, map[string]*schema.MemberDef) {
	if s == nil {
		return nil, nil
	}
	return s.Names, s.Defs
}

// IsSimpleSchema reports whether every declared member is a plain typed
// member with no constraints, defaults, choices, nested schema or reference —
// the shape a binder can check by looking at a token's kind alone. Anything
// else keeps the general validation path.
func IsSimpleSchema(s *schema.Schema) bool {
	if s == nil || s.Open != nil {
		return false
	}
	for _, name := range s.Names {
		md := s.Defs[name]
		if md == nil || len(md.Keys) > 0 || md.HasDefault || md.Choices != nil ||
			md.Schema != nil || md.SchemaRef != "" || md.AnyOf != nil ||
			len(md.Constraints) > 0 {
			return false
		}
		switch md.Type {
		case "string", "number", "int", "bool", "any":
		case "array":
			if md.Of == nil || len(md.Of.Keys) > 0 || md.Of.Schema != nil ||
				md.Of.SchemaRef != "" || len(md.Of.Constraints) > 0 {
				return false
			}
			switch md.Of.Type {
			case "string", "number", "int", "bool":
			default:
				return false
			}
		default:
			return false
		}
		if strings.ContainsAny(name, " \t") {
			return false
		}
	}
	return true
}
