package schema

// boolTypedef is the memberdef schema a typedef of this type must satisfy.
//
// bool - the smallest typedef there is, and the one that shows the
// positional form most clearly: `{bool, F}` is type, default.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var boolTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"},
	{"optional", "bool"}, {"null", "bool"},
}
