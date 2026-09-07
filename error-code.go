package internetobject

import "github.com/maniartech/InternetObject-go/internal/core"

// Code is a designated error code — stable, kebab-case, and the conformance
// contract that every Internet Object implementation shares. Messages are
// informational and are never asserted.
//
// It is a string type, so a code can be compared against a literal
// (`err.Code == "expected-integer"` still compiles) and printed with %s. Use
// the constants below: they are checked at compile time, so a typo is a build
// error rather than a comparison that is quietly always false.
//
// Codes follow the frozen <predicate>-<subject> grammar, and the four
// predicates say what kind of fault it is:
//
//	expected-…     a TYPE problem: the value is the wrong kind
//	missing-…      a PRESENCE problem: the value is not there
//	mismatched-…   a DECLARED CONSTRAINT was not met
//	out-of-range-… the type's OWN intrinsic range was exceeded
//	invalid-…      the text could not be read as the thing it announced
//
// A code does NOT determine an error's Category: io-specs requires the
// category to be derived from where the fault arose, so read [Error.Category]
// rather than inferring it from the spelling.
type Code = core.Code

// Codes raised while reading TEXT — by the tokenizer or the parser.
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

	// Malformed literals. These are raised by the tokenizer, and a typed
	// member can MASK one with its own expected-* code instead — the
	// reference's behaviour, pinned by the corpus.
	UnterminatedString    Code = "unterminated-string"
	InvalidEscapeSequence Code = "invalid-escape-sequence"
	UnknownAnnotation     Code = "unknown-annotation"
	InvalidNumber         Code = "invalid-number"
	InvalidBigInt         Code = "invalid-bigint"
	InvalidDecimal        Code = "invalid-decimal"
	InvalidBinary         Code = "invalid-binary"
	InvalidDate           Code = "invalid-date"
	InvalidTime           Code = "invalid-time"
	InvalidDateTime       Code = "invalid-datetime"
	InvalidSectionName    Code = "invalid-section-name"
	MissingSchema         Code = "missing-schema"
)

// Codes raised while COMPILING a schema.
const (
	InvalidSchema  Code = "invalid-schema"
	EmptyMemberdef Code = "empty-memberdef"
	UnknownType    Code = "unknown-type"
	ReservedType   Code = "reserved-type"
	UnknownMember  Code = "unknown-member"
	ForbiddenNull  Code = "forbidden-null"
)

// Codes raised while VALIDATING a record against its schema.
const (
	// The value is the wrong kind.
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

	// The value is not there at all.
	MissingValue Code = "missing-value"

	// A constraint the schema DECLARED was not met.
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

	// The type's own intrinsic range, which no schema declared.
	OutOfRangeInteger Code = "out-of-range-integer"

	// Formats a string type carries by name rather than by constraint.
	InvalidEmail Code = "invalid-email"
	InvalidURL   Code = "invalid-url"

	UnexpectedPositionalMember Code = "unexpected-positional-member"
)

// Codes raised by a STREAM, about the stream itself rather than a record.
const (
	StreamSourceError    Code = "stream-source-error"
	StreamAborted        Code = "stream-aborted"
	StreamBufferExceeded Code = "stream-buffer-exceeded"
)

// Error categories, as io-specs defines them. An error's Category is derived
// from WHERE the fault arose, never from its code's spelling.
const (
	CategorySyntax     = "syntax"
	CategoryValidation = "validation"
	CategoryStream     = "stream"
	CategoryGeneral    = "general"
)
