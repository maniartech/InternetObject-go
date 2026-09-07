package internetobject_test

import (
	"math"
	"slices"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The lazy decode path must be indistinguishable from the tree path: same
// bound values, same designated error codes. `IO_NO_LAZY=1` forces the tree,
// so the same input can be decoded both ways and compared. The tree path is
// the specification here; lazy is an optimization of it, and any disagreement
// is a bug in lazy.

type lazyRow struct {
	Name   string   `io:"name"`
	Age    int      `io:"age"`
	Score  float64  `io:"score"`
	Active bool     `io:"active"`
	Tags   []string `io:"tags"`
}

const lazySchema = "name: string, age: int, score: number, active: bool, tags: [string]\n---\n"

var lazyDocs = []string{
	lazySchema + "~ Alice, 30, 1.5, T, [a, b]",
	lazySchema + "~ Alice, 30, 1.5, T, [a, b]\n~ Bob, 25, -0.5, F, []",
	lazySchema + `~ "a, b", 0, 0, F, ["x y", "--- z"]`,
	lazySchema + "~ ключ, 1, 2, T, [日本]",
	lazySchema + "~ Alice, oops, 1.5, T, []",     // expected-integer
	lazySchema + "~ Alice, 1.5, 1.5, T, []",      // expected-integer (not whole)
	lazySchema + "~ 42, 30, 1.5, T, []",          // expected-string
	lazySchema + "~ Alice, 30, x, T, []",         // expected-number
	lazySchema + "~ Alice, 30, 1.5, 7, []",       // expected-boolean
	lazySchema + "~ Alice, 30, 1.5, T, notarray", // expected-array
	lazySchema + "~ Alice, 30, 1.5, T, [a], surplus",
	lazySchema + "~ Alice",
	lazySchema + "~ age: 30, name: Alice, score: 1, active: T, tags: []",
	lazySchema + "~ N, 30, 1.5, T, []", // forbidden-null
	lazySchema,
}

// decodeBothWays returns the lazy and tree results for the same document.
func decodeBothWays(t *testing.T, src string) (lazyRows, treeRows []lazyRow, lazyErr, treeErr string) {
	t.Helper()
	var a []lazyRow
	if err := io.Unmarshal(src, &a); err != nil {
		lazyErr = errText(err)
	}
	t.Setenv("IO_NO_LAZY", "1")
	var b []lazyRow
	if err := io.Unmarshal(src, &b); err != nil {
		treeErr = errText(err)
	}
	return a, b, lazyErr, treeErr
}

// equalRows compares two decodes of the same document, treating NaN as equal
// to itself.
//
// reflect.DeepEqual cannot: NaN != NaN by IEEE-754, so it called two IDENTICAL
// decodes different the moment a document contained `NaN` — the fuzzer found
// `N,N,NaN` and reported a divergence whose two sides printed the same. The
// format's own comparator has always treated NaN as equal to itself
// (value.Equal), and this is that rule applied to the bound Go structs.
//
// It is otherwise EXACTLY as strict as DeepEqual, deliberately: a nil slice
// and an empty one stay different, because "lazy returned nil where the tree
// returned empty" is precisely the kind of divergence this fuzzer exists for.
func equalRows(a, b []lazyRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		switch {
		case x.Name != y.Name, x.Age != y.Age, x.Active != y.Active:
			return false
		case (x.Tags == nil) != (y.Tags == nil), !slices.Equal(x.Tags, y.Tags):
			return false
		case x.Score != y.Score && !(math.IsNaN(x.Score) && math.IsNaN(y.Score)):
			return false
		}
	}
	return true
}

// errText reduces an error to its designated codes, which are the contract;
// messages and positions are compared separately where they matter.
func errText(err error) string {
	if list, ok := err.(io.ErrorList); ok {
		return joinCodes(list.Codes(), ",")
	}
	return "non-wire:" + err.Error()
}

func TestLazyMatchesTreePath(t *testing.T) {
	for i, src := range lazyDocs {
		lazy, tree, lazyErr, treeErr := decodeBothWays(t, src)
		if lazyErr != treeErr {
			t.Errorf("doc %d error differs:\n lazy %q\n tree %q\n src %q", i, lazyErr, treeErr, src)
			continue
		}
		if !equalRows(lazy, tree) {
			t.Errorf("doc %d values differ:\n lazy %#v\n tree %#v\n src %q", i, lazy, tree, src)
		}
	}
}

// A fault must carry the same designated code AND a real position on both
// paths — the lazy path reports at the offending token, which is what the
// tree path now does too.
func TestLazyFaultsCarryPositions(t *testing.T) {
	var rows []lazyRow
	err := io.Unmarshal(lazySchema+"~ Alice, 30, 1.5, T, []\n~ Bob, oops, 1, F, []", &rows)
	list, ok := err.(io.ErrorList)
	if !ok || len(list) == 0 {
		t.Fatalf("want an ErrorList, got %v", err)
	}
	if list[0].Code != "expected-integer" {
		t.Fatalf("code = %s", list[0].Code)
	}
	if list[0].RecordIndex != 1 || !strings.Contains(list[0].Path, "age") {
		t.Errorf("not located: %+v", list[0])
	}
	if list[0].Line != 4 {
		t.Errorf("line = %d, want 4 (%+v)", list[0].Line, list[0])
	}
}

// Shapes the lazy path must decline, so the tree path keeps owning them.
func TestLazyDeclines(t *testing.T) {
	type constrained struct {
		N int `io:"n" schema:"{int, min: 0}"`
	}
	var c []constrained
	if err := io.Unmarshal("n: {int, min: 0}\n---\n~ -5", &c); err == nil {
		t.Fatal("a constraint violation must still be reported")
	} else if list, ok := err.(io.ErrorList); !ok || list[0].Code != "mismatched-min" {
		t.Fatalf("want mismatched-min from the tree path, got %v", err)
	}

	// A headerless document has no schema; the tree path binds it positionally.
	var one lazyRow
	if err := io.Unmarshal("Alice, 30, 1.5, T, []", &one); err != nil {
		t.Fatalf("headerless single record: %v", err)
	}
	if one.Name != "Alice" || one.Age != 30 {
		t.Fatalf("headerless bind: %+v", one)
	}
}

func FuzzLazyMatchesTreePath(f *testing.F) {
	for _, src := range lazyDocs {
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		lazy, tree, lazyErr, treeErr := decodeBothWays(t, src)
		if lazyErr != treeErr {
			t.Fatalf("error differs:\n lazy %q\n tree %q\n src %q", lazyErr, treeErr, src)
		}
		if !equalRows(lazy, tree) {
			t.Fatalf("values differ:\n lazy %#v\n tree %#v\n src %q", lazy, tree, src)
		}
	})
}

// joinCodes renders a code list for a failure message.
func joinCodes(cs []io.Code, sep string) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = string(c)
	}
	return strings.Join(parts, sep)
}
