package document

import (
	"math"

	"github.com/maniartech/InternetObject-go/internal/numfmt"
)

// Writing NUMBERS: the format's own spelling, which is not Go's.

func ioNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	return numfmt.Format(f)
}

// appendIONumber is the append-style form: the specials are constants, and
// the general case defers to numfmt (ADR 0006 P1 — a numfmt.Append would
// remove the one remaining allocation here).
func appendIONumber(dst []byte, f float64) []byte {
	switch {
	case math.IsNaN(f):
		return append(dst, "NaN"...)
	case math.IsInf(f, 1):
		return append(dst, "Inf"...)
	case math.IsInf(f, -1):
		return append(dst, "-Inf"...)
	}
	return numfmt.Append(dst, f)
}

// appendTemporal is temporalLiteral in append form: time.AppendFormat writes
// straight into the buffer, so no intermediate string is built.
