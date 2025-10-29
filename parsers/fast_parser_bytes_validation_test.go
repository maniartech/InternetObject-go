package parsers

import (
	"strings"
	"testing"
)

// ========================================
// Escape Sequence Tests
// ========================================

func TestFastParserBytes_EscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple newline",
			input:    `{"text": "Hello\nWorld"}`,
			expected: "Hello\nWorld",
		},
		{
			name:     "Tab character",
			input:    `{"text": "Hello\tWorld"}`,
			expected: "Hello\tWorld",
		},
		{
			name:     "Carriage return",
			input:    `{"text": "Hello\rWorld"}`,
			expected: "Hello\rWorld",
		},
		{
			name:     "Backslash",
			input:    `{"text": "C:\\Users\\file.txt"}`,
			expected: "C:\\Users\\file.txt",
		},
		{
			name:     "Quote",
			input:    `{"text": "He said \"Hello\""}`,
			expected: `He said "Hello"`,
		},
		{
			name:     "Forward slash",
			input:    `{"text": "http:\/\/example.com"}`,
			expected: "http://example.com",
		},
		{
			name:     "Backspace",
			input:    `{"text": "Hello\bWorld"}`,
			expected: "Hello\bWorld",
		},
		{
			name:     "Form feed",
			input:    `{"text": "Hello\fWorld"}`,
			expected: "Hello\fWorld",
		},
		{
			name:     "Multiple escapes",
			input:    `{"text": "Line1\nLine2\tTabbed"}`,
			expected: "Line1\nLine2\tTabbed",
		},
		{
			name:     "Unicode escape - Basic ASCII",
			input:    `{"text": "\u0041"}`,
			expected: "A",
		},
		{
			name:     "Unicode escape - Emoji",
			input:    `{"text": "\u263A"}`,
			expected: "☺",
		},
		{
			name:     "Unicode escape - Chinese",
			input:    `{"text": "\u4E2D\u6587"}`,
			expected: "中文",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			rootIdx, err := parser.Parse()
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			obj := parser.GetValue(rootIdx)
			if obj.Type != TypeObject {
				t.Fatalf("Expected object, got %v", obj.Type)
			}

			val := parser.GetObjectValue(rootIdx, "text")
			if val == nil {
				t.Fatal("'text' key not found")
			}

			actual := parser.GetString(*val)
			if actual != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestFastParserBytes_InvalidEscapeSequences(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		errPart string
	}{
		{
			name:    "Invalid escape character",
			input:   `{"text": "Hello\xWorld"}`,
			errPart: "invalid escape sequence",
		},
		{
			name:    "Incomplete escape at end",
			input:   `{"text": "Hello\`,
			errPart: "unexpected end after escape",
		},
		{
			name:    "Invalid unicode - incomplete",
			input:   `{"text": "\u12"}`,
			errPart: "incomplete",
		},
		{
			name:    "Invalid unicode - non-hex",
			input:   `{"text": "\u12XY"}`,
			errPart: "non-hex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			_, err := parser.Parse()
			if err == nil {
				t.Fatal("Expected error, got none")
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("Expected error containing %q, got: %v", tt.errPart, err)
			}
		})
	}
}

// ========================================
// UTF-8 Validation Tests
// ========================================

func TestFastParserBytes_ValidUTF8(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
	}{
		{
			name:  "ASCII only",
			input: `{"text": "Hello World"}`,
			key:   "text",
		},
		{
			name:  "Latin characters",
			input: `{"text": "Héllo Wörld"}`,
			key:   "text",
		},
		{
			name:  "Emoji",
			input: `{"text": "Hello 😀 World"}`,
			key:   "text",
		},
		{
			name:  "Chinese",
			input: `{"text": "你好世界"}`,
			key:   "text",
		},
		{
			name:  "Japanese",
			input: `{"text": "こんにちは"}`,
			key:   "text",
		},
		{
			name:  "Arabic",
			input: `{"text": "مرحبا"}`,
			key:   "text",
		},
		{
			name:  "Mixed unicode",
			input: `{"text": "Hello 世界 🌍"}`,
			key:   "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			rootIdx, err := parser.Parse()
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			val := parser.GetObjectValue(rootIdx, tt.key)
			if val == nil {
				t.Fatalf("Key %q not found", tt.key)
			}

			str := parser.GetString(*val)
			if len(str) == 0 {
				t.Error("Got empty string")
			}
		})
	}
}

func TestFastParserBytes_InvalidUTF8(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		errPart string
	}{
		{
			name:    "Invalid continuation byte",
			input:   []byte(`{"text": "Hello ` + string([]byte{0x80}) + `"}`),
			errPart: "unexpected continuation byte",
		},
		{
			name:    "Incomplete 2-byte sequence",
			input:   []byte(`{"text": "Hello ` + string([]byte{0xC2}) + `"}`),
			errPart: "incomplete",
		},
		{
			name:    "Incomplete 3-byte sequence",
			input:   []byte(`{"text": "Hello ` + string([]byte{0xE0, 0x80}) + `"}`),
			errPart: "incomplete",
		},
		{
			name:    "Incomplete 4-byte sequence",
			input:   []byte(`{"text": "Hello ` + string([]byte{0xF0, 0x90, 0x80}) + `"}`),
			errPart: "incomplete",
		},
		{
			name:    "Invalid start byte",
			input:   []byte(`{"text": "Hello ` + string([]byte{0xF8}) + `"}`),
			errPart: "invalid start byte",
		},
		{
			name:    "Control character",
			input:   []byte(`{"text": "Hello` + string([]byte{0x01}) + `World"}`),
			errPart: "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytes(tt.input, 10)
			_, err := parser.Parse()
			if err == nil {
				t.Fatal("Expected error, got none")
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("Expected error containing %q, got: %v", tt.errPart, err)
			}
		})
	}
}

// ========================================
// Number Overflow Tests
// ========================================

func TestFastParserBytes_NumberOverflow(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		errPart string
	}{
		{
			name:    "Integer overflow - MaxInt64 + 1",
			input:   `{"num": 9223372036854775808}`,
			errPart: "overflow",
		},
		{
			name:    "Integer overflow - very large",
			input:   `{"num": 99999999999999999999}`,
			errPart: "overflow",
		},
		{
			name:    "Integer overflow - even larger",
			input:   `{"num": 123456789012345678901234567890}`,
			errPart: "overflow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			_, err := parser.Parse()
			if err == nil {
				t.Fatal("Expected overflow error, got none")
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("Expected error containing %q, got: %v", tt.errPart, err)
			}
		})
	}
}

func TestFastParserBytes_ValidLargeNumbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name:     "MaxInt64",
			input:    `{"num": 9223372036854775807}`,
			expected: 9223372036854775807,
		},
		{
			name:     "MinInt64",
			input:    `{"num": -9223372036854775808}`,
			expected: -9223372036854775808,
		},
		{
			name:     "Large positive",
			input:    `{"num": 1234567890123456}`,
			expected: 1234567890123456,
		},
		{
			name:     "Large negative",
			input:    `{"num": -1234567890123456}`,
			expected: -1234567890123456,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			rootIdx, err := parser.Parse()
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			val := parser.GetObjectValue(rootIdx, "num")
			if val == nil {
				t.Fatal("'num' key not found")
			}

			if val.Type != TypeInt {
				t.Fatalf("Expected int, got %v", val.Type)
			}

			if val.IntValue != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, val.IntValue)
			}
		})
	}
}

// ========================================
// Duplicate Key Tests
// ========================================

func TestFastParserBytes_DuplicateKeys(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		errPart string
	}{
		{
			name:    "Simple duplicate",
			input:   `{"name": "John", "name": "Jane"}`,
			errPart: "duplicate key 'name'",
		},
		{
			name:    "Three duplicates",
			input:   `{"id": 1, "id": 2, "id": 3}`,
			errPart: "duplicate key 'id'",
		},
		{
			name:    "Duplicate with different types",
			input:   `{"value": 123, "value": "text"}`,
			errPart: "duplicate key 'value'",
		},
		{
			name:    "Nested object with duplicate in inner",
			input:   `{"outer": {"inner": 1, "inner": 2}}`,
			errPart: "duplicate key 'inner'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			_, err := parser.Parse()
			if err == nil {
				t.Fatal("Expected duplicate key error, got none")
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("Expected error containing %q, got: %v", tt.errPart, err)
			}
		})
	}
}

func TestFastParserBytes_NoDuplicateKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Different keys",
			input: `{"name": "John", "age": 30}`,
		},
		{
			name:  "Similar but different keys",
			input: `{"name": "John", "name1": "Jane"}`,
		},
		{
			name:  "Nested with same key names in different objects",
			input: `{"a": {"name": "John"}, "b": {"name": "Jane"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			_, err := parser.Parse()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

// ========================================
// Trailing Content Tests
// ========================================

func TestFastParserBytes_TrailingContent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		errPart string
	}{
		{
			name:    "Trailing text",
			input:   `{"name": "John"} garbage`,
			errPart: "unexpected content after root value",
		},
		{
			name:    "Trailing number",
			input:   `{"name": "John"} 123`,
			errPart: "unexpected content after root value",
		},
		{
			name:    "Trailing object",
			input:   `{"a": 1} {"b": 2}`,
			errPart: "unexpected content after root value",
		},
		{
			name:    "Trailing comma",
			input:   `{"name": "John"},`,
			errPart: "unexpected content after root value",
		},
		{
			name:    "Array with trailing",
			input:   `[1, 2, 3] extra`,
			errPart: "unexpected content after root value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			_, err := parser.Parse()
			if err == nil {
				t.Fatal("Expected trailing content error, got none")
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("Expected error containing %q, got: %v", tt.errPart, err)
			}
		})
	}
}

func TestFastParserBytes_NoTrailingContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Object with trailing whitespace",
			input: `{"name": "John"}   `,
		},
		{
			name:  "Array with trailing whitespace",
			input: `[1, 2, 3]		`,
		},
		{
			name:  "Object with newlines",
			input: "{\n  \"name\": \"John\"\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewFastParserBytesFromString(tt.input, 10)
			_, err := parser.Parse()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

// ========================================
// Combined Validation Tests
// ========================================

func TestFastParserBytes_ComplexValidation(t *testing.T) {
	// Test with all validations working together
	input := `{
		"name": "John \"The Boss\" Doe",
		"bio": "Line 1\nLine 2\tTabbed",
		"unicode": "Hello 世界 😀",
		"escaped": "C:\\Users\\file.txt",
		"maxInt": 9223372036854775807,
		"nested": {
			"key1": "value1",
			"key2": "value2"
		}
	}`

	parser := NewFastParserBytesFromString(input, 20)
	rootIdx, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify name with escaped quotes
	nameVal := parser.GetObjectValue(rootIdx, "name")
	if nameVal == nil {
		t.Fatal("'name' not found")
	}
	name := parser.GetString(*nameVal)
	if name != `John "The Boss" Doe` {
		t.Errorf("Expected 'John \"The Boss\" Doe', got %q", name)
	}

	// Verify bio with escape sequences
	bioVal := parser.GetObjectValue(rootIdx, "bio")
	if bioVal == nil {
		t.Fatal("'bio' not found")
	}
	bio := parser.GetString(*bioVal)
	if bio != "Line 1\nLine 2\tTabbed" {
		t.Errorf("Expected escape sequences processed, got %q", bio)
	}

	// Verify unicode
	unicodeVal := parser.GetObjectValue(rootIdx, "unicode")
	if unicodeVal == nil {
		t.Fatal("'unicode' not found")
	}
	unicode := parser.GetString(*unicodeVal)
	if unicode != "Hello 世界 😀" {
		t.Errorf("Expected 'Hello 世界 😀', got %q", unicode)
	}

	// Verify escaped path
	escapedVal := parser.GetObjectValue(rootIdx, "escaped")
	if escapedVal == nil {
		t.Fatal("'escaped' not found")
	}
	escaped := parser.GetString(*escapedVal)
	if escaped != `C:\Users\file.txt` {
		t.Errorf("Expected 'C:\\Users\\file.txt', got %q", escaped)
	}

	// Verify max int
	maxIntVal := parser.GetObjectValue(rootIdx, "maxInt")
	if maxIntVal == nil {
		t.Fatal("'maxInt' not found")
	}
	if maxIntVal.IntValue != 9223372036854775807 {
		t.Errorf("Expected MaxInt64, got %d", maxIntVal.IntValue)
	}
}
