package schema

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// arrayTypedef is the memberdef schema a typedef of this type must satisfy.
//
// array. `of` carries a memberdef, so it is not type-checked here.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var arrayTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"of", ""},
	{"len", "number"}, {"minLen", "number"}, {"maxLen", "number"},
	{"optional", "bool"}, {"null", "bool"},
}

func validateArray(val any, md *MemberDef, defs Defs) any {
	arr, ok := val.([]any)
	if !ok {
		vfail(errs.ExpectedArray)
	}
	if l, ok := md.Constraints["len"].(float64); ok && float64(len(arr)) != l {
		vfail(errs.MismatchedLen)
	}
	if l, ok := md.Constraints["maxLen"].(float64); ok && float64(len(arr)) > l {
		vfail(errs.MismatchedMaxLen)
	}
	if l, ok := md.Constraints["minLen"].(float64); ok && float64(len(arr)) < l {
		vfail(errs.MismatchedMinLen)
	}
	if md.Of == nil {
		return arr // an untyped array constrains nothing, nulls included
	}
	// Validate in place. The element validators return the value they were
	// given (they check, they do not transform), so a second slice would be a
	// copy of the first — one allocation per array, per record. Only when an
	// element genuinely changes (a default, a resolved @reference) is a value
	// written back, and it is written into the slice the parser already built,
	// which nothing else references once validation returns (ADR 0006 P3).
	for i, e := range arr {
		if v := validateMember(e, true, md.Of, defs); !sameValue(v, e) {
			arr[i] = v
		}
	}
	return arr
}

// sameValue reports whether validation handed back the identical interface
// value it was given — the common case, and cheaper than assuming it did not.
func sameValue(a, b any) bool {
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	}
	return false
}
