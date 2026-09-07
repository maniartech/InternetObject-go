package schema

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// stringTypedef is the memberdef schema a typedef of this type must satisfy.
//
// `{string, minLen: 2}` and friends. STRING_TYPES in io-js2: string,
// email, url.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var stringTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"choices", "[self]"},
	{"pattern", "string"}, {"flags", "string"},
	{"len", "number"}, {"minLen", "number"}, {"maxLen", "number"},
	{"format", "string"}, {"escapeLines", "bool"}, {"encloser", "string"},
	{"optional", "bool"}, {"null", "bool"},
}

func validateString(val any, md *MemberDef) any {
	s, ok := val.(string)
	if !ok {
		vfail(errs.ExpectedString)
	}
	switch md.Type {
	case "string":
		// Compiled by compilePattern at schema-compile time — read only, never
		// written here: this member def is shared across goroutines.
		if md.reBad {
			vfail(errs.MismatchedPattern) // the pattern itself would not compile
		}
		if md.re != nil && !md.re.MatchString(s) {
			vfail(errs.MismatchedPattern)
		}
	case "email":
		if !emailRe.MatchString(s) {
			vfail(errs.InvalidEmail)
		}
	case "url":
		if !urlRe.MatchString(s) {
			vfail(errs.InvalidURL)
		}
	}
	n := -1
	length := func() int {
		if n < 0 {
			n = 0
			for range s {
				n++ // length is measured in code points, never bytes or UTF-16 units
			}
		}
		return n
	}
	if l, ok := md.Constraints["len"].(float64); ok && float64(length()) != l {
		vfail(errs.MismatchedLen)
	}
	if l, ok := md.Constraints["maxLen"].(float64); ok && float64(length()) > l {
		vfail(errs.MismatchedMaxLen)
	}
	if l, ok := md.Constraints["minLen"].(float64); ok && float64(length()) < l {
		vfail(errs.MismatchedMinLen)
	}
	// Return the ORIGINAL interface value rather than re-boxing: the caller
	// already holds this value boxed, and `return s` allocates a fresh
	// interface for a value validation did not change (ADR 0006 P7).
	return val
}
