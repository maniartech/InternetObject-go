package core_test

import (
	"testing"

	"github.com/maniartech/InternetObject-go/internal/core"
)

// Differential table against the reference implementation (io-js2
// src/core/decimal/decimal.ts), captured by probe on 2026-09-07 — SPEC 0002 §8.1.
//
// Every `want` below is the reference's ACTUAL printed output, not what it
// ought to be. Where this port deliberately answers differently the row says
// so and names the finding, so a divergence can never become invisible: if
// someone "fixes" one of these to agree, the reason is right here.
//
// Arithmetic is outside io-specs' scope, so nothing in the corpus pins any of
// it. This table is the only gate that exists.

type oracleCase struct {
	a, op, b string
	oracle   string // what the reference prints
	ours     string // what this port prints; == oracle unless `why` explains
	why      string
}

func TestDecimalAgreesWithTheOracleOrSaysWhyNot(t *testing.T) {
	cases := []oracleCase{
		// ── Add: the larger scale. Agrees on every case probed. ─────────────
		{"1.5", "+", "1.50", "3.00", "3.00", ""},
		{"0.1", "+", "0.2", "0.3", "0.3", ""},
		{"1", "+", "0.005", "1.005", "1.005", ""},
		{"100", "+", "0.01", "100.01", "100.01", ""},

		// ── Sub: the larger scale. Agrees. ──────────────────────────────────
		{"1.5", "-", "1.50", "0.00", "0.00", ""},
		{"0.3", "-", "0.1", "0.2", "0.2", ""},
		{"1", "-", "2", "-1", "-1", ""},
		{"0", "-", "0.05", "-0.05", "-0.05", ""},

		// ── Mul: WE DIVERGE, because the reference loses the product. ───────
		// It computes the exact result at scale1+scale2 and then rounds it
		// down to max(scale1, scale2), so a product smaller than the operands'
		// own scale vanishes entirely. Upstream finding #26.
		{"1.5", "*", "1.5", "2.3", "2.25", "#26: reference rounds 2.25 away"},
		{"0.01", "*", "0.01", "0.00", "0.0001", "#26: reference loses the whole product"},
		{"0.1", "*", "0.1", "0.0", "0.01", "#26: reference loses the whole product"},
		{"1.11", "*", "1.11", "1.23", "1.2321", "#26: reference rounds"},
		{"0.001", "*", "0.002", "0.000", "0.000002", "#26: reference loses the whole product"},
		{"1.50", "*", "2", "3.00", "3.00", ""},
		{"-1.5", "*", "2.00", "-3.00", "-3.000", "#26: scale is the sum, so the result is exact"},
		{"0", "*", "1.23", "0.00", "0.00", ""},
		{"2.5", "*", "4", "10.0", "10.0", ""},

		// ── Rem: sign of the dividend, larger scale. Agrees on every case. ──
		{"7", "%", "3", "1", "1", ""},
		{"-7", "%", "3", "-1", "-1", ""},
		{"7", "%", "-3", "1", "1", ""},
		{"7.5", "%", "2", "1.5", "1.5", ""},
		{"1.00", "%", "0.3", "0.10", "0.10", ""},
	}

	for _, tc := range cases {
		a, b := dec(t, tc.a), dec(t, tc.b)
		var got core.Decimal
		switch tc.op {
		case "+":
			got = a.Add(b)
		case "-":
			got = a.Sub(b)
		case "*":
			got = a.Mul(b)
		case "%":
			r, err := a.Rem(b)
			if err != nil {
				t.Fatalf("%s %% %s: %v", tc.a, tc.b, err)
			}
			got = r
		}
		if got.String() != tc.ours {
			t.Errorf("%s %s %s = %s, want %s", tc.a, tc.op, tc.b, got.String(), tc.ours)
		}
		// A row claiming agreement must actually agree, and a row claiming a
		// divergence must actually diverge — otherwise the table rots into
		// decoration.
		if tc.why == "" && tc.ours != tc.oracle {
			t.Errorf("%s %s %s: row claims agreement but ours=%s oracle=%s",
				tc.a, tc.op, tc.b, tc.ours, tc.oracle)
		}
		if tc.why != "" && tc.ours == tc.oracle {
			t.Errorf("%s %s %s: row claims a divergence (%s) but both say %s",
				tc.a, tc.op, tc.b, tc.why, tc.ours)
		}
	}
}

// Multiplication is EXACT here: the product of two decimals always has an
// exact representation at the sum of their scales, so there is never a reason
// to round it. The reference rounds to max(scale) and destroys small products
// outright (0.01 * 0.01 = 0.00). This property is what finding #26 is about.
func TestMulIsExact(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		{"0.01", "0.01", "0.0001"},
		{"0.001", "0.002", "0.000002"},
		{"0.0000001", "0.0000001", "0.00000000000001"},
		{"123.456", "0.001", "0.123456"},
	} {
		got := dec(t, tc.a).Mul(dec(t, tc.b))
		if got.String() != tc.want {
			t.Errorf("%s * %s = %s, want %s (exact)", tc.a, tc.b, got.String(), tc.want)
		}
		if got.IsZero() {
			t.Errorf("%s * %s came out as zero — the product was lost", tc.a, tc.b)
		}
		if got.Scale != dec(t, tc.a).Scale+dec(t, tc.b).Scale {
			t.Errorf("%s * %s: scale = %d, want the sum of the operands'", tc.a, tc.b, got.Scale)
		}
	}
}

// Precision matches the oracle on every case probed — this is the divergence
// finding #25 closed, pinned so it cannot reopen.
func TestPrecisionMatchesTheOracle(t *testing.T) {
	for _, tc := range []struct {
		in     string
		oracle int
	}{
		{"0", 1}, {"0.0", 1}, {"0.05", 2}, {"0.050", 3},
		{"123.45", 5}, {"0.00001", 5}, {"12345.67", 7},
	} {
		if got := dec(t, tc.in).Precision(); got != tc.oracle {
			t.Errorf("Precision(%s) = %d, oracle says %d", tc.in, got, tc.oracle)
		}
	}
}

// The reference's div takes the DIVISOR's scale, so every division by an
// integer returns an integer: 1/3 = 0, 10/4 = 3, 1/8 = 0. It also never calls
// its own calculateDivisionResultPrecisionScale, which specifies a minimum
// scale of 6. This port requires the caller to say what scale they want, and
// this test states the reference's answers so the choice is documented rather
// than merely asserted (SPEC 0002 §5).
func TestQuoTakesAnExplicitScaleUnlikeTheOracle(t *testing.T) {
	for _, tc := range []struct {
		a, b   string
		oracle string // what the reference's div returns, scale and all
		scale  int
		ours   string
	}{
		{"1.0", "3", "0", 6, "0.333333"},
		{"1", "3", "0", 6, "0.333333"},
		{"10", "4", "3", 2, "2.50"},
		{"1", "8", "0", 3, "0.125"},
	} {
		got, err := dec(t, tc.a).Quo(dec(t, tc.b), tc.scale)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.a, tc.b, err)
		}
		if got.String() != tc.ours {
			t.Errorf("%s / %s at scale %d = %s, want %s", tc.a, tc.b, tc.scale, got.String(), tc.ours)
		}
		if got.String() == tc.oracle && tc.ours != tc.oracle {
			t.Errorf("%s / %s: expected to differ from the oracle's %s", tc.a, tc.b, tc.oracle)
		}
	}
}
