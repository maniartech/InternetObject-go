package parsers

import (
	"testing"
)

func TestToken_Clone(t *testing.T) {
	original := NewToken(TokenString, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
	cloned := original.Clone()

	if cloned.Type != original.Type {
		t.Error("Cloned token type doesn't match")
	}

	if cloned.Value != original.Value {
		t.Error("Cloned token value doesn't match")
	}

	// Verify they are separate objects
	cloned.Value = "modified"
	if original.Value == "modified" {
		t.Error("Clone should be independent from original")
	}
}

func TestToken_IsError(t *testing.T) {
	pos := NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2))
	errorToken := NewErrorToken(NewSyntaxError(ErrorUnexpectedToken, "test", pos), pos)
	if !errorToken.IsError() {
		t.Error("IsError() should return true for error token")
	}

	normalToken := NewToken(TokenString, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
	if normalToken.IsError() {
		t.Error("IsError() should return false for normal token")
	}
}

func TestToken_IsStructural(t *testing.T) {
	structuralTokens := []TokenType{
		TokenCurlyOpen,
		TokenCurlyClose,
		TokenBracketOpen,
		TokenBracketClose,
		TokenColon,
		TokenComma,
		TokenCollectionStart,
		TokenSectionSep,
	}

	for _, tokenType := range structuralTokens {
		token := NewToken(tokenType, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2)))
		if !token.IsStructural() {
			t.Errorf("IsStructural() should return true for %v", tokenType)
		}
	}

	valueToken := NewToken(TokenString, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
	if valueToken.IsStructural() {
		t.Error("IsStructural() should return false for value tokens")
	}
}

func TestToken_IsValue(t *testing.T) {
	valueTokens := []TokenType{
		TokenString,
		TokenNumber,
		TokenBoolean,
		TokenNull,
		TokenBinary,
		TokenDateTime,
	}

	for _, tokenType := range valueTokens {
		token := NewToken(tokenType, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
		if !token.IsValue() {
			t.Errorf("IsValue() should return true for %v", tokenType)
		}
	}

	structuralToken := NewToken(TokenCurlyOpen, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2)))
	if structuralToken.IsValue() {
		t.Error("IsValue() should return false for structural tokens")
	}
}

func TestToken_GetPositions(t *testing.T) {
	start := NewPosition(0, 1, 1)
	end := NewPosition(4, 1, 5)
	token := NewToken(TokenString, "test", NewPositionRange(start, end))

	if token.GetStartPos() != start {
		t.Errorf("GetStartPos() = %v, want %v", token.GetStartPos(), start)
	}

	if token.GetEndPos() != end {
		t.Errorf("GetEndPos() = %v, want %v", token.GetEndPos(), end)
	}
}
