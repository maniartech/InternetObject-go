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
