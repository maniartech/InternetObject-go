package parsers

import (
	"strings"
	"testing"
)

// TestFastParserBytes_UnicodeWhitespace tests Unicode whitespace support
// matching the TypeScript isWhitespace specification
func TestFastParserBytes_UnicodeWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		desc  string
	}{
		{
			name:  "ASCII whitespace",
			input: `{  "a"  :  1  }`, // spaces
			desc:  "Regular spaces (U+0020)",
		},
		{
			name:  "Tab characters",
			input: "{\t\"a\"\t:\t1\t}", // tabs
			desc:  "Tab characters (U+0009)",
		},
		{
			name:  "Newline characters",
			input: "{\n\"a\"\n:\n1\n}",
			desc:  "Newline (U+000A)",
		},
		{
			name:  "Carriage return",
			input: "{\r\"a\"\r:\r1\r}",
			desc:  "Carriage return (U+000D)",
		},
		{
			name:  "Non-breaking space",
			input: "{\u00A0\"a\"\u00A0:\u00A01\u00A0}", // U+00A0
			desc:  "Non-breaking space (U+00A0)",
		},
		{
			name:  "Ogham space mark",
			input: "{\u1680\"a\"\u1680:\u16801\u1680}", // U+1680
			desc:  "Ogham space mark (U+1680)",
		},
		{
			name:  "En quad",
			input: "{\u2000\"a\"\u2000:\u20001\u2000}", // U+2000
			desc:  "En quad (U+2000)",
		},
		{
			name:  "Em quad",
			input: "{\u2001\"a\"\u2001:\u20011\u2001}", // U+2001
			desc:  "Em quad (U+2001)",
		},
		{
			name:  "En space",
			input: "{\u2002\"a\"\u2002:\u20021\u2002}", // U+2002
			desc:  "En space (U+2002)",
		},
		{
			name:  "Em space",
			input: "{\u2003\"a\"\u2003:\u20031\u2003}", // U+2003
			desc:  "Em space (U+2003)",
		},
		{
			name:  "Three-per-em space",
			input: "{\u2004\"a\"\u2004:\u20041\u2004}", // U+2004
			desc:  "Three-per-em space (U+2004)",
		},
		{
			name:  "Four-per-em space",
			input: "{\u2005\"a\"\u2005:\u20051\u2005}", // U+2005
			desc:  "Four-per-em space (U+2005)",
		},
		{
			name:  "Six-per-em space",
			input: "{\u2006\"a\"\u2006:\u20061\u2006}", // U+2006
			desc:  "Six-per-em space (U+2006)",
		},
		{
			name:  "Figure space",
			input: "{\u2007\"a\"\u2007:\u20071\u2007}", // U+2007
			desc:  "Figure space (U+2007)",
		},
		{
			name:  "Punctuation space",
			input: "{\u2008\"a\"\u2008:\u20081\u2008}", // U+2008
			desc:  "Punctuation space (U+2008)",
		},
		{
			name:  "Thin space",
			input: "{\u2009\"a\"\u2009:\u20091\u2009}", // U+2009
			desc:  "Thin space (U+2009)",
		},
		{
			name:  "Hair space",
			input: "{\u200A\"a\"\u200A:\u200A1\u200A}", // U+200A
			desc:  "Hair space (U+200A)",
		},
		{
			name:  "Line separator",
			input: "{\u2028\"a\"\u2028:\u20281\u2028}", // U+2028
			desc:  "Line separator (U+2028)",
		},
		{
			name:  "Paragraph separator",
			input: "{\u2029\"a\"\u2029:\u20291\u2029}", // U+2029
			desc:  "Paragraph separator (U+2029)",
		},
		{
			name:  "Narrow no-break space",
			input: "{\u202F\"a\"\u202F:\u202F1\u202F}", // U+202F
			desc:  "Narrow no-break space (U+202F)",
		},
		{
			name:  "Medium mathematical space",
			input: "{\u205F\"a\"\u205F:\u205F1\u205F}", // U+205F
			desc:  "Medium mathematical space (U+205F)",
		},
		{
			name:  "Ideographic space",
			input: "{\u3000\"a\"\u3000:\u30001\u3000}", // U+3000
			desc:  "Ideographic space (U+3000)",
		},
		{
			name:  "Zero width no-break space (BOM)",
			input: "{\uFEFF\"a\"\uFEFF:\uFEFF1\uFEFF}", // U+FEFF
			desc:  "Zero width no-break space/BOM (U+FEFF)",
		},
		{
			name:  "Mixed Unicode whitespace",
			input: "{\u00A0\u2000\u3000\"a\"\u2028:\u20291\uFEFF}",
			desc:  "Mixed Unicode whitespace characters",
		},
		{
			name:  "Control characters",
			input: "{\u0000\u0001\u0002\"a\":\u00101\u001F}",
			desc:  "Control characters (U+0000-U+001F)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 100)
			rootIdx, err := parser.Parse()
			if err != nil {
				t.Fatalf("Failed to parse %s: %v\nInput: %q", tt.desc, err, tt.input)
			}

			root := parser.GetValue(rootIdx)
			if root.Type != TypeObject {
				t.Fatalf("Expected object, got %v", root.Type)
			}

			// Verify we can access the "a" key
			val := parser.GetObjectValue(rootIdx, "a")
			if val == nil {
				t.Fatalf("Expected to find key 'a' in object")
			}

			if val.Type != TypeInt {
				t.Fatalf("Expected int type, got %v", val.Type)
			}

			if val.IntValue != 1 {
				t.Errorf("Expected value 1, got %d", val.IntValue)
			}
		})
	}
}

// TestFastParserBytes_NonWhitespace tests that non-whitespace Unicode chars are NOT treated as whitespace
func TestFastParserBytes_NonWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		desc  string
	}{
		{
			name:  "Regular character after U+00FF",
			input: `{"a":1}Ā`, // U+0100 (not whitespace)
			desc:  "U+0100 should trigger trailing content error",
		},
		{
			name:  "Character above U+FEFF",
			input: `{"a":1}𐀀`, // U+10000 (not whitespace)
			desc:  "U+10000 should trigger trailing content error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 100)
			_, err := parser.Parse()
			if err == nil {
				t.Fatalf("Expected error for %s, got none", tt.desc)
			}
			if !strings.Contains(err.Error(), "unexpected content after root value") {
				t.Errorf("Expected 'unexpected content after root value' error, got: %v", err)
			}
		})
	}
}

// TestFastParserBytes_WhitespaceInStrings tests that whitespace inside strings is preserved
func TestFastParserBytes_WhitespaceInStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Non-breaking space in string",
			input:    `{"text":"Hello\u00A0World"}`,
			expected: "Hello\u00A0World",
		},
		{
			name:     "Ideographic space in string",
			input:    `{"text":"你好\u3000世界"}`,
			expected: "你好\u3000世界",
		},
		{
			name:     "Em space in string",
			input:    `{"text":"A\u2003B"}`,
			expected: "A\u2003B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 100)
			rootIdx, err := parser.Parse()
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			val := parser.GetObjectValue(rootIdx, "text")
			if val == nil {
				t.Fatalf("Expected to find key 'text'")
			}

			str := parser.GetString(*val)
			if str != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, str)
			}
		})
	}
}
