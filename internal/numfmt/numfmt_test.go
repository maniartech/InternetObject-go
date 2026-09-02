package numfmt

import (
	"math"
	"testing"
)

// Expected values are ECMAScript String(x) outputs, verified against Node.
func TestFormat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "0"},
		{42, "42"},
		{-17, "-17"},
		{4.2, "4.2"},
		{0.5, "0.5"},
		{-0.5, "-0.5"},
		{0.000123, "0.000123"},
		{12300, "12300"},
		{-2500, "-2500"},
		{50, "50"},
		{255, "255"},
		{3735928559, "3735928559"},
		{9007199254740991, "9007199254740991"},
		{9007199254740993, "9007199254740992"}, // float64 rounding, as in JS
		{1e21, "1e+21"},
		{1e-7, "1e-7"},
		{1.5e-7, "1.5e-7"},
		{1e-6, "0.000001"},
		{123456789012345678901234567890, "1.2345678901234568e+29"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	}
	for _, c := range cases {
		if got := Format(c.in); got != c.want {
			t.Errorf("Format(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The integer fast path in Append must produce byte-for-byte what the
// ECMAScript algorithm produces. It is an optimization of appendGeneral, so
// any disagreement is a bug in the fast path — never a new spelling.

func fastEqualsGeneral(t *testing.T, f float64) {
	t.Helper()
	fast := string(Append(nil, f))
	general := string(appendGeneral(nil, f))
	if fast != general {
		t.Errorf("%v: fast %q, general %q", f, fast, general)
	}
}

func TestFastPathEqualsGeneral(t *testing.T) {
	for _, f := range []float64{
		0, 1, -1, 7, -7, 10, 100, 1e6, -1e6,
		9007199254740991, -9007199254740991, // 2^53-1, the fast path's edge
		9007199254740992, -9007199254740992, // 2^53, just outside it
		1e15, 1e16, 1e20, 1e21, 1e22, -1e21,
		0.5, -0.5, 1.5, 1e-7, 123456.789,
	} {
		fastEqualsGeneral(t, f)
	}
	// Every integer in a dense range, both signs.
	for i := -2000; i <= 2000; i++ {
		fastEqualsGeneral(t, float64(i))
	}
	// Powers of two and ten, where the two paths could most plausibly differ.
	for e := 0; e < 53; e++ {
		fastEqualsGeneral(t, float64(int64(1)<<e))
		fastEqualsGeneral(t, -float64(int64(1)<<e))
	}
}

func FuzzFastPathEqualsGeneral(f *testing.F) {
	f.Add(0.0)
	f.Add(1.0)
	f.Add(-42.0)
	f.Add(9007199254740991.0)
	f.Add(1.5)
	f.Fuzz(func(t *testing.T, x float64) {
		fastEqualsGeneral(t, x)
	})
}
