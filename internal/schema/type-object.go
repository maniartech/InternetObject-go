package schema

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// objectTypedef is the memberdef schema a typedef of this type must satisfy.
//
// object. `schema` carries a shape, so it is not type-checked here.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var objectTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"schema", ""},
	{"optional", "bool"}, {"null", "bool"},
}

func validateObjectMember(val any, md *MemberDef, defs Defs) any {
	// The reference resolves a `$ref` before the type check, so a dangling
	// reference reports undefined-schema even when the value is not an object.
	sch := md.Schema
	if sch == nil && md.SchemaRef != "" {
		s, cerr := defs.SchemaOf(strings.TrimPrefix(md.SchemaRef, "$"))
		if cerr != nil {
			panic(valFail{*cerr})
		}
		sch = s
	}
	obj, ok := val.(*core.Object)
	if !ok {
		vfail(errs.InvalidObject)
	}
	if sch == nil {
		return obj // a bare `object` member constrains nothing
	}
	v, acc, fatal := validateObject(obj, sch, defs, md.Path, true)
	if fatal != nil {
		panic(valFail{*fatal})
	}
	if len(acc) > 0 {
		panic(valFail{acc[0]})
	}
	return v
}
