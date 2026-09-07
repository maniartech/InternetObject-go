package internetobject

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// ErrorList is every fault a document carried, in order. It is the error
// value Parse and ParseSchema return.
type ErrorList []Error

func (l ErrorList) Error() string {
	parts := make([]string, len(l))
	for i, e := range l {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

// Codes returns the designated codes in order — the conformance-relevant
// projection of the list.
func (l ErrorList) Codes() []Code {
	out := make([]Code, len(l))
	for i, e := range l {
		out[i] = e.Code
	}
	return out
}

// Has reports whether any fault carries the given code.
func (l ErrorList) Has(code Code) bool {
	for _, e := range l {
		if e.Code == code {
			return true
		}
	}
	return false
}

// Is lets errors.Is(err, io.Error{Code: …}) match a list containing that code.
func (l ErrorList) Is(target error) bool {
	t, ok := target.(Error)
	return ok && l.Has(t.Code)
}

// Unwrap exposes the individual faults to errors.Is/As traversal.
func (l ErrorList) Unwrap() []error {
	out := make([]error, len(l))
	for i, e := range l {
		out[i] = e
	}
	return out
}

func toErrorList(es []errs.Error) error {
	if len(es) == 0 {
		return nil
	}
	out := make(ErrorList, len(es))
	for i, e := range es {
		out[i] = toError(e)
	}
	return out
}
