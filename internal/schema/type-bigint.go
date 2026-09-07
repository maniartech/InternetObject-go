package schema

import (
	"math/big"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// bigintTypedef is the memberdef schema a typedef of this type must satisfy.
//
// bigint. Same shape as number, with bigint-typed bounds.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var bigintTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"choices", "[self]"},
	{"min", "self"}, {"max", "self"}, {"multipleOf", "self"},
	{"format", "string"}, {"optional", "bool"}, {"null", "bool"},
}

func validateBigInt(val any, md *MemberDef, defs Defs) any {
	b, ok := val.(*big.Int)
	if !ok {
		vfail(errs.ExpectedBigInt)
	}
	bound := func(key string) (*big.Int, bool) {
		v, ok := md.Constraints[key]
		if !ok {
			return nil, false
		}
		bb, ok := resolveRef(v, defs).(*big.Int)
		if !ok {
			vfail(errs.ExpectedBigInt)
		}
		return bb, true
	}
	if m, ok := bound("min"); ok && b.Cmp(m) < 0 {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && b.Cmp(m) > 0 {
		vfail(errs.MismatchedMax)
	}
	if m, ok := bound("multipleOf"); ok {
		if m.Sign() == 0 || new(big.Int).Mod(b, m).Sign() != 0 {
			vfail(errs.MismatchedMultipleOf)
		}
	}
	return val // the original box; see validateString
}
