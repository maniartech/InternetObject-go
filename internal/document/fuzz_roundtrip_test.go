// Round-trip property fuzzer (io-test-cases PORTING-NOTES.md part 3).
//
// Generates random documents from the VALUE side and asserts, for every one:
//   - String does not panic,
//   - the output re-parses with zero errors (the highest-yield property),
//   - the projected value survives exactly (strict comparator — none of the
//     corpus comparator's cross-type leniency, which could mask data loss),
//   - a second write equals the first (idempotence).
//
// Deterministic: fixed seeds, a local xorshift PRNG, no clocks. The generator
// is built to produce the inputs that break writers: keys with spaces, colons,
// commas, quotes, `---`, keyword keys, numeric keys, empty keys; strings that
// look like numbers, keywords, dates; bare scalar roots (the reference
// fuzzer's blind spot); deep nesting; every scalar type.
//
// Known format limits the generator respects (upstream findings, not
// workarounds): `@`/`$`-leading strings are unrepresentable as data — they
// re-parse as definition references in ANY string form (io-test-cases
// FINDINGS #3), so the string generator strips those prefixes.
//
// The normal test run executes the PORTING-NOTES quick gate (8 seeds x 400).
// The full gate — 8 x 3,000 = 24,000 documents, zero failures — is the soak:
//
//	IO_FUZZ_SOAK=1 go test ./internal/document -run TestRoundTripSoak -v
package document

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

type fuzzRng uint64

func newFuzzRng(seed uint64) *fuzzRng {
	if seed == 0 {
		seed = 1
	}
	r := fuzzRng(seed)
	return &r
}

func (r *fuzzRng) next() uint64 {
	x := uint64(*r)
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*r = fuzzRng(x)
	return x
}

func (r *fuzzRng) below(n int) int { return int(r.next() % uint64(n)) }

func pick[T any](r *fuzzRng, items []T) T { return items[r.below(len(items))] }

var fuzzKeyPool = []string{
	"a", "name", "value2", "_x", "with.dot", "kebab-key", "a b", " lead",
	"trail ", "a,b", "a:b", "a{b", "a[b", "a}b", "a]b", "a~b", "a#b",
	"a---b", "null", "true", "T", "N", "F", "NaN", "Inf", "0", "10", "1.5",
	"-3", "", " ", "ключ", "日本", `a"b`, "a'b", `a\b`, "*", "?", "a?",
	"a*", "$x", "@y", "e5", "0xFF", "a\nb", "a\bb",
}

var fuzzStringFragments = []string{
	"hello", "wörld", "x", "", " ", "  ", "a,b", "a:b", "{a}", "[b]", "~t",
	"#c", `"q"`, "'s'", `\`, "---", "a---b", "null", "true", "T", "F", "N",
	"NaN", "Inf", "0", "007", "1.5", "1e5", "0xFF", "12n", "3.5m", "1.2.3",
	"10.0.0.1", "12mm", "3pm", "013ABSD", "\n", "\t", "a\nb", "€", "😀",
	"日本語", "2024-01-15", "14:30", "14:30:45", "2024-01-15T14:30:45Z",
	`d"x"`, "r'y'", `b"Zm9v"`, "undefined", "+Inf", "-Inf", "5e", "1e+",
	"1e+12n", "2.5e1n", "\b0", "\x00x", "\x1b[0m", "a\vb", "\f", "\r", "a\r\nb",
}

func genFuzzString(r *fuzzRng) string {
	var b strings.Builder
	for n := r.below(4); n > 0; n-- {
		b.WriteString(pick(r, fuzzStringFragments))
	}
	// `@`/`$`-leading strings re-parse as definition references in any string
	// form (FINDINGS #3) — unrepresentable as data by design.
	return strings.TrimLeft(b.String(), "@$")
}

func genFuzzNumber(r *fuzzRng) float64 {
	switch r.below(8) {
	case 0:
		return float64(int32(r.next()))
	case 1:
		return float64(r.next()%100) / 8.0
	case 2:
		return math.NaN()
	case 3:
		if r.below(2) == 0 {
			return math.Inf(1)
		}
		return math.Inf(-1)
	case 4:
		return math.Copysign(0, -1)
	case 5:
		return float64(int64(r.next())) * 1e-8
	case 6:
		return float64(r.next() % 9007199254740993)
	default:
		return math.Float64frombits(r.next() & 0x7FEF_FFFF_FFFF_FFFF) // finite positive
	}
}

func genFuzzTemporal(r *fuzzRng) time.Time {
	switch r.below(3) {
	case 0: // a date: zero clock, 1970-01-02 onward
		day := 1 + r.below(3650)
		return time.Unix(int64(day)*86_400, 0).UTC()
	case 1: // a time-of-day, anchored where the format anchors one
		ms := r.below(86_400_000)
		return core.TimeAnchor.Add(time.Duration(ms) * time.Millisecond)
	default: // a datetime, millisecond precision
		day := 1 + r.below(3650)
		ms := r.below(86_400_000)
		return time.Unix(int64(day)*86_400, 0).Add(time.Duration(ms) * time.Millisecond).UTC()
	}
}

func genFuzzScalar(r *fuzzRng) any {
	switch r.below(9) {
	case 0:
		return nil
	case 1:
		return r.below(2) == 0
	case 2:
		return genFuzzNumber(r)
	case 3:
		return genFuzzString(r)
	case 4:
		return new(big.Int).Mul(big.NewInt(int64(r.next())), big.NewInt(int64(int32(r.next()))))
	case 5:
		coef := big.NewInt(int64(r.next() % 1_000_000_000_000))
		if r.below(2) == 0 {
			coef.Neg(coef)
		}
		return core.Decimal{Coef: coef, Scale: r.below(9)}
	case 6:
		return genFuzzTemporal(r)
	case 7:
		b := make([]byte, r.below(12))
		for i := range b {
			b[i] = byte(r.next())
		}
		return b
	default:
		return float64(r.next() % 1000)
	}
}

func genFuzzValue(r *fuzzRng, depth int) any {
	if depth == 0 {
		return genFuzzScalar(r)
	}
	switch r.below(6) {
	case 0:
		arr := make([]any, r.below(4))
		for i := range arr {
			arr[i] = genFuzzValue(r, depth-1)
		}
		return arr
	case 1:
		return genFuzzObject(r, depth-1)
	default:
		return genFuzzScalar(r)
	}
}

// genFuzzObject builds a record: a positional prefix, then keyed members with
// distinct keys. Positional members never follow keyed ones — that ordering is
// unparseable by design.
func genFuzzObject(r *fuzzRng, depth int) *core.Object {
	obj := &core.Object{}
	for n := r.below(3); n > 0; n-- {
		obj.Members = append(obj.Members, core.Member{Positional: true, Value: genFuzzValue(r, depth)})
	}
	used := map[string]bool{}
	for n := r.below(4); n > 0; n-- {
		key := pick(r, fuzzKeyPool)
		if used[key] {
			continue
		}
		used[key] = true
		obj.Members = append(obj.Members, core.Member{Key: key, Value: genFuzzValue(r, depth)})
	}
	return obj
}

// genFuzzDoc builds a header-less single-section document, the shape every
// writer path shares: a bare scalar root (the reference fuzzer's blind spot),
// one bare record, or a `~`-collection.
func genFuzzDoc(r *fuzzRng) *Doc {
	sec := &parser.Section{Name: "data"}
	switch r.below(4) {
	case 0:
		sec.Records = []any{&core.Object{Members: []core.Member{
			{Positional: true, Value: genFuzzScalar(r)},
		}}}
	case 1:
		sec.Records = []any{genFuzzObject(r, 3)}
	default:
		sec.Collection = true
		for n := 1 + r.below(4); n > 0; n-- {
			sec.Records = append(sec.Records, genFuzzObject(r, 2))
		}
	}
	pdoc := &parser.Document{Sections: []*parser.Section{sec}}
	return &Doc{Document: pdoc, Defs: NewDefinitions(nil), SecSchemas: map[*parser.Section]*schema.Schema{}}
}

// fuzzEq is the fuzzer's strict value equality over PROJECTED values: NaN
// equals NaN, temporals compare by instant (a schema-less writer may
// legitimately re-spell the kind), decimals by exact coefficient and scale,
// objects by key and value in order. No cross-type equivalence.
func fuzzEq(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && ((math.IsNaN(x) && math.IsNaN(y)) || x == y)
	case time.Time:
		y, ok := b.(time.Time)
		return ok && x.UTC().Equal(y.UTC())
	case core.Decimal:
		y, ok := b.(core.Decimal)
		return ok && x.Scale == y.Scale && x.Coef.Cmp(y.Coef) == 0
	case *big.Int:
		y, ok := b.(*big.Int)
		return ok && x.Cmp(y) == 0
	case []byte:
		y, ok := b.([]byte)
		return ok && bytes.Equal(x, y)
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !fuzzEq(x[i], y[i]) {
				return false
			}
		}
		return true
	case *core.Object:
		y, ok := b.(*core.Object)
		if !ok || len(x.Members) != len(y.Members) {
			return false
		}
		for i := range x.Members {
			if x.Members[i].Key != y.Members[i].Key || !fuzzEq(x.Members[i].Value, y.Members[i].Value) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// dumpVal renders a projected value readably for failure reports (Go's %#v
// prints nested objects as pointers).
func dumpVal(v any) string {
	var b strings.Builder
	writeDump(&b, v)
	return b.String()
}

func writeDump(b *strings.Builder, v any) {
	switch x := v.(type) {
	case *core.Object:
		b.WriteString("{")
		for i, m := range x.Members {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "%q: ", m.Key)
			writeDump(b, m.Value)
		}
		b.WriteString("}")
	case []any:
		b.WriteString("[")
		for i, e := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			writeDump(b, e)
		}
		b.WriteString("]")
	case time.Time:
		fmt.Fprintf(b, "T(%s)", x.UTC().Format(time.RFC3339Nano))
	case core.Decimal:
		fmt.Fprintf(b, "%sm", x.String())
	case *big.Int:
		fmt.Fprintf(b, "%sn", x.String())
	case []byte:
		fmt.Fprintf(b, "b(%x)", x)
	case string:
		fmt.Fprintf(b, "%q", x)
	default:
		fmt.Fprintf(b, "%v(%T)", x, x)
	}
}

// diffPath returns the path to the first difference between two projected
// values — the triage fingerprint.
func diffPath(a, b any, path string) string {
	if fuzzEq(a, b) {
		return ""
	}
	switch x := a.(type) {
	case *core.Object:
		y, ok := b.(*core.Object)
		if !ok {
			return path + fmt.Sprintf(": %T became %T", a, b)
		}
		if len(x.Members) != len(y.Members) {
			return path + fmt.Sprintf(": %d members became %d", len(x.Members), len(y.Members))
		}
		for i := range x.Members {
			if x.Members[i].Key != y.Members[i].Key {
				return path + fmt.Sprintf(": key %q became %q", x.Members[i].Key, y.Members[i].Key)
			}
			if p := diffPath(x.Members[i].Value, y.Members[i].Value, path+"."+x.Members[i].Key); p != "" {
				return p
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok {
			return path + fmt.Sprintf(": %T became %T", a, b)
		}
		if len(x) != len(y) {
			return path + fmt.Sprintf(": %d elems became %d", len(x), len(y))
		}
		for i := range x {
			if p := diffPath(x[i], y[i], fmt.Sprintf("%s[%d]", path, i)); p != "" {
				return p
			}
		}
	}
	return path + ": " + dumpVal(a) + " became " + dumpVal(b)
}

// ── properties and shrinking ───────────────────────────────────────────────
//
// The property list is aligned with io-js2's own generative fuzzer
// (io-js2/tools/fuzz), minus the two properties that describe a feature this
// port does not have (schema inference). Its SHRINKING is the part worth
// copying outright: a soak reports a seed and a round number, and reproducing a
// failure from "document 18,431 of seed 0xC0FFEE" is archaeology. A minimized
// counterexample is one you can read in the failure message.

// fuzzProperty checks every property against one document and returns the first
// violation, or "" when the document is clean. A panic is itself a violation —
// that is the `does-not-throw` half of the list.
func fuzzProperty(d *Doc) (violation string) {
	defer func() {
		if r := recover(); r != nil {
			violation = fmt.Sprintf("panic: %v", r)
		}
	}()

	original := d.Project() // result-projects
	text := d.String()      // stringify-does-not-throw
	back := Parse(text)
	if len(back.Errors) > 0 { // output-reparses
		codes := make([]string, len(back.Errors))
		for i, e := range back.Errors {
			codes[i] = e.Code
		}
		return "output does not re-parse: " + strings.Join(codes, ", ")
	}
	reparsed := back.Project()
	if !fuzzEq(original, reparsed) { // value-preserved
		return "value changed at " + diffPath(original, reparsed, "$")
	}
	if second := back.String(); second != text { // stringify-idempotent
		return "not idempotent: second write differs"
	}
	return ""
}

// fuzzDocOf rebuilds a one-section document around the given records.
func fuzzDocOf(collection bool, records []any) *Doc {
	sec := &parser.Section{Name: "data", Collection: collection, Records: records}
	return &Doc{
		Document:   &parser.Document{Sections: []*parser.Section{sec}},
		Defs:       NewDefinitions(nil),
		SecSchemas: map[*parser.Section]*schema.Schema{},
	}
}

// shrinkFuzzValue offers simpler versions of one value: fewer members, fewer
// elements, and finally nothing at all. Sub-values are SHARED rather than
// cloned, which is safe because nothing on this path mutates a document.
func shrinkFuzzValue(v any) []any {
	switch x := v.(type) {
	case *core.Object:
		var out []any
		for i := range x.Members {
			ms := append(append([]core.Member{}, x.Members[:i]...), x.Members[i+1:]...)
			out = append(out, &core.Object{Members: ms})
		}
		for i := range x.Members {
			for _, simpler := range shrinkFuzzValue(x.Members[i].Value) {
				ms := append([]core.Member{}, x.Members...)
				ms[i].Value = simpler
				out = append(out, &core.Object{Members: ms})
			}
		}
		return out
	case []any:
		var out []any
		for i := range x {
			out = append(out, append(append([]any{}, x[:i]...), x[i+1:]...))
		}
		for i := range x {
			for _, simpler := range shrinkFuzzValue(x[i]) {
				es := append([]any{}, x...)
				es[i] = simpler
				out = append(out, es)
			}
		}
		return out
	case nil:
		return nil // already minimal
	default:
		return []any{nil} // a scalar reduces to null
	}
}

// shrinkFuzzDoc minimizes a failing document while it keeps failing. Greedy and
// bounded: each pass takes the first candidate that still fails, and the whole
// search is capped so a pathological case cannot turn a test run into a hang.
func shrinkFuzzDoc(d *Doc) *Doc {
	const maxSteps = 500
	for step := 0; step < maxSteps; step++ {
		sec := d.Sections[0]
		var improved bool
		if len(sec.Records) > 1 {
			for i := range sec.Records {
				cand := fuzzDocOf(sec.Collection,
					append(append([]any{}, sec.Records[:i]...), sec.Records[i+1:]...))
				if fuzzProperty(cand) != "" {
					d, improved = cand, true
					break
				}
			}
		}
		if !improved {
			for i := range sec.Records {
				for _, simpler := range shrinkFuzzValue(sec.Records[i]) {
					recs := append([]any{}, sec.Records...)
					recs[i] = simpler
					cand := fuzzDocOf(sec.Collection, recs)
					if fuzzProperty(cand) != "" {
						d, improved = cand, true
						break
					}
				}
				if improved {
					break
				}
			}
		}
		if !improved {
			return d
		}
	}
	return d
}

func runFuzzSeed(t *testing.T, seed uint64, rounds int) (failures int) {
	t.Helper()
	r := newFuzzRng(seed)
	for round := 0; round < rounds; round++ {
		doc := genFuzzDoc(r)
		why := fuzzProperty(doc)
		if why == "" {
			continue
		}
		failures++
		// Report the SHRUNK document, not the generated one. The generated one
		// is whatever the RNG happened to build around the defect; the shrunk
		// one is the defect.
		small := shrinkFuzzDoc(doc)
		t.Errorf("seed %#x round %d: %s"+nlFuzz+
			"  shrunk to: %s"+nlFuzz+
			"  it writes: %q",
			seed, round, why, dumpVal(small.Project()), safeString(small))
	}
	return failures
}

// nlFuzz is a newline, named so this file needs no escape in its format
// strings when they are assembled by tooling.
var nlFuzz = string(rune(10))

// safeString renders a document without letting a panic escape the reporter —
// the failure being reported may BE that panic.
func safeString(d *Doc) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("<panic: %v>", r)
		}
	}()
	return d.String()
}

var fuzzSeeds = []uint64{
	0x1, 0xDEADBEEF, 0xC0FFEE, 0x12345678,
	0x9E3779B9, 0xFEEDFACE, 0x2718281828, 0x31415926535,
}

// TestRoundTripPropertiesHold is the quick gate: 8 seeds x 400 documents in
// every normal test run.
func TestRoundTripPropertiesHold(t *testing.T) {
	failures := 0
	for _, seed := range fuzzSeeds {
		failures += runFuzzSeed(t, seed, 400)
	}
	t.Logf("fuzz: %d documents, %d failures", len(fuzzSeeds)*400, failures)
}

// TestRoundTripSoak is the PORTING-NOTES gate — 8 seeds x 3,000 = 24,000
// documents, zero failures. Opt in with IO_FUZZ_SOAK=1; IO_FUZZ_ROUNDS
// overrides the per-seed round count for deeper one-off soaks.
func TestRoundTripSoak(t *testing.T) {
	if os.Getenv("IO_FUZZ_SOAK") == "" {
		t.Skip("set IO_FUZZ_SOAK=1 to run the 24,000-document soak")
	}
	rounds := 3000
	if s := os.Getenv("IO_FUZZ_ROUNDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			rounds = n
		}
	}
	failures := 0
	for _, seed := range fuzzSeeds {
		failures += runFuzzSeed(t, seed, rounds)
	}
	t.Logf("soak: %d documents, %d failures", len(fuzzSeeds)*rounds, failures)
}
