// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

import (
	"math/big"
)

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
