package schema

import (
	"regexp"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Compile compiles a parsed schema expression (the value model of a schema
// body) rooted at the given path.
func Compile(shape any, path string) (s *Schema, cerr *errs.Error) {
	defer func() {
		if r := recover(); r != nil {
			f := r.(compileFail)
			s, cerr = nil, &f.err
		}
	}()
	return compileSchema(shape, path), nil
}

type compileFail struct{ err errs.Error }

func fail(code string) {
	panic(compileFail{errs.Error{Code: code, Line: 1, Col: 1}})
}

func compileSchema(shape any, path string) *Schema {
	obj, ok := shape.(*core.Object)
	if !ok {
		fail(errs.InvalidSchema)
	}
	if hasAbsentMember(obj) {
		fail(errs.EmptyMemberdef)
	}

	s := &Schema{Defs: map[string]*MemberDef{}}
	for i, m := range obj.Members {
		last := i == len(obj.Members)-1

		if m.Positional {
			name, ok := m.Value.(string)
			if !ok {
				fail(errs.InvalidKey)
			}
			if name == "*" && !m.Quoted {
				// A bare `*` opens the schema; it is legal only in last place.
				// A QUOTED "*" is an ordinary member named `*`.
				if !last || s.Open != nil {
					fail(errs.InvalidSchema)
				}
				s.Open = OpenAny
				continue
			}
			bare, opt, nul := name, false, false
			if !m.Quoted {
				bare, opt, nul = stripMarkers(name)
			}
			md := &MemberDef{Name: bare, Type: "any", Path: joinPath(path, bare), Optional: opt, Null: nul}
			addMember(s, md)
			continue
		}

		name, opt, nul := m.Key, false, false
		if !m.Quoted && name != "*" {
			name, opt, nul = stripMarkers(name)
		}
		if name == "*" && !m.Quoted {
			// Typed additional properties: `*: T` sets open AND adds a `*`
			// member; like the bare form it must come last.
			if !last || s.Open != nil {
				fail(errs.InvalidSchema)
			}
			md := compileMemberDef("*", m.Value, path, opt, nul)
			s.Open = md
			addMember(s, md)
			continue
		}
		md := compileMemberDef(name, m.Value, path, opt, nul)
		addMember(s, md)
	}
	// A schema declaring no members constrains nothing: `{}` and `{*}` are
	// the same open schema.
	if len(s.Names) == 0 && s.Open == nil {
		s.Open = OpenAny
	}
	return s
}

// hasAbsentMember reports an empty comma slot — a schema declares nothing
// there, so it is empty-memberdef.
func hasAbsentMember(obj *core.Object) bool {
	for i := range obj.Members {
		if obj.Members[i].Absent {
			return true
		}
	}
	return false
}

// stripMarkers removes the trailing `?` (optional) and `*` (nullable) markers
// from a bare name, in either order.
func stripMarkers(name string) (bare string, optional, null bool) {
	for len(name) > 0 {
		switch name[len(name)-1] {
		case '?':
			optional = true
		case '*':
			null = true
		default:
			return name, optional, null
		}
		name = name[:len(name)-1]
	}
	return name, optional, null
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// compileMemberDef compiles one member's definition value.
func compileMemberDef(name string, v any, parentPath string, opt, nul bool) *MemberDef {
	path := joinPath(parentPath, name)
	md := &MemberDef{Name: name, Path: path, Optional: opt, Null: nul}

	switch tv := v.(type) {
	case string:
		if strings.HasPrefix(tv, "$") {
			md.Type = "object"
			md.SchemaRef = tv
			return md
		}
		if !registeredTypes[tv] {
			fail(unusableTypeCode(tv))
		}
		md.Type = tv
		return md

	case *core.Object:
		if tn, ok := typedefTypeName(tv); ok {
			compileTypedef(md, tn, tv, path)
			return md
		}
		// Not a typedef form: an object BODY — a nested schema.
		md.Type = "object"
		md.Schema = compileSchema(tv, path)
		return md

	case []any:
		md.Type = "array"
		switch len(tv) {
		case 0:
			md.Of = &MemberDef{Type: "any", Null: true, Path: path}
		case 1:
			md.Of = compileArrayElem(tv[0], path)
		default:
			fail(errs.InvalidSchema)
		}
		return md
	}
	// A literal (number, bool, …) where a type is expected.
	fail(errs.UnknownType)
	return nil
}

// typedefTypeName decides whether a braced value is the OBJECT FORM of a
// typedef — `{string, minLen: 2}` or `{type: string, …}` — as opposed to a
// nested object body. The first positional member being a registered (or
// reserved) type name claims the typedef reading; so does a keyed `type`.
func typedefTypeName(obj *core.Object) (string, bool) {
	if len(obj.Members) > 0 && obj.Members[0].Positional {
		if s, ok := obj.Members[0].Value.(string); ok && (registeredTypes[s] || reservedTypes[s]) {
			return s, true
		}
		return "", false
	}
	if i := obj.Find("type"); i >= 0 && !obj.Members[i].Quoted {
		s, _ := obj.Members[i].Value.(string)
		return s, true
	}
	return "", false
}

// compileTypedef fills md from an object-form typedef.
func compileTypedef(md *MemberDef, typeName string, obj *core.Object, path string) {
	if !registeredTypes[typeName] {
		fail(unusableTypeCode(typeName))
	}
	md.Type = typeName
	if hasAbsentMember(obj) {
		fail(errs.EmptyMemberdef)
	}

	tdSchema := typedefSchemaFor(typeName)
	positionalPrefix := true

	for i, m := range obj.Members {
		key := m.Key
		if m.Positional {
			if i == 0 {
				continue // the type name itself, already read by typedefTypeName
			}
			// A positional entry binds to the memberdef schema's member at this
			// POSITION — `{bool, F}` is type, default. This used to fail as
			// unknown-member on the belief that a positional "declares nothing
			// it can keep", which the schema contradicts.
			if !positionalPrefix || i >= len(tdSchema) {
				fail(errs.UnknownMember)
			}
			key = tdSchema[i].name
		} else {
			positionalPrefix = false
		}
		switch key {
		case "type":
			continue
		case "optional":
			md.Optional, _ = m.Value.(bool)
		case "null":
			md.Null, _ = m.Value.(bool)
		case "default":
			if m.Value == nil {
				fail(errs.ForbiddenNull)
			}
			checkConstraintValue(typeName, "default", m.Value)
			md.HasDefault, md.Default = true, m.Value
			md.Keys = append(md.Keys, "default")
		case "choices":
			if _, ok := typedefKey(typeName, "choices"); !ok {
				fail(errs.UnknownMember)
			}
			arr, ok := m.Value.([]any)
			if !ok {
				fail(errs.ExpectedArray)
			}
			checkConstraintValue(typeName, "choices", arr)
			md.Choices = arr
			md.Keys = append(md.Keys, "choices")
		case "anyOf":
			if _, ok := typedefKey(typeName, "anyOf"); !ok {
				fail(errs.UnknownMember)
			}
			arr, ok := m.Value.([]any)
			if !ok {
				fail(errs.ExpectedArray)
			}
			for _, alt := range arr {
				md.AnyOf = append(md.AnyOf, compileOfDef(alt))
			}
			md.Keys = append(md.Keys, "anyOf")
		case "of":
			if _, ok := typedefKey(typeName, "of"); !ok {
				fail(errs.UnknownMember)
			}
			// The object-form element def starts a fresh path (matching the
			// reference: `of: {id: int}` compiles child paths from the root).
			md.Of = compileOfDef(m.Value)
		case "schema":
			if _, ok := typedefKey(typeName, "schema"); !ok {
				fail(errs.UnknownMember)
			}
			// `schema: $Name` is a reference like the short form `a: $Name`
			// (resolved lazily at validation); anything else must be a shape.
			if ref, ok := m.Value.(string); ok && strings.HasPrefix(ref, "$") {
				md.SchemaRef = ref
			} else {
				md.Schema = compileSchema(m.Value, path)
			}
		default:
			if _, ok := typedefKey(typeName, key); !ok {
				fail(errs.UnknownMember)
			}
			checkConstraintValue(typeName, key, m.Value)
			if md.Constraints == nil {
				md.Constraints = map[string]any{}
			}
			md.Constraints[key] = m.Value
			md.Keys = append(md.Keys, key)
		}
	}
	compilePattern(md)

}

// compilePattern builds the `pattern` regexp once, here, so that validation
// only ever READS a compiled schema.
//
// It used to be built lazily at the first match and cached onto the shared
// *MemberDef. That was an unsynchronized write to a schema reachable from the
// global plan cache, and the race detector confirms it: two goroutines calling
// io.Validate on the same struct type race on this field (validate.go read vs
// write). ADR 0003 D7 promises Marshal/Unmarshal are safe for concurrent use,
// so this was a correctness bug, not a tuning question.
//
// An INVALID pattern is deliberately not a compile error: the reference reports
// it per value as mismatched-pattern, so the failure is recorded here and
// raised at the same moment it always was.
func compilePattern(md *MemberDef) {
	pat, ok := md.Constraints["pattern"].(string)
	if !ok {
		return
	}
	flags := ""
	if f, ok := md.Constraints["flags"].(string); ok && strings.Contains(f, "i") {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pat)
	if err != nil {
		md.reBad = true
		return
	}
	md.re = re
}

// compileOfDef compiles the `of:` value of an object-form array typedef. Its
// element def carries an EMPTY name and a fresh path.
func compileOfDef(v any) *MemberDef {
	return compileMemberDef("", v, "", false, false)
}

// compileArrayElem compiles the element definition of the bracket form
// `[T]`: the element shares the array member's own path.
func compileArrayElem(v any, path string) *MemberDef {
	switch tv := v.(type) {
	case string:
		if strings.HasPrefix(tv, "$") {
			// `[$Name]` is an array of a referenced schema, resolved lazily at
			// validation exactly as the short member form `a: $Name` is.
			//
			// This used to fail with unknown-type, on a comment asserting the
			// reference implementation did the same. Re-probed 2026-09-06: it
			// does NOT — io-js2 accepts `books:[$B]` and projects the elements.
			// The playground's own "Multiple Sections" sample uses the form, so
			// io-go was rejecting a document the format advertises.
			return &MemberDef{Type: "object", SchemaRef: tv, Path: path}
		}
		if !registeredTypes[tv] {
			fail(unusableTypeCode(tv))
		}
		return &MemberDef{Type: tv, Path: path}
	case *core.Object:
		if tn, ok := typedefTypeName(tv); ok {
			md := &MemberDef{Path: path}
			compileTypedef(md, tn, tv, path)
			return md
		}
		return &MemberDef{Type: "object", Path: path, Schema: compileSchema(tv, path)}
	case []any:
		md := &MemberDef{Type: "array", Path: path}
		switch len(tv) {
		case 0:
			md.Of = &MemberDef{Type: "any", Null: true, Path: path}
		case 1:
			md.Of = compileArrayElem(tv[0], path)
		default:
			fail(errs.InvalidSchema)
		}
		return md
	}
	fail(errs.UnknownType)
	return nil
}
