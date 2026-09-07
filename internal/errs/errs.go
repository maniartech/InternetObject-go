// Package errs holds the error model shared by every pipeline stage: a stable
// kebab-case code and a position. Codes are the conformance contract —
// messages are informational and never asserted. Codes follow the frozen
// <predicate>-<subject> grammar; adding one is a spec change, not an
// implementation decision.
package errs

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/internal/core"
)

// Parse-stage codes. Tokenizer codes live in the tokenizer's compact enum and
// surface here through their string form.
const (
	UnexpectedToken        Code = "unexpected-token"
	ExpectedClosingBracket Code = "expected-closing-bracket"
	ExpectedValue          Code = "expected-value"
	InvalidKey             Code = "invalid-key"
	DuplicateMember        Code = "duplicate-member"
	InvalidDefinition      Code = "invalid-definition"
	UndefinedVariable      Code = "undefined-variable"
	UndefinedSchema        Code = "undefined-schema"
	DuplicateSectionName   Code = "duplicate-section-name"
)

// Schema-compilation codes.
const (
	InvalidSchema  Code = "invalid-schema"
	EmptyMemberdef Code = "empty-memberdef"
	UnknownType    Code = "unknown-type"
	ReservedType   Code = "reserved-type"
	UnknownMember  Code = "unknown-member"
	ForbiddenNull  Code = "forbidden-null"
)

// Validation codes. expected-* is a TYPE problem; missing-value a PRESENCE
// problem; mismatched-* a DECLARED constraint; out-of-range-* the type's own
// intrinsic range.
const (
	ExpectedString   Code = "expected-string"
	ExpectedNumber   Code = "expected-number"
	ExpectedInteger  Code = "expected-integer"
	ExpectedDecimal  Code = "expected-decimal"
	ExpectedBigInt   Code = "expected-bigint"
	ExpectedBoolean  Code = "expected-boolean"
	ExpectedArray    Code = "expected-array"
	ExpectedDateTime Code = "expected-datetime"
	ExpectedDate     Code = "expected-date"
	ExpectedTime     Code = "expected-time"
	InvalidObject    Code = "invalid-object"

	MissingValue Code = "missing-value"

	MismatchedMin        Code = "mismatched-min"
	MismatchedMax        Code = "mismatched-max"
	MismatchedMultipleOf Code = "mismatched-multiple-of"
	MismatchedLen        Code = "mismatched-len"
	MismatchedMinLen     Code = "mismatched-min-len"
	MismatchedMaxLen     Code = "mismatched-max-len"
	MismatchedPattern    Code = "mismatched-pattern"
	MismatchedChoice     Code = "mismatched-choice"
	MismatchedAnyOf      Code = "mismatched-any-of"
	MismatchedScale      Code = "mismatched-scale"
	MismatchedPrecision  Code = "mismatched-precision"

	OutOfRangeInteger Code = "out-of-range-integer"

	InvalidEmail Code = "invalid-email"
	InvalidURL   Code = "invalid-url"

	UnexpectedPositionalMember Code = "unexpected-positional-member"
)

// Code is the designated error code type, defined in the value model because a
// failed record carries one (core.ErrorNode). Aliased here so this package -
// where the codes are declared - reads without a core. prefix on every line.
type Code = core.Code

// Error is one accumulated fault: a designated code and a 1-based position.
type Error struct {
	Code Code
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
var syntaxCodes = map[Code]bool{
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

var streamCodes = map[Code]bool{
	"stream-source-error": true, "stream-aborted": true, "stream-buffer-exceeded": true,
}

// CategoryOf classifies a designated code.
func CategoryOf(code Code) string {
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
