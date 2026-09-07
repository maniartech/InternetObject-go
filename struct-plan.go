package internetobject

import (
	"reflect"
	"strings"
	"sync"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

type structPlan struct {
	fields   []fieldPlan
	byName   map[string]int // member name → index in fields, built once
	shape    *core.Object   // the derived schema shape, as parsed text would be
	compiled *schema.Schema // the shape, compiled once
	validate bool           // any field (own or nested) carries a `schema` tag
	fastOK   bool           // every member can be WRITTEN without the tree
	lazyOK   bool           // every member can be READ from a token span
}

var planCache sync.Map // reflect.Type → *structPlan

func planFor(t reflect.Type) (*structPlan, error) {
	if p, ok := planCache.Load(t); ok {
		return p.(*structPlan), nil
	}
	p, err := buildPlan(t, map[reflect.Type]bool{})
	if err != nil {
		return nil, err
	}
	planCache.Store(t, p)
	return p, nil
}

func buildPlan(t reflect.Type, visiting map[reflect.Type]bool) (*structPlan, error) {
	if visiting[t] {
		return nil, &MarshalError{Path: t.String(), Msg: "recursive struct types are not supported"}
	}
	visiting[t] = true
	defer delete(visiting, t)

	p := &structPlan{shape: &core.Object{}}
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || f.Anonymous {
			continue // embedded structs contribute through their visible fields
		}
		name, opts, skip := parseTag(f)
		if skip {
			continue
		}
		fp := fieldPlan{
			name:  name,
			index: f.Index,
			at:    -1,
			// `optional` is the schema fact (the member may be absent — IO's
			// `name?`); `omitempty` is the json-familiar encoding behavior
			// (skip the zero value on output), which requires optionality.
			optional: opts["optional"] || opts["omitempty"],
			omitZero: opts["omitempty"],
			nullable: f.Type.Kind() == reflect.Pointer,
		}
		switch {
		case opts["date"]:
			fp.kind = "date"
		case opts["time"]:
			fp.kind = "time"
		}
		var ann any
		var err error
		if tag, ok := f.Tag.Lookup("schema"); ok {
			// The `schema` tag holds the member's IO type annotation verbatim
			// — exactly what a schema would carry after `name:`.
			ann, err = annotationShape(tag, t.String()+"."+f.Name)
			p.validate = true
		} else {
			ann, err = annotationFor(f.Type, fp.kind, visiting, &p.validate)
		}
		if err != nil {
			return nil, err
		}
		if isPlainMemberName(fp.name) {
			key := fp.name
			if fp.optional {
				key += "?"
			}
			if fp.nullable {
				key += "*"
			}
			p.shape.Members = append(p.shape.Members, core.Member{Key: key, Value: ann})
		} else {
			// A name needing quotes cannot carry the short markers; the flags
			// move into the object-form typedef.
			if fp.optional || fp.nullable {
				ann = objectFormWithFlags(ann, fp.optional, fp.nullable)
			}
			p.shape.Members = append(p.shape.Members, core.Member{Key: fp.name, Quoted: true, Value: ann})
		}
		if len(f.Index) == 1 {
			fp.at = f.Index[0] // the common case: one cheap field lookup
		}
		fp.enc = encKindOf(f.Type)
		if fp.enc == encSlice {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			fp.elem = encKindOf(ft.Elem())
		}
		p.fields = append(p.fields, fp)
	}
	p.byName = make(map[string]int, len(p.fields))
	for i, f := range p.fields {
		p.byName[f.name] = i
	}
	compiled, cerr := schema.Compile(p.shape, "")
	if cerr != nil {
		return nil, &MarshalError{Path: t.String(), Msg: "derived schema does not compile: " + cerr.Code}
	}
	p.compiled = compiled
	p.fastOK = fastEligible(t, p)
	p.lazyOK = lazyEligible(p)
	return p, nil
}

func parseTag(f reflect.StructField) (name string, opts map[string]bool, skip bool) {
	tag := f.Tag.Get("io")
	if tag == "-" {
		return "", nil, true
	}
	name = f.Name
	opts = map[string]bool{}
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	}
	for _, o := range parts[1:] {
		opts[o] = true
	}
	return name, opts, false
}

// annotationFor derives the IO type annotation for one Go type — the value a
// parsed schema would hold in that member position. tagged is set when the
// subtree carries a `schema` tag anywhere, so the owning plan validates.
func annotationFor(t reflect.Type, kind string, visiting map[reflect.Type]bool, tagged *bool) (any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == bigIntType.Elem():
		return "bigint", nil
	case t == decimalType:
		return "decimal", nil
	case t == timeType:
		if kind != "" {
			return kind, nil
		}
		return "datetime", nil
	case t == bytesType, t == anyType:
		// Binary is a value-level fact with no schema type of its own, so the
		// derived schema admits it as `any`.
		return "any", nil
	}
	switch t.Kind() {
	case reflect.String:
		return "string", nil
	case reflect.Bool:
		return "bool", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int", nil
	case reflect.Float32, reflect.Float64:
		return "number", nil
	case reflect.Slice, reflect.Array:
		elem, err := annotationFor(t.Elem(), "", visiting, tagged)
		if err != nil {
			return nil, err
		}
		return []any{elem}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, &MarshalError{Path: t.String(), Msg: "map keys must be strings"}
		}
		elem, err := annotationFor(t.Elem(), "", visiting, tagged)
		if err != nil {
			return nil, err
		}
		return &core.Object{Members: []core.Member{{Key: "*", Value: elem}}}, nil
	case reflect.Struct:
		sub, err := buildPlan(t, visiting)
		if err != nil {
			return nil, err
		}
		if sub.validate {
			*tagged = true
		}
		return sub.shape, nil
	case reflect.Interface:
		return "any", nil
	}
	return nil, &MarshalError{Path: t.String(), Msg: "unsupported type"}
}

// annotationShape parses a `schema` struct tag: the member's type annotation
// in the format's own syntax (`{int, min: 0, max: 130}`, `[string]`, `$Ref`,
// `{string, choices: [a, b]}`). An unbraced constraint list is braced for
// convenience, so `schema:"int, min: 0"` also works. The shape is compiled
// immediately so a bad tag fails at the type's first use with the designated
// code.
func annotationShape(tag, fieldPath string) (any, error) {
	text := strings.TrimSpace(tag)
	if text == "" {
		return nil, &MarshalError{Path: fieldPath, Msg: "empty schema tag"}
	}
	if text[0] != '{' && text[0] != '[' && strings.ContainsRune(text, ',') {
		text = "{" + text + "}"
	}
	pdoc := parser.Parse("x: " + text)
	if len(pdoc.Errors) > 0 {
		return nil, &MarshalError{Path: fieldPath, Msg: "invalid schema tag: " + pdoc.Errors[0].Code}
	}
	var rec *core.Object
	if len(pdoc.Sections) == 1 && len(pdoc.Sections[0].Records) == 1 {
		rec, _ = pdoc.Sections[0].Records[0].(*core.Object)
	}
	if rec == nil || len(rec.Members) != 1 || rec.Members[0].Key != "x" {
		return nil, &MarshalError{Path: fieldPath, Msg: "invalid schema tag: not a single type annotation"}
	}
	shape := rec.Members[0].Value
	if _, cerr := schema.Compile(&core.Object{Members: []core.Member{{Key: "x", Value: shape}}}, ""); cerr != nil {
		return nil, &MarshalError{Path: fieldPath, Msg: "invalid schema tag: " + cerr.Code}
	}
	return shape, nil
}

// objectFormWithFlags rewrites a type annotation as the object-form typedef
// carrying explicit optional/"null" flags — the only spelling a QUOTED member
// name can use (quoted names never strip `?`/`*` markers).
func objectFormWithFlags(ann any, optional, nullable bool) *core.Object {
	out := &core.Object{}
	switch tv := ann.(type) {
	case string:
		out.Members = append(out.Members, core.Member{Positional: true, Value: tv})
	case []any:
		out.Members = append(out.Members,
			core.Member{Positional: true, Value: "array"})
		var elem any = "any"
		if len(tv) == 1 {
			elem = tv[0]
		}
		out.Members = append(out.Members, core.Member{Key: "of", Value: elem})
	case *core.Object:
		out.Members = append(out.Members,
			core.Member{Positional: true, Value: "object"},
			core.Member{Key: "schema", Value: tv})
	}
	if optional {
		out.Members = append(out.Members, core.Member{Key: "optional", Value: true})
	}
	if nullable {
		out.Members = append(out.Members, core.Member{Key: "null", Quoted: true, Value: true})
	}
	return out
}

// isPlainMemberName reports a name that survives the bare-key grammar with
// optional/null markers appended (markers are stripped from BARE keys only).
func isPlainMemberName(s string) bool {
	if s == "" || s == "*" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return !(s[0] >= '0' && s[0] <= '9')
}

// ── schema derivation ──────────────────────────────────────────────────────

// isModelStruct reports the value-model structs, which marshal as VALUES, not
// as records with fields.
func isModelStruct(t reflect.Type) bool {
	return t == decimalType || t == timeType
}

func isStructElem(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && !isModelStruct(t)
}
