package internetobject_test

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The public catalogue must not be able to drift from the codes the pipeline
// actually raises. Rather than trusting a hand-kept list, this reads the two
// files where codes are DECLARED — internal/errs/errs.go and the tokenizer's
// code table — and asserts the public constants cover them exactly.
//
// Adding an internal code without exposing it, or exposing one nothing raises,
// fails here. It is a source scan, which is unusual in a test, but the
// alternative is two lists that agree only by discipline.
func TestPublicCodesCoverEveryInternalCode(t *testing.T) {
	internal := codesDeclaredInternally(t)
	public := publicCodes(t)

	for _, c := range internal {
		if !public[c] {
			t.Errorf("code %q is raised by the pipeline but has no exported constant", c)
		}
	}
	declared := map[string]bool{}
	for _, c := range internal {
		declared[c] = true
	}
	for c := range public {
		if !declared[c] {
			t.Errorf("exported constant %q names a code nothing raises", c)
		}
	}
}

// Every code follows the frozen <predicate>-<subject> grammar: lower-case,
// kebab, at least two words. A code that does not is a spec violation, and the
// grammar is the only thing that keeps them predictable across ports.
func TestEveryCodeIsWellFormed(t *testing.T) {
	shape := regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)+$`)
	for c := range publicCodes(t) {
		if !shape.MatchString(c) {
			t.Errorf("code %q does not follow <predicate>-<subject>", c)
		}
	}
}

// The constants are the point of the exercise: a typo becomes a build error
// instead of a comparison that is quietly always false. They must still
// compare against plain literals, so no existing caller breaks.
func TestCodesCompareAgainstConstantsAndLiterals(t *testing.T) {
	_, err := io.Parse("age: {int, min: 10}\n---\n~ 5")
	if err == nil {
		t.Fatal("expected a fault")
	}
	var list io.ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("not an ErrorList: %T", err)
	}

	if list[0].Code != io.MismatchedMin {
		t.Errorf("Code = %v, want io.MismatchedMin", list[0].Code)
	}
	// An untyped string constant still compares, so this is not a breaking
	// change for anyone who already wrote the literal.
	if list[0].Code != "mismatched-min" {
		t.Error("a Code no longer compares against its literal")
	}
	if !list.Has(io.MismatchedMin) || list.Has(io.MismatchedMax) {
		t.Error("Has() disagrees with the codes present")
	}
	if got := list.Codes(); len(got) != 1 || got[0] != io.MismatchedMin {
		t.Errorf("Codes() = %v", got)
	}
	// %s and %v render the code, not a wrapper.
	if got := list[0].Code.String(); got != "mismatched-min" {
		t.Errorf("String() = %q", got)
	}
	if !strings.Contains(err.Error(), "mismatched-min") {
		t.Errorf("the message lost the code: %s", err)
	}
	// errors.Is against a bare literal keeps working too.
	if !errors.Is(err, io.Error{Code: io.MismatchedMin}) {
		t.Error("errors.Is with a constant failed")
	}
}

// A failed record's marker carries the same Code type as the error list, so a
// caller never has to convert between two spellings of the same thing.
func TestErrorItemCarriesTheSameCodeType(t *testing.T) {
	doc, err := io.Parse("~ $S: {n: string, a: int}\n--- $S\n~ Alice, 30\n~ Bob, nope")
	if err == nil {
		t.Fatal("expected a fault")
	}
	rows := doc.Records()
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	item, ok := rows[1].(io.ErrorItem)
	if !ok {
		t.Fatalf("row 1 is %T, want io.ErrorItem", rows[1])
	}
	if item.Code != io.ExpectedInteger {
		t.Errorf("marker code = %v, want io.ExpectedInteger", item.Code)
	}
	var code io.Code = item.Code // must assign without conversion
	if code != doc.Errors()[0].Code {
		t.Error("the marker and the error list disagree about the code")
	}
}

// The categories are io-specs' four, and a category is derived from WHERE a
// fault arose — never from the code's spelling.
func TestCategoriesAreTheSpecsFour(t *testing.T) {
	for _, tc := range []struct {
		src      string
		code     io.Code
		category string
	}{
		{"~ {a: 1", io.ExpectedClosingBracket, io.CategorySyntax},
		{"age: {int, min: 10}\n---\n~ 5", io.MismatchedMin, io.CategoryValidation},
	} {
		_, err := io.Parse(tc.src)
		if err == nil {
			t.Fatalf("%q: expected a fault", tc.src)
		}
		var list io.ErrorList
		if !errors.As(err, &list) {
			t.Fatalf("%q: not an ErrorList", tc.src)
		}
		if !list.Has(tc.code) {
			t.Errorf("%q: got %v, want %v", tc.src, list.Codes(), tc.code)
			continue
		}
		for _, e := range list {
			if e.Code == tc.code && e.Category != tc.category {
				t.Errorf("%v: category = %q, want %q", tc.code, e.Category, tc.category)
			}
		}
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// publicCodes is every exported Code constant, read from this package's own
// source so the test cannot fall behind the file it checks.
func publicCodes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("error-code.go")
	if err != nil {
		t.Fatalf("cannot read error-code.go: %v", err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*\w+\s+Code = "([a-z0-9-]+)"`).
		FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	if len(out) < 40 {
		t.Fatalf("only found %d exported codes; the scan is broken", len(out))
	}
	return out
}

// codesDeclaredInternally reads the two files where codes originate.
func codesDeclaredInternally(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, f := range []string{
		"internal/errs/errs.go",
		"internal/tokenizer/code.go",
	} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("cannot read %s: %v", f, err)
		}
		body := string(src)
		// Category names are not codes, and neither is anything inside the
		// category block.
		for _, m := range regexp.MustCompile(`"([a-z][a-z0-9]*(?:-[a-z0-9]+)+)"`).
			FindAllStringSubmatch(body, -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
