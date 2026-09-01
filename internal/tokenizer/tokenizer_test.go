package tokenizer

import (
	"strings"
	"testing"
)

// These tests pin behaviors the bootstrap CSV does not cover. Each expected
// stream was either derived from the reference tokenizer (io-js2, the oracle)
// on 2026-09-02, or — where the oracle and the specification disagree —
// follows the specification, with the divergence recorded in
// docs/FINDINGS.md. The corpus remains the primary gate; these keep the
// derived rules from regressing silently.

// render prints one stream compactly: KIND[/SUB](text[,code]) per token.
func render(s *Stream) string {
	var parts []string
	for _, t := range s.Tokens {
		p := t.Kind.String()
		if t.Sub != SubNone {
			p += "/" + t.Sub.String()
		}
		p += "(" + s.Text(t)
		if t.Err != CodeNone {
			p += "," + t.Err.String()
		}
		p += ")"
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

func TestOracleDerivedBehaviors(t *testing.T) {
	cases := []struct{ in, want string }{
		// A quote after a mid-run word does not claim an annotation; the run
		// before it is an open string and the quote starts a fresh token.
		{`a "hello"`, `STRING/OPEN_STRING(a) STRING/REGULAR_STRING("hello")`},
		{`a b"SGVsbG8="`, `STRING/OPEN_STRING(a b) STRING/REGULAR_STRING("SGVsbG8=")`},
		// The annotation claim applies to a word of at most 4 characters
		// directly abutting the quote (see maxAnnotationLen).
		{`abcd'`, `ERROR(abcd',unknown-annotation) ERROR(',unterminated-string)`},
		{`abcde'`, `STRING/OPEN_STRING(abcde) ERROR(',unterminated-string)`},
		{`don't stop`, `ERROR(don',unknown-annotation) ERROR('t stop,unterminated-string)`},
		// A numeric word before an abutting quote is a number, not a claim.
		{`5'9`, `NUMBER(5) ERROR('9,unterminated-string)`},
		// The section separator is recognized anywhere, and `--` is text.
		{`a---b`, `STRING/OPEN_STRING(a) SECTION_SEP(---) STRING/SECTION_NAME(b)`},
		{`--`, `STRING/OPEN_STRING(--)`},
		{`----`, `SECTION_SEP(---) STRING/SECTION_NAME(-)`},
		// Section names: same line only, restricted alphabet, no quoted form.
		{"a\n---\nb", `STRING/OPEN_STRING(a) SECTION_SEP(---) STRING/OPEN_STRING(b)`},
		{`--- नाम`, `SECTION_SEP(---) STRING/SECTION_NAME(नाम)`},
		{`--- my.section`, `SECTION_SEP(---) ERROR(my.section,invalid-section-name)`},
		// Newlines do not terminate a value run (rule 1 then makes it text).
		{"1\n2", "STRING/OPEN_STRING(1\n2)"},
		// A scalar word followed by more text is one open string.
		{`5e3 x`, `STRING/OPEN_STRING(5e3 x)`},
		{`true x`, `STRING/OPEN_STRING(true x)`},
		// Suffix claims over other numeric forms.
		{`1.23e2m`, `ERROR(1.23e2m,invalid-decimal)`},
		{`1.2e1n`, `ERROR(1.2e1n,invalid-bigint)`},
		{`0xFFm`, `ERROR(0xFFm,invalid-decimal)`},
		{`0o17n`, `BIGINT/OCTAL(0o17n)`},
		{`0b101n`, `BIGINT/BINARY(0b101n)`},
		// Escape edge cases.
		{`"a\`, `ERROR("a\,invalid-escape-sequence)`},
		{`"ab\"`, `ERROR("ab\",unterminated-string)`},
		// Calendar-strict dates (the spec; the reference agrees since 2026-09).
		{`d"2024-02-30"`, `ERROR(d"2024-02-30",invalid-date)`},
		{`d"2024-02-29"`, `DATETIME/DATE(d"2024-02-29")`},
		{`t"1430"`, `DATETIME/TIME(t"1430")`},
		{`d"202403"`, `DATETIME/DATE(d"202403")`},
		// Spec-mandated temporal forms the reference currently rejects or
		// mishandles — implemented per the specification (FINDINGS 1–3).
		{`t"143045.123"`, `DATETIME/TIME(t"143045.123")`},
		{`dt"2024-03-20T14:30:45+0530"`, `DATETIME/DATETIME(dt"2024-03-20T14:30:45+0530")`},
		{`dt"2024-03-20T14:30:45-08"`, `DATETIME/DATETIME(dt"2024-03-20T14:30:45-08")`},
		{`dt"2024-03-20T1430"`, `DATETIME/DATETIME(dt"2024-03-20T1430")`},
		// Section schema bindings: `--- $a` and `--- name: $ref` (the colon is
		// consumed, not emitted); a non-$ word after `name:` is missing-schema
		// with an empty token, and scanning then continues normally.
		{`--- $a`, `SECTION_SEP(---) STRING/SECTION_SCHEMA($a)`},
		{`--- name: $s`, `SECTION_SEP(---) STRING/SECTION_NAME(name) STRING/SECTION_SCHEMA($s)`},
		{`--- user$x: $s`, `SECTION_SEP(---) ERROR(user$x,invalid-section-name) COLON(:) STRING/OPEN_STRING($s)`},
		{`--- code:en: $b`, `SECTION_SEP(---) STRING/SECTION_NAME(code) ERROR(,missing-schema) STRING/OPEN_STRING(en) COLON(:) STRING/OPEN_STRING($b)`},
		// Out-of-range values stay errors.
		{`dt"2024-03-20T14:30+25:00"`, `ERROR(dt"2024-03-20T14:30+25:00",invalid-datetime)`},
		{`t"25:00"`, `ERROR(t"25:00",invalid-time)`},
	}
	for _, c := range cases {
		if got := render(Tokenize(c.in)); got != c.want {
			t.Errorf("Tokenize(%q):\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}

func TestDecodeValues(t *testing.T) {
	// dt with an offset must keep the instant: 14:30:45+05:30 is 09:00:45Z.
	s := Tokenize(`dt"2024-03-20T14:30:45+05:30"`)
	if got := s.Temporal(s.Tokens[0]).UTC().Format("2006-01-02T15:04:05.000Z"); got != "2024-03-20T09:00:45.000Z" {
		t.Errorf("offset instant = %s", got)
	}
	// Decimal scale is part of the value: 1.50m is coefficient 150, scale 2.
	s = Tokenize(`1.50m`)
	coef, scale := s.DecimalParts(s.Tokens[0])
	if coef.String() != "150" || scale != 2 {
		t.Errorf("DecimalParts(1.50m) = %s, %d; want 150, 2", coef, scale)
	}
	// A surrogate pair written as two backslash-u escapes decodes to one
	// code point (U+1F600).
	pairInput := `"` + `\` + `uD83D` + `\` + `uDE00` + `"`
	s = Tokenize(pairInput)
	if got := s.StringValue(s.Tokens[0]); got != "\U0001F600" {
		t.Errorf("surrogate pair = %q", got)
	}
	// BigInt with exponent.
	s = Tokenize(`12e5n`)
	if got := s.BigInt(s.Tokens[0]).String(); got != "1200000" {
		t.Errorf("12e5n = %s", got)
	}
	// CRLF input is normalized before tokenizing; the open string spans it.
	s = Tokenize("a\r\nb")
	if got := s.StringValue(s.Tokens[0]); got != "a\nb" {
		t.Errorf("crlf = %q", got)
	}
}

func BenchmarkTokenize(b *testing.B) {
	doc := strings.Repeat("~ John Doe, 42, true, {city: New York, zip: \"10001\"}, [1, 2, 3]\n", 100)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		Tokenize(doc)
	}
}
