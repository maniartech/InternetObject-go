package internetobject

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Error is one accumulated fault: a designated code and a 1-based position.
// Codes — never messages — are the conformance contract.
type Error struct {
	// Code is the designated kebab-case code — the conformance contract.
	// Compare it against the constants in this package, never a bare literal.
	Code Code
	// Category is derived from where the fault arose, never from the code's
	// spelling: "syntax", "validation", "stream" or "general".
	Category string
	// Path locates the fault structurally: "$" is the document root, "$[2]"
	// the third record of a collection, "$[2].age" a member of it.
	Path string
	// RecordIndex is the 0-based position of the failed record within its
	// collection, or -1 when the fault is not inside one.
	RecordIndex int
	// Line, Col are the 1-based position of the offending value.
	Line int
	Col  int
}

func (e Error) Error() string {
	if e.Path != "" && e.Path != "$" {
		return fmt.Sprintf("%s at %s (%d:%d)", e.Code, e.Path, e.Line, e.Col)
	}
	return fmt.Sprintf("%s at %d:%d", e.Code, e.Line, e.Col)
}

// Is reports whether target is an Error with the same code, so
// errors.Is(err, io.Error{Code: "mismatched-min"}) works on a single fault.
func (e Error) Is(target error) bool {
	t, ok := target.(Error)
	return ok && t.Code == e.Code
}

// toError is THE conversion from an internal fault to the public one, so
// every entry point reports the same fields.
func toError(e errs.Error) Error {
	cat := e.Category
	if cat == "" {
		cat = errs.CategoryOf(e.Code)
	}
	idx := e.RecordIndex
	if idx == 0 && e.Path == "" {
		idx = -1 // never parsed from a collection context
	}
	return Error{
		Code: e.Code, Category: cat, Path: e.Path,
		RecordIndex: idx, Line: int(e.Line), Col: int(e.Col),
	}
}
