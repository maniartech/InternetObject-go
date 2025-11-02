package parsers_test

import (
	"testing"

	"github.com/maniartech/InternetObject-go/parsers"
)

// Black box integration tests - testing public API only

func TestTokenizer_PublicAPI_SimpleValues(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantType  parsers.TokenType
	}{
		{"string", `"hello"`, 1, parsers.TokenString},
		{"number", "123", 1, parsers.TokenNumber},
		{"boolean true", "true", 1, parsers.TokenBoolean},
		{"boolean false", "false", 1, parsers.TokenBoolean},
		{"null", "null", 1, parsers.TokenNull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := parsers.NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()

			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != tt.wantCount {
				t.Errorf("Expected %d tokens, got %d", tt.wantCount, len(tokens))
			}

			if len(tokens) > 0 && tokens[0].Type != tt.wantType {
				t.Errorf("Expected token type %s, got %s", tt.wantType, tokens[0].Type)
			}
		})
	}
}

func TestTokenizer_PublicAPI_DataStructures(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "simple object",
			input:     `{name: "John", age: 30}`,
			wantError: false,
		},
		{
			name:      "simple array",
			input:     `[1, 2, 3, 4, 5]`,
			wantError: false,
		},
		{
			name:      "nested object",
			input:     `{user: {name: "John", email: "john@example.com"}}`,
			wantError: false,
		},
		{
			name:      "array of objects",
			input:     `[{id: 1}, {id: 2}]`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := parsers.NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()

			if (err != nil) != tt.wantError {
				t.Errorf("Tokenize() error = %v, wantError %v", err, tt.wantError)
			}

			if !tt.wantError && len(tokens) == 0 {
				t.Error("Expected tokens but got none")
			}
		})
	}
}

func TestTokenizer_PublicAPI_DocumentStructure(t *testing.T) {
	input := `
# Schema definition
name, age, email
---
# First user
John Doe, 30, john@example.com
---
# Second user
Jane Smith, 25, jane@example.com
`

	tokenizer := parsers.NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("Expected tokens but got none")
	}

	// Verify document structure
	sectionSeps := 0
	for _, token := range tokens {
		if token.Type == parsers.TokenSectionSep {
			sectionSeps++
		}
	}

	if sectionSeps != 2 {
		t.Errorf("Expected 2 section separators, got %d", sectionSeps)
	}
}

func TestTokenizer_PublicAPI_Collections(t *testing.T) {
	input := `
~ {name: "John", age: 30}
~ {name: "Jane", age: 25}
~ {name: "Bob", age: 35}
`

	tokenizer := parsers.NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	collectionStarts := 0
	for _, token := range tokens {
		if token.Type == parsers.TokenCollectionStart {
			collectionStarts++
		}
	}

	if collectionStarts != 3 {
		t.Errorf("Expected 3 collection starts, got %d", collectionStarts)
	}
}

func TestTokenizer_PublicAPI_AnnotatedStrings(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectType  parsers.TokenType
		expectError bool
	}{
		{
			name:        "raw string",
			input:       `r"raw\nstring"`,
			expectType:  parsers.TokenString,
			expectError: false,
		},
		{
			name:        "byte string",
			input:       `b"SGVsbG8="`,
			expectType:  parsers.TokenBinary,
			expectError: false,
		},
		{
			name:        "datetime",
			input:       `dt"2023-10-29T15:30:00Z"`,
			expectType:  parsers.TokenDateTime,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := parsers.NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()

			if (err != nil) != tt.expectError {
				t.Errorf("Tokenize() error = %v, expectError %v", err, tt.expectError)
			}

			if !tt.expectError && len(tokens) > 0 {
				if tokens[0].Type != tt.expectType {
					t.Errorf("Expected type %s, got %s", tt.expectType, tokens[0].Type)
				}
			}
		})
	}
}

func TestTokenizer_PublicAPI_ErrorRecovery(t *testing.T) {
	// Test that tokenizer handles errors gracefully
	inputs := []string{
		`"unclosed string`,
		`b"invalid-base64!!!"`,
		`dt"not-a-date"`,
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			tokenizer := parsers.NewTokenizer(input)
			tokens, err := tokenizer.Tokenize()

			// Tokenizer should not panic and should return something
			if err != nil {
				t.Logf("Got error (expected): %v", err)
			}

			if tokens == nil {
				t.Error("Tokens should not be nil even on error")
			}
		})
	}
}

func TestTokenizer_PublicAPI_RealWorldExample(t *testing.T) {
	input := `
# Internet Object Example
# User database schema and data

# Schema definition
$userSchema: {
  name: string,
  age: number,
  email: string,
  active: boolean
}

---

--- users: $userSchema

~ John Doe, 30, john@example.com, true
~ Jane Smith, 25, jane@example.com, true
~ Bob Johnson, 35, bob@example.com, false

---

--- summary

{
  total: 3,
  active: 2,
  lastUpdate: dt"2023-10-29T15:30:00Z"
}
`

	tokenizer := parsers.NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("Expected tokens but got none")
	}

	// Verify key elements
	stats := make(map[parsers.TokenType]int)
	for _, token := range tokens {
		stats[token.Type]++
	}

	if stats[parsers.TokenSectionSep] < 3 {
		t.Errorf("Expected at least 3 section separators, got %d", stats[parsers.TokenSectionSep])
	}

	if stats[parsers.TokenCollectionStart] < 3 {
		t.Errorf("Expected at least 3 collection starts, got %d", stats[parsers.TokenCollectionStart])
	}

	t.Logf("Token statistics: %+v", stats)
}

func TestTokenizer_PublicAPI_ConcurrentParsing(t *testing.T) {
	// Test thread safety - multiple tokenizers running concurrently
	inputs := []string{
		`"test1"`,
		`"test2"`,
		`{key: "value"}`,
		`[1, 2, 3]`,
		`true`,
		`42`,
	}

	done := make(chan bool, len(inputs))

	for _, input := range inputs {
		go func(inp string) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Panic during concurrent tokenization: %v", r)
				}
				done <- true
			}()

			tokenizer := parsers.NewTokenizer(inp)
			_, err := tokenizer.Tokenize()
			if err != nil {
				t.Errorf("Error during concurrent tokenization: %v", err)
			}
		}(input)
	}

	// Wait for all goroutines
	for i := 0; i < len(inputs); i++ {
		<-done
	}
}
