// Package numfmt renders a float64 exactly as ECMAScript's Number::toString
// does. The conformance corpus writes expected number values in that form,
// and the serializer later emits it, so the rendering lives in one place.
package numfmt

import (
	"math"
	"strconv"
	"strings"
)

// Format renders f per the ECMAScript Number-to-String algorithm: shortest
// round-trip digits, decimal notation for exponents in (-7, 21), scientific
// otherwise, and "0" for both zeros.
func Format(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0" // covers -0: ECMAScript renders it "0"
	}
	neg := math.Signbit(f)
	if neg {
		f = -f
	}

	// Shortest digits via Go's Ryū ('e', precision -1), e.g. "1.23e+04".
	mant, expText, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expText)

	k := len(digits) // digit count
	n := exp + 1     // decimal point position relative to the digits

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	switch {
	case k <= n && n <= 21:
		b.WriteString(digits)
		b.WriteString(strings.Repeat("0", n-k))
	case 0 < n && n <= 21:
		b.WriteString(digits[:n])
		b.WriteByte('.')
		b.WriteString(digits[n:])
	case -6 < n && n <= 0:
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", -n))
		b.WriteString(digits)
	default:
		b.WriteString(digits[:1])
		if k > 1 {
			b.WriteByte('.')
			b.WriteString(digits[1:])
		}
		b.WriteByte('e')
		if n-1 >= 0 {
			b.WriteByte('+')
		}
		b.WriteString(strconv.Itoa(n - 1))
	}
	return b.String()
}
