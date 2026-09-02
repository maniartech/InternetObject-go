package document

import (
	"regexp"
	"testing"
)

// The hand-written scanners that replaced the writer's regexes (ADR 0006 P5)
// are pinned to those regexes here: the originals are kept ONLY in this test,
// and every scanner must agree with its regex on every input below. A
// scanner that drifts from its specification is a silent writer bug, so this
// is the gate that makes the optimization safe.
var (
	reDateLike     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reTimeLike     = regexp.MustCompile(`^\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?$`)
	reDateTimeLike = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?$`)
	reKeyNumeric   = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$`)
	reKeyBareSafe  = regexp.MustCompile(`^[$A-Za-z_][A-Za-z0-9_. -]*$`)
)

// scanInputs deliberately mixes the shapes each pattern accepts, the shapes
// that only ALMOST match (the interesting half), and the writer's real-world
// strings.
var scanInputs = []string{
	"", " ", "a", "A", "_", "$", "$x", "0", "9", "a b", "a-b", "a.b", "a_b",
	"trail ", " lead", "a---b", "*", "?", "ключ", "日本", "a\tb", "a\nb",
	// numeric-looking
	"1", "12", "-3", "+3", "1.5", "1.", ".5", ".", "1e5", "1E5", "1e+5", "1e-5",
	"1e", "1e+", "e5", "1.2.3", "007", "0xFF", "12n", "3.5m", "1_000",
	// date-like
	"2024-01-15", "2024-1-15", "24-01-15", "2024-01-15x", "2024-01-15 ",
	"0000-00-00", "9999-99-99", "2024-01-1", "2024-01-155",
	// time-like
	"14:30", "14:30:45", "14:30:45.1", "14:30:45.123", "14:30:45.", "14:3",
	"1:30", "14:30:", "14:30:45.123456", "144:30", "14-30",
	// datetime-like
	"2024-01-15T14:30", "2024-01-15 14:30", "2024-01-15T14:30:45",
	"2024-01-15T14:30:45.123", "2024-01-15T14:30:45Z", "2024-01-15T14:30:45.123Z",
	"2024-01-15T14:30:45+05:30", "2024-01-15T14:30:45+0530", "2024-01-15T14:30:45+5:30",
	"2024-01-15T14:30:45-05:30", "2024-01-15T14:30:45+05", "2024-01-15X14:30",
	"2024-01-15T", "2024-01-15T14", "2024-01-15T14:30:45ZZ",
}

func TestScannersMatchTheirRegexes(t *testing.T) {
	for _, s := range scanInputs {
		if got, want := isDateLike(s), reDateLike.MatchString(s); got != want {
			t.Errorf("isDateLike(%q) = %v, regex says %v", s, got, want)
		}
		if got, want := isTimeLike(s), reTimeLike.MatchString(s); got != want {
			t.Errorf("isTimeLike(%q) = %v, regex says %v", s, got, want)
		}
		if got, want := isDateTimeLike(s), reDateTimeLike.MatchString(s); got != want {
			t.Errorf("isDateTimeLike(%q) = %v, regex says %v", s, got, want)
		}
		if got, want := isNumericKey(s), reKeyNumeric.MatchString(s); got != want {
			t.Errorf("isNumericKey(%q) = %v, regex says %v", s, got, want)
		}
		if got, want := isBareSafeKey(s), reKeyBareSafe.MatchString(s); got != want {
			t.Errorf("isBareSafeKey(%q) = %v, regex says %v", s, got, want)
		}
	}
}

// FuzzScannersMatchRegexes extends the pinning to arbitrary input: the
// scanners must agree with the regexes on everything, not only the table.
func FuzzScannersMatchRegexes(f *testing.F) {
	for _, s := range scanInputs {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if isDateLike(s) != reDateLike.MatchString(s) {
			t.Fatalf("isDateLike disagrees on %q", s)
		}
		if isTimeLike(s) != reTimeLike.MatchString(s) {
			t.Fatalf("isTimeLike disagrees on %q", s)
		}
		if isDateTimeLike(s) != reDateTimeLike.MatchString(s) {
			t.Fatalf("isDateTimeLike disagrees on %q", s)
		}
		if isNumericKey(s) != reKeyNumeric.MatchString(s) {
			t.Fatalf("isNumericKey disagrees on %q", s)
		}
		if isBareSafeKey(s) != reKeyBareSafe.MatchString(s) {
			t.Fatalf("isBareSafeKey disagrees on %q", s)
		}
	})
}
