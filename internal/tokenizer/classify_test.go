package tokenizer

import (
	"strings"
	"testing"
)

// BareValueReadsNonString is the writer's quote-this question, answered by the
// reader. These tests hold it to two things: every branch of its own logic,
// and — the one that matters — agreement with what scanValue ACTUALLY does
// when handed the same text.

// uniSpace is U+2000 EN QUAD: whitespace to the reader, an ordinary byte to the
// writer's byte table. Built from its code point so no editor can normalize it
// into an ASCII space and quietly turn these cases into duplicates.
var uniSpace = string(rune(0x2000))

func TestBareValueReadsNonString(t *testing.T) {
	sp := uniSpace
	for _, tc := range []struct {
		s    string
		want bool
		why  string
	}{
		{"", false, "empty is not a value"},
		{" x", true, "leading whitespace is outside the contract: quote"},

		{"hello", false, "ordinary text"},
		{"Person 0", false, "a later word is never classified"},
		{"a 2.5e1n", false, "even a broken claim, when it is not first"},
		{"x" + sp + "0.m", false, "...and across a Unicode space"},

		{"12", true, "a lone number reads as a number"},
		{"T", true, "a lone keyword reads as a keyword"},
		{"0x1F", true, "a lone valid based literal"},
		{"12 abc", false, "a valid scalar followed by text is an open string"},
		{"T 1", false, "so is a keyword followed by text"},
		{"0x1F 2", false, "and a valid based literal followed by text"},

		{"0.m", true, "a broken claim alone is an error token"},
		{"0.m 0", true, "a broken claim first errors whatever follows"},
		{"0.m" + sp + "0", true, "and its first word ends at the READER's space"},
		{"2.5e1n a", true, "broken bigint claim first"},
		{"0x1G", true, "broken based literal"},
	} {
		if got := BareValueReadsNonString(tc.s); got != tc.want {
			t.Errorf("BareValueReadsNonString(%q) = %v, want %v — %s", tc.s, got, tc.want, tc.why)
		}
	}
}

// readsBareAsItself reports what the scanner does with s written bare: exactly
// one open-string token whose value is s. This is ground truth, independent of
// the function under test.
func readsBareAsItself(s string) bool {
	st := Tokenize(s)
	return len(st.Tokens) == 1 && st.Tokens[0].Kind == KindString &&
		st.StringValue(st.Tokens[0]) == s
}

// The answer must match the scanner on every input, not just the table above.
// A drift between the two is exactly how a writer emits text its own reader
// rejects — or, the other way, quotes text it never needed to.
func TestBareValueReadsNonStringAgreesWithScanner(t *testing.T) {
	words := []string{
		"a", "Person", "x", "T", "F", "N", "NaN", "Inf", "-Inf", "true", "null",
		"0", "12", "-3", "+3", ".5", "1.5", "1e5", "1e", "0x1F", "0x1G", "0b101",
		"12n", "12.3n", "3.5m", "0.m", "2.5e1n", "0xFFm", "5en", "v2",
	}
	n := 0
	for _, a := range words {
		cases := []string{a}
		for _, b := range words {
			cases = append(cases, a+" "+b, a+uniSpace+b)
		}
		for _, s := range cases {
			n++
			if got, want := BareValueReadsNonString(s), !readsBareAsItself(s); got != want {
				t.Errorf("%q: BareValueReadsNonString = %v, but the scanner says reads-as-itself = %v",
					s, got, !want)
			}
		}
	}
	if n != len(words)*(1+2*len(words)) {
		t.Fatalf("checked %d cases; the generator is broken", n)
	}
}

func FuzzBareValueReadsNonStringAgreesWithScanner(f *testing.F) {
	for _, s := range []string{"Person 0", "0.m 0", "0.m" + uniSpace + "0", "12 abc", "0x1G", "T 1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// Stay inside the contract and away from what this function does not
		// decide: structural characters, quotes, comments and controls take
		// other writer paths, and edge whitespace is quoted before asking.
		if s == "" || !plainValueText(s) {
			return
		}
		if got, want := BareValueReadsNonString(s), !readsBareAsItself(s); got != want {
			t.Fatalf("%q: BareValueReadsNonString = %v, scanner reads-as-itself = %v", s, got, !want)
		}
	})
}

// plainValueText admits text whose bare reading is decided only by the
// numeric/keyword rule: no structural, quoting or control characters, no
// whitespace at either edge, and no `---`, which is the section separator and
// is quoted by the writer's own separate check (the fuzzer found it here).
func plainValueText(s string) bool {
	if strings.Contains(s, "---") {
		return false
	}
	for i, r := range s {
		switch {
		case r == ',' || r == '"' || r == '\'' || r == '\\' || r == '#' || r == '~' ||
			r == '{' || r == '}' || r == '[' || r == ']' || r == ':' || r == '@' || r == '$':
			return false
		case r < 0x20 || r == 0x7f || r == 0xFFFD:
			return false
		case isSpaceRune(r) && (i == 0 || i+len(string(r)) == len(s)):
			return false
		}
	}
	return true
}
