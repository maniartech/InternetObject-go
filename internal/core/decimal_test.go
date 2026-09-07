package core_test

import (
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/maniartech/InternetObject-go/internal/core"
)

// dec is ParseDecimal for a literal the test author knows is valid.
func dec(t *testing.T, s string) core.Decimal {
	t.Helper()
	d, err := core.ParseDecimal(s)
	if err != nil {
		t.Fatalf("ParseDecimal(%q): %v", s, err)
	}
	return d
}

func TestParseDecimalKeepsTheSpelledScale(t *testing.T) {
	for _, tc := range []struct {
		in    string
		coef  int64
		scale int
	}{
		{"0", 0, 0}, {"123", 123, 0}, {"1.5", 15, 1}, {"1.50", 150, 2},
		{"-0.05", -5, 2}, {"+1.5", 15, 1}, {"0.000", 0, 3},
		{"007.50", 750, 2}, {"-0", 0, 0},
	} {
		d := dec(t, tc.in)
		if d.Coef.Int64() != tc.coef || d.Scale != tc.scale {
			t.Errorf("ParseDecimal(%q) = {%v, %d}, want {%d, %d}",
				tc.in, d.Coef, d.Scale, tc.coef, tc.scale)
		}
	}
}

func TestParseDecimalRejectsWhatTheParserRejects(t *testing.T) {
	// The same shapes the tokenizer calls invalid-decimal, plus the ones a
	// string API invites: empty, spaces, exponents, stray suffixes.
	for _, in := range []string{
		"", "-", "+", ".", ".5", "123.", "1.2.3", "1e5", "1.5m", " 1.5",
		"1.5 ", "abc", "1_000", "0x10", "--1", "1..2",
	} {
		if d, err := core.ParseDecimal(in); err == nil {
			t.Errorf("ParseDecimal(%q) accepted it as %v", in, d)
		}
	}
}

// String is the inverse of ParseDecimal, exactly - scale included.
func TestStringRoundTrips(t *testing.T) {
	for _, in := range []string{"0", "123", "1.5", "1.50", "-0.05", "0.000", "-123.456"} {
		if got := dec(t, in).String(); got != in {
			t.Errorf("ParseDecimal(%q).String() = %q", in, got)
		}
	}
	// Spellings that are not canonical come back canonical, and stay stable.
	for in, want := range map[string]string{"007.50": "7.50", "+1.5": "1.5", "-0": "0"} {
		if got := dec(t, in).String(); got != want {
			t.Errorf("ParseDecimal(%q).String() = %q, want %q", in, got, want)
		}
	}
}

// Precision is SQL DECIMAL(p, s), not the coefficient's digit count: 0.05
// needs DECIMAL(2,2) because scale alone accounts for two digits. io-go used
// to answer 1 here and the corpus never noticed (upstream finding #25).
func TestPrecisionCountsFractionalDigits(t *testing.T) {
	for _, tc := range []struct {
		in                string
		digits, precision int
	}{
		{"0", 1, 1}, {"0.0", 1, 1}, {"0.05", 1, 2}, {"0.050", 2, 3},
		{"0.5", 1, 1}, {"123.45", 5, 5}, {"123", 3, 3}, {"-0.05", 1, 2},
		{"12345.67", 7, 7}, {"0.00001", 1, 5},
	} {
		d := dec(t, tc.in)
		if got := d.Digits(); got != tc.digits {
			t.Errorf("%s: Digits() = %d, want %d", tc.in, got, tc.digits)
		}
		if got := d.Precision(); got != tc.precision {
			t.Errorf("%s: Precision() = %d, want %d", tc.in, got, tc.precision)
		}
		if d.Precision() < d.Scale {
			t.Errorf("%s: precision %d is below scale %d", tc.in, d.Precision(), d.Scale)
		}
	}
}

// Two equalities, named apart, because scale is part of the value.
func TestEqualComparesMagnitudeAndSameComparesScaleToo(t *testing.T) {
	a, b := dec(t, "1.5"), dec(t, "1.50")
	if !a.Equal(b) || !b.Equal(a) {
		t.Error("1.5 and 1.50 must be numerically Equal")
	}
	if a.Same(b) || b.Same(a) {
		t.Error("1.5 and 1.50 must not be Same: the scale differs")
	}
	if !a.Same(dec(t, "1.5")) {
		t.Error("1.5 is not Same as itself")
	}
	if a.Cmp(b) != 0 {
		t.Errorf("Cmp = %d, want 0", a.Cmp(b))
	}
	for _, tc := range []struct{ x, y string }{
		{"1", "2"}, {"-1", "1"}, {"0.09", "0.1"}, {"-2", "-1"}, {"9.99", "10"},
	} {
		x, y := dec(t, tc.x), dec(t, tc.y)
		if x.Cmp(y) != -1 || y.Cmp(x) != 1 {
			t.Errorf("Cmp(%s, %s) = %d / %d, want -1 / 1", tc.x, tc.y, x.Cmp(y), y.Cmp(x))
		}
	}
	// Zero at any scale is zero.
	for _, z := range []string{"0", "0.0", "0.000", "-0"} {
		if d := dec(t, z); !d.IsZero() || d.Sign() != 0 {
			t.Errorf("%s: IsZero=%v Sign=%d", z, d.IsZero(), d.Sign())
		}
	}
}

func TestArithmeticScaleRules(t *testing.T) {
	for _, tc := range []struct{ a, op, b, want string }{
		// Add and Sub take the LARGER scale.
		{"1.5", "+", "1.50", "3.00"},
		{"0.1", "+", "0.2", "0.3"},
		{"1", "+", "0.005", "1.005"},
		{"1.5", "-", "1.50", "0.00"},
		{"0.3", "-", "0.1", "0.2"},
		{"1", "-", "2", "-1"},
		// Mul takes the SUM of the scales, which keeps it exact.
		{"1.5", "*", "1.5", "2.25"},
		{"1.50", "*", "2", "3.00"},
		{"-1.5", "*", "2.00", "-3.000"},
		{"0", "*", "1.23", "0.00"},
	} {
		a, b := dec(t, tc.a), dec(t, tc.b)
		var got core.Decimal
		switch tc.op {
		case "+":
			got = a.Add(b)
		case "-":
			got = a.Sub(b)
		case "*":
			got = a.Mul(b)
		}
		if got.String() != tc.want {
			t.Errorf("%s %s %s = %s, want %s", tc.a, tc.op, tc.b, got.String(), tc.want)
		}
	}
}

func TestQuoRoundsHalfAwayFromZeroAtTheRequestedScale(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		scale int
		want  string
	}{
		{"1", "3", 6, "0.333333"},
		{"2", "3", 6, "0.666667"}, // half away from zero, not truncation
		{"1.0", "3", 0, "0"},      // the reference would answer 0 here too, by accident
		{"10", "4", 1, "2.5"},
		{"10", "4", 0, "3"},   // 2.5 -> 3, away from zero
		{"-10", "4", 0, "-3"}, // -2.5 -> -3, away from zero (not -2)
		{"1", "8", 3, "0.125"},
		{"100", "3", 2, "33.33"},
		{"-1", "3", 4, "-0.3333"},
		{"6", "3", 2, "2.00"},
	} {
		got, err := dec(t, tc.a).Quo(dec(t, tc.b), tc.scale)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.a, tc.b, err)
		}
		if got.String() != tc.want {
			t.Errorf("%s / %s at scale %d = %s, want %s", tc.a, tc.b, tc.scale, got.String(), tc.want)
		}
		if got.Scale != tc.scale {
			t.Errorf("%s / %s: scale = %d, want %d", tc.a, tc.b, got.Scale, tc.scale)
		}
	}
}

// The remainder takes the sign of the DIVIDEND, as Go's % does.
func TestRemTakesTheSignOfTheDividend(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		{"7", "3", "1"}, {"-7", "3", "-1"}, {"7", "-3", "1"}, {"-7", "-3", "-1"},
		{"7.5", "2", "1.5"}, {"1.00", "0.3", "0.10"}, {"6", "3", "0"},
	} {
		got, err := dec(t, tc.a).Rem(dec(t, tc.b))
		if err != nil {
			t.Fatalf("%s %% %s: %v", tc.a, tc.b, err)
		}
		if got.String() != tc.want {
			t.Errorf("%s %% %s = %s, want %s", tc.a, tc.b, got.String(), tc.want)
		}
	}
}

func TestDivideByZeroIsAnErrorNotAPanic(t *testing.T) {
	one := dec(t, "1")
	for _, z := range []string{"0", "0.00"} {
		if _, err := one.Quo(dec(t, z), 2); err != core.ErrDivideByZero {
			t.Errorf("Quo by %s: err = %v, want ErrDivideByZero", z, err)
		}
		if _, err := one.Rem(dec(t, z)); err != core.ErrDivideByZero {
			t.Errorf("Rem by %s: err = %v, want ErrDivideByZero", z, err)
		}
	}
	// multipleOf answers a validation question, so a zero bound is "no",
	// not an error.
	if one.IsMultipleOf(dec(t, "0")) {
		t.Error("everything is a multiple of zero?")
	}
}

func TestRoundCeilFloorAndRescale(t *testing.T) {
	for _, tc := range []struct {
		in                 string
		scale              int
		round, ceil, floor string
		rescaleOK          bool
	}{
		{"1.55", 1, "1.6", "1.6", "1.5", false},
		{"1.54", 1, "1.5", "1.6", "1.5", false},
		{"-1.55", 1, "-1.6", "-1.5", "-1.6", false},
		{"2.5", 0, "3", "3", "2", false},
		{"-2.5", 0, "-3", "-2", "-3", false},
		{"1.50", 1, "1.5", "1.5", "1.5", true},
		{"1.5", 3, "1.500", "1.500", "1.500", true},
		{"0.05", 1, "0.1", "0.1", "0.0", false},
	} {
		d := dec(t, tc.in)
		if got := d.Round(tc.scale); got.String() != tc.round {
			t.Errorf("%s.Round(%d) = %s, want %s", tc.in, tc.scale, got.String(), tc.round)
		}
		if got := d.Ceil(tc.scale); got.String() != tc.ceil {
			t.Errorf("%s.Ceil(%d) = %s, want %s", tc.in, tc.scale, got.String(), tc.ceil)
		}
		if got := d.Floor(tc.scale); got.String() != tc.floor {
			t.Errorf("%s.Floor(%d) = %s, want %s", tc.in, tc.scale, got.String(), tc.floor)
		}
		// Rescale never rounds silently: it reports rather than lose a digit.
		got, ok := d.Rescale(tc.scale)
		if ok != tc.rescaleOK {
			t.Errorf("%s.Rescale(%d) ok = %v, want %v", tc.in, tc.scale, ok, tc.rescaleOK)
		}
		if ok && got.Cmp(d) != 0 {
			t.Errorf("%s.Rescale(%d) = %s, which is a different value", tc.in, tc.scale, got.String())
		}
	}
}

func TestIsMultipleOfAlignsScales(t *testing.T) {
	for _, tc := range []struct {
		d, m string
		want bool
	}{
		{"15", "5", true}, {"15", "5.0", true}, {"15.00", "5", true},
		{"16", "5", false}, {"0.30", "0.1", true}, {"0.35", "0.1", false},
		{"-15", "5", true}, {"0", "5", true},
	} {
		if got := dec(t, tc.d).IsMultipleOf(dec(t, tc.m)); got != tc.want {
			t.Errorf("%s multipleOf %s = %v, want %v", tc.d, tc.m, got, tc.want)
		}
	}
}

func TestNegAbsAndFloat64(t *testing.T) {
	d := dec(t, "-1.50")
	if got := d.Neg().String(); got != "1.50" {
		t.Errorf("Neg = %s", got)
	}
	if got := d.Abs().String(); got != "1.50" {
		t.Errorf("Abs = %s", got)
	}
	if got := dec(t, "1.50").Neg().String(); got != "-1.50" {
		t.Errorf("Neg of a positive = %s", got)
	}
	if got := dec(t, "1.25").Float64(); got != 1.25 {
		t.Errorf("Float64 = %v", got)
	}
	if got := dec(t, "-0.05").Float64(); got != -0.05 {
		t.Errorf("Float64 = %v", got)
	}
}

// `var d Decimal` is a legitimate value a caller can hold, and every operation
// must treat it as zero rather than dereference a nil coefficient.
func TestZeroValueIsUsableEverywhere(t *testing.T) {
	var z core.Decimal
	one := dec(t, "1")

	if z.String() != "0" || !z.IsZero() || z.Sign() != 0 {
		t.Errorf("zero value: %q sign=%d", z.String(), z.Sign())
	}
	if z.Digits() != 1 || z.Precision() != 1 {
		t.Errorf("zero value: digits=%d precision=%d", z.Digits(), z.Precision())
	}
	if !z.Equal(dec(t, "0")) || !z.Same(dec(t, "0")) {
		t.Error("zero value is not equal to 0")
	}
	if got := z.Add(one).String(); got != "1" {
		t.Errorf("0 + 1 = %s", got)
	}
	if got := one.Add(z).String(); got != "1" {
		t.Errorf("1 + 0 = %s", got)
	}
	if got := z.Sub(one).String(); got != "-1" {
		t.Errorf("0 - 1 = %s", got)
	}
	if got := z.Mul(one).String(); got != "0" {
		t.Errorf("0 * 1 = %s", got)
	}
	if got := z.Neg().String(); got != "0" {
		t.Errorf("-0 = %s", got)
	}
	if got := z.Round(2).String(); got != "0.00" {
		t.Errorf("round = %s", got)
	}
	if got, err := z.Quo(one, 2); err != nil || got.String() != "0.00" {
		t.Errorf("0/1 = %v, %v", got, err)
	}
	if _, err := one.Quo(z, 2); err != core.ErrDivideByZero {
		t.Errorf("1/zero-value: %v", err)
	}
	if z.Cmp(one) != -1 || one.Cmp(z) != 1 {
		t.Error("zero value does not compare")
	}
	if z.Float64() != 0 {
		t.Error("zero value Float64")
	}
}

// A result must never share a coefficient with its operands, or a later
// in-place big.Int operation on one would silently change the other. This is
// the one way the immutability invariant can be violated, so it is checked
// directly rather than assumed.
func TestResultsNeverAliasTheirOperands(t *testing.T) {
	mk := func(s string) core.Decimal { return dec(t, s) }

	ops := map[string]func(a, b core.Decimal) core.Decimal{
		"Add": func(a, b core.Decimal) core.Decimal { return a.Add(b) },
		"Sub": func(a, b core.Decimal) core.Decimal { return a.Sub(b) },
		"Mul": func(a, b core.Decimal) core.Decimal { return a.Mul(b) },
		"Quo": func(a, b core.Decimal) core.Decimal { r, _ := a.Quo(b, 4); return r },
		"Rem": func(a, b core.Decimal) core.Decimal { r, _ := a.Rem(b); return r },
	}
	for name, op := range ops {
		a, b := mk("7.5"), mk("2.5")
		got := op(a, b)
		if got.Coef == a.Coef || got.Coef == b.Coef {
			t.Errorf("%s: the result shares a coefficient with an operand", name)
		}
		// Mutating an operand afterwards must not move the result.
		before := got.String()
		a.Coef.SetInt64(999999)
		b.Coef.SetInt64(888888)
		if got.String() != before {
			t.Errorf("%s: result changed from %s to %s when an operand was mutated",
				name, before, got.String())
		}
	}

	// Unary operations and the scale changers too.
	for name, op := range map[string]func(core.Decimal) core.Decimal{
		"Neg":     func(d core.Decimal) core.Decimal { return d.Neg() },
		"Abs":     func(d core.Decimal) core.Decimal { return d.Abs() },
		"Round":   func(d core.Decimal) core.Decimal { return d.Round(1) },
		"Ceil":    func(d core.Decimal) core.Decimal { return d.Ceil(1) },
		"Floor":   func(d core.Decimal) core.Decimal { return d.Floor(1) },
		"RoundUp": func(d core.Decimal) core.Decimal { return d.Round(5) },
	} {
		d := mk("7.55")
		got := op(d)
		if got.Coef == d.Coef {
			t.Errorf("%s: the result shares its operand's coefficient", name)
		}
		before := got.String()
		d.Coef.SetInt64(1)
		if got.String() != before {
			t.Errorf("%s: result moved when its operand was mutated", name)
		}
	}

	// DecimalFromBig copies, so the caller cannot reach in afterwards.
	src := big.NewInt(150)
	d := core.DecimalFromBig(src, 2)
	src.SetInt64(999)
	if d.String() != "1.50" {
		t.Errorf("DecimalFromBig did not copy: %s", d.String())
	}

	// The shared pow10 table must never be written through. Exercising the
	// scale changers at a table exponent and then re-checking a known value
	// catches a Mul/Quo that wrote into its argument.
	_ = mk("1").Round(7).Mul(mk("1.0000001"))
	if got := mk("1").Round(3).String(); got != "1.000" {
		t.Errorf("pow10 table was corrupted: %s", got)
	}
}

func TestNewDecimalAndNegativeScaleIsClamped(t *testing.T) {
	if got := core.NewDecimal(150, 2).String(); got != "1.50" {
		t.Errorf("NewDecimal = %s", got)
	}
	// The format has no negative-scale spelling, so there is no value one
	// could round-trip to; clamping keeps every method total.
	if got := core.NewDecimal(15, -2); got.Scale != 0 || got.String() != "15" {
		t.Errorf("negative scale not clamped: %v %q", got.Scale, got.String())
	}
	if got := core.DecimalFromBig(nil, 2); got.String() != "0.00" {
		t.Errorf("nil coefficient: %s", got.String())
	}
	if got := core.NewDecimal(0, 0).Round(-1); got.Scale != 0 {
		t.Errorf("Round(-1) scale = %d", got.Scale)
	}
}

func TestBigCoefficientsStayExact(t *testing.T) {
	// Beyond float64 and beyond int64 - the reason this type exists.
	huge := "123456789012345678901234567890.123456789"
	d := dec(t, huge)
	if d.String() != huge {
		t.Errorf("round trip lost digits:\n got %s\nwant %s", d.String(), huge)
	}
	if d.Scale != 9 || d.Precision() != 39 {
		t.Errorf("scale=%d precision=%d", d.Scale, d.Precision())
	}
	sum := d.Add(d)
	if got := sum.Sub(d); got.Cmp(d) != 0 {
		t.Errorf("(d+d)-d = %s, want %s", got.String(), d.String())
	}
}

// Branches the property tests reach only by luck: a divisor whose alignment
// shifts the DENOMINATOR (a high-scale dividend divided down to a low scale),
// a clamped negative scale on the big constructor, and an over-long literal in
// an error message.
func TestQuoShiftsTheDenominatorWhenTheDividendOutscales(t *testing.T) {
	// scale + o.Scale - d.Scale is negative here (0 + 0 - 4), so the divisor
	// is scaled up instead of the dividend.
	got, err := dec(t, "1.2345").Quo(dec(t, "1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "1" {
		t.Errorf("1.2345 / 1 at scale 0 = %s, want 1", got.String())
	}
	got, err = dec(t, "9.9999").Quo(dec(t, "1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "10" { // half away from zero
		t.Errorf("9.9999 / 1 at scale 0 = %s, want 10", got.String())
	}
	got, err = dec(t, "12.345").Quo(dec(t, "0.5"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "24.7" {
		t.Errorf("12.345 / 0.5 at scale 1 = %s, want 24.7", got.String())
	}
}

func TestDecimalFromBigClampsNegativeScale(t *testing.T) {
	if got := core.DecimalFromBig(big.NewInt(15), -3); got.Scale != 0 || got.String() != "15" {
		t.Errorf("DecimalFromBig(15, -3) = %q scale %d", got.String(), got.Scale)
	}
}

func TestParseDecimalErrorNamesTheInputAndStaysShort(t *testing.T) {
	long := "1." + strings.Repeat("x", 80)
	_, err := core.ParseDecimal(long)
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(err.Error()) > 120 {
		t.Errorf("error message is %d bytes: %s", len(err.Error()), err)
	}
	if !strings.Contains(err.Error(), "…") {
		t.Errorf("a long input should be truncated in the message: %s", err)
	}
	_, err = core.ParseDecimal("nope")
	if !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("the message should name the input: %s", err)
	}
}

// A negative scale has no spelling in the format, so every entry point clamps
// it rather than producing a value that cannot be written.
func TestNegativeScaleIsClampedEverywhere(t *testing.T) {
	one := dec(t, "1")
	got, err := dec(t, "10").Quo(one, -5)
	if err != nil || got.Scale != 0 || got.String() != "10" {
		t.Errorf("Quo at a negative scale = %q scale %d (%v)", got.String(), got.Scale, err)
	}
	for name, d := range map[string]core.Decimal{
		"Round": one.Round(-1), "Ceil": one.Ceil(-1), "Floor": one.Floor(-1),
	} {
		if d.Scale != 0 {
			t.Errorf("%s(-1) scale = %d", name, d.Scale)
		}
	}
	if r, ok := one.Rescale(-1); !ok || r.Scale != 0 {
		t.Errorf("Rescale(-1) = %q, %v", r.String(), ok)
	}
}

// Scales past the shared power-of-ten table must still be exact — the table is
// an optimisation for real documents, not a limit.
func TestScalesBeyondThePow10Table(t *testing.T) {
	big40 := "0." + strings.Repeat("0", 39) + "1" // scale 40
	d := dec(t, big40)
	if d.Scale != 40 || d.String() != big40 {
		t.Fatalf("scale %d, printed %q", d.Scale, d.String())
	}
	if got := d.Add(d).String(); got != "0."+strings.Repeat("0", 39)+"2" {
		t.Errorf("adding at scale 40 = %s", got)
	}
	if got := d.Mul(d).Scale; got != 80 {
		t.Errorf("scale-80 product came out at %d", got)
	}
	if got := dec(t, "1").Round(40); got.Scale != 40 || got.Cmp(dec(t, "1")) != 0 {
		t.Errorf("rounding up to scale 40 = %q", got.String())
	}
	q, err := dec(t, "1").Quo(dec(t, "3"), 40)
	if err != nil || q.Scale != 40 || !strings.HasPrefix(q.String(), "0.3333333333") {
		t.Errorf("1/3 at scale 40 = %q (%v)", q.String(), err)
	}
}

// A decimal that can only be built from its own coefficient is one nobody can
// use. These are the routes a caller actually has.
func TestConversionsFromNativeTypes(t *testing.T) {
	if got := core.DecimalFromInt(42); got.String() != "42" || got.Scale != 0 {
		t.Errorf("DecimalFromInt(42) = %q scale %d", got.String(), got.Scale)
	}
	if got := core.DecimalFromInt(-7); got.String() != "-7" {
		t.Errorf("DecimalFromInt(-7) = %q", got.String())
	}
	// The unsigned range reaches past int64, so it must not go through one.
	if got := core.DecimalFromInt(uint64(18446744073709551615)); got.String() != "18446744073709551615" {
		t.Errorf("DecimalFromInt(max uint64) = %q", got.String())
	}
	if got := core.DecimalFromInt(int8(-128)); got.String() != "-128" {
		t.Errorf("DecimalFromInt(int8) = %q", got.String())
	}

	// A float has no honest scale of its own, so the caller supplies one.
	for _, tc := range []struct {
		f     float64
		scale int
		want  string
	}{
		{19.99, 2, "19.99"}, {19.99, 4, "19.9900"}, {19.99, 0, "20"},
		{-2.5, 0, "-3"}, {0.5, 0, "1"}, {1.0 / 3.0, 5, "0.33333"},
	} {
		got, err := core.DecimalFromFloat(tc.f, tc.scale)
		if err != nil {
			t.Fatalf("DecimalFromFloat(%v, %d): %v", tc.f, tc.scale, err)
		}
		if got.String() != tc.want {
			t.Errorf("DecimalFromFloat(%v, %d) = %q, want %q", tc.f, tc.scale, got.String(), tc.want)
		}
	}
	// A non-finite float has no decimal value at all.
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := core.DecimalFromFloat(f, 2); err == nil {
			t.Errorf("DecimalFromFloat(%v) was accepted", f)
		}
	}
	if d, _ := core.DecimalFromFloat(1.5, -1); d.Scale != 0 {
		t.Errorf("a negative scale was not clamped: %d", d.Scale)
	}
}

// Int64 reports rather than truncating: a silent truncation in an exact type
// is the failure the type exists to prevent.
func TestInt64IsExactOrRefuses(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
		ok   bool
	}{
		{"42", 42, true}, {"42.0", 42, true}, {"42.000", 42, true},
		{"-42", -42, true}, {"0", 0, true}, {"0.0", 0, true},
		{"42.5", 0, false}, {"0.1", 0, false},
		{"9223372036854775807", 9223372036854775807, true},
		{"9223372036854775808", 0, false}, // one past int64
		{"99999999999999999999999", 0, false},
	} {
		got, ok := dec(t, tc.in).Int64()
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Int64(%s) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	// Rounding is opt-in and explicit.
	if got, ok := dec(t, "42.5").Round(0).Int64(); !ok || got != 43 {
		t.Errorf("Round(0).Int64() = %d, %v", got, ok)
	}
}

// Without these, encoding/json reflects over the struct and emits
// {"Coef":1999,"Scale":2} — the representation leaking into a caller's API,
// and unreadable back.
func TestStandardEncoders(t *testing.T) {
	d := dec(t, "19.99")

	b, err := json.Marshal(d)
	if err != nil || string(b) != `"19.99"` {
		t.Fatalf("MarshalJSON = %s, %v", b, err)
	}
	var back core.Decimal
	if err := json.Unmarshal(b, &back); err != nil || !back.Same(d) {
		t.Errorf("JSON round trip: %v, %v", back, err)
	}
	// A JSON NUMBER is accepted too — it is what other producers send — and is
	// read through its text, so no float is involved.
	for _, in := range []string{`19.99`, `"19.99"`, `0.1`, `-0.05`} {
		var v core.Decimal
		if err := json.Unmarshal([]byte(in), &v); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", in, err)
		}
		want := strings.Trim(in, `"`)
		if v.String() != want {
			t.Errorf("UnmarshalJSON(%s) = %q, want %q", in, v.String(), want)
		}
	}
	// null leaves the zero value, as encoding/json does elsewhere.
	var z core.Decimal
	if err := json.Unmarshal([]byte("null"), &z); err != nil || !z.IsZero() {
		t.Errorf("null = %v, %v", z, err)
	}
	if err := json.Unmarshal([]byte(`"nope"`), &z); err == nil {
		t.Error("a non-decimal string was accepted")
	}

	// Text, for every codec that honours TextMarshaler.
	tb, err := d.MarshalText()
	if err != nil || string(tb) != "19.99" {
		t.Fatalf("MarshalText = %s, %v", tb, err)
	}
	var td core.Decimal
	if err := td.UnmarshalText(tb); err != nil || !td.Same(d) {
		t.Errorf("text round trip: %v, %v", td, err)
	}
	if err := td.UnmarshalText([]byte("nope")); err == nil {
		t.Error("UnmarshalText accepted rubbish")
	}

	// Inside a struct, which is where the leak actually showed.
	type row struct {
		Price core.Decimal `json:"price"`
	}
	out, _ := json.Marshal(row{d})
	if string(out) != `{"price":"19.99"}` {
		t.Errorf("in a struct: %s", out)
	}
	if strings.Contains(string(out), "Coef") {
		t.Errorf("the representation leaked: %s", out)
	}
}
