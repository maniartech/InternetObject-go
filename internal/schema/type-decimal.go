package schema

import (
	"math/big"

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
	if pr, ok := md.Constraints["precision"].(float64); ok && float64(decimalDigits(d)) > pr {
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
	if m, ok := bound("min"); ok && cmpDecimal(d, m) < 0 {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && cmpDecimal(d, m) > 0 {
		vfail(errs.MismatchedMax)
	}
	if m, ok := bound("multipleOf"); ok && !decimalMultiple(d, m) {
		vfail(errs.MismatchedMultipleOf)
	}
	return val // the original box; see validateString
}

// decimalDigits counts a decimal's significant digits (its precision).
func decimalDigits(d core.Decimal) int {
	s := new(big.Int).Abs(d.Coef).String()
	if s == "0" {
		return 1
	}
	return len(s)
}

// cmpDecimal compares two decimals numerically, aligning scales.
func cmpDecimal(a, b core.Decimal) int {
	av, bv := a.Coef, b.Coef
	if a.Scale < b.Scale {
		av = new(big.Int).Mul(av, pow10(b.Scale-a.Scale))
	} else if b.Scale < a.Scale {
		bv = new(big.Int).Mul(bv, pow10(a.Scale-b.Scale))
	}
	return av.Cmp(bv)
}

func decimalMultiple(d, m core.Decimal) bool {
	dv, mv := d.Coef, m.Coef
	if d.Scale < m.Scale {
		dv = new(big.Int).Mul(dv, pow10(m.Scale-d.Scale))
	} else if m.Scale < d.Scale {
		mv = new(big.Int).Mul(mv, pow10(d.Scale-m.Scale))
	}
	if mv.Sign() == 0 {
		return false
	}
	return new(big.Int).Mod(dv, mv).Sign() == 0
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
