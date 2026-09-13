package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// Through the public API: Marshal quotes a string only when bare text would not
// read back as that string, and every spelling round-trips. Pins OPEN-QUESTIONS
// #4 (2026-09-13), where `Person 0` was being quoted for no reason and the
// 1,000-record marshal fell from faster than encoding/json to 1.5x slower.
func TestMarshalQuotesOnlyWhatTheReaderNeeds(t *testing.T) {
	type row struct {
		Name string `io:"name"`
	}
	sp := string(rune(0x2000)) // whitespace to the reader
	for _, tc := range []struct {
		s    string
		bare bool
	}{
		{"Person 0", true},            // the regression
		{"hello 12.5 world", true},    // no later word is ever classified
		{"a 2.5e1n", true},            // not even a broken claim
		{"12 abc", true},              // a valid scalar followed by text
		{"0.m" + sp + "0", false},     // the bug the regressing commit fixed
		{"0.m 0", false},              // a broken claim first errors
		{"42", false},                 // a lone number reads as a number
		{"T", false},                  // a lone keyword reads as a keyword
		{"person0@example.com", true}, // `@` matters only at the start
	} {
		text, err := io.Marshal(row{tc.s})
		if err != nil {
			t.Fatalf("Marshal(%q): %v", tc.s, err)
		}
		lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
		value := lines[len(lines)-1]
		if gotBare := value == tc.s; gotBare != tc.bare {
			t.Errorf("Marshal(%q) wrote the value as %s; want bare=%v", tc.s, value, tc.bare)
		}
		var back row
		if err := io.Unmarshal(text, &back); err != nil {
			t.Errorf("Unmarshal(%q): %v", text, err)
		} else if back.Name != tc.s {
			t.Errorf("round trip: %q -> %q -> %q", tc.s, text, back.Name)
		}
	}
}
