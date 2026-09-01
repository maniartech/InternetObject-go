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
