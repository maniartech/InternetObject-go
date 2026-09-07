package internetobject

import (
	"math/big"

	"github.com/maniartech/InternetObject-go/internal/core"
)

// Decimal is an exact fixed-point number — a coefficient and a scale — for
// money and anything else a float would quietly corrupt.
//
// SCALE IS PART OF THE VALUE. `1.5m` and `1.50m` have the same magnitude and
// are different values: the writer preserves the scale it read. So there are
// two equalities and they are named apart — [Decimal.Equal] compares
// magnitude, [Decimal.Same] compares magnitude and scale.
//
// Never compare two decimals with `==`: the coefficient is a pointer, so `==`
// is identity and two decimals read from the same text are unequal under it.
//
// A Decimal is immutable; every operation returns a new value. The zero value
// behaves as `0m`.
type Decimal = core.Decimal

// ErrDivideByZero is returned by [Decimal.Quo] and [Decimal.Rem]. A decimal is
// data, and data divides by zero, so this is an error rather than a panic.
var ErrDivideByZero = core.ErrDivideByZero

// ParseDecimal reads a decimal literal without its `m` suffix — "1.50",
// "-0.05", "123" — using the same grammar the document parser uses, so text
// this accepts is text a document accepts.
//
// The scale comes from the spelling: ParseDecimal("1.50") has scale 2 and
// ParseDecimal("1.5") has scale 1.
func ParseDecimal(s string) (Decimal, error) { return core.ParseDecimal(s) }

// NewDecimal builds a decimal from a coefficient and a scale:
// NewDecimal(150, 2) is 1.50m.
func NewDecimal(coef int64, scale int) Decimal { return core.NewDecimal(coef, scale) }

// DecimalFromBig builds a decimal from an arbitrary-precision coefficient,
// copying it so later changes to the argument cannot reach the value.
func DecimalFromBig(coef *big.Int, scale int) Decimal { return core.DecimalFromBig(coef, scale) }
