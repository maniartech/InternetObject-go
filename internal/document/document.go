// Package document is the top of the pipeline: it parses a source text, binds
// each data section to its schema (explicit `$ref`, the default `$schema`, or
// the inline header schema), compiles schemas lazily in one place, validates
// records, and surfaces deferred literal errors. The result is a
// parser.Document whose records hold VALIDATED values.
package document

import (
	"strconv"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Doc is a loaded (parsed, bound, validated) document.
type Doc struct {
	*parser.Document
	Defs       *Definitions
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
	defs := NewDefinitions(pdoc.Header)
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
			SurfaceDeferred(def.Value, &herrs)
		}
		if len(herrs) > 0 {
			doc.Errors = append(doc.Errors, herrs[0])
			return doc
		}

		// Every NAMED schema is compiled here, whether or not anything
		// references it. io-go used to compile them lazily, so a header could
		// carry a `$Draft: {title: nosuchtype}` that nothing referenced and the
		// document parsed CLEAN — the reference rejects it with unknown-type
		// (probed 2026-09-06). Lazy compilation was also what made the writer
		// silently drop such a definition: it compiled in order to write, found
		// it broken, and skipped it. Compiling here fixes the divergence and
		// removes the writer's problem at the source rather than teaching the
		// writer to render shapes it cannot compile.
		for _, def := range pdoc.Header.Defs {
			if def.Kind != parser.DefSchema {
				continue
			}
			if _, cerr := defs.SchemaOf(def.Key); cerr != nil {
				doc.Errors = append(doc.Errors, *cerr)
				return doc
			}
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
				if verr := ResolveVars(rec, defs); verr != nil {
					e := *verr
					if e.Category == "" {
						e.Category = errs.CategoryOf(e.Code)
					}
					if sec.Collection {
						e.RecordIndex, e.Path = i, "$["+strconv.Itoa(i)+"]"
					}
					doc.AddSectionError(sec, e)
					sec.Records[i] = errorNodeFor(e)
					if !sec.Collection {
						return doc
					}
					continue
				}
				surfaceSectionDeferred(doc, sec, rec)
			}
			continue
		}
		for i, rec := range sec.Records {
			obj, ok := rec.(*core.Object)
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
				doc.AddSectionError(sec, verrs...)
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
			surfaceSectionDeferred(doc, sec, validated)
		}
	}
	return doc
}

// surfaceSectionDeferred reports a record's deferred literal faults as the
// section's own, so every route into a section's error list runs through
// AddSectionError.
func surfaceSectionDeferred(doc *Doc, sec *parser.Section, rec any) {
	var derrs []errs.Error
	SurfaceDeferred(rec, &derrs)
	doc.AddSectionError(sec, derrs...)
}

// errorNodeFor is THE conversion from an accumulated fault to the marker that
// stands in for the failed record inside projected data (ADR 0005 D4), so the
// marker and the error list can never disagree about what went wrong.
func errorNodeFor(e errs.Error) core.ErrorNode {
	return core.ErrorNode{
		Code: e.Code, Category: e.Category, Path: e.Path,
		RecordIndex: e.RecordIndex, Line: e.Line, Col: e.Col,
	}
}

// NewUnvalidated wraps a hand-built parser.Document for WRITING: each section
// is bound to its schema (so the writer can emit records positionally), but
// records are not validated — the builder is trusted to have produced
// conforming values. Used by the struct marshaler.
func NewUnvalidated(pdoc *parser.Document) (*Doc, *errs.Error) {
	defs := NewDefinitions(pdoc.Header)
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
// slice, a Definitions carrying two more maps, and a per-section schema map —
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
	defs := NewDefinitions(header)
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
			if _, ok := rec.(core.ErrorNode); ok {
				n++
			}
		}
	}
	return n < len(doc.Errors)
}

// sectionSchema resolves the schema a section is bound to, or nil when it has
// none.
func sectionSchema(sec *parser.Section, defs *Definitions) (*schema.Schema, *errs.Error) {
	if sec.SchemaName != "" {
		return defs.SchemaOf(sec.SchemaName)
	}
	if defs.Header == nil {
		return nil, nil
	}
	if _, ok := defs.Header.Schemas["schema"]; ok {
		return defs.SchemaOf("schema")
	}
	if defs.Header.Inline != nil {
		// The inline schema is cached on its own field — the compiled map is
		// keyed by declared names, and "" is a legal declared name (`~ $: x`).
		if defs.inline == nil {
			s, cerr := schema.Compile(defs.Header.Inline, "")
			if cerr != nil {
				return nil, cerr
			}
			defs.inline = s
		}
		return defs.inline, nil
	}
	return nil, nil
}

// SurfaceDeferred walks a record collecting the deferred malformed-literal
// errors parsing left behind (no schema masked or reported them).
func SurfaceDeferred(v any, out *[]errs.Error) {
	switch x := v.(type) {
	case core.ErrorValue:
		*out = append(*out, errs.Error{Code: x.Code, Line: x.Line, Col: x.Col})
	case *core.Object:
		for _, m := range x.Members {
			SurfaceDeferred(m.Value, out)
		}
	case []any:
		for _, e := range x {
			SurfaceDeferred(e, out)
		}
	}
}
