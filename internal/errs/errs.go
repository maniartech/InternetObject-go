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

// Error is one accumulated fault: a designated code and a 1-based position.
type Error struct {
	Code string
	Line int32
	Col  int32
}

func (e Error) Error() string {
	return fmt.Sprintf("%s at %d:%d", e.Code, e.Line, e.Col)
}
