package internetobject_test

import (
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	io "github.com/maniartech/InternetObject-go"
)

// The direct encode path must be byte-identical to the tree path. It exists
// only to avoid building a tree the writer immediately discards; the moment
// the two disagree, the format has two behaviors and the optimization is a
// bug. `IO_NO_FAST_PATH=1` forces the tree path, so the same input can be
// encoded both ways and compared.

type fastCase struct {
	Name   string     `io:"name"`
	Age    int        `io:"age"`
	Score  float64    `io:"score"`
	Active bool       `io:"active"`
	Nick   *string    `io:"nick"`
	Tags   []string   `io:"tags,omitempty"`
	When   time.Time  `io:"when"`
	Day    time.Time  `io:"day,date"`
	Blob   []byte     `io:"blob"`
	Big    *big.Int   `io:"big"`
	Dec    io.Decimal `io:"dec"`
	Opt    string     `io:"opt,optional"`
}

func fastSamples() []fastCase {
	nick := "Ally"
	weird := "a, b: {c} \"quoted\" ~ #x --- \n\t\r\\ null T 1.5 0xFF 12n 2024-01-15"
	return []fastCase{
		{}, // all zero values
		{Name: "Alice", Age: 30, Score: 1.5, Active: true, Nick: &nick,
			Tags: []string{"a", "b"}, When: time.Date(2024, 1, 15, 14, 30, 45, 123e6, time.UTC),
			Day: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Blob: []byte{1, 2, 3},
			Big: new(big.Int).Lsh(big.NewInt(1), 80), Dec: io.Decimal{Coef: big.NewInt(150), Scale: 2},
			Opt: "set"},
		{Name: weird, Age: -5, Score: math.NaN(), Tags: []string{"", " ", weird}},
		{Name: "", Score: math.Inf(-1), Tags: []string{}},
		{Name: "0", Age: 1 << 52, Score: math.SmallestNonzeroFloat64},
		{Name: "ключ 日本 😀", Score: -0.0},
	}
}

// marshalBothWays returns the fast-path and tree-path renderings.
func marshalBothWays(t *testing.T, v any) (fast, tree string) {
	t.Helper()
	fast, err := io.Marshal(v)
	if err != nil {
		t.Fatalf("fast path: %v", err)
	}
	io.WithTreeEncode(func() { tree, err = io.Marshal(v) })
	if err != nil {
		t.Fatalf("tree path: %v", err)
	}
	return fast, tree
}

func TestFastPathMatchesTreePath(t *testing.T) {
	for i, c := range fastSamples() {
		fast, tree := marshalBothWays(t, c)
		if fast != tree {
			t.Errorf("sample %d single record differs:\n fast %q\n tree %q", i, fast, tree)
		}
	}
	// …and as a collection, which exercises the record separator and the
	// trailing-hole rule across records.
	fast, tree := marshalBothWays(t, fastSamples())
	if fast != tree {
		t.Fatalf("collection differs:\n fast %q\n tree %q", fast, tree)
	}
	// The output must still be a document that re-parses to the same values.
	var back []fastCase
	if err := io.Unmarshal(fast, &back); err != nil {
		t.Fatalf("fast output does not re-parse: %v\n%s", err, fast)
	}
}

// Types the fast path must decline, falling back to the tree.
func TestFastPathFallsBack(t *testing.T) {
	type nested struct {
		Inner fastCase       `io:"inner"`
		M     map[string]int `io:"m"`
	}
	type constrained struct {
		N int `io:"n" schema:"{int, min: 0}"`
	}
	for _, v := range []any{
		nested{M: map[string]int{"a": 1}},
		constrained{N: 5},
	} {
		fast, tree := marshalBothWays(t, v)
		if fast != tree {
			t.Errorf("fallback differs for %T:\n fast %q\n tree %q", v, fast, tree)
		}
	}
}

// FuzzFastPathMatchesTreePath drives arbitrary field values through both
// paths. Run with: go test -fuzz=FuzzFastPathMatchesTreePath .
func FuzzFastPathMatchesTreePath(f *testing.F) {
	f.Add("Alice", 30, 1.5, true, "tag")
	f.Add("", 0, 0.0, false, "")
	f.Add("a, b: {c} ~ #x --- \\", -1, math.NaN(), true, "0xFF")
	f.Add("null", 1<<52, math.Inf(1), false, "12n")

	f.Fuzz(func(t *testing.T, name string, age int, score float64, active bool, tag string) {
		v := fastCase{Name: name, Age: age, Score: score, Active: active}
		if tag != "" {
			v.Tags = []string{tag}
		}
		fast, err := io.Marshal(v)
		if err != nil {
			return // both paths refuse the same inputs; TestFastPath… pins that
		}
		var tree string
		io.WithTreeEncode(func() { tree, err = io.Marshal(v) })
		if err != nil {
			t.Fatalf("tree path refused what the fast path accepted: %v", err)
		}
		if fast != tree {
			t.Fatalf("paths differ:\n fast %q\n tree %q", fast, tree)
		}
	})
}

// The fast path must refuse exactly what the tree path refuses.
func TestFastPathRefusalsMatch(t *testing.T) {
	type big64 struct {
		N int64 `io:"n"`
	}
	v := big64{N: 1 << 60}
	_, fastErr := io.Marshal(v)
	var treeErr error
	io.WithTreeEncode(func() { _, treeErr = io.Marshal(v) })
	if (fastErr == nil) != (treeErr == nil) {
		t.Fatalf("disagree on refusal: fast=%v tree=%v", fastErr, treeErr)
	}
	if fastErr != nil && !strings.Contains(fastErr.Error(), "overflows") {
		t.Fatalf("unexpected message: %v", fastErr)
	}
}

// A type whose `schema` tags must be validated, with one field per constraint
// family the fast encoder checks, plus the shapes it must still hand to the
// tree: a tag that changes the type, an array-level constraint, a default.
type constrainedFast struct {
	Name  string   `io:"name"  schema:"{string, minLen: 2, maxLen: 8, pattern: '^[A-Z]'}"`
	Code  string   `io:"code"  schema:"{string, len: 3, choices: [abc, xyz]}"`
	Age   int      `io:"age"   schema:"{int, min: 0, max: 130, multipleOf: 5}"`
	Score float64  `io:"score" schema:"{number, choices: [1.5, 2.5]}"`
	Units uint16   `io:"units" schema:"{int, max: 1000}"`
	OK    bool     `io:"ok"    schema:"bool"`
	Nick  *string  `io:"nick"  schema:"{string, minLen: 2}"`
	Tags  []string `io:"tags"  schema:"[{string, choices: [a, b]}]"`
	Refs  []*int   `io:"refs"`
}

// looseFast validates (one field is constrained) but leaves most fields
// unconstrained, so a fuzzer can pass the constraint and still reach the
// values an unconstrained field can fail: an `@`-string, a nil where null is
// not declared, a value `omitempty` leaves out.
type looseFast struct {
	Name  string   `io:"name"`
	Age   int      `io:"age"  schema:"{int, min: 0}"`
	Nick  *string  `io:"nick" schema:"{string, \"null\": false}"`
	Tags  []string `io:"tags"`
	Note  string   `io:"note,omitempty"`
	Label *string  `io:"label"`
}

// Values a validating type's unconstrained fields can still fail, which the
// fast encoder once wrote (review of SPEC 0003 §5.2): an `@`-string is a
// variable reference the record cannot resolve, and `omitempty` leaves out a
// member a tag declares required.
type refInPointer struct {
	Name *string `io:"name"`
	Age  int     `io:"age" schema:"{int, min: 0}"`
}

type refTaggedString struct {
	Name string `io:"name" schema:"string"`
	Age  int    `io:"age"  schema:"{int, min: 0}"`
}

type nilNotNull struct {
	Count *int `io:"count" schema:"{int, \"null\": false}"`
}

type omitRequired struct {
	Name string `io:"name,omitempty" schema:"{string, optional: false}"`
	Age  int    `io:"age"`
}

type omitRequiredSlice struct {
	Tags []int `io:"tags,omitempty" schema:"{array, of: int, optional: false}"`
	Age  int   `io:"age" schema:"{int, min: 0}"`
}

func TestFastPathRefusesWhatAnUnconstrainedFieldCanFail(t *testing.T) {
	ref, lone, nick := "@x", "@", "Al"
	for _, v := range []any{
		looseFast{Name: "@x", Nick: &nick},
		looseFast{Name: "Ann", Nick: &nick, Tags: []string{"a", "@x"}},
		looseFast{Name: "Ann", Nick: &nick, Label: &ref},
		looseFast{Name: "@", Nick: &nick, Label: &lone}, // a lone @ is text
		[]looseFast{{Name: "Ann", Nick: &nick}, {Name: "@x", Nick: &nick}},
		looseFast{Name: "Ann"}, // nil where the tag says null: false
		looseFast{Name: "Ann", Nick: &nick, Note: "set"},
		refInPointer{Name: &ref},
		refTaggedString{Name: "@x"},
		omitRequired{Age: 1},
		omitRequired{Name: "Ann", Age: 1},
		omitRequiredSlice{Age: 1},
		nilNotNull{},
	} {
		sameMarshalBothWays(t, v)
	}
	// Each of these must be refused, or the comparisons above prove nothing.
	for _, v := range []any{
		looseFast{Name: "@x", Nick: &nick}, looseFast{Name: "Ann"},
		refInPointer{Name: &ref}, refTaggedString{Name: "@x"}, omitRequired{Age: 1}, nilNotNull{},
	} {
		if out, err := io.Marshal(v); err == nil {
			t.Errorf("%+v was written: %q", v, out)
		}
	}
	if _, err := io.Marshal(looseFast{Name: "@", Nick: &nick, Label: &lone}); err != nil {
		t.Errorf("a lone @ is text, not a reference: %v", err)
	}
}

type typeChangingTag struct {
	N int `io:"n" schema:"string"`
}

type arrayLevelConstraint struct {
	Tags []string `io:"tags" schema:"{array, of: string, minLen: 2}"`
}

// marshalResult is one path's whole outcome: its text, or its error's text.
func marshalResult(v any) string {
	text, err := io.Marshal(v)
	if err != nil {
		return "error: " + err.Error()
	}
	return text
}

func sameMarshalBothWays(t *testing.T, v any) {
	t.Helper()
	fast := marshalResult(v)
	var tree string
	io.WithTreeEncode(func() { tree = marshalResult(v) })
	if fast != tree {
		t.Errorf("%+v\n fast %q\n tree %q", v, fast, tree)
	}
}

// constrainedFastSamples returns values the schema accepts — including the
// nil pointer and nil slice a derived schema allows — and values it refuses,
// one per constraint family.
func constrainedFastSamples() (valid, invalid []constrainedFast) {
	nick, short := "Al", "A"
	one := 1
	good := constrainedFast{Name: "Alice", Code: "abc", Age: 30, Score: 1.5, Units: 7, OK: true,
		Nick: &nick, Tags: []string{"a", "b"}, Refs: []*int{&one}}
	variant := func(mutate func(*constrainedFast)) constrainedFast {
		c := good
		mutate(&c)
		return c
	}
	valid = []constrainedFast{
		good,
		variant(func(c *constrainedFast) { c.Nick = nil }), // a pointer derives a nullable member
		variant(func(c *constrainedFast) { c.Tags = nil }),
		variant(func(c *constrainedFast) { c.Tags = []string{} }),
	}
	for _, mutate := range []func(*constrainedFast){
		func(c *constrainedFast) { c.Name = "A" },                // minLen
		func(c *constrainedFast) { c.Name = "Alexander" },        // maxLen
		func(c *constrainedFast) { c.Name = "alice" },            // pattern
		func(c *constrainedFast) { c.Code = "abcd" },             // len
		func(c *constrainedFast) { c.Code = "abd" },              // string choices
		func(c *constrainedFast) { c.Age = -5 },                  // min
		func(c *constrainedFast) { c.Age = 135 },                 // max
		func(c *constrainedFast) { c.Age = 31 },                  // multipleOf
		func(c *constrainedFast) { c.Score = 2 },                 // number choices
		func(c *constrainedFast) { c.Units = 1001 },              // max on an unsigned field
		func(c *constrainedFast) { c.Nick = &short },             // a constraint through a pointer
		func(c *constrainedFast) { c.Tags = []string{"a", "c"} }, // an element's choices
		func(c *constrainedFast) { c.Refs = []*int{nil} },        // a nil element of a non-null array
		func(c *constrainedFast) { c.Age, c.Name = -5, "a" },     // two faults in one record
	} {
		invalid = append(invalid, variant(mutate))
	}
	return valid, invalid
}

// The fast encoder validates constrained types by asking the validator
// (SPEC 0003 §5.2); on any refusal it declines and the tree reports. Both paths
// must produce the same text, or the same error, for every sample — and for
// the samples as collections, where one bad record fails the lot.
func TestFastPathMatchesTreePathOnConstrainedTypes(t *testing.T) {
	valid, invalid := constrainedFastSamples()
	for _, c := range valid {
		if _, err := io.Marshal(c); err != nil {
			t.Fatalf("a valid sample is refused: %v\n%+v", err, c)
		}
		sameMarshalBothWays(t, c)
	}
	for _, c := range invalid {
		if _, err := io.Marshal(c); err == nil {
			t.Errorf("an invalid sample is accepted, so it tests nothing: %+v", c)
		}
		sameMarshalBothWays(t, c)
	}
	sameMarshalBothWays(t, valid)
	sameMarshalBothWays(t, append(valid, invalid...))
	sameMarshalBothWays(t, typeChangingTag{N: 1})
	sameMarshalBothWays(t, arrayLevelConstraint{Tags: []string{"a"}})
	sameMarshalBothWays(t, arrayLevelConstraint{Tags: []string{"a", "b"}})
}

// FuzzFastPathMatchesTreePathOnConstrainedTypes drives arbitrary values
// through a constrained type on both paths, comparing text or error.
func FuzzFastPathMatchesTreePathOnConstrainedTypes(f *testing.F) {
	f.Add("Alice", "abc", 30, 1.5, uint16(7), true, "Al", "a")
	f.Add("alice", "abd", -5, 2.0, uint16(1001), false, "", "c")
	f.Add("@x", "abc", 30, 1.5, uint16(7), true, "@y", "@z")
	f.Fuzz(func(t *testing.T, name, code string, age int, score float64, units uint16, ok bool, nick, tag string) {
		v := constrainedFast{Name: name, Code: code, Age: age, Score: score, Units: units, OK: ok}
		if nick != "" {
			v.Nick = &nick
		}
		if tag != "" {
			v.Tags = []string{tag}
		}
		sameMarshalBothWays(t, v)

		// The same arguments through a type that mostly constrains nothing, so
		// a value can pass the one constraint and still reach what an
		// unconstrained field can fail.
		loose := looseFast{Name: name, Age: age, Note: code, Tags: []string{tag}}
		if nick != "" {
			loose.Nick = &nick
		}
		if ok {
			loose.Label = &code
		}
		sameMarshalBothWays(t, loose)
	})
}
