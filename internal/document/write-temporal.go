package document

import (
	"time"
)

// Writing TEMPORALS: date, time and datetime literals.

func appendTemporal(dst []byte, t time.Time, declared string) []byte {
	u := t.UTC()
	// The SCHEMA names the spelling when the member declares one — the normal
	// case in a schema-first format, and it loses nothing. An undeclared
	// temporal is spelled from what its instant evidences, the same
	// normalization the writer applies to a string's open/raw/quoted form.
	kind := declared
	if kind == "" {
		kind = InferTemporalKind(u)
	}
	switch kind {
	case "date":
		dst = append(dst, 'd', '"')
		dst = u.AppendFormat(dst, "2006-01-02")
	case "time":
		dst = append(dst, 't', '"')
		if u.Nanosecond() != 0 {
			dst = u.AppendFormat(dst, "15:04:05.000")
		} else {
			dst = u.AppendFormat(dst, "15:04:05")
		}
	default:
		dst = append(dst, 'd', 't', '"')
		dst = u.AppendFormat(dst, "2006-01-02T15:04:05.000Z")
	}
	return append(dst, '"')
}

// InferTemporalKind reports the kind an instant EVIDENCES, for a host value
// that genuinely carries none: the 1900-01-01 sentinel date is a time, an
// all-zero clock is a date, anything else a datetime. Our own model always
// carries a kind, so the writer never needs this — it is kept for callers
// converting from a kindless source (PORTING-NOTES rule 15).
func InferTemporalKind(u time.Time) string {
	y, mo, day := u.Date()
	h, mi, sec := u.Clock()
	switch {
	case y == 1900 && mo == 1 && day == 1:
		return "time"
	case h == 0 && mi == 0 && sec == 0 && u.Nanosecond() == 0:
		return "date"
	}
	return "datetime"
}

// temporalLiteral renders a temporal value under the declared kind, or the
// kind the value itself evidences when none is declared: the 1900-01-01
// sentinel date is a time, an all-zero time is a date, anything else a
// datetime.
func temporalLiteral(t time.Time, declared string) string {
	return string(appendTemporal(nil, t, declared))
}

// ── strings and keys ───────────────────────────────────────────────────────

// Hot-path classification is table- and scanner-based, never regexp: these
// run on every string and every key written, and Go's RE2 engine allocates
// match state per call (ADR 0006 P5). Each function is pinned to the regex it
// replaced by a table-driven equivalence test in write_scan_test.go.

// bareSafeKeyByte is the `[A-Za-z0-9_. -]` continuation set of the bare-key
// grammar; the first byte additionally allows `$` but never a digit.
