// Package schema compiles a parsed schema expression into a normalized,
// order-preserving description of members and their constraints — the stage
// between parsing and validation. Compilation carries constraints; it does
// not enforce them against data (that is validation's job).
//
// Compilation FAILS FAST: the first fault aborts with one designated code,
// unlike document parsing, which accumulates.
package schema

import (
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/value"
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

// MemberDef is one member's compiled definition.
type MemberDef struct {
	Name     string
	Type     string
	Path     string
	Optional bool
	Null     bool

	HasDefault bool
	Default    any
	Choices    []any // nil when not declared

	Of        *MemberDef   // array element definition
	Schema    *Schema      // nested schema, for object bodies
	SchemaRef string       // a `$name` reference, resolved lazily at validation
	AnyOf     []*MemberDef // union alternatives, for `{any, anyOf: [...]}`

	// Constraints holds the carried per-type constraint keys (min, max,
	// multipleOf, len, minLen, maxLen, pattern, precision, scale, …), and
	// Keys their declaration order (default/choices/anyOf included), which the
	// writer reproduces.
	Constraints map[string]any
	Keys        []string

	// The `pattern` constraint, compiled once by compilePattern at compile
	// time. Written only there; validation reads. reBad records a pattern that
	// would not compile, whose fault surfaces per value at validation.
	re    *regexp.Regexp
	reBad bool
}

// The registered type names. A name outside this set is unknown-type — unless
// the spec reserves it for a future version, which is reserved-type: a
// format-level rejection, not a typo. (uint64/float32/float64 are registered
// and compile; their rejection happens at validation. int64 is not registered
// at all, so it reports reserved-type here.)
var registeredTypes = map[string]bool{
	"string": true, "email": true, "url": true,
	"number": true, "int": true, "uint": true, "float": true,
	"int8": true, "int16": true, "int32": true,
	"uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
	"bigint": true, "decimal": true,
	"bool":     true,
	"datetime": true, "date": true, "time": true,
	"any": true, "array": true, "object": true,
}

var reservedTypes = map[string]bool{
	"int64": true, "uint64": true, "float32": true, "float64": true,
}

// unusableTypeCode names why a type name cannot be used: reserved for a
// future version, or no such type at all.
func unusableTypeCode(name string) string {
	if reservedTypes[name] {
		return errs.ReservedType
	}
	return errs.UnknownType
}

// constraintKeys lists the per-family constraint keys each type accepts
// beyond the universal ones (type, optional, null, default). A key outside
// the set is unknown-member. Derived from the reference implementation's
// per-type memberdef schemas.
var (
	stringKeys   = keySet("choices", "pattern", "flags", "len", "minLen", "maxLen", "format", "escapeLines", "encloser")
	numberKeys   = keySet("choices", "min", "max", "multipleOf", "format")
	decimalKeys  = keySet("choices", "min", "max", "multipleOf", "precision", "scale")
	datetimeKeys = keySet("choices", "min", "max")
	boolKeys     = keySet()
	objectKeys   = keySet("schema")
	arrayKeys    = keySet("of", "len", "minLen", "maxLen")
	anyKeys      = keySet("choices", "anyOf", "isSchema")
)

func keySet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

func allowedKeys(typeName string) map[string]bool {
	switch typeName {
	case "string", "email", "url":
		return stringKeys
	case "decimal":
		return decimalKeys
	case "datetime", "date", "time":
		return datetimeKeys
	case "bool":
		return boolKeys
	case "object":
		return objectKeys
	case "array":
		return arrayKeys
	case "any":
		return anyKeys
	default: // the number family, bigint included
		return numberKeys
	}
}

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
	obj, ok := shape.(*value.Object)
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
func hasAbsentMember(obj *value.Object) bool {
	for i := range obj.Members {
		if obj.Members[i].Absent {
			return true
		}
	}
	return false
}

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

	case *value.Object:
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
func typedefTypeName(obj *value.Object) (string, bool) {
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
func compileTypedef(md *MemberDef, typeName string, obj *value.Object, path string) {
	if !registeredTypes[typeName] {
		fail(unusableTypeCode(typeName))
	}
	md.Type = typeName
	allowed := allowedKeys(typeName)
	if hasAbsentMember(obj) {
		fail(errs.EmptyMemberdef)
	}

	for i, m := range obj.Members {
		if m.Positional {
			if i == 0 {
				continue // the type name itself
			}
			// A stray positional in a typedef declares nothing it can keep.
			fail(errs.UnknownMember)
		}
		key := m.Key
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
			if !allowed["choices"] {
				fail(errs.UnknownMember)
			}
			arr, ok := m.Value.([]any)
			if !ok {
				fail(errs.ExpectedArray)
			}
			md.Choices = arr
			md.Keys = append(md.Keys, "choices")
		case "anyOf":
			if !allowed["anyOf"] {
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
			if !allowed["of"] {
				fail(errs.UnknownMember)
			}
			// The object-form element def starts a fresh path (matching the
			// reference: `of: {id: int}` compiles child paths from the root).
			md.Of = compileOfDef(m.Value)
		case "schema":
			if !allowed["schema"] {
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
			if !allowed[key] {
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
	case *value.Object:
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

// checkConstraintValue type-checks one constraint's VALUE against what the
// member's type expects — the reference validates the object-form typedef
// against a per-type memberdef schema, so `{int, default: notanumber}` is
// expected-number and `{bigint, multipleOf: 5}` is expected-bigint, at
// compile. An @-reference is resolved later and skipped here.
func checkConstraintValue(typeName, key string, v any) {
	if s, ok := v.(string); ok && strings.HasPrefix(s, "@") {
		return
	}
	expect := func(code string, ok bool) {
		if !ok {
			fail(code)
		}
	}
	switch key {
	case "len", "minLen", "maxLen", "precision", "scale":
		_, ok := v.(float64)
		expect(errs.ExpectedNumber, ok)
	case "pattern", "flags", "format", "encloser":
		_, ok := v.(string)
		expect(errs.ExpectedString, ok)
	case "escapeLines":
		_, ok := v.(bool)
		expect(errs.ExpectedBoolean, ok)
	case "min", "max", "multipleOf", "default":
		switch familyOf(typeName) {
		case famString:
			_, ok := v.(string)
			expect(errs.ExpectedString, ok)
		case famBigInt:
			_, ok := v.(*big.Int)
			expect(errs.ExpectedBigInt, ok)
		case famDecimal:
			_, ok := v.(value.Decimal)
			expect(errs.ExpectedDecimal, ok)
		case famTemporal:
			_, ok := v.(time.Time)
			expect(errs.ExpectedDateTime, ok)
		case famBool:
			_, ok := v.(bool)
			expect(errs.ExpectedBoolean, ok)
		case famArray:
			_, ok := v.([]any)
			expect(errs.ExpectedArray, ok)
		case famObject:
			_, ok := v.(*value.Object)
			expect(errs.InvalidObject, ok)
		case famNumber:
			_, ok := v.(float64)
			expect(errs.ExpectedNumber, ok)
		}
	}
}

// The type families, shared by compile-time constraint checks and validation.
type family uint8

const (
	famAny family = iota
	famString
	famNumber
	famBigInt
	famDecimal
	famBool
	famTemporal
	famArray
	famObject
)

func familyOf(typeName string) family {
	switch typeName {
	case "any":
		return famAny
	case "string", "email", "url":
		return famString
	case "bigint":
		return famBigInt
	case "decimal":
		return famDecimal
	case "bool":
		return famBool
	case "datetime", "date", "time":
		return famTemporal
	case "array":
		return famArray
	case "object":
		return famObject
	}
	return famNumber
}
