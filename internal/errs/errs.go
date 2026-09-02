// Package errs holds the error model shared by every pipeline stage: a stable
// kebab-case code and a position. Codes are the conformance contract —
// messages are informational and never asserted. Codes follow the frozen
// <predicate>-<subject> grammar; adding one is a spec change, not an
// implementation decision.
package errs

import "fmt"

// Parse-stage codes. Tokenizer codes live in the tokenizer's compact enum and
// surface here through their string form.
const (
	UnexpectedToken        = "unexpected-token"
	ExpectedClosingBracket = "expected-closing-bracket"
	ExpectedValue          = "expected-value"
	InvalidKey             = "invalid-key"
	DuplicateMember        = "duplicate-member"
	InvalidDefinition      = "invalid-definition"
	UndefinedVariable      = "undefined-variable"
	UndefinedSchema        = "undefined-schema"
	DuplicateSectionName   = "duplicate-section-name"
)

// Schema-compilation codes.
const (
	InvalidSchema  = "invalid-schema"
	EmptyMemberdef = "empty-memberdef"
	UnknownType    = "unknown-type"
	ReservedType   = "reserved-type"
	UnknownMember  = "unknown-member"
	ForbiddenNull  = "forbidden-null"
)

// Validation codes. expected-* is a TYPE problem; missing-value a PRESENCE
// problem; mismatched-* a DECLARED constraint; out-of-range-* the type's own
// intrinsic range.
const (
	ExpectedString   = "expected-string"
	ExpectedNumber   = "expected-number"
	ExpectedInteger  = "expected-integer"
	ExpectedDecimal  = "expected-decimal"
	ExpectedBigInt   = "expected-bigint"
	ExpectedBoolean  = "expected-boolean"
	ExpectedArray    = "expected-array"
	ExpectedDateTime = "expected-datetime"
	ExpectedDate     = "expected-date"
	ExpectedTime     = "expected-time"
	InvalidObject    = "invalid-object"

	MissingValue = "missing-value"

	MismatchedMin        = "mismatched-min"
	MismatchedMax        = "mismatched-max"
	MismatchedMultipleOf = "mismatched-multiple-of"
	MismatchedLen        = "mismatched-len"
	MismatchedMinLen     = "mismatched-min-len"
	MismatchedMaxLen     = "mismatched-max-len"
	MismatchedPattern    = "mismatched-pattern"
	MismatchedChoice     = "mismatched-choice"
	MismatchedAnyOf      = "mismatched-any-of"
	MismatchedScale      = "mismatched-scale"
	MismatchedPrecision  = "mismatched-precision"

	OutOfRangeInteger = "out-of-range-integer"

	InvalidEmail = "invalid-email"
	InvalidURL   = "invalid-url"

	UnexpectedPositionalMember = "unexpected-positional-member"
)

// Error is one accumulated fault: a designated code and a 1-based position.
type Error struct {
	Code string
	// Category is derived from where the fault arose, never from the code's
	// spelling (io-specs/streaming/error-model.md makes that a MUST).
	Category string
	// Path locates the fault structurally: "$" is the document root, "$[2]"
	// the third record of a collection, ".age" a member, "[0]" an element.
	Path string
	// RecordIndex is the 0-based position within a collection, -1 outside one.
	RecordIndex int
	Line        int32
	Col         int32
}

// Error categories (io-specs/streaming/error-model.md).
const (
	CategorySyntax     = "syntax"
	CategoryValidation = "validation"
	CategoryStream     = "stream"
	CategoryGeneral    = "general"
)

// syntaxCodes are the faults raised while reading text — by the tokenizer or
// the parser. Everything else that is not a stream fault is a validation
// fault. This is THE category decision, made once and shared by the document
// and streaming paths (previously the streaming reader owned a private copy
// and then dropped the result at the public boundary).
var syntaxCodes = map[string]bool{
	UnexpectedToken: true, ExpectedClosingBracket: true, ExpectedValue: true,
	InvalidKey: true, InvalidDefinition: true, DuplicateSectionName: true,
	UnexpectedPositionalMember: true, InvalidSchema: true, EmptyMemberdef: true,
	DuplicateMember: true,
	// tokenizer codes, which are a separate enum surfaced as strings
	"unterminated-string": true, "invalid-escape-sequence": true,
	"unknown-annotation": true, "invalid-binary": true, "invalid-number": true,
	"invalid-bigint": true, "invalid-decimal": true, "invalid-date": true,
	"invalid-time": true, "invalid-datetime": true,
	"invalid-section-name": true, "missing-schema": true,
}

var streamCodes = map[string]bool{
	"stream-source-error": true, "stream-aborted": true, "stream-buffer-exceeded": true,
}

// CategoryOf classifies a designated code.
func CategoryOf(code string) string {
	switch {
	case syntaxCodes[code]:
		return CategorySyntax
	case streamCodes[code]:
		return CategoryStream
	}
	return CategoryValidation
}

// At returns a copy of e positioned at line/col — used where a fault is
// raised without position context and the caller knows it.
func (e Error) At(line, col int32) Error {
	e.Line, e.Col = line, col
	return e
}

func (e Error) Error() string {
	return fmt.Sprintf("%s at %d:%d", e.Code, e.Line, e.Col)
}
