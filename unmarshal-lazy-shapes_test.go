package internetobject_test

import (
	"reflect"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The lazy decode path against the general one, over the shapes the
// differential fuzzer never reaches: it decodes into one fixed struct with no
// optional, nullable, pointer, narrow-integer or integer-array member, so every
// branch for those went unchecked. Two validation bypasses shipped in exactly
// that gap (a missing required member, and a positional member after keyed
// ones), invisible because the fuzzer itself was comparing the lazy path with
// itself until 2026-09-14.
//
// Every case here must decode IDENTICALLY both ways — the same values, the same
// designated codes. The general path is the specification; lazy is only allowed
// to be faster.

type optRow struct {
	Name string `io:"name"`
	Nick string `io:"nick"`
	Age  int    `io:"age"`
}

type nullRow struct {
	Name *string `io:"name"`
	Age  *int    `io:"age"`
}

type narrowRow struct {
	A int8    `io:"a"`
	B uint16  `io:"b"`
	C float32 `io:"c"`
	D []int   `io:"d"`
}

// codesOf reduces an error to its designated codes, or marks a non-wire error.
func codesOf(err error) string {
	if err == nil {
		return ""
	}
	if list, ok := err.(io.ErrorList); ok {
		return joinCodes(list.Codes(), ",")
	}
	return "non-wire"
}

// sameBothWays decodes src into a fresh *T twice — lazy path, then the forced
// general path — and fails if the two disagree in any way.
func sameBothWays[T any](t *testing.T, src string) {
	t.Helper()
	var lazy, tree T
	lazyErr := codesOf(io.Unmarshal(src, &lazy))
	var treeErr string
	io.WithTreeDecode(func() { treeErr = codesOf(io.Unmarshal(src, &tree)) })

	if lazyErr != treeErr {
		t.Errorf("%q\n  error differs: lazy %q, tree %q", src, lazyErr, treeErr)
		return
	}
	if !reflect.DeepEqual(lazy, tree) {
		t.Errorf("%q\n  values differ:\n  lazy %+v\n  tree %+v", src, lazy, tree)
	}
}

// sameBothWaysWith is sameBothWays for UnmarshalWith against schema: the
// document with its header, and the same data with the header removed — the
// form a caller holding the schema out of band sends.
func sameBothWaysWith[T any](t *testing.T, schema *io.Schema, header, data string) {
	t.Helper()
	for _, src := range []string{header + data, data} {
		var lazy, tree T
		lazyErr := codesOf(io.UnmarshalWith(src, &lazy, schema))
		var treeErr string
		io.WithTreeDecode(func() { treeErr = codesOf(io.UnmarshalWith(src, &tree, schema)) })
		if lazyErr != treeErr {
			t.Errorf("UnmarshalWith %q\n  error differs: lazy %q, tree %q", src, lazyErr, treeErr)
			continue
		}
		if !reflect.DeepEqual(lazy, tree) {
			t.Errorf("UnmarshalWith %q\n  values differ:\n  lazy %+v\n  tree %+v", src, lazy, tree)
		}
	}
}

// headerSchema compiles the schema a `…\n---\n` header declares.
func headerSchema(t *testing.T, header string) *io.Schema {
	t.Helper()
	s, err := io.ParseSchema(strings.TrimSuffix(header, "\n---\n"))
	if err != nil {
		t.Fatalf("%q: %v", header, err)
	}
	return s
}

// The same shapes through UnmarshalWith (SPEC 0003 §5.3), which now takes the
// lazy path with the supplied schema in place of the header's.
func TestLazyShapesMatchTreePathWithASuppliedSchema(t *testing.T) {
	const opt = "name: string, nick?: string, age: int\n---\n"
	optSchema := headerSchema(t, opt)
	for _, d := range []string{
		"~ Alice, Al, 30", "~ Alice, , 30", "~ Alice", "~ , Al, 30", "~ name: Alice, age: 30",
		"~ age: 0, name: A, 0", "~ Alice, Al, 30, extra", "~ @x, Al, 30", "~ Alice, Al, 30\n~ Bob",
		"Alice, Al, 30", "",
	} {
		sameBothWaysWith[[]optRow](t, optSchema, opt, d)
		if !strings.Contains(d, "\n") {
			sameBothWaysWith[optRow](t, optSchema, opt, d)
		}
	}
	for _, c := range constrainedShapes {
		schema, err := io.ParseSchema(strings.TrimSuffix(c.header, "\n---\n"))
		if err != nil {
			continue // a header naming an undefined variable does not compile alone
		}
		sameBothWaysWith[[]optRow](t, schema, c.header, c.doc)
		sameBothWaysWith[optRow](t, schema, c.header, c.doc)
	}

	// The supplied schema wins over a header that is LOOSER than it: the header
	// alone would accept age 30, the supplied schema does not.
	strict, err := io.ParseSchema("name: string, nick?: string, age: {int, max: 10}")
	if err != nil {
		t.Fatal(err)
	}
	sameBothWaysWith[[]optRow](t, strict, opt, "~ Alice, Al, 30")
	sameBothWaysWith[optRow](t, strict, opt, "~ Alice, Al, 30")
	var r optRow
	if err := io.UnmarshalWith(opt+"~ Alice, Al, 30", &r, strict); err == nil {
		t.Error("the supplied schema's max was not applied over the header's schema")
	}

	// The supplied schema wins, but a broken header still fails the document:
	// the general path reads it either way.
	for _, src := range []string{
		"~ $draft: {title: nosuchtype}\n---\n~ Alice, Al, 30",
		"~ @n: 1b\n---\n~ Alice, Al, 30",
		"age: int\n---\n~ Alice, Al, 30", // a different header schema, overridden
		// A header literal that does not parse fails the document on the tree
		// path, whatever schema is supplied; the lazy path once bound these
		// (review of SPEC 0003 §5.3).
		"~ meta: d\"2024-99-99\"\n---\n~ Alice, Al, 30",
		"~ meta: 12.5n\n---\n~ Alice, Al, 30",
		"~ meta: 0xZZ\n---\n~ Alice, Al, 30",
		"~ meta: dt\"nope\"\n---\n~ Alice, Al, 30",
		"~ meta: [d\"2024-99-99\"]\n---\n~ Alice, Al, 30",
		"~ meta: {a: t\"99:99\"}\n---\n~ Alice, Al, 30",
		"~ meta: d\"2024-01-15\"\n---\n~ Alice, Al, 30", // a good literal, for contrast
	} {
		var lazy, tree []optRow
		lazyErr := codesOf(io.UnmarshalWith(src, &lazy, optSchema))
		var treeErr string
		io.WithTreeDecode(func() { treeErr = codesOf(io.UnmarshalWith(src, &tree, optSchema)) })
		if lazyErr != treeErr || !reflect.DeepEqual(lazy, tree) {
			t.Errorf("UnmarshalWith %q: lazy %q %+v, tree %q %+v", src, lazyErr, lazy, treeErr, tree)
		}
	}
}

// FuzzUnmarshalWithMatchesTreePath drives an arbitrary schema and document
// through UnmarshalWith both ways, with the document's header kept and
// removed, into a slice and a single record.
func FuzzUnmarshalWithMatchesTreePath(f *testing.F) {
	f.Add("name: string, nick?: string, age: int", "name: string, nick?: string, age: int\n---\n~ Alice, Al, 30")
	f.Add("name: {string, minLen: 3}, nick?: string, age: {int, max: 10}", "~ meta: d\"2024-01-15\"\n---\n~ Al, x, 30\n~ Bob, , 3")
	f.Add("name: string, nick?: string, age: int", "~ meta: 12.5n\n---\n~ Alice, Al, 30")
	f.Add("name: string, nick?: string, age: int", "Alice, Al, 30")
	f.Fuzz(func(t *testing.T, def, src string) {
		schema, err := io.ParseSchema(def)
		if err != nil {
			return
		}
		data := src
		if i := strings.Index(src, "\n---\n"); i >= 0 {
			data = src[i+len("\n---\n"):]
		}
		for _, doc := range []string{src, data} {
			var lazyRows, treeRows []optRow
			lazyErr := codesOf(io.UnmarshalWith(doc, &lazyRows, schema))
			var treeErr string
			io.WithTreeDecode(func() { treeErr = codesOf(io.UnmarshalWith(doc, &treeRows, schema)) })
			if lazyErr != treeErr || !reflect.DeepEqual(lazyRows, treeRows) {
				t.Fatalf("UnmarshalWith(%q) against %q:\n lazy %q %+v\n tree %q %+v", doc, def, lazyErr, lazyRows, treeErr, treeRows)
			}
			var lazyOne, treeOne optRow
			lazyErr = codesOf(io.UnmarshalWith(doc, &lazyOne, schema))
			io.WithTreeDecode(func() { treeErr = codesOf(io.UnmarshalWith(doc, &treeOne, schema)) })
			if lazyErr != treeErr || lazyOne != treeOne {
				t.Fatalf("UnmarshalWith(%q) into a record against %q:\n lazy %q %+v\n tree %q %+v", doc, def, lazyErr, lazyOne, treeErr, treeOne)
			}
		}
	})
}

func TestLazyShapesMatchTreePath(t *testing.T) {
	const opt = "name: string, nick?: string, age: int\n---\n"
	optDocs := []string{
		"~ Alice, Al, 30",
		"~ Alice, , 30", // an empty slot for an OPTIONAL member
		"~ Alice,,30",
		"~ Alice",                          // required age never arrives
		"~ Alice, Al",                      // …nor here
		"~ , Al, 30",                       // an empty slot for a REQUIRED member
		"~ name: Alice, age: 30",           // keyed; optional nick absent
		"~ age: 30, nick: X, name: Y",      // keyed, out of order
		"~ name: Alice, name: Bob, age: 1", // the same member twice
		"~ age: 0, name: A, 0",             // positional after keyed
		"~ Alice, nick: Al, age: 30",       // keyed after positional
		"~ Alice, Al, 30, extra",           // surplus member
		"~ , , ",
		"~ Alice, Al, 30\n~ Bob", // a bad second record
		"~ Alice, Al, 30\n~ Bob, , 25",
		"~ @x, Al, 30",      // an undefined variable reference, open
		`~ "@x", Al, 30`,    // quoted: a reference in any string form
		"~ @, Al, 30",       // a lone @ is plain text
		"~ Alice, @nick, 1", // a reference in an optional member
	}
	const null = "name*: string, age*: int\n---\n"
	nullDocs := []string{
		"~ Alice, 30", "~ N, N", "~ Alice, N", "~ N, 30",
		"~ , 30", // an empty slot for a nullable but REQUIRED member
		"~ Alice",
	}
	const narrow = "a: int, b: int, c: number, d: [int]\n---\n"
	narrowDocs := []string{
		"~ 127, 65535, 1.5, [1, 2]",
		"~ 128, 0, 0, []", // int8 overflow
		"~ -129, 0, 0, []",
		"~ 1, -1, 0, []",    // negative into uint16
		"~ 1, 70000, 0, []", // uint16 overflow
		"~ 1, 1, 1e40, []",  // beyond float32
		"~ 1, 1, 0, [1.5]",  // a non-integer element
		"~ 1, 1, 0, [a]",
		"~ 1, 1, 0, [N]",
		"~ 1.5, 1, 0, []",
		"~ a: 1, b: 2, 3, [4]", // keyed first, then positional onto UNFILLED fields
	}

	// Constraints and choices, one family per header, each with a document that
	// passes and one that does not. The fast path asks the validator itself
	// (schema.Accepts) and declines on a refusal, so both paths must agree on
	// every one: the passing value bound identically, the failing one reported
	// with the same code. TestLazyConstraintFamiliesAreLive proves each row can
	// fail.
	for _, c := range constrainedShapes {
		sameBothWays[[]optRow](t, c.header+c.doc)
		sameBothWays[optRow](t, c.header+c.doc)
	}

	// Branches a constrained schema reaches that the rows above do not: a
	// constraint on an array as a whole (declined), a bool judged by choices,
	// and an unsigned field with a bound.
	type flagRow struct {
		B bool   `io:"b"`
		U uint16 `io:"u"`
		D []int  `io:"d"`
	}
	for _, c := range []struct{ header, doc string }{
		{"b: bool, u: int, d: {array, of: int, minLen: 1}\n---\n", "~ T, 1, []"},
		{"b: bool, u: int, d: {array, of: int, minLen: 1}\n---\n", "~ T, 1, [2]"},
		{"b: {any, choices: [T]}, u: int, d: [int]\n---\n", "~ F, 1, []"},
		{"b: {any, choices: [T]}, u: int, d: [int]\n---\n", "~ T, 1, []"},
		{"b: bool, u: {int, max: 10}, d: [int]\n---\n", "~ T, 11, []"},
		{"b: bool, u: {int, max: 10}, d: [int]\n---\n", "~ T, 10, []"},
	} {
		sameBothWays[flagRow](t, c.header+c.doc)
		sameBothWays[[]flagRow](t, c.header+c.doc)
	}

	for _, d := range optDocs {
		sameBothWays[[]optRow](t, opt+d)
		if !strings.Contains(d, "\n") {
			sameBothWays[optRow](t, opt+d)
		}
	}
	for _, d := range nullDocs {
		sameBothWays[[]nullRow](t, null+d)
		sameBothWays[nullRow](t, null+d)
	}
	for _, d := range narrowDocs {
		sameBothWays[[]narrowRow](t, narrow+d)
		sameBothWays[narrowRow](t, narrow+d)
	}
}

// constrainedShapes is one row per constraint family the fast decoder now
// accepts: a header and a document for optRow (name, nick?, age).
var constrainedShapes = []struct{ family, header, doc string }{
	{"minLen", "name: {string, minLen: 3}, nick?: string, age: int\n---\n", "~ Al, x, 1"},
	{"maxLen", "name: {string, maxLen: 3}, nick?: string, age: int\n---\n", "~ Alice, x, 1"},
	{"len", "name: {string, len: 3}, nick?: string, age: int\n---\n", "~ Alice, x, 1"},
	{"pattern", "name: {string, pattern: '^[A-Z]'}, nick?: string, age: int\n---\n", "~ alice, x, 1"},
	{"string choices", "name: {string, choices: [Ann, Bob]}, nick?: string, age: int\n---\n", "~ Cat, x, 1"},
	{"min", "name: string, nick?: string, age: {int, min: 18}\n---\n", "~ Ann, x, 17"},
	{"max", "name: string, nick?: string, age: {int, max: 130}\n---\n", "~ Ann, x, 131"},
	{"multipleOf", "name: string, nick?: string, age: {int, multipleOf: 5}\n---\n", "~ Ann, x, 12"},
	{"number choices", "name: string, nick?: string, age: {int, choices: [1, 2]}\n---\n", "~ Ann, x, 3"},
	{"optional constrained", "name: string, nick?: {string, minLen: 2}, age: int\n---\n", "~ Ann, x, 3"},
	{"bound naming a variable", "name: string, nick?: string, age: {int, min: @n}\n---\n", "~ Ann, x, 3"},
}

// Every constrainedShapes row must be one the constraint actually refuses —
// otherwise its agreement proves nothing about the check. Its passing twin
// (the same header, with a value that satisfies it) must decode cleanly.
func TestLazyConstraintFamiliesAreLive(t *testing.T) {
	passing := map[string]string{
		"minLen": "~ Ann, x, 1", "maxLen": "~ Al, x, 1", "len": "~ Ann, x, 1",
		"pattern": "~ Alice, x, 1", "string choices": "~ Bob, x, 1",
		"min": "~ Ann, x, 18", "max": "~ Ann, x, 130", "multipleOf": "~ Ann, x, 15",
		"number choices": "~ Ann, x, 2", "optional constrained": "~ Ann, xy, 3",
	}
	for _, c := range constrainedShapes {
		var r optRow
		if err := io.Unmarshal(c.header+c.doc, &r); err == nil {
			t.Errorf("%s: %q was accepted; the row tests nothing", c.family, c.doc)
		}
		if ok, has := passing[c.family]; has {
			sameBothWays[optRow](t, c.header+ok)
			if err := io.Unmarshal(c.header+ok, &r); err != nil {
				t.Errorf("%s: passing twin %q refused: %v", c.family, ok, err)
			}
		}
	}
}
