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
	Line int32
	Col  int32
}

func (e Error) Error() string {
	return fmt.Sprintf("%s at %d:%d", e.Code, e.Line, e.Col)
}
