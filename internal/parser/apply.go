package parser

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The parse-stage schema binder. Full compilation and validation are later
// pipeline stages; at parse time a bound schema does exactly three things:
//
//  1. maps a record's positional members onto the schema's member names;
//  2. resolves every $reference the schema shape carries (a name that
//     resolves to nothing is undefined-schema — found at bind time, since
//     resolution is lazy and lives in exactly one place);
//  3. leaves types, constraints and markers alone for the schema/validation
//     stages.
func (p *parser) apply() {
	if len(p.doc.Errors) > 0 {
		// A structurally broken document is not bound; codes stay exact.
		return
	}
	for _, sec := range p.doc.Sections {
		shape := p.sectionSchema(sec)
		if shape == nil {
			continue
		}
		p.checkRefs(shape, map[string]bool{})
		names := schemaMemberNames(shape)
		for _, rec := range sec.Records {
			if obj, ok := rec.(*value.Object); ok {
				mapPositional(obj, names)
			}
		}
	}
}

// sectionSchema resolves the schema shape a section is bound to: its explicit
// `$ref`, else the header's default (`$schema` or the inline header schema).
// An explicit reference that resolves to nothing is undefined-schema.
func (p *parser) sectionSchema(sec *Section) any {
	h := p.doc.Header
	if sec.SchemaName != "" {
		if h != nil {
			if s, ok := h.Schemas[sec.SchemaName]; ok {
				return s
			}
		}
		p.doc.Errors = append(p.doc.Errors, errs.Error{Code: errs.UndefinedSchema, Line: 1, Col: 1})
		return nil
	}
	if h == nil {
		return nil
	}
	if s, ok := h.Schemas["schema"]; ok {
		return s
	}
	return h.Inline
}

// checkRefs walks a schema shape and resolves every $reference against the
// header's named schemas, following references into the shapes they name.
func (p *parser) checkRefs(shape any, visited map[string]bool) {
	switch v := shape.(type) {
	case string:
		if !strings.HasPrefix(v, "$") || len(v) < 2 {
			return
		}
		name := v[1:]
		if visited[name] {
			return // a recursive reference is legal; it resolves lazily
		}
		visited[name] = true
		if p.doc.Header != nil {
			if s, ok := p.doc.Header.Schemas[name]; ok {
				p.checkRefs(s, visited)
				return
			}
		}
		p.doc.Errors = append(p.doc.Errors, errs.Error{Code: errs.UndefinedSchema, Line: 1, Col: 1})
	case *value.Object:
		for i := range v.Members {
			p.checkRefs(v.Members[i].Value, visited)
		}
	case []any:
		for _, e := range v {
			p.checkRefs(e, visited)
		}
	}
}

// schemaMemberNames extracts the ordered member names of a schema shape:
// the key of a keyed member, or the bare-name value of a positional one, with
// the `?` and `*` markers stripped from bare (unquoted) names — a quoted name
// is literal. The wildcard `*` declares openness, not a member.
func schemaMemberNames(shape any) []string {
	obj, ok := shape.(*value.Object)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(obj.Members))
	for _, m := range obj.Members {
		switch {
		case !m.Positional:
			name := m.Key
			if !m.Quoted {
				name = strings.TrimRight(name, "?*")
			}
			names = append(names, name)
		default:
			s, ok := m.Value.(string)
			if !ok || s == "*" {
				continue
			}
			names = append(names, strings.TrimRight(s, "?*"))
		}
	}
	return names
}

// mapPositional renames a record's positional members after the schema's
// member names, by member index. Members past the declared list keep their
// position; the validation stage owns rejecting them.
func mapPositional(obj *value.Object, names []string) {
	for i := range obj.Members {
		if obj.Members[i].Positional && i < len(names) {
			obj.Members[i].Key = names[i]
			obj.Members[i].Positional = false
		}
	}
}
