// Package document is the top of the pipeline: it parses a source text, binds
// each data section to its schema (explicit `$ref`, the default `$schema`, or
// the inline header schema), compiles schemas lazily in one place, validates
// records, and surfaces deferred literal errors. The result is a
// parser.Document whose records hold VALIDATED values.
package document

import (
	"strconv"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// Doc is a loaded (parsed, bound, validated) document.
type Doc struct {
	*parser.Document
	Defs       *docDefs
	SecSchemas map[*parser.Section]*schema.Schema
	// cachedHeader, when non-empty, is the already-rendered header text. Set
	// only by NewWithSchemaHeader, whose caller has rendered it once for a
	// schema it reuses.
	cachedHeader string
	// soloSchema is the schema every section binds to, for a document that has
	// exactly one of each. It exists so the marshal path need not allocate a
	// map to state a fact it already knows.
	soloSchema *schema.Schema
}

// schemaFor is THE lookup for a section's schema, so the one-section marshal
// path and the parsed path answer the question the same way.
func (d *Doc) schemaFor(sec *parser.Section) *schema.Schema {
	if d.soloSchema != nil {
		return d.soloSchema
	}
	return d.SecSchemas[sec]
}

// Load parses and validates one document, binding each section to the schema
// its own header names.
func Parse(src string) *Doc { return parse(src, nil) }

// ParseWith parses and validates one document against an ALREADY COMPILED
// schema, which overrides whatever the document's own header would bind (ADR
// 0004 D5: attached > header > tag-derived). The header is still read, so
// `@variables` and `$refs` it defines stay resolvable inside records.
func ParseWith(src string, override *schema.Schema) *Doc { return parse(src, override) }

func parse(src string, override *schema.Schema) *Doc {
	pdoc := parser.Parse(src)
	defs := newDefs(pdoc.Header)
	doc := &Doc{Document: pdoc, Defs: defs, SecSchemas: map[*parser.Section]*schema.Schema{}}

	// A fatal parse error abandons everything, as the reference does; the
	// error list already carries it.
	if hasFatalParse(pdoc) {
		return doc
	}

	// A malformed literal in a header definition is fatal — the reference
	// throws invalid-number/invalid-bigint/… while reading the header, so a
	// deferred error there never survives into a "clean" document (found by
	// the byte fuzzer: `~A:0B---` parsed clean holding an ErrorValue).
	if pdoc.Header != nil {
		var herrs []errs.Error
		for _, def := range pdoc.Header.Defs {
			surfaceDeferred(def.Value, &herrs)
		}
		if len(herrs) > 0 {
			doc.Errors = append(doc.Errors, herrs[0])
			return doc
		}
	}

	for _, sec := range pdoc.Sections {
		sch := override
		if sch == nil {
			var cerr *errs.Error
			sch, cerr = sectionSchema(sec, defs)
			if cerr != nil {
				doc.Errors = append(doc.Errors, *cerr)
				return doc // a broken binding is fatal, like a thrown compile error
			}
		}
		doc.SecSchemas[sec] = sch
		if sch == nil {
			// No schema: variable references resolve in place, and deferred
			// literal errors surface as themselves.
			for i, rec := range sec.Records {
				if verr := resolveVars(rec, defs); verr != nil {
					e := *verr
					if e.Category == "" {
						e.Category = errs.CategoryOf(e.Code)
					}
					if sec.Collection {
						e.RecordIndex, e.Path = i, "$["+strconv.Itoa(i)+"]"
					}
					doc.Errors = append(doc.Errors, e)
					sec.Records[i] = errorNodeFor(e)
					if !sec.Collection {
						return doc
					}
					continue
				}
				surfaceDeferred(rec, &doc.Errors)
			}
			continue
		}
		for i, rec := range sec.Records {
			obj, ok := rec.(*value.Object)
			if !ok {
				continue // an ErrorNode from parse recovery stays as it is
			}
			path := "$"
			recIndex := -1
			if sec.Collection {
				path, recIndex = "$["+strconv.Itoa(i)+"]", i
			}
			validated, verrs := schema.ValidateRecordAt(obj, sch, defs, sec.Collection, path)
			if len(verrs) > 0 {
				for j := range verrs {
					verrs[j].RecordIndex = recIndex
				}
				doc.Errors = append(doc.Errors, verrs...)
				sec.Records[i] = errorNodeFor(verrs[0])
				if !sec.Collection {
					return doc // a bare record fails fast
				}
				continue
			}
			sec.Records[i] = validated
			// A deferred malformed-literal that flowed through an untyped
			// (`any`) subtree survives validation unmasked; the reference
			// throws its code (typed members mask with expected-* instead —
			// ISSUE-23). Surface it like the schema-less route does.
			surfaceDeferred(validated, &doc.Errors)
		}
	}
	return doc
}

// errorNodeFor is THE conversion from an accumulated fault to the marker that
// stands in for the failed record inside projected data (ADR 0005 D4), so the
// marker and the error list can never disagree about what went wrong.
func errorNodeFor(e errs.Error) value.ErrorNode {
	return value.ErrorNode{
		Code: e.Code, Category: e.Category, Path: e.Path,
		RecordIndex: e.RecordIndex, Line: e.Line, Col: e.Col,
	}
}

// NewUnvalidated wraps a hand-built parser.Document for WRITING: each section
// is bound to its schema (so the writer can emit records positionally), but
// records are not validated — the builder is trusted to have produced
// conforming values. Used by the struct marshaler.
func NewUnvalidated(pdoc *parser.Document) (*Doc, *errs.Error) {
	defs := newDefs(pdoc.Header)
	doc := &Doc{Document: pdoc, Defs: defs, SecSchemas: map[*parser.Section]*schema.Schema{}}
	for _, sec := range pdoc.Sections {
		sch, cerr := sectionSchema(sec, defs)
		if cerr != nil {
			return nil, cerr
		}
		doc.SecSchemas[sec] = sch
	}
	return doc, nil
}

// NewWithSchema wraps a hand-built parser.Document for WRITING against an
// already-compiled schema: every section binds to it (records emit
// positionally) and it is written as the document header. Records are not
// re-validated here — the caller validates.
// SchemaHeaderText renders the header a schema-only document carries. It is a
// PURE FUNCTION of the compiled schema — NewWithSchema builds a header holding
// nothing but that schema — which is what makes it safe to compute once and
// reuse. Rendering it is not cheap: it was ~45% of the allocations of every
// single-record MarshalWith, for a value that never changes.
func SchemaHeaderText(s *schema.Schema) string {
	return NewWithSchema(&parser.Document{}, s).writeHeader()
}

// WriteSchemaDoc renders an already-rendered header followed by one section's
// records, and allocates nothing else.
//
// The parsed-document scaffolding — a parser.Header carrying a map and a defs
// slice, a docDefs carrying two more maps, and a per-section schema map —
// exists so a header can be RENDERED and names RESOLVED. A marshal has already
// rendered its header (cached on the schema) and has no names to resolve, so it
// was paying for four maps it never read: ~38% of the allocations of a
// single-record MarshalWith, measured 2026-09-05.
func WriteSchemaDoc(header string, sec *parser.Section, s *schema.Schema) string {
	d := &Doc{
		Document:     &parser.Document{Sections: []*parser.Section{sec}},
		cachedHeader: header,
		soloSchema:   s,
	}
	return d.String()
}

func NewWithSchema(pdoc *parser.Document, s *schema.Schema) *Doc {
	header := &parser.Header{
		Schemas: map[string]any{"schema": s},
		Defs:    []parser.HeaderDef{{Kind: parser.DefSchema, Key: "schema", Value: s}},
	}
	pdoc.Header = header
	defs := newDefs(header)
	// The name is already compiled; seeding the cache IS the statement that
	// this schema needs no shape to resolve.
	defs.compiled["schema"] = s
	doc := &Doc{Document: pdoc, Defs: defs, SecSchemas: map[*parser.Section]*schema.Schema{}}
	for _, sec := range pdoc.Sections {
		doc.SecSchemas[sec] = s
	}
	return doc
}

// CompileSchemaString parses a schema definition string and compiles it — the
// schemaDef pipeline stage, used by the conformance suite and (later) the
// public API.
func ParseSchema(src string) (*schema.Schema, *errs.Error) {
	doc := parser.Parse(src)
	if len(doc.Errors) > 0 {
		e := doc.Errors[0]
		return nil, &e
	}
	var root any
	if len(doc.Sections) == 1 && len(doc.Sections[0].Records) == 1 {
		root = doc.Sections[0].Records[0]
	}
	return schema.Compile(root, "")
}

// hasFatalParse reports whether parsing was abandoned (as opposed to
// record-recovered errors, which leave ErrorNodes behind and let the rest of
// the document proceed).
func hasFatalParse(doc *parser.Document) bool {
	if len(doc.Errors) == 0 {
		return false
	}
	// Recovered faults always leave an ErrorNode; a fatal fault leaves none.
	n := 0
	for _, sec := range doc.Sections {
		for _, rec := range sec.Records {
			if _, ok := rec.(value.ErrorNode); ok {
				n++
			}
		}
	}
	return n < len(doc.Errors)
}

// docDefs resolves names for validation, compiling named schemas lazily and
// exactly once.
type docDefs struct {
	header   *parser.Header
	compiled map[string]*schema.Schema
	failed   map[string]*errs.Error
	inline   *schema.Schema // the compiled bare-expression header schema
}

func newDefs(h *parser.Header) *docDefs {
	return &docDefs{header: h, compiled: map[string]*schema.Schema{}, failed: map[string]*errs.Error{}}
}

// SchemaOf resolves and compiles the named schema, chasing `$ref` aliases. A
// self- or mutually-referential alias chain is invalid-definition — the
// reference crashes with a bare stack overflow here (upstream finding), and a
// designated code is the non-crashing spelling of that behavior.
func (d *docDefs) SchemaOf(name string) (*schema.Schema, *errs.Error) {
	name = strings.TrimPrefix(name, "$")
	seen := map[string]bool{}
	for {
		if s, ok := d.compiled[name]; ok {
			return s, nil
		}
		if e, ok := d.failed[name]; ok {
			return nil, e
		}
		if seen[name] {
			e := &errs.Error{Code: errs.InvalidDefinition, Line: 1, Col: 1}
			d.failed[name] = e
			return nil, e
		}
		seen[name] = true
		var shape any
		if d.header != nil {
			if v, ok := d.header.Schemas[name]; ok {
				shape = v
			}
		}
		if shape == nil {
			e := &errs.Error{Code: errs.UndefinedSchema, Line: 1, Col: 1}
			d.failed[name] = e
			return nil, e
		}
		// A named schema may itself be a `$ref` to another one.
		if ref, ok := shape.(string); ok && strings.HasPrefix(ref, "$") {
			name = strings.TrimPrefix(ref, "$")
			continue
		}
		s, cerr := schema.Compile(shape, "")
		if cerr != nil {
			d.failed[name] = cerr
			return nil, cerr
		}
		d.compiled[name] = s
		return s, nil
	}
}

// Var resolves a variable by (sigil-less) name, chasing @-references so a
// definition may name one parsed later. A missing name is undefined-variable;
// a self- or mutually-referential chain is invalid-definition.
func (d *docDefs) Var(name string) (any, *errs.Error) {
	seen := map[string]bool{}
	for {
		if d.header == nil {
			return nil, &errs.Error{Code: errs.UndefinedVariable, Line: 1, Col: 1}
		}
		if seen[name] {
			return nil, &errs.Error{Code: errs.InvalidDefinition, Line: 1, Col: 1}
		}
		seen[name] = true
		v, ok := d.header.Vars[name]
		if !ok {
			return nil, &errs.Error{Code: errs.UndefinedVariable, Line: 1, Col: 1}
		}
		if s, ok := v.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
			name = s[1:]
			continue
		}
		return v, nil
	}
}

// sectionSchema resolves the schema a section is bound to, or nil when it has
// none.
func sectionSchema(sec *parser.Section, defs *docDefs) (*schema.Schema, *errs.Error) {
	if sec.SchemaName != "" {
		return defs.SchemaOf(sec.SchemaName)
	}
	if defs.header == nil {
		return nil, nil
	}
	if _, ok := defs.header.Schemas["schema"]; ok {
		return defs.SchemaOf("schema")
	}
	if defs.header.Inline != nil {
		// The inline schema is cached on its own field — the compiled map is
		// keyed by declared names, and "" is a legal declared name (`~ $: x`).
		if defs.inline == nil {
			s, cerr := schema.Compile(defs.header.Inline, "")
			if cerr != nil {
				return nil, cerr
			}
			defs.inline = s
		}
		return defs.inline, nil
	}
	return nil, nil
}

// resolveVars resolves every @-string VALUE in a record in place — quoted or
// open, by design references in any string form (io-test-cases FINDINGS #3).
// Keys stay literal. Returns the first resolution error.
func resolveVars(v any, defs *docDefs) *errs.Error {
	switch x := v.(type) {
	case *value.Object:
		for i := range x.Members {
			mv := x.Members[i].Value
			if s, ok := mv.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
				r, verr := defs.Var(s[1:])
				if verr != nil {
					return verr
				}
				x.Members[i].Value = r
				continue
			}
			if verr := resolveVars(mv, defs); verr != nil {
				return verr
			}
		}
	case []any:
		for i, e := range x {
			if s, ok := e.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
				r, verr := defs.Var(s[1:])
				if verr != nil {
					return verr
				}
				x[i] = r
				continue
			}
			if verr := resolveVars(e, defs); verr != nil {
				return verr
			}
		}
	}
	return nil
}

// surfaceDeferred walks a record collecting the deferred malformed-literal
// errors parsing left behind (no schema masked or reported them).
func surfaceDeferred(v any, out *[]errs.Error) {
	switch x := v.(type) {
	case value.ErrorValue:
		*out = append(*out, errs.Error{Code: x.Code, Line: x.Line, Col: x.Col})
	case *value.Object:
		for _, m := range x.Members {
			surfaceDeferred(m.Value, out)
		}
	case []any:
		for _, e := range x {
			surfaceDeferred(e, out)
		}
	}
}
