package parsers

import (
	"testing"
)

func TestTokenizer_SimpleString(t *testing.T) {
	input := `"hello world"`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) != 1 {
		t.Fatalf("Expected 1 token, got %d", len(tokens))
	}

	token := tokens[0]
	if token.Type != TokenString {
		t.Errorf("Expected token type STRING, got %v", token.Type)
	}

	if token.Value != "hello world" {
		t.Errorf("Expected value 'hello world', got '%v'", token.Value)
	}
}

func TestTokenizer_Numbers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  interface{}
		tokenType TokenType
	}{
		{"integer", "42", int64(42), TokenNumber},
		{"float", "3.14", 3.14, TokenNumber},
		{"negative", "-10", int64(-10), TokenNumber},
		{"hex", "0xFF", int64(255), TokenNumber},
		{"octal", "0o77", int64(63), TokenNumber},
		{"binary", "0b1010", int64(10), TokenNumber},
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
				t.Errorf("Expected token type %v, got %v", tt.tokenType, token.Type)
			}

			// For floats, compare with tolerance
			if fExpected, ok := tt.expected.(float64); ok {
				if fActual, ok := token.Value.(float64); ok {
					diff := fActual - fExpected
					if diff < -0.0001 || diff > 0.0001 {
						t.Errorf("Expected value %v, got %v", tt.expected, token.Value)
					}
				} else {
					t.Errorf("Expected float64 value, got %T", token.Value)
				}
			} else {
				if token.Value != tt.expected {
					t.Errorf("Expected value %v, got %v", tt.expected, token.Value)
				}
			}
		})
	}
}

func TestTokenizer_Booleans(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"T", true},
		{"false", false},
		{"F", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()

			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			token := tokens[0]
			if token.Type != TokenBoolean {
				t.Errorf("Expected token type BOOLEAN, got %v", token.Type)
			}

			if token.Value != tt.expected {
				t.Errorf("Expected value %v, got %v", tt.expected, token.Value)
			}
		})
	}
}

func TestTokenizer_Null(t *testing.T) {
	tests := []string{"null", "N"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			tokenizer := NewTokenizer(input)
			tokens, err := tokenizer.Tokenize()

			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			token := tokens[0]
			if token.Type != TokenNull {
				t.Errorf("Expected token type NULL, got %v", token.Type)
			}

			if token.Value != nil {
				t.Errorf("Expected value nil, got %v", token.Value)
			}
		})
	}
}

func TestTokenizer_Symbols(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenType
	}{
		{"{", TokenCurlyOpen},
		{"}", TokenCurlyClose},
		{"[", TokenBracketOpen},
		{"]", TokenBracketClose},
		{":", TokenColon},
		{",", TokenComma},
		{"~", TokenCollectionStart},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokens, err := tokenizer.Tokenize()

			if err != nil {
				t.Fatalf("Tokenize() error = %v", err)
			}

			if len(tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(tokens))
			}

			token := tokens[0]
			if token.Type != tt.expected {
				t.Errorf("Expected token type %v, got %v", tt.expected, token.Type)
			}
		})
	}
}

func TestTokenizer_SimpleObject(t *testing.T) {
	input := `{name: "John", age: 30}`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	// Expected: { name : "John" , age : 30 }
	// That's 11 tokens
	expectedTypes := []TokenType{
		TokenCurlyOpen,
		TokenString, // name
		TokenColon,
		TokenString, // "John"
		TokenComma,
		TokenString, // age
		TokenColon,
		TokenNumber, // 30
		TokenCurlyClose,
	}

	if len(tokens) != len(expectedTypes) {
		t.Fatalf("Expected %d tokens, got %d", len(expectedTypes), len(tokens))
	}

	for i, expected := range expectedTypes {
		if tokens[i].Type != expected {
			t.Errorf("Token %d: expected type %v, got %v", i, expected, tokens[i].Type)
		}
	}
}

func TestTokenizer_Array(t *testing.T) {
	input := `[1, 2, 3]`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	// Expected: [ 1 , 2 , 3 ]
	// That's 7 tokens
	expectedTypes := []TokenType{
		TokenBracketOpen,
		TokenNumber,
		TokenComma,
		TokenNumber,
		TokenComma,
		TokenNumber,
		TokenBracketClose,
	}

	if len(tokens) != len(expectedTypes) {
		t.Fatalf("Expected %d tokens, got %d", len(expectedTypes), len(tokens))
	}

	for i, expected := range expectedTypes {
		if tokens[i].Type != expected {
			t.Errorf("Token %d: expected type %v, got %v", i, expected, tokens[i].Type)
		}
	}
}

func TestTokenizer_Comments(t *testing.T) {
	input := `# This is a comment
"hello"`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	// Comments should be skipped, only string token should remain
	if len(tokens) != 1 {
		t.Fatalf("Expected 1 token (comment should be skipped), got %d", len(tokens))
	}

	if tokens[0].Type != TokenString {
		t.Errorf("Expected STRING token, got %v", tokens[0].Type)
	}
}

func TestTokenizer_SectionSeparator(t *testing.T) {
	input := `---`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) != 1 {
		t.Fatalf("Expected 1 token, got %d", len(tokens))
	}

	if tokens[0].Type != TokenSectionSep {
		t.Errorf("Expected SECTION_SEP token, got %v", tokens[0].Type)
	}
}

func TestTokenizer_SectionWithName(t *testing.T) {
	input := `--- users`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("Expected 2 tokens (--- and name), got %d", len(tokens))
	}

	if tokens[0].Type != TokenSectionSep {
		t.Errorf("Expected SECTION_SEP token, got %v", tokens[0].Type)
	}

	if tokens[1].SubType != SubSectionName {
		t.Errorf("Expected SECTION_NAME subtype, got %v", tokens[1].SubType)
	}

	if tokens[1].Value != "users" {
		t.Errorf("Expected section name 'users', got '%v'", tokens[1].Value)
	}
}

func TestTokenizer_EscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"newline", `"hello\nworld"`, "hello\nworld"},
		{"tab", `"hello\tworld"`, "hello\tworld"},
		{"quote", `"say \"hello\""`, `say "hello"`},
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

func TestTokenizer_OpenString(t *testing.T) {
	input := `hello world`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}

	if len(tokens) != 1 {
		t.Fatalf("Expected 1 token, got %d", len(tokens))
	}

	token := tokens[0]
	if token.Type != TokenString {
		t.Errorf("Expected STRING token, got %v", token.Type)
	}

	if token.SubType != SubOpenString {
		t.Errorf("Expected OPEN_STRING subtype, got %v", token.SubType)
	}

	if token.Value != "hello world" {
		t.Errorf("Expected value 'hello world', got '%v'", token.Value)
	}
}
