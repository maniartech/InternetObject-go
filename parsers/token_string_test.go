package parsers

import (
	"testing"
)

func TestTokenType_String(t *testing.T) {
	tests := []struct {
		tokenType TokenType
		expected  string
	}{
		// Structural tokens
		{TokenCurlyOpen, "{"},
		{TokenCurlyClose, "}"},
		{TokenBracketOpen, "["},
		{TokenBracketClose, "]"},
		{TokenColon, ":"},
		{TokenComma, ","},
		{TokenCollectionStart, "~"},
		{TokenSectionSep, "---"},

		// Value tokens
		{TokenString, "STRING"},
		{TokenNumber, "NUMBER"},
		{TokenBigInt, "BIGINT"},
		{TokenDecimal, "DECIMAL"},
		{TokenBoolean, "BOOLEAN"},
		{TokenNull, "NULL"},
		{TokenUndefined, "UNDEFINED"},
		{TokenBinary, "BINARY"},
		{TokenDateTime, "DATETIME"},

		// Special tokens
		{TokenWhitespace, "WHITESPACE"},
		{TokenUnknown, "UNKNOWN"},
		{TokenError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.tokenType.String()
			if result != tt.expected {
				t.Errorf("TokenType(%d).String() = %q, want %q", tt.tokenType, result, tt.expected)
			}
		})
	}
}

func TestParser_TokenString(t *testing.T) {
	tests := []struct {
		name     string
		token    *Token
		expected string
	}{
		{
			name:     "nil token",
			token:    nil,
			expected: "end of input",
		},
		{
			name:     "structural token - curly open",
			token:    NewToken(TokenCurlyOpen, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2))),
			expected: "{",
		},
		{
			name:     "structural token - section separator",
			token:    NewToken(TokenSectionSep, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(3, 1, 4))),
			expected: "---",
		},
		{
			name:     "value token - string",
			token:    NewToken(TokenString, "hello", NewPositionRange(NewPosition(0, 1, 1), NewPosition(5, 1, 6))),
			expected: "hello",
		},
		{
			name:     "value token - number",
			token:    NewToken(TokenNumber, 42, NewPositionRange(NewPosition(0, 1, 1), NewPosition(2, 1, 3))),
			expected: "42",
		},
		{
			name:     "value token - boolean",
			token:    NewToken(TokenBoolean, true, NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5))),
			expected: "true",
		},
		{
			name:     "value token - null",
			token:    NewToken(TokenNull, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5))),
			expected: "NULL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &Parser{}
			result := parser.tokenString(tt.token)
			if result != tt.expected {
				t.Errorf("tokenString() = %q, want %q", result, tt.expected)
			}
		})
	}
}
