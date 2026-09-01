package tokenizer

import (
	"strings"
	"unicode/utf8"
)

// Tokenize scans one input into a Stream. It never fails: malformed constructs
// become ERROR tokens, because the format accumulates errors rather than
// failing fast. Line endings are normalized first (CRLF and CR become LF), so
// token text and positions refer to Stream.Src, not the caller's original.
func Tokenize(src string) *Stream {
	s := &scanner{src: normalizeNewlines(src), line: 1, col: 1}
	s.run()
	return &Stream{Src: s.src, Tokens: s.toks}
}

func normalizeNewlines(src string) string {
	if !strings.ContainsRune(src, '\r') {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		if src[i] == '\r' {
			if i+1 < len(src) && src[i+1] == '\n' {
				continue // CRLF: keep the LF only
			}
			b.WriteByte('\n') // lone CR
			continue
		}
		b.WriteByte(src[i])
	}
	return b.String()
}

// isTermByte marks the ASCII bytes that terminate a value run wherever they
// appear: structural characters, both quotes (a quote may not appear in an
// open string), and the comment mark. The three-hyphen section separator is
// the one multi-byte terminator; see tripleDash.
var isTermByte = [256]bool{
	'{': true, '}': true, '[': true, ']': true,
	':': true, ',': true, '~': true,
	'"': true, '\'': true, '#': true,
}

// isASCIISpace covers U+0000..U+0020 — all whitespace per the spec.
func isASCIISpace(c byte) bool { return c <= 0x20 }

// isUniSpace reports the non-ASCII whitespace code points the spec lists
// (whitespaces.md EBNF). The ASCII range is handled byte-wise before this.
func isUniSpace(r rune) bool {
	switch r {
	case 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

func isSpaceRune(r rune) bool { return r <= 0x20 || isUniSpace(r) }

type scanner struct {
	src  string
	pos  int
	line int32
	col  int32
	toks []Token
}

func (s *scanner) add(t Token) { s.toks = append(s.toks, t) }

func (s *scanner) run() {
	for {
		s.skipSpaceAndComments()
		if s.pos >= len(s.src) {
			return
		}
		start, line, col := s.pos, s.line, s.col
		single := func(k Kind) {
			s.pos++
			s.col++
			s.add(Token{Kind: k, Start: int32(start), End: int32(s.pos), Line: line, Col: col})
		}
		switch c := s.src[s.pos]; c {
		case '{':
			single(KindCurlyOpen)
		case '}':
			single(KindCurlyClose)
		case '[':
			single(KindBracketOpen)
		case ']':
			single(KindBracketClose)
		case ':':
			single(KindColon)
		case ',':
			single(KindComma)
		case '~':
			single(KindCollectionStart)
		case '"', '\'':
			s.scanRegular(start, line, col)
		case '-':
			if s.tripleDash(s.pos) {
				s.pos += 3
				s.col += 3
				s.add(Token{Kind: KindSectionSep, Start: int32(start), End: int32(s.pos), Line: line, Col: col})
				s.scanSectionHeader()
			} else {
				s.scanValue()
			}
		default:
			s.scanValue()
		}
	}
}

// tripleDash reports whether the section separator `---` begins at i. It is
// recognized anywhere, not only at line starts — `a---b` reads as the open
// string `a`, a separator, and the section name `b`.
func (s *scanner) tripleDash(i int) bool {
	return i+3 <= len(s.src) && s.src[i] == '-' && s.src[i+1] == '-' && s.src[i+2] == '-'
}

// terminatorAt reports whether a value run ends at i.
func (s *scanner) terminatorAt(i int) bool {
	c := s.src[i]
	return isTermByte[c] || (c == '-' && s.tripleDash(i))
}

func (s *scanner) skipSpaceAndComments() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c == '#' {
			// A comment runs to the end of the line; the newline itself is
			// consumed as whitespace on the next iteration.
			rest := s.src[s.pos:]
			nl := strings.IndexByte(rest, '\n')
			if nl < 0 {
				nl = len(rest)
			}
			s.col += int32(utf8.RuneCountInString(rest[:nl]))
			s.pos += nl
			continue
		}
		if c < utf8.RuneSelf {
			if !isASCIISpace(c) {
				return
			}
			if c == '\n' {
				s.line++
				s.col = 1
			} else {
				s.col++
			}
			s.pos++
			continue
		}
		r, w := utf8.DecodeRuneInString(s.src[s.pos:])
		if !isUniSpace(r) {
			return
		}
		s.pos += w
		s.col++
	}
}

// scanWord advances over one word: up to the next whitespace or terminator.
func (s *scanner) scanWord() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c < utf8.RuneSelf {
			if isASCIISpace(c) || s.terminatorAt(s.pos) {
				return
			}
			s.pos++
			s.col++
			continue
		}
		r, w := utf8.DecodeRuneInString(s.src[s.pos:])
		if isUniSpace(r) {
			return
		}
		s.pos += w
		s.col++
	}
}

// peekPastSpace returns the index of the next non-whitespace byte at or after
// i, without moving the scanner.
func (s *scanner) peekPastSpace(i int) int {
	for i < len(s.src) {
		c := s.src[i]
		if c < utf8.RuneSelf {
			if !isASCIISpace(c) {
				return i
			}
			i++
			continue
		}
		r, w := utf8.DecodeRuneInString(s.src[i:])
		if !isUniSpace(r) {
			return i
		}
		i += w
	}
	return i
}

// maxAnnotationLen bounds the annotation claim: a word of at most this length
// directly abutting a quote reads as an annotation name (r, b, d, t, dt — or
// unknown-annotation). A longer word before a quote is an ordinary open
// string, and the quote starts a fresh token. The specification states no
// length rule; 4 matches the reference implementation, from which the corpus
// derives. Tracked as a spec gap in docs/FINDINGS.md.
const maxAnnotationLen = 4

// scanValue scans one value starting at a non-structural character: a number,
// bigint, decimal, keyword, annotated string, or open string.
//
// The word/run split implements the two numeric rules (number.md): a token is
// numeric only when the ENTIRE run is a valid literal (rule 1 — all or
// nothing), and a base prefix or type suffix that fails to decode is an error
// rather than text (rule 2 — a marker is a claim).
func (s *scanner) scanValue() {
	start, line, col := s.pos, s.line, s.col
	s.scanWord()
	word := s.src[start:s.pos]
	kind, sub, code := classifyWord(word)

	if code != CodeNone {
		// A claimed-and-broken literal: the error covers the word only, and
		// scanning resumes right after it (`0b 1010` → error `0b`, number 1010).
		s.add(Token{Kind: KindError, Err: code, Start: int32(start), End: int32(s.pos), Line: line, Col: col})
		return
	}

	if kind != KindString {
		// A complete scalar word stands only when nothing but whitespace
		// separates it from the next terminator or the end of input; any
		// further text makes the whole run an open string (`5e3 x`).
		j := s.peekPastSpace(s.pos)
		if j >= len(s.src) || s.terminatorAt(j) {
			s.add(Token{Kind: kind, Sub: sub, Start: int32(start), End: int32(s.pos), Line: line, Col: col})
			return
		}
	} else if s.pos < len(s.src) && (s.src[s.pos] == '"' || s.src[s.pos] == '\'') {
		// A word directly abutting a quote is an annotation claim.
		switch word {
		case "r":
			s.scanRaw(start, line, col)
			return
		case "b":
			s.scanBinary(start, line, col)
			return
		case "d":
			s.scanTemporal(start, line, col, SubDate)
			return
		case "t":
			s.scanTemporal(start, line, col, SubTime)
			return
		case "dt":
			s.scanTemporal(start, line, col, SubDateTime)
			return
		default:
			if len(word) <= maxAnnotationLen {
				// The error token includes the quote for context, but scanning
				// resumes AT the quote, which then reads as its own string.
				s.add(Token{Kind: KindError, Err: CodeUnknownAnnotation,
					Start: int32(start), End: int32(s.pos + 1), Line: line, Col: col})
				return
			}
		}
	}

	// Open-string run: continue to the next terminator (newlines do not end a
	// run), then trim trailing whitespace.
	s.scanRun()
	end := s.pos
	for end > start {
		r, w := utf8.DecodeLastRuneInString(s.src[start:end])
		if !isSpaceRune(r) {
			break
		}
		end -= w
	}
	s.add(Token{Kind: KindString, Sub: SubOpenString, Start: int32(start), End: int32(end), Line: line, Col: col})
}

// scanRun advances to the next terminator, consuming internal whitespace and
// newlines (an open string may span lines).
func (s *scanner) scanRun() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c < utf8.RuneSelf {
			if s.terminatorAt(s.pos) {
				return
			}
			if c == '\n' {
				s.line++
				s.col = 1
			} else {
				s.col++
			}
			s.pos++
			continue
		}
		_, w := utf8.DecodeRuneInString(s.src[s.pos:])
		s.pos += w
		s.col++
	}
}

// scanSectionHeader scans what may follow a `---` separator on the same line:
// an optional section name, and an optional `$schema` binding written either
// bare (`--- $a`) or after the name (`--- name: $a`, colon not emitted).
//
// The name grammar is restricted — letters, marks, digits, `-`, `_` — and
// anchored: the name word runs to the next whitespace, colon, `#` or end of
// line (quotes and sigils INCLUDED, so `user'x` is one invalid word, not a
// legal prefix plus junk). A word violating the grammar is
// invalid-section-name; after `name:` anything but a `$ref` is missing-schema
// (an empty-token error), and scanning then continues normally.
func (s *scanner) scanSectionHeader() {
	start, line, col, ok := s.sectionWord()
	if !ok {
		return
	}
	word := s.src[start:s.pos]
	if word[0] == '$' {
		s.add(Token{Kind: KindString, Sub: SubSectionSchema,
			Start: int32(start), End: int32(s.pos), Line: line, Col: col})
		return
	}
	valid := true
	for _, r := range word {
		if !isSectionNameRune(r) {
			valid = false
			break
		}
	}
	if !valid {
		s.add(Token{Kind: KindError, Err: CodeInvalidSectionName,
			Start: int32(start), End: int32(s.pos), Line: line, Col: col})
		return
	}
	s.add(Token{Kind: KindString, Sub: SubSectionName,
		Start: int32(start), End: int32(s.pos), Line: line, Col: col})

	// An optional `: $schema` binding follows the name on the same line.
	i, cols := s.horizontalSpaceEnd(s.pos)
	if i >= len(s.src) || s.src[i] != ':' {
		return
	}
	s.pos, s.col = i+1, s.col+cols+1
	refStart, refLine, refCol, ok := s.sectionWord()
	if !ok || s.src[refStart] != '$' {
		if ok {
			// the word was not a schema ref; put it back for the main loop
			s.pos, s.line, s.col = refStart, refLine, refCol
		}
		s.add(Token{Kind: KindError, Err: CodeMissingSchema,
			Start: int32(s.pos), End: int32(s.pos), Line: s.line, Col: s.col})
		return
	}
	s.add(Token{Kind: KindString, Sub: SubSectionSchema,
		Start: int32(refStart), End: int32(s.pos), Line: refLine, Col: refCol})
}

// horizontalSpaceEnd returns the index after any same-line whitespace at i,
// and how many runes it spans.
func (s *scanner) horizontalSpaceEnd(i int) (int, int32) {
	cols := int32(0)
	for i < len(s.src) {
		r, w := utf8.DecodeRuneInString(s.src[i:])
		if r == '\n' || !isSpaceRune(r) {
			break
		}
		i += w
		cols++
	}
	return i, cols
}

// sectionWord consumes one section-header word: same line, terminated by
// whitespace, `:`, or `#`. A word that would begin with a value-structural
// character (brace, bracket, comma, tilde, quote) is not a section word at
// all — the caller leaves it to the main loop. ok=false when there is none;
// otherwise the word is s.src[start:s.pos].
func (s *scanner) sectionWord() (start int, line, col int32, ok bool) {
	i, cols := s.horizontalSpaceEnd(s.pos)
	if i >= len(s.src) {
		return 0, 0, 0, false
	}
	switch s.src[i] {
	case '\n', ':', '#', '{', '}', '[', ']', ',', '~', '"', '\'':
		return 0, 0, 0, false
	}
	s.pos, s.col = i, s.col+cols
	wstart, wline, wcol := s.pos, s.line, s.col
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c < utf8.RuneSelf {
			if c == '\n' || c == ':' || c == '#' || isASCIISpace(c) {
				break
			}
			s.pos++
			s.col++
			continue
		}
		r, w := utf8.DecodeRuneInString(s.src[s.pos:])
		if isUniSpace(r) {
			break
		}
		s.pos += w
		s.col++
	}
	return wstart, wline, wcol, true
}

// scanRegular scans a quoted string with escape processing. start is the
// token's first byte (the quote); the scanner sits on the quote.
func (s *scanner) scanRegular(start int, line, col int32) {
	q := s.src[s.pos]
	s.pos++
	s.col++
	escErr := false
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == '\\':
			if s.pos+1 >= len(s.src) {
				// A backslash with nothing after it: the escape itself is the
				// fault, not the missing quote.
				s.pos++
				s.col++
				s.add(Token{Kind: KindError, Err: CodeInvalidEscape,
					Start: int32(start), End: int32(s.pos), Line: line, Col: col})
				return
			}
			n := s.src[s.pos+1]
			consumed := 2
			switch {
			case n == 'u':
				// A marker escape claims its digits (regular-strings.md): a
				// malformed \u or \x is an error, unlike other unknown
				// escapes, which stay lenient.
				if countHex(s.src, s.pos+2, 4) == 4 {
					consumed = 6
				} else {
					escErr = true
				}
			case n == 'x':
				if countHex(s.src, s.pos+2, 2) == 2 {
					consumed = 4
				} else {
					escErr = true
				}
			case n >= utf8.RuneSelf:
				consumed = 1 // let the loop handle the multi-byte rune itself
			}
			if n == '\n' && consumed == 2 {
				s.pos += 2
				s.line++
				s.col = 1
			} else {
				s.pos += consumed
				s.col += int32(consumed)
			}
		case c == q:
			s.pos++
			s.col++
			t := Token{Kind: KindString, Sub: SubRegularString,
				Start: int32(start), End: int32(s.pos), Line: line, Col: col}
			if escErr {
				t = Token{Kind: KindError, Err: CodeInvalidEscape,
					Start: int32(start), End: int32(s.pos), Line: line, Col: col}
			}
			s.add(t)
			return
		case c == '\n':
			s.pos++
			s.line++
			s.col = 1
		case c < utf8.RuneSelf:
			s.pos++
			s.col++
		default:
			_, w := utf8.DecodeRuneInString(s.src[s.pos:])
			s.pos += w
			s.col++
		}
	}
	s.add(Token{Kind: KindError, Err: CodeUntermString,
		Start: int32(start), End: int32(len(s.src)), Line: line, Col: col})
}

// scanRaw scans r'…' / r"…", where the only special sequence is the doubled
// enclosing quote. start is the position of the r prefix.
func (s *scanner) scanRaw(start int, line, col int32) {
	q := s.src[s.pos]
	s.pos++
	s.col++
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == q:
			if s.pos+1 < len(s.src) && s.src[s.pos+1] == q {
				s.pos += 2
				s.col += 2
				continue
			}
			s.pos++
			s.col++
			s.add(Token{Kind: KindString, Sub: SubRawString,
				Start: int32(start), End: int32(s.pos), Line: line, Col: col})
			return
		case c == '\n':
			s.pos++
			s.line++
			s.col = 1
		case c < utf8.RuneSelf:
			s.pos++
			s.col++
		default:
			_, w := utf8.DecodeRuneInString(s.src[s.pos:])
			s.pos += w
			s.col++
		}
	}
	s.add(Token{Kind: KindError, Err: CodeUntermString,
		Start: int32(start), End: int32(len(s.src)), Line: line, Col: col})
}

// scanQuotedPlain consumes a quoted span with no escapes (binary and temporal
// literals) and returns the inner content bounds, or ok=false when the
// closing quote is missing (the unterminated error is already emitted).
func (s *scanner) scanQuotedPlain(start int, line, col int32) (innerStart, innerEnd int, ok bool) {
	q := s.src[s.pos]
	s.pos++
	s.col++
	innerStart = s.pos
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == q:
			innerEnd = s.pos
			s.pos++
			s.col++
			return innerStart, innerEnd, true
		case c == '\n':
			s.pos++
			s.line++
			s.col = 1
		case c < utf8.RuneSelf:
			s.pos++
			s.col++
		default:
			_, w := utf8.DecodeRuneInString(s.src[s.pos:])
			s.pos += w
			s.col++
		}
	}
	s.add(Token{Kind: KindError, Err: CodeUntermString,
		Start: int32(start), End: int32(len(s.src)), Line: line, Col: col})
	return 0, 0, false
}

// scanBinary scans b'…' / b"…" and validates the base64 payload.
func (s *scanner) scanBinary(start int, line, col int32) {
	innerStart, innerEnd, ok := s.scanQuotedPlain(start, line, col)
	if !ok {
		return
	}
	t := Token{Kind: KindBinary, Sub: SubBinaryString,
		Start: int32(start), End: int32(s.pos), Line: line, Col: col}
	if !validBase64(s.src[innerStart:innerEnd]) {
		t = Token{Kind: KindError, Err: CodeInvalidBinary,
			Start: int32(start), End: int32(s.pos), Line: line, Col: col}
	}
	s.add(t)
}

// scanTemporal scans d'…', t'…' or dt'…' and validates the content for its
// kind. The error code names the kind: invalid-date, invalid-time,
// invalid-datetime.
func (s *scanner) scanTemporal(start int, line, col int32, sub Sub) {
	innerStart, innerEnd, ok := s.scanQuotedPlain(start, line, col)
	if !ok {
		return
	}
	t := Token{Kind: KindDateTime, Sub: sub,
		Start: int32(start), End: int32(s.pos), Line: line, Col: col}
	if _, ok := parseTemporal(s.src[innerStart:innerEnd], sub); !ok {
		code := CodeInvalidDateTime
		switch sub {
		case SubDate:
			code = CodeInvalidDate
		case SubTime:
			code = CodeInvalidTime
		}
		t = Token{Kind: KindError, Err: code,
			Start: int32(start), End: int32(s.pos), Line: line, Col: col}
	}
	s.add(t)
}

// countHex returns how many of the n bytes at src[i:] are hex digits.
func countHex(src string, i, n int) int {
	k := 0
	for ; k < n && i+k < len(src); k++ {
		if !isHexDigit(src[i+k]) {
			break
		}
	}
	return k
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// validBase64 checks standard RFC 4648 base64 with `=` padding: quads of the
// standard alphabet, padding only at the end.
func validBase64(s string) bool {
	if len(s)%4 != 0 {
		return false
	}
	pad := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '=':
			pad++
			if pad > 2 || i < len(s)-2 {
				return false
			}
		case pad > 0:
			return false // data after padding
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
		default:
			return false
		}
	}
	return true
}
