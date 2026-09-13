package document

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// The writer's string spelling must be SAFE — never bare when the reader would
// read something else — and MINIMAL — never quoted when bare text reads back as
// itself. The round-trip fuzzers have always guarded the first. Nothing guarded
// the second, which is how a fix that quoted `Person 0` for no reason made the
// 1,000-record marshal 3% larger and pushed it from 1.17x faster than
// encoding/json to 1.5x slower, unnoticed (OPEN-QUESTIONS #4, 2026-09-13).
//
// Both directions are asserted against the READER itself, not a list of
// expectations: the scanner is the only authority on what bare text means.

// readsBareAsItself is ground truth: the scanner turns bare s into exactly one
// string token whose value is s.
func readsBareAsItself(s string) bool {
	st := tokenizer.Tokenize(s)
	return len(st.Tokens) == 1 && st.Tokens[0].Kind == tokenizer.KindString &&
		st.StringValue(st.Tokens[0]) == s
}

// deliberatelyQuoted lists text the writer quotes although the reader would
// read it bare — kept, on purpose, and named here so the property below does
// not hide a new over-quote behind an old one:
//
//   - date-, time- and datetime-like text (`2024-03-20`), which a consumer
//     binding the value to a temporal member would otherwise have to guess
//     about;
//   - `undefined`, a keyword in the reference's host language.
func deliberatelyQuoted(s string) bool {
	return s == "undefined" || isDateLike(s) || isTimeLike(s) || isDateTimeLike(s)
}

// decidedElsewhere reports text whose spelling is decided by rules other than
// the keyword/number one: structural and quoting characters, controls, a
// leading reference sigil, edge whitespace and the section separator.
func decidedElsewhere(s string) bool {
	if s == "" || strings.Contains(s, "---") {
		return true
	}
	if s[0] == '@' || s[0] == '$' {
		return true // a variable or schema reference — only at the START of a value
	}
	for i, r := range s {
		switch {
		case strings.ContainsRune(`,"'\#~{}[]:`, r), r == '*':
			return true
		case r < 0x20 || r == 0x7f || r == 0xFFFD:
			return true
		case tokenizer.IsSpaceRune(r) && (i == 0 || i+len(string(r)) == len(s)):
			return true
		}
	}
	return false
}

// checkSafe holds for EVERY string: whatever spelling the writer picks — bare,
// open-escaped, raw or quoted — the reader turns it into exactly one string
// token carrying s. It deliberately has no excluded domain.
//
// An earlier version of this test excluded structural and control characters
// from BOTH properties, because minimality is decided elsewhere there. Safety is
// not, and the exclusion hid the one bug that mattered: `0X0<TAB>"` is written
// `0X0\t\"`, a single word to the reader and a broken hex claim. The round-trip
// fuzzers caught it; this test did not. Minimality needs a narrow domain;
// safety never does.
func checkSafe(t *testing.T, s string) {
	t.Helper()
	written := string(appendAutoString(nil, s))
	st := tokenizer.Tokenize(written)
	if len(st.Tokens) != 1 || st.Tokens[0].Kind != tokenizer.KindString {
		t.Errorf("UNSAFE: %q written as %s, which the reader does not read as one string", s, written)
		return
	}
	if got := st.StringValue(st.Tokens[0]); got != s {
		t.Errorf("UNSAFE: %q written as %s reads back as %q", s, written, got)
	}
}

// checkMinimal holds where the keyword/number rule alone decides the spelling:
// never quoted when the bare text reads back as itself.
func checkMinimal(t *testing.T, s string) {
	t.Helper()
	written := string(appendAutoString(nil, s))
	if written != s && readsBareAsItself(s) {
		t.Errorf("OVER-QUOTED: %q reads back bare, but was written %s", s, written)
	}
}

func TestStringQuotingIsSafeAndMinimal(t *testing.T) {
	sp := string(rune(0x2000)) // a reader space the writer's byte table does not know
	for _, s := range []string{
		// the regression: a later numeric word never needs quotes
		"Person 0", "Person 1000", "v2 release", "T 1", "1 T", "12 abc", "1 2",
		"x 0.m", "a 2.5e1n", "a 0x1G", "x" + sp + "0.m", "person0@example.com",
		// the bug the regressing commit fixed: still quoted
		"0.m", "0.m 0", "0.m" + sp + "0", "2.5e1n", "2.5e1n a", "0x1G",
		// lone scalars and keywords read as themselves, so they are quoted
		"12", "-3", ".5", "1e5", "0x1F", "12n", "3.5m", "T", "null", "NaN", "-Inf",
	} {
		if decidedElsewhere(s) || deliberatelyQuoted(s) {
			t.Fatalf("%q is outside the minimality domain; the table is wrong", s)
		}
		checkSafe(t, s)
		checkMinimal(t, s)
	}

	// Safety alone, across the spellings minimality does not decide — including
	// the escaped-whitespace case the round-trip fuzzers found (2026-09-13).
	for _, s := range []string{
		"0X0\t\"", "0B0\t\"", "0x1\t{", "12\n{", "5\t:x", "a,b", "{x}", "@var", "$ref",
		"", " lead", "trail ", "---", "2024-03-20", "undefined", "tab\there", "line\nbreak",
	} {
		checkSafe(t, s)
	}
}

func FuzzStringQuotingIsSafeAndMinimal(f *testing.F) {
	for _, s := range []string{"Person 0", "0.m 0", "0.m" + string(rune(0x2000)) + "0",
		"12 abc", "a 2.5e1n", "0X0\t\"", "0B0\t\""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return // the writer passes bytes through; invalid UTF-8 has no string to compare
		}
		checkSafe(t, s)
		if !decidedElsewhere(s) && !deliberatelyQuoted(s) {
			checkMinimal(t, s)
		}
	})
}
