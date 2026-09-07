package document

import (
	"math/big"
	"strings"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Writing a TYPEDEF: rendering a compiled schema back into the memberdef
// text it was compiled from. The inverse of internal/schema's compile.

func (d *Doc) writeSchemaBody(s *schema.Schema) string {
	var parts []string
	for _, name := range s.Names {
		if name == "*" {
			continue // handled through Open below
		}
		parts = append(parts, d.memberDeclaration(name, s.Defs[name]))
	}
	switch o := s.Open.(type) {
	case *schema.MemberDef:
		if ann := d.memberAnnotation(o); ann != "" {
			parts = append(parts, "*:"+ann)
		} else {
			parts = append(parts, "*")
		}
	default:
		if s.Open != nil {
			parts = append(parts, "*")
		}
	}
	return strings.Join(parts, ", ")
}

// memberDeclaration renders one `name?*: annotation` declaration; a name that
// needs quoting cannot carry the short markers and uses the long form.
func (d *Doc) memberDeclaration(name string, md *schema.MemberDef) string {
	key := formatObjectKey(name)
	ann := d.memberAnnotation(md)

	if key == name || (!md.Optional && !md.Null) {
		markers := ""
		if md.Optional {
			markers += "?"
		}
		if md.Null {
			markers += "*"
		}
		if ann != "" {
			return key + markers + ": " + ann
		}
		return key + markers
	}
	// Long form: `"a,b": {number, optional: T, "null": T}`.
	body := d.longFormBodyOf(md)
	var flags []string
	if md.Optional {
		flags = append(flags, "optional: T")
	}
	if md.Null {
		flags = append(flags, `"null": T`)
	}
	return key + ": {" + strings.Join(append([]string{body}, flags...), ", ") + "}"
}

// longFormBodyOf renders the inside of a long-form memberdef (everything
// before the optional/"null" flags), for members whose names need quoting.
func (d *Doc) longFormBodyOf(md *schema.MemberDef) string {
	if md.SchemaRef != "" {
		if md.Type == "array" {
			return "array, of: " + refSpelling(md.SchemaRef)
		}
		return "object, schema: " + refSpelling(md.SchemaRef)
	}
	if md.Type == "object" && md.Schema != nil {
		return "object, schema: " + d.nestedSchemaAnnotation(md.Schema)
	}
	if md.Type == "array" && md.Of != nil {
		if isUntypedElem(md.Of) {
			return "array"
		}
		return "array, of: " + d.arrayElemAnnotation(md.Of)
	}
	typeName := md.Type
	if typeName == "" {
		typeName = "any"
	}
	parts := []string{typeName}
	for _, key := range md.Keys {
		var v any
		switch key {
		case "default":
			v = md.Default
		case "choices":
			v = md.Choices
		default:
			v = md.Constraints[key]
		}
		parts = append(parts, key+":"+d.constraintValue(typeName, v))
	}
	return strings.Join(parts, ", ")
}

// memberAnnotation renders a memberdef's type annotation — empty for a bare
// `any` with no constraints.
func (d *Doc) memberAnnotation(md *schema.MemberDef) string {
	if md.SchemaRef != "" {
		if md.Type == "array" {
			return "[" + refSpelling(md.SchemaRef) + "]"
		}
		return refSpelling(md.SchemaRef)
	}
	if md.Type == "object" && md.Schema != nil {
		return d.nestedSchemaAnnotation(md.Schema)
	}
	if md.Type == "" || md.Type == "any" {
		if len(md.Keys) == 0 {
			return ""
		}
		return d.typeWithConstraints("any", md)
	}
	if md.Type == "array" && md.Of != nil && len(md.Keys) == 0 {
		return "[" + d.arrayElemAnnotation(md.Of) + "]"
	}
	if len(md.Keys) > 0 || (md.Type == "array" && md.Of != nil) {
		return d.typeWithConstraints(md.Type, md)
	}
	return md.Type
}

func (d *Doc) nestedSchemaAnnotation(s *schema.Schema) string {
	var fields []string
	for _, name := range s.Names {
		if name == "*" {
			continue
		}
		fields = append(fields, d.memberDeclaration(name, s.Defs[name]))
	}
	switch o := s.Open.(type) {
	case *schema.MemberDef:
		if ann := d.memberAnnotation(o); ann != "" {
			fields = append(fields, "*: "+ann)
		} else {
			fields = append(fields, "*")
		}
	default:
		if s.Open != nil && len(fields) > 0 {
			fields = append(fields, "*")
		}
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

// isUntypedElem reports the element `[]` compiles to: a NULLABLE any with
// nothing else said about it (schema.compileArrayElem).
//
// It has no spelling of its own. In the bracket form it is written by writing
// nothing — `[]` — and in the long form by leaving `of:` off entirely, since an
// array with no `of` is equally untyped. Writing it as `any` instead silently
// drops the nullability, and the document stops re-parsing the moment it holds
// a null element.
func isUntypedElem(of *schema.MemberDef) bool {
	return of != nil && of.Type == "any" && of.Null && of.SchemaRef == "" &&
		of.Schema == nil && of.Of == nil && len(of.Keys) == 0
}

func (d *Doc) arrayElemAnnotation(of *schema.MemberDef) string {
	if isUntypedElem(of) {
		return ""
	}
	if of.SchemaRef != "" {
		return refSpelling(of.SchemaRef)
	}
	if of.Type == "object" && of.Schema != nil {
		return d.nestedSchemaAnnotation(of.Schema)
	}
	if of.Type == "array" && of.Of != nil {
		return "[" + d.arrayElemAnnotation(of.Of) + "]"
	}
	if len(of.Keys) > 0 {
		return d.typeWithConstraints(of.Type, of)
	}
	return of.Type
}

func (d *Doc) typeWithConstraints(typeName string, md *schema.MemberDef) string {
	parts := []string{typeName}
	for _, key := range md.Keys {
		var v any
		switch key {
		case "default":
			v = md.Default
		case "choices":
			v = md.Choices
		case "anyOf":
			var alts []string
			for _, alt := range md.AnyOf {
				alts = append(alts, d.arrayElemAnnotation(alt))
			}
			parts = append(parts, "anyOf:["+strings.Join(alts, ", ")+"]")
			continue
		default:
			v = md.Constraints[key]
		}
		parts = append(parts, key+":"+d.constraintValue(typeName, v))
	}
	if md.Type == "array" && md.Of != nil {
		if !isUntypedElem(md.Of) {
			parts = append(parts[:1], append([]string{"of: " + d.arrayElemAnnotation(md.Of)}, parts[1:]...)...)
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// constraintValue renders a constraint's value: @-references resolved,
// strings always quoted.
func (d *Doc) constraintValue(typeName string, v any) string {
	// A @-reference is resolved — the corpus pins that
	// (serializer/headers.io :: header_variable_in_choices) — but ONLY when the
	// resolved value is legal for this member's type.
	//
	// `{string, choices: [@b]}` with `~ @b: 0` resolved to `choices: [0]`, a
	// NUMBER in a string member's choices. The re-parse rejected the schema,
	// writeHeader dropped the definition it could not compile, and the second
	// write differed from the first. Keeping the reference in that case obeys
	// this file's own law: never emit text your own reader rejects. Found by
	// the idempotence property.
	if s, ok := v.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
		if r, verr := d.Defs.Var(s[1:]); verr == nil && schema.ValueFitsType(typeName, r) {
			v = r
		}
	}
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return regularString(x)
	case bool:
		if x {
			return "T"
		}
		return "F"
	case float64:
		return ioNumber(x)
	case *big.Int:
		return x.String() + "n"
	case core.Decimal:
		return x.String() + "m"
	case time.Time:
		return temporalLiteral(x, "")
	case []any:
		var elems []string
		for _, e := range x {
			elems = append(elems, d.constraintValue(typeName, e))
		}
		return "[" + strings.Join(elems, ", ") + "]"
	}
	// Anything the cases above do not name — an object default, binary, a
	// deferred literal — is still a VALUE, and the writer must spell it. This
	// used to return "", emitting `default:` with nothing after it: text the
	// reader rejects with expected-value. Found by the round-trip fuzzer on
	// `A:[{any,{}}]`, where the positional form binds `{}` to default.
	return d.writeValue(v, nil)
}

// ── sections and records ───────────────────────────────────────────────────
//
// The per-record path is APPEND-STYLE: one caller-owned []byte buffer is
// threaded through the whole recursion and every piece is appended into it,
// exactly as encoding/json's encodeState and the standard library's Append*
// family work. The previous shape — a []string of formatted parts joined at
// every nesting level — allocated once per value, once per level, and copied
// the whole document at each join (ADR 0006 P1). The header and schema
// writers above stay string-based: they run once per document, not per
// record.

// partWriter emits comma-separated parts into a buffer, holding back empty
// parts so trailing ones vanish while interior ones keep their comma — the
// streaming equivalent of trimming a []string before strings.Join.
