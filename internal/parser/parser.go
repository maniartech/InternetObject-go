// Package parser turns Internet Object source text into a Document: a header
// of definitions plus data sections, with errors accumulated rather than
// thrown.
//
// Error discipline mirrors the reference implementation's observable
// behavior: a structural fault OUTSIDE a `~`-collection abandons the parse
// with that single error; a fault INSIDE a collection record is captured, the
// record becomes an ErrorNode, and parsing resumes at the next `~` boundary
// (the format's accumulate-and-continue promise). A duplicate section name is
// accumulated and the section renamed, never fatal.
package parser

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// Document is one parsed Internet Object document.
type Document struct {
	Header   *Header
	Sections []*Section
	Errors   []errs.Error
}

// Header holds the definitions written before the first `---`.
type Header struct {
	Plain   *value.Object  // plain key: value definitions, in order, last wins
	Schemas map[string]any // $name → schema shape (sigil stripped), last wins
	Vars    map[string]any // @name → value (sigil stripped), last wins
	Inline  any            // the schema shape when the header is a bare schema expression
}

// Section is one data section.
type Section struct {
	Name       string // "" when unnamed
	SchemaName string // explicit `$ref` binding (sigil stripped), "" when none
	Collection bool   // the body is a `~`-collection
	Records    []any  // record values; a non-collection section has at most one
}

// Parse parses one document. It never panics or returns an error: faults are
// accumulated on the Document per the discipline above.
func Parse(src string) *Document {
	p := &parser{s: tokenizer.Tokenize(src), doc: &Document{}}
	p.run()
	return p.doc
}

// fail carries a fatal parse error to the nearest recovery boundary.
type fail struct{ err errs.Error }

type parser struct {
	s   *tokenizer.Stream
	i   int
	doc *Document
}

func (p *parser) peek() (tokenizer.Token, bool) {
	if p.i < len(p.s.Tokens) {
		return p.s.Tokens[p.i], true
	}
	return tokenizer.Token{}, false
}

func (p *parser) next() tokenizer.Token {
	t := p.s.Tokens[p.i]
	p.i++
	return t
}

func (p *parser) atEnd() bool { return p.i >= len(p.s.Tokens) }

// die aborts to the nearest recovery boundary with a designated code.
func (p *parser) die(code string, t tokenizer.Token) {
	panic(fail{errs.Error{Code: code, Line: t.Line, Col: t.Col}})
}

// dieToken aborts with a tokenizer ERROR token's own code.
func (p *parser) dieToken(t tokenizer.Token) {
	p.die(t.Err.String(), t)
}

func (p *parser) run() {
	defer func() {
		if r := recover(); r != nil {
			f := r.(fail)
			p.doc.Errors = append(p.doc.Errors, f.err)
		}
	}()

	// The part before the first `---` is the header — but only when a `---`
	// exists at all; otherwise the whole document is data.
	hasSep := false
	for _, t := range p.s.Tokens {
		if t.Kind == tokenizer.KindSectionSep {
			hasSep = true
			break
		}
	}

	if hasSep {
		if t, ok := p.peek(); ok && t.Kind != tokenizer.KindSectionSep {
			p.parseHeader()
		}
		for !p.atEnd() {
			p.next() // the SECTION_SEP
			p.parseSection()
		}
	} else if !p.atEnd() {
		p.parseSectionBody(&Section{})
	}

	p.renameDuplicateSections()
	p.apply()
}

// ── header ─────────────────────────────────────────────────────────────────

func (p *parser) parseHeader() {
	h := &Header{Schemas: map[string]any{}, Vars: map[string]any{}}
	p.doc.Header = h

	t, _ := p.peek()
	if t.Kind != tokenizer.KindCollectionStart {
		// A bare header is an inline schema expression: `name, age\n---\n…`.
		h.Inline = p.parseRecord()
		if t, ok := p.peek(); ok && t.Kind != tokenizer.KindSectionSep {
			p.die(errs.UnexpectedToken, t)
		}
		return
	}

	// A `~`-definitions collection: each record is one `key: value`.
	for {
		t, ok := p.peek()
		if !ok || t.Kind == tokenizer.KindSectionSep {
			return
		}
		p.next() // the ~
		p.parseDefinition(h)
	}
}

// parseDefinition reads one `~ key: value` header definition.
func (p *parser) parseDefinition(h *Header) {
	kt, ok := p.peek()
	if !ok || kt.Kind == tokenizer.KindSectionSep || kt.Kind == tokenizer.KindCollectionStart {
		return // an empty `~` line defines nothing
	}
	if kt.Kind == tokenizer.KindError {
		p.dieToken(kt)
	}
	ct, hasColon := p.peekAt(1)
	if kt.Kind != tokenizer.KindString || !hasColon || ct.Kind != tokenizer.KindColon {
		p.die(errs.InvalidDefinition, kt)
	}
	key := p.s.StringValue(kt)
	p.i += 2
	val := p.parseMemberValue(ct)

	switch {
	case strings.HasPrefix(key, "$") && kt.Sub == tokenizer.SubOpenString:
		h.Schemas[key[1:]] = val
	case strings.HasPrefix(key, "@") && kt.Sub == tokenizer.SubOpenString:
		h.Vars[key[1:]] = val
	default:
		if h.Plain == nil {
			h.Plain = &value.Object{}
		}
		if i := h.Plain.Find(key); i >= 0 {
			h.Plain.Members[i].Value = val // a duplicate definition: last wins
		} else {
			h.Plain.Members = append(h.Plain.Members,
				value.Member{Key: key, Quoted: kt.Sub != tokenizer.SubOpenString, Value: val})
		}
	}

	// Nothing else may follow a definition before the next `~` or `---`.
	if t, ok := p.peek(); ok &&
		t.Kind != tokenizer.KindCollectionStart && t.Kind != tokenizer.KindSectionSep {
		p.die(errs.InvalidDefinition, t)
	}
}

func (p *parser) peekAt(n int) (tokenizer.Token, bool) {
	if p.i+n < len(p.s.Tokens) {
		return p.s.Tokens[p.i+n], true
	}
	return tokenizer.Token{}, false
}

// ── sections ───────────────────────────────────────────────────────────────

// parseSection reads one section: the optional name/schema tokens the
// tokenizer produced after `---`, then the body up to the next `---`.
func (p *parser) parseSection() {
	sec := &Section{}
	for {
		t, ok := p.peek()
		if !ok {
			break
		}
		if t.Kind == tokenizer.KindError &&
			(t.Err == tokenizer.CodeInvalidSectionName || t.Err == tokenizer.CodeMissingSchema) {
			p.dieToken(t)
		}
		if t.Kind == tokenizer.KindString && t.Sub == tokenizer.SubSectionName {
			sec.Name = p.s.Text(t)
			p.i++
			continue
		}
		if t.Kind == tokenizer.KindString && t.Sub == tokenizer.SubSectionSchema {
			sec.SchemaName = p.s.Text(t)[1:] // strip the $ sigil
			if sec.Name == "" {
				sec.Name = sec.SchemaName // `--- $a` names the section after its schema
			}
			p.i++
			continue
		}
		break
	}
	p.parseSectionBody(sec)
}

func (p *parser) parseSectionBody(sec *Section) {
	p.doc.Sections = append(p.doc.Sections, sec)
	t, ok := p.peek()
	if !ok || t.Kind == tokenizer.KindSectionSep {
		return // an empty section
	}

	if t.Kind == tokenizer.KindCollectionStart {
		sec.Collection = true
		for {
			t, ok := p.peek()
			if !ok || t.Kind == tokenizer.KindSectionSep {
				return
			}
			p.next() // the ~
			sec.Records = append(sec.Records, p.parseCollectionRecord())
		}
	}

	sec.Records = append(sec.Records, p.parseRecord())
	if t, ok := p.peek(); ok && t.Kind != tokenizer.KindSectionSep {
		p.die(errs.UnexpectedToken, t)
	}
}

// parseCollectionRecord parses one `~` record with recovery: a fault is
// captured, the record becomes an ErrorNode, and the cursor skips to the next
// record boundary.
func (p *parser) parseCollectionRecord() (rec any) {
	defer func() {
		if r := recover(); r != nil {
			f := r.(fail)
			p.doc.Errors = append(p.doc.Errors, f.err)
			rec = value.ErrorNode{Code: f.err.Code}
			for !p.atEnd() {
				if k := p.s.Tokens[p.i].Kind; k == tokenizer.KindCollectionStart || k == tokenizer.KindSectionSep {
					break
				}
				p.i++
			}
		}
	}()
	rec = p.parseRecord()
	if t, ok := p.peek(); ok &&
		t.Kind != tokenizer.KindCollectionStart && t.Kind != tokenizer.KindSectionSep {
		p.die(errs.UnexpectedToken, t)
	}
	return rec
}

// recordEnd reports whether a token ends a bare (unbraced) record.
func recordEnd(t tokenizer.Token) bool {
	return t.Kind == tokenizer.KindCollectionStart || t.Kind == tokenizer.KindSectionSep
}

// ── records, objects, arrays, members ──────────────────────────────────────

// parseRecord parses a bare (unbraced) record: members separated by commas,
// with lenient commas, ending at `~`, `---` or end of input. A record whose
// sole content is one braced object IS that object (the braces are the
// record's own enclosure); any other single positional value is the member
// named "0" (the non-record-root promotion).
func (p *parser) parseRecord() any {
	obj := &value.Object{}
	sawComma := false
	for {
		t, ok := p.peek()
		if !ok || recordEnd(t) {
			break
		}
		if t.Kind == tokenizer.KindComma {
			sawComma = true
			p.i++
			continue
		}
		if t.Kind == tokenizer.KindCurlyClose || t.Kind == tokenizer.KindBracketClose {
			p.die(errs.UnexpectedToken, t)
		}
		p.parseMember(obj)
		// After a member: a comma, or the record's end.
		t, ok = p.peek()
		if !ok || recordEnd(t) {
			break
		}
		switch t.Kind {
		case tokenizer.KindComma:
			// consumed at the top of the loop
		case tokenizer.KindCurlyClose, tokenizer.KindBracketClose:
			p.die(errs.UnexpectedToken, t)
		default:
			p.die(errs.UnexpectedToken, t)
		}
	}

	if len(obj.Members) == 1 && obj.Members[0].Positional {
		if inner, ok := obj.Members[0].Value.(*value.Object); ok {
			return inner // the braces were the record's enclosure
		}
	}
	if len(obj.Members) == 0 && !sawComma {
		return nil // nothing at all — distinct from `,` which is an empty record
	}
	return obj
}

// parseMember parses one member into obj: `key: value` or a positional value.
func (p *parser) parseMember(obj *value.Object) {
	t, _ := p.peek()
	if t.Kind == tokenizer.KindColon {
		p.die(errs.UnexpectedToken, t)
	}
	if t.Kind == tokenizer.KindError {
		p.dieToken(t)
	}

	if ct, ok := p.peekAt(1); ok && ct.Kind == tokenizer.KindColon {
		// Something followed by a colon claims to be a key.
		switch t.Kind {
		case tokenizer.KindString:
			key := p.s.StringValue(t)
			quoted := t.Sub != tokenizer.SubOpenString
			p.i += 2
			v := p.parseMemberValue(ct)
			p.addMember(obj, value.Member{Key: key, Quoted: quoted, Value: v}, t)
			return
		case tokenizer.KindNumber, tokenizer.KindBigInt, tokenizer.KindDecimal,
			tokenizer.KindBoolean, tokenizer.KindNull, tokenizer.KindDateTime, tokenizer.KindBinary:
			p.die(errs.InvalidKey, t)
		}
	}

	v := p.parseValue()
	if nt, ok := p.peek(); ok && nt.Kind == tokenizer.KindColon {
		// A structured value (array/object) cannot name a member.
		p.die(errs.UnexpectedToken, nt)
	}
	p.addMember(obj, value.Member{Positional: true, Value: v}, t)
}

// addMember appends m, rejecting a duplicate member name (quoting does not
// make a different key).
func (p *parser) addMember(obj *value.Object, m value.Member, at tokenizer.Token) {
	if !m.Positional && obj.Find(m.Key) >= 0 {
		p.die(errs.DuplicateMember, at)
	}
	obj.Members = append(obj.Members, m)
}

// parseMemberValue parses the value after a key's colon, requiring one to be
// present: the grammar demands a value here, so its absence is
// expected-value (a token problem, distinct from validation's missing-value).
func (p *parser) parseMemberValue(colon tokenizer.Token) any {
	t, ok := p.peek()
	if !ok || recordEnd(t) || t.Kind == tokenizer.KindComma ||
		t.Kind == tokenizer.KindCurlyClose || t.Kind == tokenizer.KindBracketClose {
		p.die(errs.ExpectedValue, colon)
	}
	if t.Kind == tokenizer.KindColon {
		p.die(errs.UnexpectedToken, t)
	}
	return p.parseValue()
}

// parseValue parses one value.
func (p *parser) parseValue() any {
	t := p.next()
	switch t.Kind {
	case tokenizer.KindNull:
		return nil
	case tokenizer.KindBoolean:
		return p.s.Bool(t)
	case tokenizer.KindNumber:
		return p.s.Number(t)
	case tokenizer.KindBigInt:
		return p.s.BigInt(t)
	case tokenizer.KindDecimal:
		coef, scale := p.s.DecimalParts(t)
		return value.Decimal{Coef: coef, Scale: scale}
	case tokenizer.KindBinary:
		return p.s.Bytes(t)
	case tokenizer.KindDateTime:
		kind := value.KindDateTime
		switch t.Sub {
		case tokenizer.SubDate:
			kind = value.KindDate
		case tokenizer.SubTime:
			kind = value.KindTime
		}
		return value.Temporal{T: p.s.Temporal(t), Kind: kind}
	case tokenizer.KindString:
		v := p.s.StringValue(t)
		if t.Sub == tokenizer.SubOpenString && isVarRef(v) {
			return p.resolveVar(v[1:], t)
		}
		return v
	case tokenizer.KindCurlyOpen:
		return p.parseObject(t)
	case tokenizer.KindBracketOpen:
		return p.parseArray(t)
	case tokenizer.KindError:
		p.dieToken(t)
	}
	p.die(errs.UnexpectedToken, t)
	return nil
}

// isVarRef reports a whole-word variable reference: @ plus a name with no
// whitespace. A quoted "@name" is an ordinary string.
func isVarRef(s string) bool {
	return len(s) > 1 && s[0] == '@' && !strings.ContainsAny(s[1:], " \t\n")
}

func (p *parser) resolveVar(name string, t tokenizer.Token) any {
	if p.doc.Header != nil {
		if v, ok := p.doc.Header.Vars[name]; ok {
			return v
		}
	}
	p.die(errs.UndefinedVariable, t)
	return nil
}

// parseObject parses a braced object. Stray commas are tolerated inside
// braces — a leading, doubled or trailing comma separates nothing and is
// skipped.
func (p *parser) parseObject(open tokenizer.Token) any {
	obj := &value.Object{}
	for {
		t, ok := p.peek()
		if !ok || t.Kind == tokenizer.KindSectionSep {
			p.die(errs.ExpectedClosingBracket, open)
		}
		switch t.Kind {
		case tokenizer.KindComma:
			p.i++
			continue
		case tokenizer.KindCurlyClose:
			p.i++
			return obj
		case tokenizer.KindCollectionStart:
			p.die(errs.ExpectedClosingBracket, open)
		case tokenizer.KindBracketClose:
			p.die(errs.UnexpectedToken, t)
		}
		p.parseMember(obj)
		t, ok = p.peek()
		if !ok || t.Kind == tokenizer.KindSectionSep {
			p.die(errs.ExpectedClosingBracket, open)
		}
		switch t.Kind {
		case tokenizer.KindComma, tokenizer.KindCurlyClose:
			// handled at the top of the loop
		case tokenizer.KindCollectionStart:
			p.die(errs.ExpectedClosingBracket, open)
		default:
			p.die(errs.UnexpectedToken, t)
		}
	}
}

// parseArray parses a bracketed array. Commas are strict: exactly one value
// before each comma, no trailing comma.
func (p *parser) parseArray(open tokenizer.Token) any {
	arr := []any{}
	if t, ok := p.peek(); ok && t.Kind == tokenizer.KindBracketClose {
		p.i++
		return arr
	}
	for {
		t, ok := p.peek()
		if !ok {
			p.die(errs.ExpectedClosingBracket, open)
		}
		if t.Kind == tokenizer.KindCollectionStart || t.Kind == tokenizer.KindSectionSep {
			p.die(errs.ExpectedClosingBracket, open)
		}
		if t.Kind == tokenizer.KindComma || t.Kind == tokenizer.KindCurlyClose {
			p.die(errs.UnexpectedToken, t)
		}
		arr = append(arr, p.parseValue())
		if nt, ok := p.peek(); ok && nt.Kind == tokenizer.KindColon {
			p.die(errs.UnexpectedToken, nt)
		}
		t, ok = p.peek()
		if !ok {
			p.die(errs.ExpectedClosingBracket, open)
		}
		switch t.Kind {
		case tokenizer.KindBracketClose:
			p.i++
			return arr
		case tokenizer.KindComma:
			p.i++
		default:
			p.die(errs.UnexpectedToken, t)
		}
	}
}

// ── section-name uniqueness ────────────────────────────────────────────────

// renameDuplicateSections reports duplicate-section-name for every repeat and
// renames it by appending _2, _3, … to the ORIGINAL name, counting names
// already taken, per name and not per document (CONFORMANCE §8).
func (p *parser) renameDuplicateSections() {
	if len(p.doc.Sections) < 2 {
		return
	}
	seen := map[string]bool{}
	for _, sec := range p.doc.Sections {
		if !seen[sec.Name] {
			seen[sec.Name] = true
			continue
		}
		p.doc.Errors = append(p.doc.Errors, errs.Error{Code: errs.DuplicateSectionName, Line: 1, Col: 1})
		for n := 2; ; n++ {
			candidate := fmt.Sprintf("%s_%d", sec.Name, n)
			if !seen[candidate] {
				sec.Name = candidate
				seen[candidate] = true
				break
			}
		}
	}
}
