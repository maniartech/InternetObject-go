package conformance

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// The schemaDef comparator: compile a schema definition string and compare
// the compiled shape against the corpus's neutral encoding — an ordered
// `members` list plus `open` — by SUBSET: every key the case lists must be
// present and equal; implementation bookkeeping beyond that is tolerated.
// List LENGTH is asserted (a schema with an extra member is a different
// schema), and so is member order.

// RunSchemaDefCase runs one schemaDef-kind case.
func RunSchemaDefCase(row SuiteRow) []string {
	compiled, cerr := document.ParseSchema(row.SchemaDef)
	var codes []string
	if cerr != nil {
		codes = []string{string(cerr.Code)}
	}
	if !stringsEqual(codes, row.ErrorCodes) {
		return []string{fmt.Sprintf("codes  expected=%v  actual=%v", row.ErrorCodes, codes)}
	}
	if len(row.ErrorCodes) > 0 {
		return nil // the code was the assertion
	}
	var problems []string
	subsetMismatches(row.Expected, neutralSchema(compiled), "", &problems)
	if len(problems) > 0 {
		problems = append([]string{fmt.Sprintf("schemaDef=%q", row.SchemaDef)}, problems...)
	}
	return problems
}

// neutralSchema projects a compiled schema onto the corpus's neutral shape.
func neutralSchema(s *schema.Schema) *core.Object {
	var open any
	switch o := s.Open.(type) {
	case nil:
		open = false
	case *schema.MemberDef:
		open = neutralMemberDef(o)
	default: // OpenAny
		open = true
	}
	members := make([]any, len(s.Names))
	for i, n := range s.Names {
		members[i] = neutralMemberDef(s.Defs[n])
	}
	return &core.Object{Members: []core.Member{
		{Key: "open", Value: open},
		{Key: "members", Value: members},
	}}
}

func neutralMemberDef(md *schema.MemberDef) *core.Object {
	o := &core.Object{}
	put := func(k string, v any) {
		o.Members = append(o.Members, core.Member{Key: k, Value: v})
	}
	put("name", md.Name)
	put("type", md.Type)
	put("path", md.Path)
	put("optional", md.Optional)
	put("null", md.Null)
	if md.HasDefault {
		put("default", md.Default)
	}
	if md.Choices != nil {
		put("choices", md.Choices)
	}
	if md.Of != nil {
		put("of", neutralMemberDef(md.Of))
	}
	if md.Schema != nil {
		put("schema", neutralSchema(md.Schema))
	}
	for k, v := range md.Constraints {
		put(k, v)
	}
	return o
}

// subsetMismatches collects every place actual fails to CONTAIN expected.
func subsetMismatches(expected, actual any, at string, out *[]string) {
	switch e := expected.(type) {
	case *core.Object:
		a, ok := actual.(*core.Object)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: expected an object, actual=%s", at, Show(actual)))
			return
		}
		for _, m := range e.Members {
			path := m.Key
			if at != "" {
				path = at + "." + m.Key
			}
			i := a.Find(m.Key)
			if i < 0 {
				*out = append(*out, fmt.Sprintf("%s: expected=%s actual=<absent>", path, Show(m.Value)))
				continue
			}
			subsetMismatches(m.Value, a.Members[i].Value, path, out)
		}
	case []any:
		a, ok := actual.([]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: expected a list of %d, actual=%s", at, len(e), Show(actual)))
			return
		}
		if len(e) != len(a) {
			*out = append(*out, fmt.Sprintf("%s: expected %d entries, actual %d", at, len(e), len(a)))
		}
		for i := range e {
			if i < len(a) {
				subsetMismatches(e[i], a[i], fmt.Sprintf("%s[%d]", at, i), out)
			}
		}
	default:
		if !core.Equal(expected, actual) {
			*out = append(*out, fmt.Sprintf("%s: expected=%s actual=%s", at, Show(expected), Show(actual)))
		}
	}
}
