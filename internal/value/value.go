// Package value is the Internet Object value model: the concrete Go types a
// parsed document decodes into, the ONE predicate that separates value types
// from records, and the corpus's structural equality.
//
// The closed set of value types:
//
//	nil        null
//	bool       boolean
//	float64    number (including NaN and ±Inf)
//	*big.Int   bigint — every digit past 2^53 survives
//	Decimal    decimal — scale is part of the value (1.50m ≠ 1.5m)
//	string     string
//	[]byte     binary
//	time.Time  date | time | datetime — one value, three spellings
//	*Object    an ordered key/value record
//	[]any      an array
package value

import (
	"math/big"
	"time"
)

// Object is an ordered collection of members. Order is preserved end to end;
// comparison ignores it (see Equal), serialization does not.
type Object struct {
	Members []Member
	// Line, Col locate the object's first token, 1-based. An absence fault
	// (a member that is missing entirely) has no value to point at and is
	// reported here instead — the reference does the same (ADR 0005 D2).
	Line, Col int32
}

// Member is one object member. A positional member has no key of its own —
// its identity is its index.
type Member struct {
	Key        string
	Quoted     bool // the key (or positional string value) was written quoted
	Positional bool // no key was written
	Absent     bool // an empty comma slot: a positional hole with no value
	Value      any
	// Line, Col locate this member's VALUE, 1-based — where a validation
	// fault about it is reported. Zero when the member was not parsed from
	// source (built by a marshaler, or an empty comma slot).
	Line, Col int32
}

// Find returns the index of the first keyed member with the given key, or -1.
func (o *Object) Find(key string) int {
	for i := range o.Members {
		if !o.Members[i].Positional && o.Members[i].Key == key {
			return i
		}
	}
	return -1
}

// Decimal is an exact fixed-point value: Coef × 10^-Scale. The scale is part
// of the value — 1.50m is coefficient 150, scale 2, and is distinct from
// 1.5m in serialized form.
//
// DECIDED 2026-09-03, and not to be revisited without new evidence: this is
// the one carrier type in the value model, because Go has no decimal to be
// native to. `big.Float` is binary, so it cannot hold 0.1 exactly; `big.Rat`
// has no scale, so 1.50m and 1.5m would become the same value and the
// trailing zero would be lost on write. Everything else in the model is the
// Go type a developer would have chosen anyway — string, bool, float64,
// *big.Int, []byte, nil, []any — and Temporal embeds time.Time.
type Decimal struct {
	Coef  *big.Int
	Scale int
}

// String renders the decimal's exact digits at its scale — the canonical
// spelling, shared by the writer and the corpus comparator.
func (d Decimal) String() string {
	if d.Coef == nil {
		// The zero value of the struct is a legitimate value a caller can
		// hold (`var d Decimal`); it reads as zero at its scale rather than
		// panicking on the nil coefficient.
		d.Coef = new(big.Int)
	}
	digits := new(big.Int).Abs(d.Coef).String()
	sign := ""
	if d.Coef.Sign() < 0 {
		sign = "-"
	}
	if d.Scale == 0 {
		return sign + digits
	}
	for len(digits) <= d.Scale {
		digits = "0" + digits
	}
	cut := len(digits) - d.Scale
	return sign + digits[:cut] + "." + digits[cut:]
}

// A temporal value is a plain time.Time.
//
// DECIDED 2026-09-03: the three literals — `d"…"`, `t"…"`, `dt"…"` — are
// SPELLINGS of one value, not three types. Validation says so itself ("the
// three temporal kinds are interchangeable at the type check"), and the
// corpus comparator compares temporals by instant with the kind ignored. So
// the kind belongs with the other presentational facts the value model does
// not keep: an open string, a raw string and a quoted string all decode to
// one Go string too, and the writer re-picks the leanest spelling on output.
//
// On write the kind comes from the schema when the member declares one —
// which, in a schema-first format, is the normal case and loses nothing. For
// an undeclared temporal the writer infers from the instant, exactly as the
// reference does. io-test-cases PORTING-NOTES rule 15 asks a KINDED host
// (Rust's Temporal, Python's date/time/datetime) to keep its kind; Go's
// standard library has one temporal type, so this is not one, and the rule's
// own scope excludes it. Recorded as a deliberate divergence in FINDINGS.
//
// TimeAnchor is the date a time-of-day carries: the format has no bare clock,
// so `t"14:30"` is this date at that clock — the reference's convention, and
// what makes two implementations agree on the instant.
var TimeAnchor = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// ErrorNode marks a record that failed to parse or validate inside a
// collection: the fault was reported and the surrounding records survived.
type ErrorNode struct {
	Code        string
	Category    string
	Path        string
	RecordIndex int
	Line, Col   int32
}

// ErrorValue is a malformed VALUE literal (a bad datetime, bigint, decimal or
// number) whose error is deferred rather than fatal: parsing continues, and
// the fault surfaces either as the recorded code (no schema) or as the typed
// member's own expected-* code (a schema masks it — reference behavior,
// io-test-cases ISSUE-23).
type ErrorValue struct {
	Code string
	Line int32
	Col  int32
}

// IsScalar is THE record-versus-value decision, made once. It lists the value
// types explicitly and lets "record" be what is left, so the next value type
// added to the format extends this list instead of being silently walked as a
// record (the highest-yield trap in PORTING-NOTES).
func IsScalar(v any) bool {
	switch v.(type) {
	case nil, bool, float64, string, *big.Int, Decimal, []byte, time.Time:
		return true
	}
	return false
}
