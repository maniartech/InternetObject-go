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
