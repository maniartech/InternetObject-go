package schema

// anyTypedef is the memberdef schema a typedef of this type must satisfy.
//
// any, which alone may carry anyOf.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var anyTypedef = []typedefMember{
	{"type", "string"}, {"default", ""}, {"choices", ""}, {"anyOf", ""},
	{"isSchema", "bool"}, {"optional", "bool"}, {"null", "bool"},
}

// tryAlternative validates val against one anyOf alternative, reporting
// success instead of failing.
func tryAlternative(val any, md *MemberDef, defs Defs) (v any, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isFail := r.(valFail); !isFail {
				panic(r)
			}
			v, ok = nil, false
		}
	}()
	return validateMember(val, true, md, defs), true
}
