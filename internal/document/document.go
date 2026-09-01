// Package document is the top of the pipeline: it parses a source text, binds
// each data section to its schema (explicit `$ref`, the default `$schema`, or
// the inline header schema), compiles schemas lazily in one place, validates
// records, and surfaces deferred literal errors. The result is a
// parser.Document whose records hold VALIDATED values.
package document

import (
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
}

// Load parses and validates one document.
func Load(src string) *Doc {
	pdoc := parser.Parse(src)
	defs := newDefs(pdoc.Header)
	doc := &Doc{Document: pdoc, Defs: defs, SecSchemas: map[*parser.Section]*schema.Schema{}}

	// A fatal parse error abandons everything, as the reference does; the
	// error list already carries it.
	if hasFatalParse(pdoc) {
		return doc
	}

	for _, sec := range pdoc.Sections {
		sch, cerr := sectionSchema(sec, defs)
		if cerr != nil {
			doc.Errors = append(doc.Errors, *cerr)
			return doc // a broken binding is fatal, like a thrown compile error
		}
		doc.SecSchemas[sec] = sch
		if sch == nil {
			// No schema: the section's deferred literal errors surface as
			// themselves.
			for _, rec := range sec.Records {
				surfaceDeferred(rec, &doc.Errors)
			}
			continue
		}
		for i, rec := range sec.Records {
			obj, ok := rec.(*value.Object)
			if !ok {
				continue // an ErrorNode from parse recovery stays as it is
			}
			validated, verrs := schema.ValidateRecord(obj, sch, defs, sec.Collection)
			if len(verrs) > 0 {
				doc.Errors = append(doc.Errors, verrs...)
				sec.Records[i] = value.ErrorNode{Code: verrs[0].Code}
				if !sec.Collection {
					return doc // a bare record fails fast
				}
				continue
			}
			sec.Records[i] = validated
		}
	}
	return doc
}

// CompileSchemaString parses a schema definition string and compiles it — the
// schemaDef pipeline stage, used by the conformance suite and (later) the
// public API.
func CompileSchemaString(src string) (*schema.Schema, *errs.Error) {
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
}

func newDefs(h *parser.Header) *docDefs {
	return &docDefs{header: h, compiled: map[string]*schema.Schema{}, failed: map[string]*errs.Error{}}
}

// SchemaOf resolves and compiles the named schema.
func (d *docDefs) SchemaOf(name string) (*schema.Schema, *errs.Error) {
	name = strings.TrimPrefix(name, "$")
	if s, ok := d.compiled[name]; ok {
		return s, nil
	}
	if e, ok := d.failed[name]; ok {
		return nil, e
	}
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
		return d.SchemaOf(ref[1:])
	}
	s, cerr := schema.Compile(shape, "")
	if cerr != nil {
		d.failed[name] = cerr
		return nil, cerr
	}
	d.compiled[name] = s
	return s, nil
}

// Var resolves a variable by (sigil-less) name.
func (d *docDefs) Var(name string) (any, bool) {
	if d.header == nil {
		return nil, false
	}
	v, ok := d.header.Vars[name]
	return v, ok
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
		if s, ok := defs.compiled[""]; ok {
			return s, nil
		}
		s, cerr := schema.Compile(defs.header.Inline, "")
		if cerr != nil {
			return nil, cerr
		}
		defs.compiled[""] = s
		return s, nil
	}
	return nil, nil
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
