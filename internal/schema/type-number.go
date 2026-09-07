package schema

import (
	"math"
	"regexp"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// numberTypedef is the memberdef schema a typedef of this type must satisfy.
//
// the whole number family: number, int, uint and the sized ints.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var numberTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"choices", "[self]"},
	{"min", "self"}, {"max", "self"}, {"multipleOf", "self"},
	{"format", "string"}, {"optional", "bool"}, {"null", "bool"},
}

// The email and url expressions, ported from the reference implementation.
// Both are deliberately UNANCHORED — a substring match accepts (which is why
// `a@b@c.com` passes), exactly as the reference behaves.
var (
	emailRe = regexp.MustCompile(`(?:[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*|"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*")@(?:(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?|\[(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?|[a-z0-9-]*[a-z0-9]:(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21-\x5a\x53-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])+)\])`)
	urlRe   = regexp.MustCompile(`(([A-Za-z]{3,9}:(?://)?)(?:[\-;:&=\+\$,\w]+@)?[A-Za-z0-9\.\-]+|(?:www\.|[\-;:&=\+\$,\w]+@)[A-Za-z0-9\.\-]+)((?:/[\+~%/\.\w\-_]*)?\??(?:[\-\+=&;%@\.\w_]*)#?(?:[\.\!/\\\w]*))?`)
)

// integerTypes are the schema types whose values must be whole numbers.
var integerTypes = map[string]bool{
	"int": true, "uint": true,
	"int8": true, "int16": true, "int32": true,
	"uint8": true, "uint16": true, "uint32": true,
}

// intrinsic bounds of the sized types (nil = unbounded on that side)
func typeBounds(t string) (min, max float64, bounded bool) {
	switch t {
	case "int8":
		return -128, 127, true
	case "int16":
		return -32768, 32767, true
	case "int32":
		return -2147483648, 2147483647, true
	case "uint8":
		return 0, 255, true
	case "uint16":
		return 0, 65535, true
	case "uint32":
		return 0, 4294967295, true
	case "uint":
		return 0, math.Inf(1), true
	}
	return 0, 0, false
}

func validateNumber(val any, md *MemberDef, defs Defs) any {
	expected := errs.ExpectedNumber
	if integerTypes[md.Type] {
		expected = errs.ExpectedInteger
	}
	f, ok := val.(float64)
	if !ok {
		vfail(expected)
	}
	if integerTypes[md.Type] && f != math.Trunc(f) {
		vfail(errs.ExpectedInteger)
	}
	// declared bounds first (the author's constraint), the type's own after
	if m, ok := numBound(md, "min", defs); ok && f < m {
		vfail(errs.MismatchedMin)
	}
	if m, ok := numBound(md, "max", defs); ok && f > m {
		vfail(errs.MismatchedMax)
	}
	if lo, hi, bounded := typeBounds(md.Type); bounded && (f < lo || f > hi) {
		vfail(errs.OutOfRangeInteger)
	}
	if m, ok := numBound(md, "multipleOf", defs); ok && math.Mod(f, m) != 0 {
		vfail(errs.MismatchedMultipleOf)
	}
	return val // the original box; see validateString
}

func numBound(md *MemberDef, key string, defs Defs) (float64, bool) {
	v, ok := md.Constraints[key]
	if !ok {
		return 0, false
	}
	f, ok := resolveRef(v, defs).(float64)
	if !ok {
		vfail(errs.ExpectedNumber)
	}
	return f, true
}
