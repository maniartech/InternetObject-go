package document

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// Deciding HOW a string must be spelled.
//
// This file only ANSWERS the question - would this text read back as itself,
// or would the reader see a number, a keyword, a date? The emitters in
// write-string.go act on the answer.

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

// autoString picks the leanest spelling that reads back as the same string.
