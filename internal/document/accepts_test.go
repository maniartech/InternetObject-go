package document

import (
	"testing"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// MemberDef.Accepts must agree with validating a record holding the value: it
// is the fast paths' whole account of a member's constraints.
func TestAcceptsAgreesWithRecordValidation(t *testing.T) {
	cases := []struct {
		def    string
		values []any
	}{
		{"{string, minLen: 2, maxLen: 4}", []any{"a", "ab", "abcd", "abcde", 1.0, nil, "@x", "@"}},
		{"{string, pattern: '^[A-Z]', choices: [Ann, ann]}", []any{"Ann", "ann", "Bob"}},
		{"{string, len: 2}", []any{"ab", "日本", "a"}},
		{"{int, min: 0, max: 10, multipleOf: 5}", []any{0.0, 5.0, 7.0, -5.0, 15.0, 2.5, "5"}},
		{"{number, choices: [1.5, 2]}", []any{1.5, 2.0, 3.0}},
		{"{any, choices: [T, 1]}", []any{true, false, 1.0, "T"}},
		{"{int, min: @n}", []any{1.0}},
		{"string", []any{"x", nil}},
		{`{string, "null": T}`, []any{nil, "x"}},
	}
	for _, c := range cases {
		s, cerr := ParseSchema("v: " + c.def)
		if cerr != nil {
			t.Fatalf("%s: %v", c.def, cerr)
		}
		md := s.Defs["v"]
		for _, v := range c.values {
			rec := &core.Object{Members: []core.Member{{Key: "v", Value: v}}}
			want := len(schema.CheckRecord(rec, s, schema.NoDefs{}, true)) == 0
			if got := md.Accepts(v); got != want {
				t.Errorf("%s: Accepts(%#v) = %v, record validation says %v", c.def, v, got, want)
			}
		}
		if !md.Standalone() {
			t.Errorf("%s: not standalone", c.def)
		}
	}
}
