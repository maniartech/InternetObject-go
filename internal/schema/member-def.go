package schema

import (
	"regexp"
)

// MemberDef is one member's compiled definition.
type MemberDef struct {
	Name     string
	Type     string
	Path     string
	Optional bool
	Null     bool

	HasDefault bool
	Default    any
	Choices    []any // nil when not declared

	Of        *MemberDef   // array element definition
	Schema    *Schema      // nested schema, for object bodies
	SchemaRef string       // a `$name` reference, resolved lazily at validation
	AnyOf     []*MemberDef // union alternatives, for `{any, anyOf: [...]}`

	// Constraints holds the carried per-type constraint keys (min, max,
	// multipleOf, len, minLen, maxLen, pattern, precision, scale, …), and
	// Keys their declaration order (default/choices/anyOf included), which the
	// writer reproduces.
	Constraints map[string]any
	Keys        []string

	// The `pattern` constraint, compiled once by compilePattern at compile
	// time. Written only there; validation reads. reBad records a pattern that
	// would not compile, whose fault surfaces per value at validation.
	re    *regexp.Regexp
	reBad bool
}
