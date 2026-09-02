package document

import (
	"encoding/base64"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

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

// String renders the loaded document in canonical form: header included,
// schemas spelled with types, keys emitted only where a name is not
// recoverable ("extras" mode).
func (d *Doc) String() string {
	var parts []string

	if d.Header != nil {
		if h := d.writeHeader(); h != "" {
			parts = append(parts, h)
		}
	}

	for _, sec := range d.Sections {
		hasNamedSchema := sec.SchemaName != "" && sec.SchemaName != "schema"
		// A name the section-name grammar cannot spell is unwritable; it can
		// only have been borrowed from a schema selector (`--- $$` names the
		// section "$"), and the selector-only spelling reproduces that.
		hasRealName := sec.Name != "" && !reservedSectionNames[sec.Name] &&
			tokenizer.ValidSectionName(sec.Name)

		if len(parts) > 0 && (hasRealName || hasNamedSchema) {
			parts = append(parts, "") // a blank line before a named/bound section
		}
		switch {
		case hasRealName && hasNamedSchema:
			parts = append(parts, "--- "+sec.Name+": $"+sec.SchemaName)
		case hasRealName:
			parts = append(parts, "--- "+sec.Name)
		case hasNamedSchema:
			parts = append(parts, "--- $"+sec.SchemaName)
		default:
			parts = append(parts, "---")
		}

		if text := d.writeSection(sec); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
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
				lines = append(lines, "~ $"+def.Key+": "+refSpelling(ref))
				continue
			}
			s, cerr := d.Defs.SchemaOf(def.Key)
			if cerr != nil || s == nil {
				continue
			}
			body := d.writeSchemaBody(s)
			lines = append(lines, "~ $"+def.Key+": {"+body+"}")
		case parser.DefVar:
			lines = append(lines, "~ @"+def.Key+": "+d.writeValue(def.Value, nil))
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
		parts = append(parts, key+":"+d.constraintValue(v))
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

func (d *Doc) arrayElemAnnotation(of *schema.MemberDef) string {
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
		parts = append(parts, key+":"+d.constraintValue(v))
	}
	if md.Type == "array" && md.Of != nil {
		parts = append(parts[:1], append([]string{"of: " + d.arrayElemAnnotation(md.Of)}, parts[1:]...)...)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// constraintValue renders a constraint's value: @-references resolved,
// strings always quoted.
func (d *Doc) constraintValue(v any) string {
	if s, ok := v.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
		if r, verr := d.Defs.Var(s[1:]); verr == nil {
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
	case value.Temporal:
		return temporalLiteral(x, "")
	case []any:
		var elems []string
		for _, e := range x {
			elems = append(elems, d.constraintValue(e))
		}
		return "[" + strings.Join(elems, ", ") + "]"
	}
	return ""
}

// ── sections and records ───────────────────────────────────────────────────

func (d *Doc) writeSection(sec *parser.Section) string {
	sch := d.SecSchemas[sec]
	if sec.Collection {
		var lines []string
		for _, rec := range sec.Records {
			if obj, ok := rec.(*value.Object); ok {
				lines = append(lines, "~ "+d.writeBareRecord(obj, sch))
			}
		}
		return strings.Join(lines, "\n")
	}
	if len(sec.Records) == 0 {
		return ""
	}
	obj, ok := sec.Records[0].(*value.Object)
	if !ok {
		return ""
	}
	line := d.writeBareRecord(obj, sch)
	if line == "" {
		return "{}" // an empty bare record must still put a record on the page
	}
	return line
}

// writeRecord renders one record's members, schema order first.
func (d *Doc) writeRecord(obj *value.Object, sch *schema.Schema) string {
	var parts []string

	if sch != nil {
		handled := map[string]bool{}
		for _, name := range sch.Names {
			if name == "*" {
				continue
			}
			md := sch.Defs[name]
			if i := obj.Find(name); i >= 0 {
				parts = append(parts, d.writeValueWithDef(obj.Members[i].Value, md))
			} else if md.Optional && !md.HasDefault {
				parts = append(parts, "") // hold the position
			}
			handled[name] = true
		}
		for len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1] // trailing empties carry no information
		}
		for _, m := range obj.Members {
			if !m.Positional && handled[m.Key] {
				continue
			}
			var md *schema.MemberDef
			if o, ok := sch.Open.(*schema.MemberDef); ok {
				md = o
			}
			formatted := d.writeValueWithDef(m.Value, md)
			if m.Positional {
				parts = append(parts, formatted)
			} else {
				parts = append(parts, formatObjectKey(m.Key)+": "+formatted)
			}
		}
		return strings.Join(parts, ", ")
	}

	// No schema: a member is positional when keyless or when its key equals
	// its own index; every other name is unrecoverable and must be written.
	// An absent member is a HOLE, not a null: held as an empty slot in the
	// middle, dropped at the end.
	for i, m := range obj.Members {
		if m.Absent {
			parts = append(parts, "")
			continue
		}
		formatted := d.writeValue(m.Value, nil)
		if m.Positional || m.Key == strconv.Itoa(i) {
			parts = append(parts, formatted)
		} else {
			parts = append(parts, formatObjectKey(m.Key)+": "+formatted)
		}
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ", ")
}

// writeBareRecord renders a record for a BARE emit site — a `~` line or a
// section's single record. A bare line that is exactly one keyless braced
// object is ambiguous unenclosed: the re-parser absorbs those braces as the
// record's own (ISSUE-15), schema or no schema, dropping a nesting level.
// Enclosing applies here only; a nested object's braces come from writeValue,
// where absorption never happens.
func (d *Doc) writeBareRecord(obj *value.Object, sch *schema.Schema) string {
	line := d.writeRecord(obj, sch)
	present, lastIsObject := 0, false
	for _, m := range obj.Members {
		if m.Absent {
			continue
		}
		present++
		_, lastIsObject = m.Value.(*value.Object)
	}
	if present == 1 && lastIsObject && strings.HasPrefix(line, "{") {
		return "{" + line + "}"
	}
	return line
}

// writeValueWithDef renders a member value under its definition (the declared
// temporal kind wins; a nested schema renders its object positionally).
func (d *Doc) writeValueWithDef(v any, md *schema.MemberDef) string {
	if md == nil {
		return d.writeValue(v, nil)
	}
	if v == nil {
		return "N"
	}
	if t, ok := v.(value.Temporal); ok {
		switch md.Type {
		case "date", "time", "datetime":
			return temporalLiteral(t, md.Type)
		}
	}
	if obj, ok := v.(*value.Object); ok {
		sch := md.Schema
		if sch == nil && md.SchemaRef != "" {
			sch, _ = d.Defs.SchemaOf(strings.TrimPrefix(md.SchemaRef, "$"))
		}
		return "{" + d.writeRecord(obj, sch) + "}"
	}
	if arr, ok := v.([]any); ok {
		var elems []string
		for _, e := range arr {
			elems = append(elems, d.writeValueWithDef(e, md.Of))
		}
		return "[" + strings.Join(elems, ", ") + "]"
	}
	return d.writeValue(v, md)
}

// writeValue renders one value with no (or a scalar) definition in scope.
func (d *Doc) writeValue(v any, md *schema.MemberDef) string {
	switch x := v.(type) {
	case nil:
		return "N"
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
	case []byte:
		return `b"` + base64.StdEncoding.EncodeToString(x) + `"`
	case value.Temporal:
		return temporalLiteral(x, "")
	case string:
		return autoString(x)
	case *value.Object:
		return "{" + d.writeRecord(x, nil) + "}"
	case []any:
		var elems []string
		for _, e := range x {
			elems = append(elems, d.writeValue(e, nil))
		}
		return "[" + strings.Join(elems, ", ") + "]"
	}
	return ""
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

// temporalLiteral renders a temporal value under the declared kind, or the
// kind the value itself evidences when none is declared: the 1900-01-01
// sentinel date is a time, an all-zero time is a date, anything else a
// datetime.
func temporalLiteral(t value.Temporal, declared string) string {
	u := t.T.UTC()
	kind := declared
	if kind == "" {
		y, mo, day := u.Date()
		h, mi, s := u.Clock()
		ms := u.Nanosecond() / 1e6
		switch {
		case y == 1900 && mo == 1 && day == 1:
			kind = "time"
		case h == 0 && mi == 0 && s == 0 && ms == 0:
			kind = "date"
		default:
			kind = "datetime"
		}
	}
	switch kind {
	case "date":
		return `d"` + u.Format("2006-01-02") + `"`
	case "time":
		if u.Nanosecond() != 0 {
			return `t"` + u.Format("15:04:05.000") + `"`
		}
		return `t"` + u.Format("15:04:05") + `"`
	default:
		return `dt"` + u.Format("2006-01-02T15:04:05.000Z") + `"`
	}
}

// ── strings and keys ───────────────────────────────────────────────────────

var (
	reDateLike     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reTimeLike     = regexp.MustCompile(`^\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?$`)
	reDateTimeLike = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?$`)
	reKeyNumeric   = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$`)
	reKeyKeyword   = regexp.MustCompile(`^(?:true|false|null|T|F|N|Inf|NaN)$`)
	reKeyBareSafe  = regexp.MustCompile(`^[$A-Za-z_][A-Za-z0-9_. -]*$`)
)

var ambiguousWords = map[string]bool{
	"null": true, "N": true, "true": true, "T": true, "false": true, "F": true,
	"Inf": true, "+Inf": true, "-Inf": true, "NaN": true, "undefined": true,
}

func isAmbiguousString(s string) bool {
	if s == "" || ambiguousWords[s] {
		return true
	}
	if strings.TrimSpace(s) != s {
		return true
	}
	if strings.Contains(s, "---") {
		return true
	}
	// The reader's own word classifier answers "would this read back as a
	// keyword, a number, or a broken numeric claim?" — the writer quotes
	// whenever it would, so the two can never disagree on a bare word.
	if tokenizer.WordReadsNonString(s) {
		return true
	}
	// A claimed-and-broken word (`2.5e1n`) errors even mid-run, where an
	// ordinary numeric word would just join the open string. Scanned in
	// place: strings.Fields here allocated a slice for every string written.
	for i := 0; i < len(s); {
		for i < len(s) && isSpaceByte(s[i]) {
			i++
		}
		start := i
		for i < len(s) && !isSpaceByte(s[i]) {
			i++
		}
		if start < i && tokenizer.WordIsBrokenClaim(s[start:i]) {
			return true
		}
	}
	return reDateLike.MatchString(s) || reTimeLike.MatchString(s) || reDateTimeLike.MatchString(s)
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

// autoString picks the leanest spelling that reads back as the same string:
// quoted when ambiguous, comma-carrying or control-carrying, open with
// escaped structural characters, raw when \n\r\t would need escaping, bare
// otherwise.
func autoString(s string) string {
	// A raw carriage return is newline-normalized by the reader in every
	// unescaped spelling, so \r joins the must-quote set alongside the other
	// controls.
	if isAmbiguousString(s) || strings.ContainsRune(s, ',') ||
		strings.ContainsRune(s, '\r') || hasBareUnsafeControl(s) {
		return regularString(s)
	}
	if strings.ContainsAny(s, "{}[]:#\"'\\~") {
		return openEscaped(s)
	}
	if strings.ContainsAny(s, "\n\t") {
		return `r"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// regularString spells s as a regular quoted string. Every C0 control
// character is escaped — named where the reader names one, \u00XX otherwise —
// so the quoted form never carries a raw control byte.
func regularString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				b.WriteString(`\u00`)
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xF])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func openEscaped(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '{', '}', '[', ']', ':', '#', '"', '\'', '\\', '~':
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// formatObjectKey quotes a key whose bare spelling would not read back as
// that key: numerics, keywords, and anything outside the identifier-like set.
func formatObjectKey(key string) string {
	bareSafe := reKeyBareSafe.MatchString(key) &&
		!strings.HasSuffix(key, " ") && !strings.Contains(key, "---")
	if reKeyNumeric.MatchString(key) || reKeyKeyword.MatchString(key) || !bareSafe {
		return regularString(key)
	}
	return key
}

// isSpaceByte reports ASCII whitespace — the word separators a bare run can
// carry (multi-byte Unicode spaces never appear inside one unquoted word).
func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
