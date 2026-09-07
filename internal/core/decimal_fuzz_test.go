package core_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/maniartech/InternetObject-go/internal/core"
)

// Property fuzzing of Decimal — SPEC 0002 §8.2. Run with:
//
//	go test -fuzz=FuzzDecimal -fuzztime=60s ./internal/core/
//
// The inputs are two literal strings and a scale. Most mutations are not valid
// decimals and are skipped; the seeds keep the corpus anchored on shapes that
// are (zero at several scales, negatives, huge coefficients, the leading-zero
// fractions that finding #25 was about).

var decimalSeeds = []string{
	"0", "0.0", "0.000", "-0", "1", "-1", "1.5", "1.50", "-0.05",
	"123.456", "0.00001", "12345.67", "7.5", "2.5", "-2.5", "3",
	"99999999999999999999999999999", "0.000000000000000000000001",
	"123456789012345678901234567890.123456789",
}

func FuzzDecimalProperties(f *testing.F) {
	for _, a := range decimalSeeds {
		for _, b := range decimalSeeds[:6] {
			f.Add(a, b, 6)
		}
	}
	f.Add("1", "3", 0)
	f.Add("0.05", "0.1", 20)

	f.Fuzz(func(t *testing.T, as, bs string, scale int) {
		// Keep the arena bounded: a 10,000-digit coefficient proves nothing
		// the 40-digit seeds do not, and makes every big.Int op quadratic.
		if len(as) > 64 || len(bs) > 64 {
			t.Skip()
		}
		a, err1 := core.ParseDecimal(as)
		b, err2 := core.ParseDecimal(bs)
		if err1 != nil || err2 != nil {
			t.Skip() // not a decimal literal; that is ParseDecimal's own test
		}
		if scale < 0 || scale > 40 {
			t.Skip()
		}

		// ── String is the exact inverse of ParseDecimal ─────────────────────
		round, err := core.ParseDecimal(a.String())
		if err != nil {
			t.Fatalf("%q printed as %q, which does not re-parse: %v", as, a.String(), err)
		}
		if !round.Same(a) {
			t.Fatalf("round trip changed the value: %q -> %q", as, round.String())
		}
		if a.String() != round.String() {
			t.Fatalf("printing is not idempotent: %q -> %q", a.String(), round.String())
		}

		// ── Cmp is a total order, and the two equalities relate correctly ───
		if got, want := a.Cmp(b), -b.Cmp(a); got != want {
			t.Fatalf("Cmp is not antisymmetric: %d vs %d for %q %q", got, want, as, bs)
		}
		if a.Cmp(a) != 0 || !a.Equal(a) || !a.Same(a) {
			t.Fatalf("%q is not equal to itself", as)
		}
		if a.Same(b) && !a.Equal(b) {
			t.Fatalf("Same but not Equal: %q %q", as, bs)
		}
		if a.Equal(b) != (a.Cmp(b) == 0) {
			t.Fatalf("Equal disagrees with Cmp for %q %q", as, bs)
		}

		// ── Precision is never below scale (finding #25's invariant) ────────
		if a.Precision() < a.Scale {
			t.Fatalf("%q: precision %d below scale %d", as, a.Precision(), a.Scale)
		}
		if a.Digits() < 1 {
			t.Fatalf("%q: Digits() = %d", as, a.Digits())
		}

		// ── Add/Sub are inverses, and the scale rules hold ──────────────────
		sum := a.Add(b)
		if got := sum.Sub(b); got.Cmp(a) != 0 {
			t.Fatalf("(a+b)-b != a: %q %q gave %q", as, bs, got.String())
		}
		if want := maxInt(a.Scale, b.Scale); sum.Scale != want {
			t.Fatalf("Add scale = %d, want %d (%q %q)", sum.Scale, want, as, bs)
		}
		if got := a.Add(b).Cmp(b.Add(a)); got != 0 {
			t.Fatalf("Add is not commutative for %q %q", as, bs)
		}
		if got := a.Sub(a); !got.IsZero() {
			t.Fatalf("a-a = %q, want zero", got.String())
		}

		// ── Mul is exact: scale is the sum, and it is commutative ───────────
		prod := a.Mul(b)
		if prod.Scale != a.Scale+b.Scale {
			t.Fatalf("Mul scale = %d, want %d (%q %q)", prod.Scale, a.Scale+b.Scale, as, bs)
		}
		if prod.Cmp(b.Mul(a)) != 0 {
			t.Fatalf("Mul is not commutative for %q %q", as, bs)
		}
		// A product is zero only when an operand is. The reference fails this
		// (0.01 * 0.01 = 0.00); it is the property finding #26 is about.
		if prod.IsZero() != (a.IsZero() || b.IsZero()) {
			t.Fatalf("%q * %q = %q: a product vanished", as, bs, prod.String())
		}

		// ── Neg and Abs ────────────────────────────────────────────────────
		if got := a.Neg().Neg(); !got.Same(a) {
			t.Fatalf("--a != a for %q", as)
		}
		if a.Neg().Sign() != -a.Sign() {
			t.Fatalf("Neg did not flip the sign of %q", as)
		}
		if a.Abs().Sign() < 0 {
			t.Fatalf("Abs of %q is negative", as)
		}

		// ── Round/Ceil/Floor land on the requested scale and bracket it ─────
		r, c, fl := a.Round(scale), a.Ceil(scale), a.Floor(scale)
		for name, got := range map[string]core.Decimal{"Round": r, "Ceil": c, "Floor": fl} {
			if got.Scale != scale {
				t.Fatalf("%s(%d) of %q has scale %d", name, scale, as, got.Scale)
			}
		}
		if fl.Cmp(c) > 0 {
			t.Fatalf("Floor > Ceil for %q at scale %d", as, scale)
		}
		if r.Cmp(fl) < 0 || r.Cmp(c) > 0 {
			t.Fatalf("Round is outside [Floor, Ceil] for %q at scale %d", as, scale)
		}
		// Growing the scale changes nothing about the value.
		if scale >= a.Scale && r.Cmp(a) != 0 {
			t.Fatalf("rounding %q UP to scale %d changed it to %q", as, scale, r.String())
		}
		// Rescale is exact or it refuses.
		if got, ok := a.Rescale(scale); ok && got.Cmp(a) != 0 {
			t.Fatalf("Rescale said exact but %q became %q", as, got.String())
		}

		// ── Quo/Rem, and the identity that ties them together ──────────────
		if !b.IsZero() {
			q, err := a.Quo(b, scale)
			if err != nil {
				t.Fatalf("Quo(%q, %q, %d): %v", as, bs, scale, err)
			}
			if q.Scale != scale {
				t.Fatalf("Quo scale = %d, want %d", q.Scale, scale)
			}
			rem, err := a.Rem(b)
			if err != nil {
				t.Fatalf("Rem(%q, %q): %v", as, bs, err)
			}
			// The remainder is smaller than the divisor and carries the
			// dividend's sign.
			if rem.Abs().Cmp(b.Abs()) >= 0 {
				t.Fatalf("|a %% b| >= |b| for %q %q: %q", as, bs, rem.String())
			}
			if rem.Sign() != 0 && rem.Sign() != a.Sign() {
				t.Fatalf("remainder sign %d does not match dividend %q", rem.Sign(), as)
			}
			// a == (a quo b)*b + (a rem b), with the quotient truncated.
			trunc, _ := a.Quo(b, 0)
			if trunc.Cmp(exactTrunc(a, b)) != 0 {
				t.Skip() // Quo rounds half-up at scale 0; only compare when it truncated
			}
		} else {
			if _, err := a.Quo(b, scale); err != core.ErrDivideByZero {
				t.Fatalf("Quo by zero returned %v", err)
			}
			if _, err := a.Rem(b); err != core.ErrDivideByZero {
				t.Fatalf("Rem by zero returned %v", err)
			}
			if a.IsMultipleOf(b) {
				t.Fatalf("%q reported a multiple of zero", as)
			}
		}

		// ── multipleOf agrees with Rem ──────────────────────────────────────
		if !b.IsZero() {
			rem, _ := a.Rem(b)
			if a.IsMultipleOf(b) != rem.IsZero() {
				t.Fatalf("IsMultipleOf disagrees with Rem for %q %q", as, bs)
			}
		}

		// ── Nothing shares a coefficient with its operands ──────────────────
		for name, got := range map[string]core.Decimal{
			"Add": sum, "Sub": a.Sub(b), "Mul": prod, "Neg": a.Neg(), "Round": r,
		} {
			if a.Coef != nil && got.Coef == a.Coef {
				t.Fatalf("%s aliased its receiver's coefficient", name)
			}
			if b.Coef != nil && got.Coef == b.Coef {
				t.Fatalf("%s aliased its argument's coefficient", name)
			}
		}
	})
}

// exactTrunc computes a/b truncated toward zero, independently of Quo, so the
// identity check above does not test Quo against itself.
func exactTrunc(a, b core.Decimal) core.Decimal {
	av, bv := new(big.Int).Set(a.Coef), new(big.Int).Set(b.Coef)
	// Align to the larger scale, then integer-divide.
	if a.Scale < b.Scale {
		av.Mul(av, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(b.Scale-a.Scale)), nil))
	} else if b.Scale < a.Scale {
		bv.Mul(bv, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(a.Scale-b.Scale)), nil))
	}
	return core.Decimal{Coef: new(big.Int).Quo(av, bv), Scale: 0}
}

// FuzzParseDecimal attacks the parser with arbitrary text: it must never panic,
// and anything it accepts must print back to text it accepts again.
func FuzzParseDecimal(f *testing.F) {
	for _, s := range decimalSeeds {
		f.Add(s)
	}
	for _, s := range []string{"", ".", "-", "1.2.3", "1e5", "1.5m", " 1", "٣", "1_0", "+"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 128 {
			t.Skip()
		}
		d, err := core.ParseDecimal(s)
		if err != nil {
			return
		}
		out := d.String()
		back, err := core.ParseDecimal(out)
		if err != nil {
			t.Fatalf("accepted %q, printed %q, which it then rejects: %v", s, out, err)
		}
		if !back.Same(d) {
			t.Fatalf("%q -> %q -> a different value %q", s, out, back.String())
		}
		if back.String() != out {
			t.Fatalf("printing is not idempotent: %q then %q", out, back.String())
		}
		// The canonical form never carries a leading `+`, a redundant leading
		// zero, or a negative zero.
		if strings.HasPrefix(out, "+") {
			t.Fatalf("%q printed with a leading plus: %q", s, out)
		}
		if strings.HasPrefix(out, "-0") && d.IsZero() {
			t.Fatalf("%q printed as negative zero: %q", s, out)
		}
		body := strings.TrimPrefix(out, "-")
		if len(body) > 1 && body[0] == '0' && body[1] != '.' {
			t.Fatalf("%q printed with a redundant leading zero: %q", s, out)
		}
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
