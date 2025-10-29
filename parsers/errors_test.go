package parsers

import (
	"strings"
	"testing"
)

func TestIOError_Error(t *testing.T) {
	pos := NewPositionRange(NewPosition(10, 2, 5), NewPosition(15, 2, 10))
	err := NewIOErrorWithPos(ErrorUnexpectedToken, "unexpected character", pos)

	errMsg := err.Error()
	if !strings.Contains(errMsg, "unexpected character") {
		t.Errorf("Error() should contain message, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "2:5") {
		t.Errorf("Error() should contain position, got: %s", errMsg)
	}
}

func TestNewIOError(t *testing.T) {
	err := NewIOError(ErrorStringNotClosed, "string not closed")

	if err.Code != ErrorStringNotClosed {
		t.Errorf("Code = %v, want %v", err.Code, ErrorStringNotClosed)
	}
	if err.Message != "string not closed" {
		t.Errorf("Message = %s, want 'string not closed'", err.Message)
	}
}

func TestNewIOErrorEOF(t *testing.T) {
	err := NewIOErrorEOF(ErrorStringNotClosed, "unexpected end of file")

	if !err.IsEOF {
		t.Error("IsEOF should be true")
	}
	if err.Message != "unexpected end of file" {
		t.Errorf("Message = %s, want 'unexpected end of file'", err.Message)
	}
}

func TestNewSyntaxError(t *testing.T) {
	pos := NewPositionRange(NewPosition(5, 1, 6), NewPosition(10, 1, 11))
	err := NewSyntaxError(ErrorInvalidEscapeSeq, "invalid escape sequence", pos)

	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid escape sequence") {
		t.Errorf("Error message should contain description: %s", errMsg)
	}
}

func TestNewSyntaxErrorEOF(t *testing.T) {
	err := NewSyntaxErrorEOF(ErrorStringNotClosed, "unexpected EOF")

	if !err.IsEOF {
		t.Error("IsEOF should be true for EOF error")
	}
}

func TestNewValidationError(t *testing.T) {
	pos := NewPositionRange(NewPosition(20, 3, 1), NewPosition(25, 3, 6))
	err := NewValidationError(ErrorInvalidKey, "invalid value", pos)

	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid value") {
		t.Errorf("Error message should contain description: %s", errMsg)
	}
}
