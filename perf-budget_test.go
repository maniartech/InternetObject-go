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
// a side effect. When a budget is beaten by a wide margin the test says so;
// lower it, so the win cannot be given back silently.

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
	ratchetMargin  = 0.95 // at or below this fraction of budget, ask for a lower one
)

func perfBudgets(t *testing.T) []budget {
	one := benchPerson{Name: "Alice", Age: 30, Email: "alice@example.com",
		Active: true, Score: 99.5, Tags: []string{"admin"}}
	oneText, err := io.Marshal(one)
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	return []budget{
		{"Marshal 1,000 structs", func() {
			_, err := io.Marshal(benchData)
			must(err)
		}, 20, 180_680}, // 22 -> 20 on 2026-09-14: the fast path stopped calling
		// os.Getenv per Marshal, which allocates on Windows (UTF-16 conversion).
		{"Unmarshal 1,000 structs", func() {
			var out []benchPerson
			must(io.Unmarshal(benchIOText, &out))
		}, 4_025, 1_145_675},
		{"Parse 1,000 records dynamically", func() {
			doc, err := io.Parse(benchIOText)
			must(err)
			_ = doc.Value()
		}, 17_952, 1_456_562},
		{"Unmarshal one small record", func() {
			var p benchPerson
			must(io.Unmarshal(oneText, &p))
		}, 14, 2_160},
		{"Validate 1,000 structs", func() {
			must(io.Validate(benchData))
		}, 12_009, 659_154},
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
		if bytes < b.maxBytes*ratchetMargin {
			t.Logf("  %s beat its byte budget by %.1f%%: lower it to %.0f so the win is kept",
				b.name, 100*(1-bytes/b.maxBytes), bytes)
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
