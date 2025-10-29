package parsers

import (
	"testing"
)

func TestIsAlpha(t *testing.T) {
	tests := []struct {
		char     rune
		expected bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', false},
		{'9', false},
		{'_', false},
		{' ', false},
	}

	for _, tt := range tests {
		result := isAlpha(tt.char)
		if result != tt.expected {
			t.Errorf("isAlpha(%c) = %v, want %v", tt.char, result, tt.expected)
		}
	}
}

func TestIsAlphaNumeric(t *testing.T) {
	tests := []struct {
		char     rune
		expected bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', false},
		{' ', false},
		{'-', false},
	}

	for _, tt := range tests {
		result := isAlphaNumeric(tt.char)
		if result != tt.expected {
			t.Errorf("isAlphaNumeric(%c) = %v, want %v", tt.char, result, tt.expected)
		}
	}
}

func TestIsWhitespace_AllCases(t *testing.T) {
	// Test all whitespace characters
	whitespaces := []rune{' ', '\t', '\n', '\r', '\f', '\v'}
	for _, ws := range whitespaces {
		if !isWhitespace(ws) {
			t.Errorf("isWhitespace(%q) should return true", ws)
		}
	}

	// Test non-whitespace
	nonWhitespaces := []rune{'a', '0', '_', '-'}
	for _, nws := range nonWhitespaces {
		if isWhitespace(nws) {
			t.Errorf("isWhitespace(%q) should return false", nws)
		}
	}
}

func TestTokenizer_Peek(t *testing.T) {
	tokenizer := NewTokenizer("abc")

	// Peek should not advance position
	char := tokenizer.peek()
	if char != 'a' {
		t.Errorf("peek() = %c, want 'a'", char)
	}

	// Position should still be at start
	if tokenizer.pos != 0 {
		t.Errorf("peek() should not advance position, pos = %d", tokenizer.pos)
	}

	// Advance and peek again
	tokenizer.advance(1)
	char = tokenizer.peek()
	if char != 'b' {
		t.Errorf("peek() after advance = %c, want 'b'", char)
	}
}

func TestTokenizer_SkipToNextTokenBoundary(t *testing.T) {
	tests := []struct {
		input    string
		expected int // expected position after skip
	}{
		{"abc def", 3}, // should skip to space
		{"123,456", 3}, // should skip to comma
		{"test{}", 4},  // should skip to {
		{"value:", 5},  // should skip to :
		{"item]", 4},   // should skip to ]
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokenizer := NewTokenizer(tt.input)
			tokenizer.skipToNextTokenBoundary()

			if tokenizer.pos != tt.expected {
				t.Errorf("skipToNextTokenBoundary() position = %d, want %d", tokenizer.pos, tt.expected)
			}
		})
	}
}
