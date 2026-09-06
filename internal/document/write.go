package document

import (
	"encoding/base64"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/maniartech/InternetObject-go/internal/numfmt"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The canonical writer. One rule governs everything here: a writer must never
// emit text its own reader cannot read back as the same value. The concrete
// choices (when a string is quoted, escaped or raw; when a key is written;
// how the header and sections are laid out) mirror the reference writer,
// which the round-trip corpus pins.

// reservedSectionNames are the parser defaults a writer treats as no name.
var reservedSectionNames = map[string]bool{"data": true, "schema": true, "$schema": true}

// IsDefaultSectionName reports a name the parser supplied rather than the
// document: `---` becomes the section "data". Callers that ask "did the author
// NAME this section?" must go through here, not compare against "data"
// themselves, or the answer drifts between the writer and everyone else.
func IsDefaultSectionName(name string) bool {
	return name == "" || reservedSectionNames[name]
}

// String renders the loaded document in canonical form: header included,
// schemas spelled with types, keys emitted only where a name is not
// recoverable ("extras" mode).
func (d *Doc) String() string {
	// One buffer for the whole document, sized from the record count so it
	// doubles at most once or twice (ADR 0006 P1).
	n := 0
	for _, sec := range d.Sections {
		n += len(sec.Records)
	}
	dst := make([]byte, 0, 64+64*n)

	wrote := false
	if d.Header != nil || d.cachedHeader != "" {
		if h := d.writeHeader(); h != "" {
			dst = append(dst, h...)
			wrote = true
		}
	}

	for _, sec := range d.Sections {
		hasNamedSchema := sec.SchemaName != "" && sec.SchemaName != "schema"
		// A name the section-name grammar cannot spell is unwritable; it can
		// only have been borrowed from a schema selector (`--- $$` names the
		// section "$"), and the selector-only spelling reproduces that.
		hasRealName := sec.Name != "" && !reservedSectionNames[sec.Name] &&
			tokenizer.ValidSectionName(sec.Name)

		if wrote {
			dst = append(dst, '\n')
			if hasRealName || hasNamedSchema {
				dst = append(dst, '\n') // a blank line before a named/bound section
			}
		}
		wrote = true
		switch {
		case hasRealName && hasNamedSchema:
			dst = append(dst, "--- "...)
			dst = append(dst, sec.Name...)
			dst = append(dst, ": $"...)
			dst = append(dst, sec.SchemaName...)
		case hasRealName:
			dst = append(dst, "--- "...)
			dst = append(dst, sec.Name...)
		case hasNamedSchema:
			dst = append(dst, "--- $"...)
			dst = append(dst, sec.SchemaName...)
		default:
			dst = append(dst, "---"...)
		}

		mark := len(dst)
		dst = append(dst, '\n')
		body := len(dst)
		dst = d.appendSection(dst, sec)
		if len(dst) == body {
			dst = dst[:mark] // the section wrote nothing; drop the newline
		}
	}
	return string(dst)
}

// SchemaText renders a compiled schema's member declarations in canonical
// syntax — the text between the braces of `{…}`, also valid as a schema-only
// document header.
func SchemaText(s *schema.Schema) string {
	d := &Doc{Defs: newDefs(nil)}
	return d.writeSchemaBody(s)
}

// ── header ─────────────────────────────────────────────────────────────────

func (d *Doc) writeHeader() string {
	if d.cachedHeader != "" {
		return d.cachedHeader
	}
	h := d.Header
	// Schema-only mode: a header holding nothing but the default schema is
	// written as the bare schema line.
	schemaOnly := (len(h.Defs) == 1 && h.Defs[0].Kind == parser.DefSchema && h.Defs[0].Key == "schema") ||
		(len(h.Defs) == 0 && h.Inline != nil)
	if schemaOnly {
		s, cerr := d.Defs.SchemaOf("schema")
		if cerr != nil || s == nil {
			if h.Inline != nil {
				s, _ = schema.Compile(h.Inline, "")
			}
		}
		if s != nil {
			return d.writeSchemaBody(s)
		}
		return ""
	}

	var lines []string
	for _, def := range h.Defs {
		switch def.Kind {
		case parser.DefSchema:
			if ref, ok := def.Value.(string); ok && strings.HasPrefix(ref, "$") {
				lines = append(lines, "~ $"+headerName(def.Key)+": "+refSpelling(ref))
				continue
			}
			s, cerr := d.Defs.SchemaOf(def.Key)
			if cerr != nil || s == nil {
				continue
			}
			body := d.writeSchemaBody(s)
			lines = append(lines, "~ $"+headerName(def.Key)+": {"+body+"}")
		case parser.DefVar:
			lines = append(lines, "~ @"+headerName(def.Key)+": "+d.writeValue(def.Value, nil))
		default:
			lines = append(lines, "~ "+formatObjectKey(def.Key)+": "+d.writeValue(def.Value, nil))
		}
	}
	return strings.Join(lines, "\n")
}

// ── schemas ────────────────────────────────────────────────────────────────

// writeSchemaBody renders a compiled schema's member declarations (no braces).
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
	case value.Decimal:
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
type partWriter struct {
	count   int // parts committed, including flushed empties
	pending int // empty parts held back
}

// sep writes the separator for the next part, flushing any held empties.
func (w *partWriter) sep(dst []byte) []byte {
	for ; w.pending > 0; w.pending-- {
		if w.count > 0 {
			dst = append(dst, ',', ' ')
		}
		w.count++
	}
	if w.count > 0 {
		dst = append(dst, ',', ' ')
	}
	w.count++
	return dst
}

func (w *partWriter) empty() { w.pending++ }

func (d *Doc) appendSection(dst []byte, sec *parser.Section) []byte {
	sch := d.schemaFor(sec)
	if sec.Collection {
		first := true
		for _, rec := range sec.Records {
			obj, ok := rec.(*value.Object)
			if !ok {
				continue
			}
			if !first {
				dst = append(dst, '\n')
			}
			first = false
			dst = append(dst, '~', ' ')
			dst = d.appendBareRecord(dst, obj, sch)
		}
		return dst
	}
	if len(sec.Records) == 0 {
		return dst
	}
	obj, ok := sec.Records[0].(*value.Object)
	if !ok {
		return dst
	}
	mark := len(dst)
	dst = d.appendBareRecord(dst, obj, sch)
	if len(dst) == mark {
		dst = append(dst, '{', '}') // an empty bare record still needs a record
	}
	return dst
}

// appendRecord renders one record's members, schema order first.
func (d *Doc) appendRecord(dst []byte, obj *value.Object, sch *schema.Schema) []byte {
	var w partWriter

	if sch != nil {
		for _, name := range sch.Names {
			if name == "*" {
				continue
			}
			md := sch.Defs[name]
			if i := obj.Find(name); i >= 0 {
				dst = w.sep(dst)
				dst = d.appendValueWithDef(dst, obj.Members[i].Value, md)
			} else if md.Optional && !md.HasDefault {
				w.empty() // hold the position; trailing ones are dropped
			}
		}
		for _, m := range obj.Members {
			// "Already written above" is exactly "declared by the schema" —
			// every name in sch.Names is emitted in that loop, and the bare
			// wildcard is skipped there. Reading the compiled schema's own map
			// avoids building a per-record `handled` map (ADR 0006 P2).
			if !m.Positional && m.Key != "*" && sch.Defs[m.Key] != nil {
				continue
			}
			var md *schema.MemberDef
			if o, ok := sch.Open.(*schema.MemberDef); ok {
				md = o
			}
			dst = w.sep(dst)
			if !m.Positional {
				dst = appendObjectKey(dst, m.Key)
				dst = append(dst, ':', ' ')
			}
			dst = d.appendValueWithDef(dst, m.Value, md)
		}
		return dst
	}

	// No schema: a member is positional when keyless or when its key equals
	// its own index; every other name is unrecoverable and must be written.
	// An absent member is a HOLE, not a null: held as an empty slot in the
	// middle, dropped at the end.
	for i, m := range obj.Members {
		if m.Absent {
			w.empty()
			continue
		}
		dst = w.sep(dst)
		if !m.Positional && m.Key != strconv.Itoa(i) {
			dst = appendObjectKey(dst, m.Key)
			dst = append(dst, ':', ' ')
		}
		dst = d.appendValue(dst, m.Value, nil)
	}
	return dst
}

// appendBareRecord renders a record for a BARE emit site — a `~` line or a
// section's single record. A bare line that is exactly one keyless braced
// object is ambiguous unenclosed: the re-parser absorbs those braces as the
// record's own (ISSUE-15), schema or no schema, dropping a nesting level.
// Enclosing applies here only; a nested object's braces come from appendValue,
// where absorption never happens.
func (d *Doc) appendBareRecord(dst []byte, obj *value.Object, sch *schema.Schema) []byte {
	mark := len(dst)
	dst = d.appendRecord(dst, obj, sch)

	present, lastIsObject := 0, false
	for _, m := range obj.Members {
		if m.Absent {
			continue
		}
		present++
		_, lastIsObject = m.Value.(*value.Object)
	}
	if present == 1 && lastIsObject && len(dst) > mark && dst[mark] == '{' {
		// Wrap in place: one shift, and only for this rare shape.
		dst = append(dst, 0)
		copy(dst[mark+1:], dst[mark:])
		dst[mark] = '{'
		dst = append(dst, '}')
	}
	return dst
}

// appendValueWithDef renders a member value under its definition (the declared
// temporal kind wins; a nested schema renders its object positionally).
func (d *Doc) appendValueWithDef(dst []byte, v any, md *schema.MemberDef) []byte {
	if md == nil {
		return d.appendValue(dst, v, nil)
	}
	if v == nil {
		return append(dst, 'N')
	}
	if t, ok := v.(time.Time); ok {
		switch md.Type {
		case "date", "time", "datetime":
			return appendTemporal(dst, t, md.Type)
		}
	}
	if obj, ok := v.(*value.Object); ok {
		sch := md.Schema
		if sch == nil && md.SchemaRef != "" {
			sch, _ = d.Defs.SchemaOf(strings.TrimPrefix(md.SchemaRef, "$"))
		}
		dst = append(dst, '{')
		dst = d.appendRecord(dst, obj, sch)
		return append(dst, '}')
	}
	if arr, ok := v.([]any); ok {
		dst = append(dst, '[')
		for i, e := range arr {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = d.appendValueWithDef(dst, e, md.Of)
		}
		return append(dst, ']')
	}
	return d.appendValue(dst, v, md)
}

// appendValue renders one value with no (or a scalar) definition in scope.
func (d *Doc) appendValue(dst []byte, v any, md *schema.MemberDef) []byte {
	switch x := v.(type) {
	case nil:
		return append(dst, 'N')
	case bool:
		if x {
			return append(dst, 'T')
		}
		return append(dst, 'F')
	case float64:
		return appendIONumber(dst, x)
	case *big.Int:
		dst = x.Append(dst, 10)
		return append(dst, 'n')
	case value.Decimal:
		dst = append(dst, x.String()...)
		return append(dst, 'm')
	case []byte:
		dst = append(dst, 'b', '"')
		dst = base64.StdEncoding.AppendEncode(dst, x)
		return append(dst, '"')
	case time.Time:
		return appendTemporal(dst, x, "")
	case string:
		return appendAutoString(dst, x)
	case *value.Object:
		dst = append(dst, '{')
		dst = d.appendRecord(dst, x, nil)
		return append(dst, '}')
	case []any:
		dst = append(dst, '[')
		for i, e := range x {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = d.appendValue(dst, e, nil)
		}
		return append(dst, ']')
	}
	return dst
}

// writeValue keeps the string form for the header path, which runs once per
// document and reads better as a string.
func (d *Doc) writeValue(v any, md *schema.MemberDef) string {
	return string(d.appendValue(nil, v, md))
}

// ── scalars ────────────────────────────────────────────────────────────────

// ioNumber renders a float64 in IO spelling: ECMAScript shortest form with
// the IO names for the specials.
func ioNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	return numfmt.Format(f)
}

// appendIONumber is the append-style form: the specials are constants, and
// the general case defers to numfmt (ADR 0006 P1 — a numfmt.Append would
// remove the one remaining allocation here).
func appendIONumber(dst []byte, f float64) []byte {
	switch {
	case math.IsNaN(f):
		return append(dst, "NaN"...)
	case math.IsInf(f, 1):
		return append(dst, "Inf"...)
	case math.IsInf(f, -1):
		return append(dst, "-Inf"...)
	}
	return numfmt.Append(dst, f)
}

// appendTemporal is temporalLiteral in append form: time.AppendFormat writes
// straight into the buffer, so no intermediate string is built.
func appendTemporal(dst []byte, t time.Time, declared string) []byte {
	u := t.UTC()
	// The SCHEMA names the spelling when the member declares one — the normal
	// case in a schema-first format, and it loses nothing. An undeclared
	// temporal is spelled from what its instant evidences, the same
	// normalization the writer applies to a string's open/raw/quoted form.
	kind := declared
	if kind == "" {
		kind = InferTemporalKind(u)
	}
	switch kind {
	case "date":
		dst = append(dst, 'd', '"')
		dst = u.AppendFormat(dst, "2006-01-02")
	case "time":
		dst = append(dst, 't', '"')
		if u.Nanosecond() != 0 {
			dst = u.AppendFormat(dst, "15:04:05.000")
		} else {
			dst = u.AppendFormat(dst, "15:04:05")
		}
	default:
		dst = append(dst, 'd', 't', '"')
		dst = u.AppendFormat(dst, "2006-01-02T15:04:05.000Z")
	}
	return append(dst, '"')
}

// InferTemporalKind reports the kind an instant EVIDENCES, for a host value
// that genuinely carries none: the 1900-01-01 sentinel date is a time, an
// all-zero clock is a date, anything else a datetime. Our own model always
// carries a kind, so the writer never needs this — it is kept for callers
// converting from a kindless source (PORTING-NOTES rule 15).
func InferTemporalKind(u time.Time) string {
	y, mo, day := u.Date()
	h, mi, sec := u.Clock()
	switch {
	case y == 1900 && mo == 1 && day == 1:
		return "time"
	case h == 0 && mi == 0 && sec == 0 && u.Nanosecond() == 0:
		return "date"
	}
	return "datetime"
}

// temporalLiteral renders a temporal value under the declared kind, or the
// kind the value itself evidences when none is declared: the 1900-01-01
// sentinel date is a time, an all-zero time is a date, anything else a
// datetime.
func temporalLiteral(t time.Time, declared string) string {
	return string(appendTemporal(nil, t, declared))
}

// ── strings and keys ───────────────────────────────────────────────────────

// Hot-path classification is table- and scanner-based, never regexp: these
// run on every string and every key written, and Go's RE2 engine allocates
// match state per call (ADR 0006 P5). Each function is pinned to the regex it
// replaced by a table-driven equivalence test in write_scan_test.go.

// bareSafeKeyByte is the `[A-Za-z0-9_. -]` continuation set of the bare-key
// grammar; the first byte additionally allows `$` but never a digit.
var bareSafeKeyByte = func() (t [256]bool) {
	for c := 'a'; c <= 'z'; c++ {
		t[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		t[c] = true
	}
	for c := '0'; c <= '9'; c++ {
		t[c] = true
	}
	t['_'], t['.'], t[' '], t['-'] = true, true, true, true
	return
}()

var keywordKeys = map[string]bool{
	"true": true, "false": true, "null": true,
	"T": true, "F": true, "N": true, "Inf": true, "NaN": true,
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

func allDigitsIn(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigitByte(s[i]) {
			return false
		}
	}
	return len(s) > 0
}

// isBareSafeKey replaces `^[$A-Za-z_][A-Za-z0-9_. -]*$`.
func isBareSafeKey(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if c != '$' && c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !bareSafeKeyByte[s[i]] {
			return false
		}
	}
	return true
}

// isNumericKey replaces `^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$`.
func isNumericKey(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	intDigits := 0
	for i < len(s) && isDigitByte(s[i]) {
		i++
		intDigits++
	}
	fracDigits := 0
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigitByte(s[i]) {
			i++
			fracDigits++
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		k := 0
		for i < len(s) && isDigitByte(s[i]) {
			i++
			k++
		}
		if k == 0 {
			return false
		}
	}
	return i == len(s)
}

// isDateLike replaces `^\d{4}-\d{2}-\d{2}$`.
func isDateLike(s string) bool {
	return len(s) == 10 && s[4] == '-' && s[7] == '-' &&
		allDigitsIn(s[0:4]) && allDigitsIn(s[5:7]) && allDigitsIn(s[8:10])
}

// isTimeLike replaces `^\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?$`.
func isTimeLike(s string) bool {
	if len(s) < 5 || s[2] != ':' || !allDigitsIn(s[0:2]) || !allDigitsIn(s[3:5]) {
		return false
	}
	if len(s) == 5 {
		return true
	}
	if len(s) < 8 || s[5] != ':' || !allDigitsIn(s[6:8]) {
		return false
	}
	if len(s) == 8 {
		return true
	}
	return s[8] == '.' && allDigitsIn(s[9:])
}

// isDateTimeLike replaces the date + `T`/space + time + optional-zone regex.
func isDateTimeLike(s string) bool {
	if len(s) < 16 || !isDateLike(s[:10]) || (s[10] != 'T' && s[10] != ' ') {
		return false
	}
	rest := s[11:]
	if z := len(rest) - 1; z >= 0 && rest[z] == 'Z' {
		return isTimeLike(rest[:z])
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] == '+' || rest[i] == '-' {
			return isTimeLike(rest[:i]) && isZoneOffset(rest[i:])
		}
	}
	return isTimeLike(rest)
}

// isZoneOffset matches `[+-]\d{2}:?\d{2}`.
func isZoneOffset(s string) bool {
	if len(s) < 5 || (s[0] != '+' && s[0] != '-') {
		return false
	}
	body := s[1:]
	if len(body) == 5 && body[2] == ':' {
		return allDigitsIn(body[0:2]) && allDigitsIn(body[3:5])
	}
	return len(body) == 4 && allDigitsIn(body)
}

// Deciding how to spell a string used to call strings.ContainsAny,
// ContainsRune and Contains several times over — each rescanning the string,
// and ContainsAny rebuilding a 256-bit ASCII set on EVERY call, which the CPU
// profile showed as roughly half of encode time. One table-driven pass now
// collects every fact at once, and the whole-string checks (keyword, numeric,
// temporal) run only when the cheap pass has not already decided.
const (
	clStruct = 1 << iota // structural: needs open-escaping
	clQuote              // comma, CR, or a C0 control: must be quoted
	clRaw                // newline or tab: the raw spelling covers it
	clSpace              // ASCII whitespace: a word boundary
	clDigit              // a digit: only then can a numeric claim exist
	clDash               // a hyphen: only then can the text contain "---"
)

var strClass = func() (t [256]byte) {
	for _, c := range []byte(`{}[]:#"'\~`) {
		t[c] |= clStruct
	}
	t[','] |= clQuote
	t['\r'] |= clQuote
	for c := 0; c < 0x20; c++ {
		if c != '\n' && c != '\r' && c != '\t' {
			t[c] |= clQuote // a raw control ends the run on re-read
		}
	}
	t['\n'] |= clRaw
	t['\t'] |= clRaw
	for _, c := range []byte{' ', '\t', '\n', '\r', '\v', '\f'} {
		t[c] |= clSpace
	}
	for c := '0'; c <= '9'; c++ {
		t[c] |= clDigit
	}
	t['-'] |= clDash
	return
}()

// ambiguousWord reports the words that read back as something other than
// themselves. A switch compiles to a length-and-prefix test, which beats
// hashing a map key for every string written.
func ambiguousWord(s string) bool {
	switch s {
	case "null", "N", "true", "T", "false", "F",
		"Inf", "+Inf", "-Inf", "NaN", "undefined":
		return true
	}
	return false
}

// wouldNotReadBack reports whether the bare text would read back as anything
// other than this string: a keyword, a number, a broken numeric claim, or a
// temporal literal. These need the whole string, so they run only when the
// character-class pass has not already forced quoting.
func wouldNotReadBack(s string, flags byte, numStart bool) bool {
	if ambiguousWord(s) {
		return true
	}
	if !numStart {
		// No word begins with a digit, sign or point, so the text cannot read
		// back as a number, a broken numeric claim or a temporal literal.
		return false
	}
	// The reader's own classifier answers the numeric question, so writer and
	// reader can never disagree about a bare word.
	if tokenizer.WordReadsNonString(s) {
		return true
	}
	// A claimed-and-broken word (`2.5e1n`) errors even mid-run, where an
	// ordinary numeric word would just join the open string. Only text with a
	// digit AND a word boundary can hide one.
	if flags&clDigit != 0 && flags&clSpace != 0 {
		for i := 0; i < len(s); {
			for i < len(s) && strClass[s[i]]&clSpace != 0 {
				i++
			}
			start := i
			for i < len(s) && strClass[s[i]]&clSpace == 0 {
				i++
			}
			if start < i && tokenizer.WordIsBrokenClaim(s[start:i]) {
				return true
			}
		}
	}
	// A temporal literal always contains digits.
	if flags&clDigit != 0 {
		return isDateLike(s) || isTimeLike(s) || isDateTimeLike(s)
	}
	return false
}

// refSpelling spells a `$name` schema reference. Most refs are plain words
// and read back bare; one carrying quote, space or structural characters is
// spelled as a quoted string — the compiler cannot see quotedness, so the
// string `"$x"` compiles to the same reference `$x` does.
func refSpelling(ref string) string {
	if !strings.ContainsAny(ref, " \t\n\r") && autoString(ref) == ref {
		return ref
	}
	return regularString(ref)
}

// hasBareUnsafeControl reports a C0 control character other than \n\r\t —
// characters that survive neither a bare word, an open string, nor a raw
// string (a raw \b splits the word and the remainder re-reads as a number:
// silent data loss, found by the fuzzer). Such strings must be quoted with
// escapes.
func hasBareUnsafeControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			return true
		}
	}
	return false
}

// autoString picks the leanest spelling that reads back as the same string.
func autoString(s string) string {
	return string(appendAutoString(nil, s))
}

// appendAutoString is the append-style form every record path uses: quoted
// when the text is ambiguous or carries a character a bare run cannot hold,
// open with escaped structural characters, raw when only a newline or tab
// needs covering, bare otherwise.
func appendAutoString(dst []byte, s string) []byte {
	if s == "" {
		return appendRegularString(dst, s)
	}

	// One pass collects every character fact, including whether any WORD
	// starts with a character that could begin a number. Only such text can
	// read back as a number, a broken claim or a temporal, so everything else
	// skips those whole-string checks entirely — which is most real text.
	var flags byte
	numStart := false
	atWordStart := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		cl := strClass[c]
		flags |= cl
		if atWordStart && (cl&clDigit != 0 || c == '+' || c == '-' || c == '.') {
			numStart = true
		}
		atWordStart = cl&clSpace != 0
	}

	// Untrimmed text must be quoted — which is a fact about the EDGES, not
	// about containing a space: testing `flags&clSpace` ran a whole TrimSpace
	// over every string with a space in the middle. Only a multi-byte edge
	// needs the Unicode-aware check.
	if strClass[s[0]]&clSpace != 0 || strClass[s[len(s)-1]]&clSpace != 0 {
		flags |= clQuote
	} else if s[0] >= utf8.RuneSelf || s[len(s)-1] >= utf8.RuneSelf {
		// Multi-byte edges ask the READER's whitespace rule, not
		// unicode.IsSpace: the two disagree (U+FEFF), and a value the reader
		// would skip must never be written bare.
		first, _ := utf8.DecodeRuneInString(s)
		last, _ := utf8.DecodeLastRuneInString(s)
		if tokenizer.IsSpaceRune(first) || tokenizer.IsSpaceRune(last) {
			flags |= clQuote
		}
	}
	if flags&clDash != 0 && strings.Contains(s, "---") {
		flags |= clQuote
	}
	if flags&clQuote == 0 && wouldNotReadBack(s, flags, numStart) {
		flags |= clQuote
	}

	switch {
	case flags&clQuote != 0:
		return appendRegularString(dst, s)
	case flags&clStruct != 0:
		return appendOpenEscaped(dst, s)
	case flags&clRaw != 0:
		dst = append(dst, 'r', '"')
		for i := 0; i < len(s); i++ {
			if s[i] == '"' {
				dst = append(dst, '"')
			}
			dst = append(dst, s[i])
		}
		return append(dst, '"')
	}
	return append(dst, s...)
}

// regularString spells s as a regular quoted string. Every C0 control
// character is escaped — named where the reader names one, \u00XX otherwise —
// so the quoted form never carries a raw control byte.
func regularString(s string) string {
	return string(appendRegularString(nil, s))
}

func appendRegularString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			dst = append(dst, '\\', '\\')
		case '"':
			dst = append(dst, '\\', '"')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return append(dst, '"')
}

func openEscaped(s string) string {
	return string(appendOpenEscaped(nil, s))
}

func appendOpenEscaped(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '{', '}', '[', ']', ':', '#', '"', '\'', '\\', '~':
			dst = append(dst, '\\', c)
			continue
		}
		// A control character written raw ENDS the run on re-read, silently
		// truncating the value (the same defect as FINDINGS #10, and open
		// strings process the full escape set, so an escape round-trips).
		if c < 0x20 {
			dst = appendControlEscape(dst, c)
			continue
		}
		dst = append(dst, c)
	}
	return dst
}

// appendControlEscape spells one C0 control character — named where the
// reader names one, `\u00XX` otherwise. Shared by the quoted and open
// spellings so the two never disagree about an escape.
func appendControlEscape(dst []byte, c byte) []byte {
	switch c {
	case '\n':
		return append(dst, '\\', 'n')
	case '\r':
		return append(dst, '\\', 'r')
	case '\t':
		return append(dst, '\\', 't')
	case '\b':
		return append(dst, '\\', 'b')
	case '\f':
		return append(dst, '\\', 'f')
	}
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
}

// headerName spells a `@variable` or `$schema` name. Such a name is read back
// as a bare run and CANNOT be quoted, so every character that would end that
// run — the reader's own terminator set, whitespace, a `---`, a backslash or
// a control — is escaped instead. Values do not share this path: they have
// quoting available and use it (a comma, for instance, forces a quoted
// string rather than an escape).
func headerName(name string) string {
	var dst []byte
	for i := 0; i < len(name); {
		c := name[i]
		if c < utf8.RuneSelf {
			switch {
			case c == '\\' || tokenizer.IsTerminatorByte(c) || c == ' ':
				dst = append(dst, '\\', c)
			case c < 0x20:
				dst = appendControlEscape(dst, c)
			case c == '-' && i+2 < len(name) && name[i+1] == '-' && name[i+2] == '-':
				dst = append(dst, '\\', c)
			default:
				dst = append(dst, c)
			}
			i++
			continue
		}
		r, w := utf8.DecodeRuneInString(name[i:])
		if tokenizer.IsSpaceRune(r) {
			dst = append(dst, '\\') // the rune itself then flows as content
		}
		dst = append(dst, name[i:i+w]...)
		i += w
	}
	return string(dst)
}

// formatObjectKey quotes a key whose bare spelling would not read back as
// that key: numerics, keywords, and anything outside the identifier-like set.
func formatObjectKey(key string) string {
	return string(appendObjectKey(nil, key))
}

// keyIsBare reports whether a key can be written unquoted — THE key-quoting
// decision, made once and shared by both spellings.
func keyIsBare(key string) bool {
	return isBareSafeKey(key) &&
		!strings.HasSuffix(key, " ") && !strings.Contains(key, "---") &&
		!isNumericKey(key) && !keywordKeys[key]
}

// appendObjectKey is formatObjectKey in append form.
func appendObjectKey(dst []byte, key string) []byte {
	if keyIsBare(key) {
		return append(dst, key...)
	}
	return appendRegularString(dst, key)
}

// isSpaceByte reports ASCII whitespace — the word separators a bare run can
// carry (multi-byte Unicode spaces never appear inside one unquoted word).
func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// ── exported spelling helpers ──────────────────────────────────────────────
//
// These are THE sites that decide how a value is spelled. A caller that walks
// Go values directly (the marshaler's fast path) appends through these, so
// there is exactly one implementation of every quoting, number and temporal
// rule no matter which traversal produced the value (ADR 0006 roadmap item 5).

// AppendString appends a string in its leanest safe spelling.
func AppendString(dst []byte, s string) []byte { return appendAutoString(dst, s) }

// AppendNumber appends a float64 in IO spelling.
func AppendNumber(dst []byte, f float64) []byte { return appendIONumber(dst, f) }

// AppendTemporalValue appends a temporal under the declared kind ("" to infer).
func AppendTemporalValue(dst []byte, t time.Time, declared string) []byte {
	return appendTemporal(dst, t, declared)
}

// AppendKey appends an object key, quoted only when it must be.
func AppendKey(dst []byte, key string) []byte { return appendObjectKey(dst, key) }
