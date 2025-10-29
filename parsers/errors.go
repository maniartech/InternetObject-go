package parsers

import (
	"fmt"
)

// ErrorCode represents a specific type of parsing error.
// These codes match the TypeScript implementation for consistency.
type ErrorCode string

// Error codes matching the TypeScript implementation
const (
	ErrorUnexpectedToken       ErrorCode = "unexpected-token"
	ErrorExpectingBracket      ErrorCode = "expecting-bracket"
	ErrorValueRequired         ErrorCode = "value-required"
	ErrorInvalidKey            ErrorCode = "invalid-key"
	ErrorStringNotClosed       ErrorCode = "string-not-closed"
	ErrorInvalidEscapeSeq      ErrorCode = "invalid-escape-sequence"
	ErrorInvalidDateTime       ErrorCode = "invalid-datetime"
	ErrorUnsupportedAnnotation ErrorCode = "unsupported-annotation"
	ErrorInvalidDefinition     ErrorCode = "invalid-definition"
	ErrorSchemaMissing         ErrorCode = "schema-missing"
	ErrorDuplicateSection      ErrorCode = "duplicate-section"
)

// IOError represents a base error in Internet Object parsing.
// It includes position information and error codes for precise error reporting.
type IOError struct {
	Code     ErrorCode     // Error code identifying the type of error
	Message  string        // Human-readable error message
	Position PositionRange // Position where the error occurred
	IsEOF    bool          // True if error occurred at end of file
}

// Error implements the error interface.
func (e *IOError) Error() string {
	if e.IsEOF {
		return fmt.Sprintf("%s: %s at EOF", e.Code, e.Message)
	}
	if e.Position.IsValid() {
		return fmt.Sprintf("%s: %s at %s", e.Code, e.Message, e.Position.Start)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewIOError creates a new IOError with the specified code and message.
func NewIOError(code ErrorCode, message string) *IOError {
	return &IOError{
		Code:    code,
		Message: message,
		IsEOF:   false,
	}
}

// NewIOErrorWithPos creates a new IOError with position information.
func NewIOErrorWithPos(code ErrorCode, message string, pos PositionRange) *IOError {
	return &IOError{
		Code:     code,
		Message:  message,
		Position: pos,
		IsEOF:    false,
	}
}

// NewIOErrorEOF creates a new IOError that occurred at end of file.
func NewIOErrorEOF(code ErrorCode, message string) *IOError {
	return &IOError{
		Code:    code,
		Message: message,
		IsEOF:   true,
	}
}

// SyntaxError represents a syntax error during parsing.
type SyntaxError struct {
	*IOError
}

// NewSyntaxError creates a new syntax error.
func NewSyntaxError(code ErrorCode, message string, pos PositionRange) *SyntaxError {
	return &SyntaxError{
		IOError: NewIOErrorWithPos(code, message, pos),
	}
}

// NewSyntaxErrorEOF creates a new syntax error at EOF.
func NewSyntaxErrorEOF(code ErrorCode, message string) *SyntaxError {
	return &SyntaxError{
		IOError: NewIOErrorEOF(code, message),
	}
}

// ValidationError represents a semantic validation error.
type ValidationError struct {
	*IOError
}

// NewValidationError creates a new validation error.
func NewValidationError(code ErrorCode, message string, pos PositionRange) *ValidationError {
	return &ValidationError{
		IOError: NewIOErrorWithPos(code, message, pos),
	}
}
