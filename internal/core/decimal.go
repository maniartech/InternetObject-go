// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

import (
	"errors"
	"math/big"
	"strings"
)

// Decimal is an exact fixed-point number: a coefficient and the number of
// digits that sit after the point. `1.50m` is {150, 2}.
//
// SCALE IS PART OF THE VALUE. `1.5m` and `1.50m` have the same magnitude and
// are different values — the writer preserves the scale it read, and the
// corpus pins that. Which of the two equalities a caller wants is therefore an
// explicit choice: Equal compares magnitude, Same compares magnitude AND scale.
//
// A Decimal is IMMUTABLE. Every method returns a new value with a fresh
// coefficient and never writes through Coef, so results can never alias their
// operands. Callers must observe the same rule: do not mutate Coef in place.
//
// Do not compare two decimals with `==`. Coef is a pointer, so `==` is
// identity — two decimals read from the same text are `==`-unequal. Go cannot
// forbid it on a struct; Equal and Same are the operations that mean something.
//
// The zero value is a usable zero: `var d Decimal` reads and behaves as `0m`.
type Decimal struct {
	Coef  *big.Int
	Scale int
}

// ErrDivideByZero is returned by Quo and Rem rather than panicking: a decimal
// is data, and data divides by zero.
var ErrDivideByZero = errors.New("internet-object: decimal divide by zero")

// coef returns the coefficient, treating the zero value as zero. Every method
// goes through it, which is what makes `var d Decimal` safe everywhere.
func (d Decimal) coef() *big.Int {
	if d.Coef == nil {
		return new(big.Int)
	}
	return d.Coef
}

// NewDecimal builds a decimal from a coefficient and a scale: NewDecimal(150, 2)
// is 1.50m. A negative scale is clamped to zero — the format has no such
// spelling, so there is no value it could round-trip to.
func NewDecimal(coef int64, scale int) Decimal {
	if scale < 0 {
		scale = 0
	}
	return Decimal{Coef: big.NewInt(coef), Scale: scale}
}

// DecimalFromBig builds a decimal from a big coefficient, COPYING it so the
// caller cannot later mutate the value from the outside.
func DecimalFromBig(coef *big.Int, scale int) Decimal {
	if scale < 0 {
		scale = 0
	}
	c := new(big.Int)
	if coef != nil {
		c.Set(coef)
	}
	return Decimal{Coef: c, Scale: scale}
}

// String renders the decimal without its `m` suffix, at its own scale:
// {150, 2} is "1.50". The output is exact, never rounded, and re-parses
// through ParseDecimal to the same value.
func (d Decimal) String() string {
	c := d.coef()
	digits := new(big.Int).Abs(c).String()
	sign := ""
	if c.Sign() < 0 {
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

// Digits is the number of digits in the coefficient, with zero counting as one.
// It is NOT the precision — see Precision.
func (d Decimal) Digits() int {
	s := new(big.Int).Abs(d.coef()).String()
	return len(s)
}

// Precision is the decimal's total digit capacity, as SQL DECIMAL(p, s) counts
// it: the integer digits plus every fractional digit.
//
//	123.45m -> 5      0.05m -> 2      0m -> 1      0.0m -> 1
//
// The subtlety is `0.05m`, whose coefficient is 5: counting the coefficient's
// digits gives 1, but the value needs DECIMAL(2,2) to hold it, because scale
// alone accounts for two digits. So precision is never less than scale. The
// reference computes the same number, and io-go used to compute the
// coefficient's digit count instead — a divergence nothing in the corpus
// caught (upstream finding #25).
func (d Decimal) Precision() int {
	if n := d.Digits(); n > d.Scale {
		return n
	}
	return d.Scale
}

// Sign reports -1, 0 or +1.
func (d Decimal) Sign() int { return d.coef().Sign() }

// IsZero reports whether the value is zero at any scale: 0m, 0.0m and 0.00m
// are all zero.
func (d Decimal) IsZero() bool { return d.coef().Sign() == 0 }

// align returns the two coefficients scaled to a common scale, which is the
// larger of the two. Neither operand is modified.
func align(a, b Decimal) (av, bv *big.Int, scale int) {
	av, bv = a.coef(), b.coef()
	switch {
	case a.Scale < b.Scale:
		av = new(big.Int).Mul(av, pow10(b.Scale-a.Scale))
		return av, bv, b.Scale
	case b.Scale < a.Scale:
		bv = new(big.Int).Mul(bv, pow10(a.Scale-b.Scale))
		return av, bv, a.Scale
	}
	return av, bv, a.Scale
}

// Cmp compares two decimals by MAGNITUDE, aligning their scales first, and
// returns -1, 0 or +1. This is what min/max bounds mean: 1.5m and 1.50m
// compare equal.
func (d Decimal) Cmp(o Decimal) int {
	av, bv, _ := align(d, o)
	return av.Cmp(bv)
}

// Equal reports numeric equality: 1.5m equals 1.50m.
func (d Decimal) Equal(o Decimal) bool { return d.Cmp(o) == 0 }

// Same reports STRUCTURAL equality — the same magnitude at the same scale — so
// 1.5m and 1.50m are not the same. This is the equality the format's own value
// comparison uses, because scale is part of the value.
func (d Decimal) Same(o Decimal) bool {
	return d.Scale == o.Scale && d.coef().Cmp(o.coef()) == 0
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	return Decimal{Coef: new(big.Int).Neg(d.coef()), Scale: d.Scale}
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	return Decimal{Coef: new(big.Int).Abs(d.coef()), Scale: d.Scale}
}

// Add returns d + o at the larger of the two scales, exactly.
func (d Decimal) Add(o Decimal) Decimal {
	av, bv, scale := align(d, o)
	return Decimal{Coef: new(big.Int).Add(av, bv), Scale: scale}
}

// Sub returns d - o at the larger of the two scales, exactly.
func (d Decimal) Sub(o Decimal) Decimal {
	av, bv, scale := align(d, o)
	return Decimal{Coef: new(big.Int).Sub(av, bv), Scale: scale}
}

// Mul returns d * o at the SUM of the two scales, which is the scale that
// makes the product exact: 1.5m * 1.5m is 2.25m.
func (d Decimal) Mul(o Decimal) Decimal {
	return Decimal{
		Coef:  new(big.Int).Mul(d.coef(), o.coef()),
		Scale: d.Scale + o.Scale,
	}
}

// Quo returns d / o at the REQUESTED scale, rounded half away from zero.
//
// The scale is a parameter because division has no exact answer to inherit one
// from: 1m/3m does not terminate. Every fixed-point library that takes this
// seriously makes the caller say — SQL, big.Float's precision, and the
// shopspring package alike. (The reference instead uses the divisor's scale,
// so 1.0m/3m yields 0m, while its own RDBMS helper says the scale should be at
// least 6 and is never called; SPEC 0002 §5 records why this port does not
// copy that.)
//
// A zero divisor returns ErrDivideByZero.
func (d Decimal) Quo(o Decimal, scale int) (Decimal, error) {
	if o.IsZero() {
		return Decimal{}, ErrDivideByZero
	}
	if scale < 0 {
		scale = 0
	}
	// (d.coef / 10^d.Scale) / (o.coef / 10^o.Scale) at 10^scale is
	// d.coef * 10^(scale + o.Scale - d.Scale) / o.coef.
	num := new(big.Int).Set(d.coef())
	den := new(big.Int).Set(o.coef())
	if shift := scale + o.Scale - d.Scale; shift >= 0 {
		num.Mul(num, pow10(shift))
	} else {
		den.Mul(den, pow10(-shift))
	}
	return Decimal{Coef: quoHalfUp(num, den), Scale: scale}, nil
}

// Rem returns the remainder of d / o, truncating the quotient toward zero, so
// the result takes the SIGN OF THE DIVIDEND — Go's `%` rule, and the
// reference's. The scale is the larger of the two.
//
// A zero divisor returns ErrDivideByZero.
func (d Decimal) Rem(o Decimal) (Decimal, error) {
	if o.IsZero() {
		return Decimal{}, ErrDivideByZero
	}
	av, bv, scale := align(d, o)
	// big.Int.Rem truncates the quotient toward zero, which is exactly the
	// sign-of-the-dividend rule.
	return Decimal{Coef: new(big.Int).Rem(av, bv), Scale: scale}, nil
}

// IsMultipleOf reports whether d is an exact multiple of o. A zero divisor is
// never a multiple, rather than an error: this answers a validation question
// (`multipleOf`), where the answer for a nonsensical bound is "no".
func (d Decimal) IsMultipleOf(o Decimal) bool {
	if o.IsZero() {
		return false
	}
	av, bv, _ := align(d, o)
	return new(big.Int).Rem(av, bv).Sign() == 0
}

// Round returns d at the given scale, rounding half AWAY FROM ZERO: 0.5 rounds
// to 1 and -0.5 to -1. Rounding to a larger scale is exact (zeros are added).
func (d Decimal) Round(scale int) Decimal {
	return d.rescale(scale, roundHalfUp)
}

// Ceil returns d at the given scale, rounding toward positive infinity.
func (d Decimal) Ceil(scale int) Decimal {
	return d.rescale(scale, func(num, den *big.Int) *big.Int {
		q, r := new(big.Int).QuoRem(num, den, new(big.Int))
		if r.Sign() > 0 {
			q.Add(q, big.NewInt(1))
		}
		return q
	})
}

// Floor returns d at the given scale, rounding toward negative infinity.
func (d Decimal) Floor(scale int) Decimal {
	return d.rescale(scale, func(num, den *big.Int) *big.Int {
		q, r := new(big.Int).QuoRem(num, den, new(big.Int))
		if r.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		}
		return q
	})
}

// Rescale returns d at the given scale WITHOUT rounding, reporting false when
// that would lose a digit. Use it when silently dropping precision would be a
// bug; use Round when rounding is what you mean.
func (d Decimal) Rescale(scale int) (Decimal, bool) {
	r := d.rescale(scale, func(num, den *big.Int) *big.Int {
		return new(big.Int).Quo(num, den)
	})
	if r.Cmp(d) != 0 {
		return Decimal{}, false
	}
	return r, true
}

// rescale moves d to the given scale, using div to resolve the digits that do
// not survive. Growing the scale is exact and never calls div.
func (d Decimal) rescale(scale int, div func(num, den *big.Int) *big.Int) Decimal {
	if scale < 0 {
		scale = 0
	}
	switch {
	case scale == d.Scale:
		return Decimal{Coef: new(big.Int).Set(d.coef()), Scale: scale}
	case scale > d.Scale:
		return Decimal{Coef: new(big.Int).Mul(d.coef(), pow10(scale-d.Scale)), Scale: scale}
	default:
		return Decimal{Coef: div(d.coef(), pow10(d.Scale-scale)), Scale: scale}
	}
}

// quoHalfUp divides num by den, rounding half away from zero. Working on
// absolute values and restoring the sign is what makes 0.5 round to 1 and
// -0.5 to -1, rather than both toward positive infinity.
func quoHalfUp(num, den *big.Int) *big.Int {
	neg := (num.Sign() < 0) != (den.Sign() < 0)
	n := new(big.Int).Abs(num)
	dv := new(big.Int).Abs(den)
	q, r := new(big.Int).QuoRem(n, dv, new(big.Int))
	if r.Lsh(r, 1).Cmp(dv) >= 0 { // 2*r >= den  =>  the dropped part is >= a half
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q
}

// roundHalfUp is quoHalfUp in the shape rescale wants.
func roundHalfUp(num, den *big.Int) *big.Int { return quoHalfUp(num, den) }

// ParseDecimal reads a decimal literal WITHOUT its `m` suffix: "1.50",
// "-0.05", "123". The grammar is the tokenizer's, minus the suffix — an
// optional sign, at least one integer digit, and if a point is present at
// least one digit after it.
//
// Scale comes from the text, so "1.50" is {150, 2} and "1.5" is {15, 1}: the
// spelling a caller writes is the value they get, exactly as it is when the
// same text is parsed from a document.
func ParseDecimal(s string) (Decimal, error) {
	body := s
	neg := false
	if len(body) > 0 && (body[0] == '+' || body[0] == '-') {
		neg = body[0] == '-'
		body = body[1:]
	}
	intPart, fracPart := body, ""
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
		intPart, fracPart = body[:dot], body[dot+1:]
	}
	if !allDigits(intPart) || !allDigits(fracPart) ||
		intPart == "" || (strings.ContainsRune(body, '.') && fracPart == "") {
		return Decimal{}, errors.New("internet-object: invalid decimal " + quoteForError(s))
	}
	// SetString cannot fail here: allDigits has already established that every
	// byte is 0-9 and that there is at least one of them.
	coef, _ := new(big.Int).SetString(intPart+fracPart, 10)
	if neg {
		coef.Neg(coef)
	}
	return Decimal{Coef: coef, Scale: len(fracPart)}, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// quoteForError renders the offending text without pulling fmt into core.
func quoteForError(s string) string {
	if len(s) > 32 {
		s = s[:32] + "…"
	}
	return `"` + s + `"`
}

// Float64 returns the value as a float64, which is LOSSY for anything a float
// cannot represent — the reason this type exists. Use it for display and
// arithmetic that tolerates error, never to round-trip a value.
func (d Decimal) Float64() float64 {
	f, _ := new(big.Float).Quo(
		new(big.Float).SetInt(d.coef()),
		new(big.Float).SetInt(pow10(d.Scale)),
	).Float64()
	return f
}

// pow10 returns 10^n as a big.Int. Small exponents — every scale a real
// document uses — come from a table rather than an Exp call.
func pow10(n int) *big.Int {
	if n >= 0 && n < len(pow10Table) {
		return pow10Table[n]
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// pow10Table holds 10^0 … 10^31. These are SHARED and must never be mutated:
// every use above is a read (Mul, Quo, Cmp write to their receiver, not their
// arguments).
var pow10Table = func() [32]*big.Int {
	var t [32]*big.Int
	for i := range t {
		t[i] = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(i)), nil)
	}
	return t
}()
