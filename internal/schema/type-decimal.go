package schema

import (
	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// decimalTypedef is the memberdef schema a typedef of this type must satisfy.
//
// decimal, which adds precision and scale.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var decimalTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"choices", "[self]"},
	{"precision", "number"}, {"scale", "number"},
	{"min", "self"}, {"max", "self"}, {"multipleOf", "self"},
	{"optional", "bool"}, {"null", "bool"},
}

func validateDecimal(val any, md *MemberDef, defs Defs) any {
	d, ok := val.(core.Decimal)
	if !ok {
		vfail(errs.ExpectedDecimal)
	}
	if sc, ok := md.Constraints["scale"].(float64); ok && float64(d.Scale) != sc {
		vfail(errs.MismatchedScale)
	}
	if pr, ok := md.Constraints["precision"].(float64); ok && float64(d.Precision()) > pr {
		vfail(errs.MismatchedPrecision)
	}
	bound := func(key string) (core.Decimal, bool) {
		v, ok := md.Constraints[key]
		if !ok {
			return core.Decimal{}, false
		}
		dd, ok := resolveRef(v, defs).(core.Decimal)
		if !ok {
			vfail(errs.ExpectedDecimal)
		}
		return dd, true
	}
	if m, ok := bound("min"); ok && d.Cmp(m) < 0 {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && d.Cmp(m) > 0 {
		vfail(errs.MismatchedMax)
	}
	if m, ok := bound("multipleOf"); ok && !d.IsMultipleOf(m) {
		vfail(errs.MismatchedMultipleOf)
	}
	return val // the original box; see validateString
}
