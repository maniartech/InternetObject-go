package parsers

import (
	"math"
	"testing"
	"time"
)

// White box tests for tokenizer - additional coverage

func TestTokenizer_AnnotatedStrings(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		tokenType TokenType
		subType   string
		checkFunc func(t *testing.T, token *Token)
	}{
		{
			name:      "raw string",
			input:     `r"hello\nworld"`,
			tokenType: TokenString,
			subType:   "RAW_STRING",
			checkFunc: func(t *testing.T, token *Token) {
				if token.Value != `hello\nworld` {
					t.Errorf("Expected raw string to preserve escapes, got %q", token.Value)
				}
			},
		},
		{
			name:      "byte string",
			input:     `b"SGVsbG8="`,
			tokenType: TokenBinary,
			subType:   "BINARY_STRING",
			checkFunc: func(t *testing.T, token *Token) {
				bytes, ok := token.Value.([]byte)
				if !ok {
					t.Errorf("Expected byte slice, got %T", token.Value)
					return
				}
				expected := "Hello"
				if string(bytes) != expected {
					t.Errorf("Expected decoded value %q, got %q", expected, string(bytes))
				}
			},
		},
		{
			name:      "datetime",
			input:     `dt"2023-10-29T15:30:00Z"`,
			tokenType: TokenDateTime,
			subType:   "DATETIME",
			checkFunc: func(t *testing.T, token *Token) {
				_, ok := token.Value.(time.Time)
				if !ok {
					t.Errorf("Expected time.Time, got %T", token.Value)
				}
			},
		},
		{
			name:      "date",
			input:     `d"2023-10-29"`,
			tokenType: TokenDateTime,
			subType:   "DATE",
			checkFunc: func(t *testing.T, token *Token) {
				_, ok := token.Value.(time.Time)
				if !ok {
					t.Errorf("Expected time.Time, got %T", token.Value)
				}
			},
		},
		{
			name:      "time",
			input:     `t"15:30:00"`,
			tokenType: TokenDateTime,
			subType:   "TIME",
			checkFunc: func(t *testing.T, token *Token) {
				_, ok := token.Value.(time.Time)
				if !ok {
					t.Errorf("Expected time.Time, got %T", token.Value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			token := tokens[0]
			if token.Type != tt.tokenType {
				t.Errorf("Expected token type %s, got %s", tt.tokenType, token.Type)
			}

			if token.SubType != tt.subType {
				t.Errorf("Expected subtype %s, got %s", tt.subType, token.SubType)
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, &token)
			}
		})
	}
}

func TestTokenizer_ErrorCases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectError   bool
		errorContains string
	}{
		{
			name:        "unclosed string",
			input:       `"hello`,
			expectError: false, // Tokenizer creates error token but doesn't fail
		},
		{
			name:        "invalid base64",
			input:       `b"not-valid-base64!!!"`,
			expectError: false, // Creates error token
		},
		{
			name:        "invalid datetime",
			input:       `dt"not-a-date"`,
			expectError: false, // Creates error token
		},
		{
			name:        "invalid date",
			input:       `d"99-99-99"`,
			expectError: false, // Creates error token
		},
		{
			name:        "invalid time",
			input:       `t"99:99:99"`,
			expectError: false, // Creates error token
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check if error token was created
			if len(tokens) > 0 && tokens[0].Type == TokenError {
				// This is expected for error cases
				if tokens[0].Value == nil {
					t.Errorf("Error token should have error value")
				}
			}
		})
	}
}

func TestTokenizer_SpecialNumbers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkFunc func(t *testing.T, value interface{})
	}{
		{
			name:  "positive infinity",
			input: "Inf",
			checkFunc: func(t *testing.T, value interface{}) {
				if f, ok := value.(float64); !ok || !math.IsInf(f, 1) {
					t.Errorf("Expected +Inf, got %v", value)
				}
			},
		},
		{
			name:  "negative infinity",
			input: "-Inf",
			checkFunc: func(t *testing.T, value interface{}) {
				if f, ok := value.(float64); !ok || !math.IsInf(f, -1) {
					t.Errorf("Expected -Inf, got %v", value)
				}
			},
		},
		{
			name:  "positive infinity with sign",
			input: "+Inf",
			checkFunc: func(t *testing.T, value interface{}) {
				if f, ok := value.(float64); !ok || !math.IsInf(f, 1) {
					t.Errorf("Expected +Inf, got %v", value)
				}
			},
		},
		{
			name:  "NaN",
			input: "NaN",
			checkFunc: func(t *testing.T, value interface{}) {
				if f, ok := value.(float64); !ok || !math.IsNaN(f) {
					t.Errorf("Expected NaN, got %v", value)
				}
			},
		},
		{
			name:  "scientific notation",
			input: "1.23e-10",
			checkFunc: func(t *testing.T, value interface{}) {
				expected := 1.23e-10
				if f, ok := value.(float64); !ok || math.Abs(f-expected) > 1e-15 {
					t.Errorf("Expected %v, got %v", expected, value)
				}
			},
		},
		{
			name:  "decimal with m suffix",
			input: "3.14m",
			checkFunc: func(t *testing.T, value interface{}) {
				if _, ok := value.(float64); !ok {
					t.Errorf("Expected float64 for decimal, got %T", value)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, tokens[0].Value)
			}
		})
	}
}

func TestTokenizer_ComplexDocument(t *testing.T) {
	input := `
# Header comment
name, age, active
---
--- users: $userSchema
~ John Doe, 30, true
~ Jane Smith, 25, false
---
{admin: true, email: "admin@example.com"}
`

	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	// Verify we got tokens
	if len(tokens) == 0 {
		t.Fatal("Expected tokens but got none")
	}

	// Count section separators
	sectionCount := 0
	collectionCount := 0
	for _, token := range tokens {
		if token.Type == TokenSectionSep {
			sectionCount++
		}
		if token.Type == TokenCollectionStart {
			collectionCount++
		}
	}

	if sectionCount != 3 {
		t.Errorf("Expected 3 section separators, got %d", sectionCount)
	}

	if collectionCount != 2 {
		t.Errorf("Expected 2 collection starts, got %d", collectionCount)
	}
}

func TestTokenizer_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"only whitespace", "   \n\t  ", 0},
		{"only comment", "# comment", 0},
		{"multiple comments", "# line 1\n# line 2", 0},
		{"comma only", ",", 1},
		{"multiple commas", ",,,", 3},
		{"trailing comma", "[1,2,]", 6}, // [ 1 , 2 , ]
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != tt.expected {
				t.Errorf("Expected %d tokens, got %d", tt.expected, len(tokens))
			}
		})
	}
}

func TestTokenizer_UnicodeEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"unicode escape", `"hello\u0041world"`, "helloAworld"},
		{"hex escape", `"test\x41end"`, "testAend"},
		{"multiple escapes", `"\u0048\u0065\u006c\u006c\u006f"`, "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			if tokens[0].Value != tt.expected {
				t.Errorf("Expected value %q, got %q", tt.expected, tokens[0].Value)
			}
		})
	}
}

func TestTokenizer_NumberMergedWithOpenString(t *testing.T) {
	input := "42abc"
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) != 1 {
		t.Fatalf("Expected 1 merged token, got %d", len(tokens))
	}

	token := tokens[0]
	if token.Type != TokenString {
		t.Errorf("Expected STRING token, got %s", token.Type)
	}

	if token.SubType != "OPEN_STRING" {
		t.Errorf("Expected OPEN_STRING subtype, got %s", token.SubType)
	}
}

func TestTokenizer_SectionSchemaVariations(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expectSectionName bool
		expectSchema      bool
		sectionName       string
		schemaName        string
	}{
		{
			name:              "schema only",
			input:             "--- $users",
			expectSectionName: false,
			expectSchema:      true,
			schemaName:        "$users",
		},
		{
			name:              "name only",
			input:             "--- data",
			expectSectionName: true,
			expectSchema:      false,
			sectionName:       "data",
		},
		{
			name:              "name and schema",
			input:             "--- mydata: $schema",
			expectSectionName: true,
			expectSchema:      true,
			sectionName:       "mydata",
			schemaName:        "$schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			// First token should be section separator
			if len(tokens) < 1 || tokens[0].Type != TokenSectionSep {
				t.Fatal("Expected section separator as first token")
			}

			foundName := false
			foundSchema := false

			for _, token := range tokens[1:] {
				if token.SubType == string(TokenSectionName) {
					foundName = true
					if token.Value != tt.sectionName {
						t.Errorf("Expected section name %q, got %q", tt.sectionName, token.Value)
					}
				}
				if token.SubType == string(TokenSectionSchema) {
					foundSchema = true
					if token.Value != tt.schemaName {
						t.Errorf("Expected schema name %q, got %q", tt.schemaName, token.Value)
					}
				}
			}

			if tt.expectSectionName && !foundName {
				t.Error("Expected to find section name but didn't")
			}
			if tt.expectSchema && !foundSchema {
				t.Error("Expected to find schema but didn't")
			}
		})
	}
}

func TestTokenizer_WhitespaceNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "carriage return newline",
			input:    "\"hello\r\nworld\"",
			expected: "hello\nworld",
		},
		{
			name:     "carriage return only",
			input:    "\"hello\rworld\"",
			expected: "hello\nworld",
		},
		{
			name:     "multiple spaces in open string",
			input:    "hello    world",
			expected: "hello    world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			if tokens[0].Value != tt.expected {
				t.Errorf("Expected value %q, got %q", tt.expected, tokens[0].Value)
			}
		})
	}
}
