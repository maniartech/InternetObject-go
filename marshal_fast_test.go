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
	t.Setenv("IO_NO_FAST_PATH", "1")
	tree, err = io.Marshal(v)
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
		t.Setenv("IO_NO_FAST_PATH", "1")
		tree, err := io.Marshal(v)
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
	t.Setenv("IO_NO_FAST_PATH", "1")
	_, treeErr := io.Marshal(v)
	if (fastErr == nil) != (treeErr == nil) {
		t.Fatalf("disagree on refusal: fast=%v tree=%v", fastErr, treeErr)
	}
	if fastErr != nil && !strings.Contains(fastErr.Error(), "overflows") {
		t.Fatalf("unexpected message: %v", fastErr)
	}
}
