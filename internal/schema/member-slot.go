package schema

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// memberSlot is one declared member's validation state. The three facts live
// in ONE slice so a record costs a single allocation here — three parallel
// slices cost three, and the two maps this replaced cost two plus hashing
// (ADR 0006 P2).
type memberSlot struct {
	val       any
	filled    bool // a value was produced (absent members leave this false)
	processed bool // this member has been dealt with; a second key is a duplicate
}

// absent marks a member that legitimately produced no value (optional, no
// default).
type absentType struct{}

var absent = absentType{}

type valFail struct{ err errs.Error }
