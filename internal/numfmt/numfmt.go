// Package numfmt renders a float64 exactly as ECMAScript's Number::toString
// does. The conformance corpus writes expected number values in that form,
// and the serializer later emits it, so the rendering lives in one place.
package numfmt

import (
	"math"
	"strconv"
)

// Format renders f per the ECMAScript Number-to-String algorithm: shortest
// round-trip digits, decimal notation for exponents in (-7, 21), scientific
// otherwise, and "0" for both zeros.
func Format(f float64) string {
	return string(Append(nil, f))
}

// Append is Format writing into a caller-owned buffer, so a number never
// materializes an intermediate string on the writer's hot path (ADR 0006 P1).
func Append(dst []byte, f float64) []byte {
	switch {
	case math.IsNaN(f):
		return append(dst, "NaN"...)
	case math.IsInf(f, 1):
		return append(dst, "Infinity"...)
	case math.IsInf(f, -1):
		return append(dst, "-Infinity"...)
	case f == 0:
		return append(dst, '0') // covers -0: ECMAScript renders it "0"
	}
	neg := math.Signbit(f)
	if neg {
		f = -f
	}

	// Shortest digits via Go's Ryū ('e', precision -1), e.g. "1.23e+04".
	// scratch is stack-sized: a float64 never needs more than 32 bytes here.
	var scratch [32]byte
	form := strconv.AppendFloat(scratch[:0], f, 'e', -1, 64)
	ePos := len(form) - 1
	for form[ePos] != 'e' {
		ePos--
	}
	mant, expText := form[:ePos], form[ePos+1:]
	exp, _ := strconv.Atoi(string(expText))

	// digits = the mantissa without its point, written into a second scratch.
	var digitBuf [32]byte
	digits := digitBuf[:0]
	for _, c := range mant {
		if c != '.' {
			digits = append(digits, c)
		}
	}

	k := len(digits) // digit count
	n := exp + 1     // decimal point position relative to the digits

	if neg {
		dst = append(dst, '-')
	}
	switch {
	case k <= n && n <= 21:
		dst = append(dst, digits...)
		dst = appendZeros(dst, n-k)
	case 0 < n && n <= 21:
		dst = append(dst, digits[:n]...)
		dst = append(dst, '.')
		dst = append(dst, digits[n:]...)
	case -6 < n && n <= 0:
		dst = append(dst, '0', '.')
		dst = appendZeros(dst, -n)
		dst = append(dst, digits...)
	default:
		dst = append(dst, digits[0])
		if k > 1 {
			dst = append(dst, '.')
			dst = append(dst, digits[1:]...)
		}
		dst = append(dst, 'e')
		if n-1 >= 0 {
			dst = append(dst, '+')
		}
		dst = strconv.AppendInt(dst, int64(n-1), 10)
	}
	return dst
}

func appendZeros(dst []byte, n int) []byte {
	for ; n > 0; n-- {
		dst = append(dst, '0')
	}
	return dst
}
