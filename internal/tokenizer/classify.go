package tokenizer

import "unicode"

// This file is the ONE site that decides what a whitespace-free word is:
// keyword, number, bigint, decimal, a claimed-and-broken literal, or ordinary
// text. Every caller goes through classifyWord — the porting notes' central
// lesson is that this decision, duplicated, is where silent data loss starts.
//
// The two rules (io-specs number.md):
//
//	Rule 1 — all or nothing: a run is numeric only if the ENTIRE run is a
//	valid literal; otherwise it is an open string, never a partial number.
//	Rule 2 — a marker is a claim: the base prefixes 0x/0o/0b and the type
//	suffixes m/n can only mean "number", so a run that carries one and does
//	not decode is an error, not a string.

// WordReadsNonString reports whether a bare, whitespace-free word would read
// back as something other than ordinary text: a keyword, a number in any
// form, or a claimed-and-broken literal. It is the writer's quote-this test,
// answered by the reader's own classifier so the two can never disagree.
func WordReadsNonString(w string) bool {
	if w == "" {
		return false
	}
	k, _, _ := classifyWord(w)
	return k != KindString
}

// WordIsBrokenClaim reports whether a bare word is a claimed-and-broken
// numeric literal (rule 2) — an error even in the middle of an open-string
// run, where an ordinary number would just join the run. The writer quotes
// any string containing one.
func WordIsBrokenClaim(w string) bool {
	if w == "" {
		return false
	}
	k, _, _ := classifyWord(w)
	return k == KindError
}

// classifyWord classifies one non-empty word. A CodeNone/KindString result
// means "ordinary text" — the scanner then continues the open-string run.
func classifyWord(w string) (Kind, Sub, Code) {
	switch w {
	case "true", "false", "T", "F":
		return KindBoolean, SubNone, CodeNone
	case "null", "N":
		return KindNull, SubNone, CodeNone
	case "NaN", "Inf", "+Inf", "-Inf":
		return KindNumber, SubNone, CodeNone
	}

	c := w[0]
	if c != '+' && c != '-' && c != '.' && (c < '0' || c > '9') {
		return KindString, SubOpenString, CodeNone
	}
	body := w
	if c == '+' || c == '-' {
		body = w[1:]
	}

	if len(body) >= 2 && body[0] == '0' && baseFor(body[1]) != 0 {
		return classifyBased(body)
	}

	if n := len(body); n > 0 {
		switch body[n-1] {
		case 'n': // bigint claim
			m := body[:n-1]
			if !isNumberForm(m) && !isDigitsDots(m) {
				return KindString, SubOpenString, CodeNone // `5en`, `123nn`: nothing claimed
			}
			if isBigIntForm(m) {
				return KindBigInt, SubNone, CodeNone
			}
			return KindError, SubNone, CodeInvalidBigInt // `12.3n`, `12e-5n`
		case 'm': // decimal claim
			m := body[:n-1]
			if !isNumberForm(m) && !isDigitsDots(m) {
				return KindString, SubOpenString, CodeNone // `5em`, `123.45mm`
			}
			if isDecimalForm(m) {
				return KindDecimal, SubNone, CodeNone
			}
			return KindError, SubNone, CodeInvalidDecimal // `.5m`, `123.m`, `1.23e2m`
		}
	}

	if isNumberForm(body) {
		return KindNumber, SubNone, CodeNone
	}
	return KindString, SubOpenString, CodeNone
}

// baseFor maps a base-marker character to its radix (0 when not a marker).
func baseFor(c byte) int {
	switch c {
	case 'x', 'X':
		return 16
	case 'o', 'O':
		return 8
	case 'b', 'B':
		return 2
	}
	return 0
}

// classifyBased classifies a word that begins with a base prefix (sign already
// stripped). The prefix is a claim: anything that does not decode as that base
// is an error, never a string.
func classifyBased(body string) (Kind, Sub, Code) {
	base := baseFor(body[1])
	sub := SubHex
	switch base {
	case 8:
		sub = SubOctal
	case 2:
		sub = SubBinary
	}
	digits := body[2:]
	kind := KindNumber
	if n := len(digits); n > 0 {
		switch digits[n-1] {
		case 'n':
			kind = KindBigInt
			digits = digits[:n-1]
		case 'm':
			// A decimal cannot be written in another base: `0xFFm` claims a
			// decimal over a valid hex mantissa and is invalid-decimal; a
			// broken mantissa is just an invalid number.
			if len(digits) > 1 && validDigits(digits[:n-1], base) {
				return KindError, SubNone, CodeInvalidDecimal
			}
			return KindError, SubNone, CodeInvalidNumber
		}
	}
	if len(digits) == 0 || !validDigits(digits, base) {
		return KindError, SubNone, CodeInvalidNumber
	}
	return kind, sub, CodeNone
}

func validDigits(s string, base int) bool {
	for i := 0; i < len(s); i++ {
		if digitVal(s[i]) >= base {
			return false
		}
	}
	return len(s) > 0
}

func digitVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 99
}

// isNumberForm reports a full match of the plain number grammar (no sign):
// (digit+ ["." digit*] | "." digit+) [("e"|"E") ["+"|"-"] digit+]
func isNumberForm(s string) bool {
	i, n := 0, len(s)
	digits := func() int {
		k := 0
		for i < n && s[i] >= '0' && s[i] <= '9' {
			i++
			k++
		}
		return k
	}
	intDigits := digits()
	fracDigits := -1
	if i < n && s[i] == '.' {
		i++
		fracDigits = digits()
	}
	if intDigits == 0 && fracDigits <= 0 {
		return false // ".", "", "e5"
	}
	if i < n && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < n && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if digits() == 0 {
			return false
		}
	}
	return i == n
}

// isDigitsDots reports a word of only digits and dots with at least one digit
// — not a valid number, but numeric enough that an m/n suffix on it is a
// claim (`12.34.56m` is invalid-decimal, not text).
func isDigitsDots(s string) bool {
	hasDigit := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] >= '0' && s[i] <= '9':
			hasDigit = true
		case s[i] == '.':
		default:
			return false
		}
	}
	return hasDigit
}

// maxBigIntExponent bounds a bigint literal's exponent. An unbounded one is
// a denial of service — `1e1444444440n` would materialize 1.4 billion digits
// — and the reference has no designed bound either: it grinds and then throws
// V8's bare "Maximum BigInt size exceeded", an uncoded error (FINDINGS #13).
//
// Measured on this machine: 1e4 is instant, 1e5 costs ~1ms, 1e6 costs ~2.8s
// and was still enough for a fuzz worker to be killed. 10,000 digits is far
// beyond any real datum and decodes in microseconds, so that is the bound;
// beyond it the marker is a broken claim, which is invalid-bigint.
const maxBigIntExponent = 10_000

// isBigIntForm: digit+ [("e"|"E") ["+"] digit+] — integers only, and only a
// non-negative exponent (12e5n is 1200000n; 12e-5n cannot be an integer).
func isBigIntForm(s string) bool {
	i, n := 0, len(s)
	k := 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
		k++
	}
	if k == 0 {
		return false
	}
	if i == n {
		return true
	}
	if s[i] != 'e' && s[i] != 'E' {
		return false
	}
	i++
	if i < n && s[i] == '+' {
		i++
	}
	k = 0
	exp := 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		if exp <= maxBigIntExponent {
			exp = exp*10 + int(s[i]-'0')
		}
		i++
		k++
	}
	return k > 0 && i == n && exp <= maxBigIntExponent
}

// isDecimalForm: digit+ ["." digit+] — a decimal requires a leading digit
// and, with a point, a trailing digit; scientific notation is not supported.
func isDecimalForm(s string) bool {
	i, n := 0, len(s)
	k := 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
		k++
	}
	if k == 0 {
		return false
	}
	if i == n {
		return true
	}
	if s[i] != '.' {
		return false
	}
	i++
	k = 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
		k++
	}
	return k > 0 && i == n
}

// isSectionNameRune reports the section-name alphabet: letters, marks,
// digits, hyphen and underscore. There is no quoted form.
func isSectionNameRune(r rune) bool {
	return r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r)
}

// ValidSectionName reports whether name satisfies the section-name grammar —
// the writer's "can this name be spelled at all?" test, answered by the
// scanner's own rule.
func ValidSectionName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !isSectionNameRune(r) {
			return false
		}
	}
	return true
}
