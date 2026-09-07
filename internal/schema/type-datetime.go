package schema

import (
	"time"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// temporalTypedef is the memberdef schema a typedef of this type must satisfy.
//
// datetime, date and time.
//
// ORDER IS THE CONTRACT: it is what binds the positional form.
var temporalTypedef = []typedefMember{
	{"type", "string"}, {"default", "self"}, {"choices", "[self]"},
	{"min", "self"}, {"max", "self"},
	{"optional", "bool"}, {"null", "bool"},
}

func validateTemporal(val any, md *MemberDef, defs Defs) any {
	expected := errs.ExpectedDateTime
	switch md.Type {
	case "date":
		expected = errs.ExpectedDate
	case "time":
		expected = errs.ExpectedTime
	}
	t, ok := val.(time.Time)
	if !ok {
		// the three temporal kinds are interchangeable at the type check;
		// anything else — including a deferred malformed literal — is not
		vfail(expected)
	}
	// The declared type does NOT truncate the VALUE. `validation/temporal-depth.io`
	// pins both directions — a date under `time` keeps its 2024 date, a time under
	// `date` keeps its 12:00 clock — so the instant survives the annotation and the
	// declared kind decides only the SPELLING, at the writer. Truncating here fails
	// those two cases; measured 2026-09-03, do not retry without changing the corpus.
	//
	// The declared type DOES scope the comparison, which is a different question and
	// was decided separately (FINDINGS #22, 2026-09-03). See compareAs.
	bound := func(key string) (time.Time, bool) {
		v, ok := md.Constraints[key]
		if !ok {
			return time.Time{}, false
		}
		tt, ok := resolveRef(v, defs).(time.Time)
		if !ok {
			vfail(errs.ExpectedDateTime)
		}
		return tt, true
	}
	tv := compareAs(t, md.Type)
	if m, ok := bound("min"); ok && tv < compareAs(m, md.Type) {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && tv > compareAs(m, md.Type) {
		vfail(errs.MismatchedMax)
	}
	return val // the original box; see validateString
}

// compareAs reduces a temporal to the part its DECLARED type governs, so that
// `min`/`max` compare like with like: dates against dates, clocks against
// clocks, and whole instants only where the member is a `datetime`.
//
// The alternative — comparing raw instants regardless of annotation — is what
// the reference does, and it produces two outcomes the format cannot justify:
// a `{date, max: d"2024-03-20"}` rejects `dt"2024-03-20T14:30Z"` even though
// its DATE is exactly the bound and the same schema will discard that clock on
// output; and a `{time, max: t"15:00:00"}` rejects a 2024 datetime of 14:30 —
// not because 14:30 is late, but because a bare time-of-day is anchored at
// 1900-01-01 and 2024 is after 1900, which is not a meaningful comparison.
//
// The format had already decided that the annotation governs precision: any
// temporal is permitted under any annotation, and the writer truncates to the
// declared kind. Comparison was the one place that rule was not applied.
// Scoping it here makes the three operations agree.
//
// FINDINGS #22, decided by the format's owner 2026-09-03; a DELIBERATE
// divergence from io-js2 until the other ports follow. No corpus
// case covers a bound across annotations, so nothing pins the old behavior —
// see temporal_test.go, which pins this one.
func compareAs(t time.Time, declared string) int64 {
	u := t.UTC()
	switch declared {
	case "date":
		y, m, d := u.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).UnixMilli()
	case "time":
		return int64(u.Hour())*3600000 + int64(u.Minute())*60000 +
			int64(u.Second())*1000 + int64(u.Nanosecond())/1e6
	}
	return u.UnixMilli()
}
