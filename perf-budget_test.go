//go:build !race

// The race detector's instrumentation allocates on purpose, so these budgets
// describe the uninstrumented build only.

package internetobject_test

import (
	"math"
	"os"
	"runtime"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// Performance budgets: the gate that was missing.
//
// On 2026-09-07 a correctness fix made the 1,000-record marshal quote strings
// it did not need to. The document grew 3%, the output buffer crossed its size
// estimate, and encode went from 1.17x faster than encoding/json to 1.5x
// slower. The commit's own gate said "allocation counts unchanged" — true for
// the benchmarks it looked at, and nothing looked at the rest. It was found by
// hand six days later (OPEN-QUESTIONS #4).
//
// These budgets fail the ordinary `go test` run instead. They measure bytes and
// allocations per operation, exactly as `-benchmem` does, because both are
// exact and load-independent: they read the same on a busy laptop and a shared
// CI runner, where nanoseconds are noise.
//
// WHEN A BUDGET FAILS: if the change is a real regression, fix it. If it is the
// price of a correctness fix, raise the budget IN THE SAME COMMIT and say why —
// that is the point: a performance cost becomes a reviewed decision instead of
// a side effect. A budget beaten by a wide margin FAILS too, asking for the
// lower figure: a win that is not written down can be given back silently, and
// a budget far above the truth would let it.

// budget is one operation's ceiling.
type budget struct {
	name     string
	op       func()
	maxAlloc float64 // allocations per operation
	maxBytes float64 // bytes allocated per operation
}

// Tolerances absorb the small drift left between supported Go releases (1.27
// counts one more allocation than 1.26 on the dynamic parse) while still
// failing on any regression worth the name — the one this gate exists for was
// +54% bytes. Older releases are handled by budgetGoMinor, not by widening these.
const (
	bytesTolerance = 1.03
	allocTolerance = 1.02
	ratchetMargin  = 0.90 // at or below this fraction of budget, demand a lower one
)

// A newer Go release that allocates 10% less on a small operation would trip the
// ratchet for reasons that are not this library's. If that happens, re-measure
// on that release and bound the budgets above as well as below (as budgetGoMinor
// bounds them below), rather than loosening the margin.

func perfBudgets(t *testing.T) []budget {
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	return []budget{
		{"Marshal 1,000 structs", func() {
			_, err := io.Marshal(benchData)
			must(err)
		}, 4, 180_248}, // 22 -> 20 on 2026-09-14: no os.Getenv per Marshal (it
		// allocates on Windows). 20 -> 4 the same day (SPEC 0003 §5.1): the schema
		// header is rendered once per type, not once per call.
		{"Unmarshal 1,000 structs", func() {
			var out []benchPerson
			must(io.Unmarshal(benchIOText, &out))
		}, 4_021, 1_145_531}, // 4,024 -> 4,020 on 2026-09-14 (SPEC 0003 §5.1); 4,021 once
		// measure refilled pools after each GC (§5.2) — HEAD reads the same: drift.
		{"Parse 1,000 records dynamically", func() {
			doc, err := io.Parse(benchIOText)
			must(err)
			_ = doc.Value()
		}, 17_950, 1_456_488},
		{"Unmarshal one small record", func() {
			var p benchPerson
			must(io.Unmarshal(oneIO, &p))
		}, 10, 1_968}, // 14 -> 10 on 2026-09-14 (§5.1): no unread Definitions, no
		// second scan. encoding/json spends 11.
		{"Validate 1,000 structs", func() {
			must(io.Validate(benchData))
		}, 12_009, 659_154},

		// Constrained schemas and the runtime-schema functions (SPEC 0003 §5.6).
		// Measured 2026-09-14 BEFORE any fast path accepted a constraint: these
		// are the honest starting line, and each §5 step must lower them.
		{"Unmarshal 1,000 constrained", func() {
			var out []constrainedPerson
			must(io.Unmarshal(constrainedIOText, &out))
		}, 9_022, 1_229_498}, // 18,997 -> 18,974 (§5.1: no framing before a
		// decline); the schema then gained `pattern` and `choices` (19,014), and
		// §5.2 took it to 9,022 — the fast decoder checks constraints itself.
		{"Marshal 1,000 constrained", func() {
			_, err := io.Marshal(constrainedData)
			must(err)
		}, 5_005, 256_194}, // 12,136 with `pattern`/`choices` -> 5,005 (§5.2)
		{"Unmarshal one constrained record", func() {
			var p constrainedPerson
			must(io.Unmarshal(constrainedOneIO, &p))
		}, 15, 5_928}, // 82 -> 71 (§5.1); 108 with `pattern`/`choices` -> 15 (§5.2)
		{"UnmarshalWith one record", func() {
			var p benchPerson
			must(io.UnmarshalWith(constrainedOneIO, &p, constrainedSchema))
		}, 15, 5_928}, // generated code's Unmarshal. 60 -> 15 (§5.3): it takes the lazy path.
		// (47 -> 60 before that, when the benchmark schema gained `pattern` and `choices`.)
		{"UnmarshalWith one headerless record", func() {
			var p benchPerson
			must(io.UnmarshalWith(constrainedOneRow, &p, constrainedSchema))
		}, 14, 1_352}, // 27 -> 14 (§5.3): header-less input is framed
		{"UnmarshalWith 1,000 records", func() {
			var out []benchPerson
			must(io.UnmarshalWith(constrainedIOText, &out, constrainedSchema))
		}, 9_022, 1_233_388}, // 18,966 -> 9,022 (§5.3)
		{"MarshalWith one record", func() {
			_, err := io.MarshalWith(onePerson, constrainedSchema)
			must(err)
		}, 7, 632}, // 17 -> 7 (§5.3): the direct encoder, against the given schema
		{"MarshalWith 1,000 records", func() {
			_, err := io.MarshalWith(benchData, constrainedSchema)
			must(err)
		}, 5_004, 252_248}, // 12,017 -> 5,004 (§5.3)
		// A header's bare schema expression is compiled once per document, not
		// once per section that binds to it (review of SPEC 0004 A1: 69 -> 96
		// allocations when it was recompiled per section).
		{"Parse a 4-section document", func() {
			_, err := io.Parse(fourSections)
			must(err)
		}, 67, 5_632},
		// A builder started from a parsed document resolves that document's
		// schemas once, not once per record added (same review: +7 allocations
		// per Add when it did not). A fresh builder each time, so the section's
		// growing record slice cannot make the figure drift.
		{"NewBuilderFrom, then Add twice", func() {
			sec := io.NewBuilderFrom(builderDoc).Section("", "p")
			must(sec.Add(builderRecord))
			must(sec.Add(builderRecord))
		}, 94, 5_168},
		{"ValidateWith one record", func() {
			must(io.ValidateWith(onePerson, constrainedSchema))
		}, 12, 672}, // generated code's constructor and every setter
	}
}

const fourSections = "name: string, age: int\n--- a\n~ A, 1\n--- b\n~ B, 2\n--- c\n~ C, 3\n--- d\n~ D, 4"

var (
	builderDoc    *io.Document
	builderRecord = map[string]any{"name": "B", "home": map[string]any{"city": "Y"}}
)

func init() {
	var err error
	builderDoc, err = io.Parse("~ $addr: {city: string}\n~ $p: {name: string, home: $addr}\n--- $p\n~ A, {X}")
	if err != nil {
		panic(err)
	}
}

// The lazy decoder takes a CRLF document as it takes the same document with LF
// endings: normalizing them costs one allocation, and falling back to the tree
// would cost several times the whole decode. Until 2026-09-14 a multi-line CRLF
// header made the lazy path decline — it sliced the caller's text with offsets
// into the normalized copy — and nothing noticed, because declining is always
// correct. This is the test that notices.
func TestLazyPathTakesCRLFDocuments(t *testing.T) {
	if v := forcedRoute(); v != "" {
		t.Skipf("%s forces a general route", v)
	}
	lf := "~ $other: {x: int}\n~ $schema: {name: string, age: int, score: number, active: bool, tags: [string]}\n" +
		"---\n~ Alice, 30, 1.5, T, [a]\n~ Bob, 25, 2, F, []\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	decode := func(src string) func() {
		return func() {
			var rows []lazyRow
			if err := io.Unmarshal(src, &rows); err != nil || len(rows) != 2 {
				t.Fatalf("%q: %v %v", src, rows, err)
			}
		}
	}
	lfAllocs, _ := measure(10, decode(lf))
	crlfAllocs, _ := measure(10, decode(crlf))
	if crlfAllocs > lfAllocs+1 {
		t.Errorf("CRLF decode spends %.0f allocations against LF's %.0f: it is not taking the lazy path", crlfAllocs, lfAllocs)
	}
}

// The fast encoder takes a type whose `schema` tags carry constraints
// (SPEC 0003 §5.2) — otherwise the constrained differential tests compare the
// tree with itself, as an invalid tag once made them do. Taking it is visible in
// allocations: the tree builds a record per call, the fast path does not.
func TestFastEncoderTakesConstrainedTypes(t *testing.T) {
	if v := forcedRoute(); v != "" {
		t.Skipf("%s forces a general route", v)
	}
	valid, _ := constrainedFastSamples()
	v := valid[0]
	marshal := func() {
		if _, err := io.Marshal(v); err != nil {
			t.Fatal(err)
		}
	}
	fast, _ := measure(10, marshal)
	var tree float64
	io.WithTreeEncode(func() { tree, _ = measure(10, marshal) })
	if fast*2 > tree {
		t.Errorf("Marshal spends %.0f allocations, the tree path %.0f: the fast encoder is not taking the type", fast, tree)
	}
}

// MarshalWith takes the direct encoder (SPEC 0003 §5.3), visible in
// allocations as for Marshal.
func TestFastEncoderTakesMarshalWith(t *testing.T) {
	if v := forcedRoute(); v != "" {
		t.Skipf("%s forces a general route", v)
	}
	marshal := func() {
		if _, err := io.MarshalWith(onePerson, constrainedSchema); err != nil {
			t.Fatal(err)
		}
	}
	fast, _ := measure(10, marshal)
	var tree float64
	io.WithTreeEncode(func() { tree, _ = measure(10, marshal) })
	if fast*2 > tree {
		t.Errorf("MarshalWith spends %.0f allocations, the tree path %.0f: the fast encoder is not taking it", fast, tree)
	}
}

// measure returns allocations and bytes per call, as -benchmem computes them.
// It warms first, so the plan and header caches are measured in the steady
// state every real caller sees, and reports the MINIMUM of three rounds:
// another test's goroutine can only add to a round, never subtract, so the
// minimum is the least noisy estimate of this operation alone.
func measure(n int, op func()) (allocs, bytes float64) {
	op()
	allocs, bytes = -1, -1
	for round := 0; round < 3; round++ {
		runtime.GC()
		// The collection just emptied every sync.Pool — regexp keeps its
		// matchers in one — so one untimed call refills them. Without it a
		// `pattern` constraint made the bytes of an operation swing by
		// kilobytes from run to run, on paths nothing had changed.
		op()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for i := 0; i < n; i++ {
			op()
		}
		runtime.ReadMemStats(&after)
		a := float64(after.Mallocs-before.Mallocs) / float64(n)
		b := float64(after.TotalAlloc-before.TotalAlloc) / float64(n)
		if allocs < 0 || a < allocs {
			allocs = a
		}
		if bytes < 0 || b < bytes {
			bytes = b
		}
	}
	return allocs, bytes
}

// budgetGoMinor is the Go release the budgets were measured with. They are
// enforced on that release and every later one. An OLDER compiler allocates
// differently for reasons that have nothing to do with this library: measured
// 2026-09-13, Go 1.24.2 spends 24 allocations on the 1,000-record marshal where
// Go 1.26.0 and 1.27.1 both spend 22. Tolerance wide enough to absorb that
// would also absorb the 22 -> 23 regression this gate exists to catch, so
// older releases are skipped instead. A newer release allocating MORE is still
// reported, because that is worth knowing.
const budgetGoMinor = 26

// goMinor returns the minor version of the running Go release (26 for
// "go1.26.0"), or -1 when the version string has no release number, such as a
// development build — which is then held to the budgets.
func goMinor() int {
	v, ok := strings.CutPrefix(runtime.Version(), "go1.")
	if !ok {
		return -1
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// forcedRoute reports an environment that deliberately disables a fast path.
// The budgets describe the fast paths, so they do not apply there.
func forcedRoute() string {
	for _, v := range []string{"IO_NO_LAZY", "IO_NO_FAST_PATH", "IO_NO_HEADER_CACHE"} {
		if os.Getenv(v) != "" {
			return v
		}
	}
	return ""
}

func TestPerformanceBudgets(t *testing.T) {
	if v := forcedRoute(); v != "" {
		t.Skipf("%s forces a general route; the budgets describe the fast routes", v)
	}
	if m := goMinor(); m >= 0 && m < budgetGoMinor {
		t.Skipf("budgets are measured on go1.%d and later; %s allocates differently", budgetGoMinor, runtime.Version())
	}
	for _, b := range perfBudgets(t) {
		allocs, bytes := measure(10, b.op)
		t.Logf("%-34s %8.0f allocs (budget %6.0f)  %10.0f B (budget %9.0f)",
			b.name, allocs, b.maxAlloc, bytes, b.maxBytes)

		// Floor, not +1 slack: on a 22-allocation operation one more IS the
		// regression (it was, here), and 2% of a small count rounds to zero.
		if allocs > math.Floor(b.maxAlloc*allocTolerance) {
			t.Errorf("%s: %.0f allocs/op exceeds the budget of %.0f", b.name, allocs, b.maxAlloc)
		}
		if bytes > b.maxBytes*bytesTolerance {
			t.Errorf("%s: %.0f bytes/op exceeds the budget of %.0f (+%.1f%%)",
				b.name, bytes, b.maxBytes, 100*(bytes/b.maxBytes-1))
		}
		if allocs <= b.maxAlloc*ratchetMargin || bytes <= b.maxBytes*ratchetMargin {
			t.Errorf("%s beat its budget (%.0f allocs, %.0f B): lower it to that, so the win is kept",
				b.name, allocs, bytes)
		}
	}
}

// The document's size is the format's whole argument against JSON, and it is a
// direct readout of the writer's spelling decisions: an over-quoting change
// moves it before it moves anything else. Exact, because it is deterministic.
func TestWireSizeBudget(t *testing.T) {
	const want = 64_125 // 1,000 benchPerson records; JSON is 114,374
	if got := len(benchIOText); got != want {
		t.Errorf("the 1,000-record document is %d bytes, budget %d (%+d). "+
			"If the writer's spelling changed on purpose, update the budget and say why.",
			got, want, got-want)
	}
}
