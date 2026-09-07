package schema

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Defs resolves names during validation: compiled schemas by (sigil-less)
// name, and variables.
type Defs interface {
	SchemaOf(name string) (*Schema, *errs.Error)
	// Var resolves a variable by sigil-less name; the error is
	// undefined-variable for a missing one, invalid-definition for a cycle.
	Var(name string) (any, *errs.Error)
}

// NoDefs is the empty namespace: nothing is defined in it.
//
// Validation may be asked to run with no definitions at all — a caller
// validating a Go value against a compiled schema has no document, so it has
// no header either. Handing it this rather than a nil interface is what keeps
// every `@reference` and `$ref` lookup a REPORTED fault instead of a nil
// dereference deep inside a member check.
type NoDefs struct{}

func (NoDefs) SchemaOf(name string) (*Schema, *errs.Error) {
	return nil, &errs.Error{Code: errs.UndefinedSchema, Line: 1, Col: 1}
}

func (NoDefs) Var(name string) (any, *errs.Error) {
	return nil, &errs.Error{Code: errs.UndefinedVariable, Line: 1, Col: 1}
}

// defsOr substitutes the empty namespace for a nil one, so no validation path
// has to check.
func defsOr(d Defs) Defs {
	if d == nil {
		return NoDefs{}
	}
	return d
}
