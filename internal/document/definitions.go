package document

// Definitions is a document's header seen as a NAMESPACE: the named schemas
// and @variables a record may refer to. io-js2 gives it a core class of its
// own (src/core/definitions.ts); here it is the one type that both the
// document and the streaming reader resolve names through.

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Definitions resolves names for validation, compiling named schemas lazily and
// exactly once.
type Definitions struct {
	// Header is the parsed header these definitions came from.
	Header   *parser.Header
	compiled map[string]*schema.Schema
	failed   map[string]*errs.Error
	inline   *schema.Schema // the compiled bare-expression header schema
}

func NewDefinitions(h *parser.Header) *Definitions {
	return &Definitions{Header: h, compiled: map[string]*schema.Schema{}, failed: map[string]*errs.Error{}}
}

// SchemaOf resolves and compiles the named schema, chasing `$ref` aliases. A
// self- or mutually-referential alias chain is invalid-definition — the
// reference crashes with a bare stack overflow here (upstream finding), and a
// designated code is the non-crashing spelling of that behavior.
func (d *Definitions) SchemaOf(name string) (*schema.Schema, *errs.Error) {
	name = strings.TrimPrefix(name, "$")
	seen := map[string]bool{}
	for {
		if s, ok := d.compiled[name]; ok {
			return s, nil
		}
		if e, ok := d.failed[name]; ok {
			return nil, e
		}
		if seen[name] {
			e := &errs.Error{Code: errs.InvalidDefinition, Line: 1, Col: 1}
			d.failed[name] = e
			return nil, e
		}
		seen[name] = true
		var shape any
		if d.Header != nil {
			if v, ok := d.Header.Schemas[name]; ok {
				shape = v
			}
		}
		if shape == nil {
			e := &errs.Error{Code: errs.UndefinedSchema, Line: 1, Col: 1}
			d.failed[name] = e
			return nil, e
		}
		// A named schema may itself be a `$ref` to another one.
		if ref, ok := shape.(string); ok && strings.HasPrefix(ref, "$") {
			name = strings.TrimPrefix(ref, "$")
			continue
		}
		s, cerr := schema.Compile(shape, "")
		if cerr != nil {
			d.failed[name] = cerr
			return nil, cerr
		}
		d.compiled[name] = s
		return s, nil
	}
}

// Var resolves a variable by (sigil-less) name, chasing @-references so a
// definition may name one parsed later. A missing name is undefined-variable;
// a self- or mutually-referential chain is invalid-definition.
func (d *Definitions) Var(name string) (any, *errs.Error) {
	seen := map[string]bool{}
	for {
		if d.Header == nil {
			return nil, &errs.Error{Code: errs.UndefinedVariable, Line: 1, Col: 1}
		}
		if seen[name] {
			return nil, &errs.Error{Code: errs.InvalidDefinition, Line: 1, Col: 1}
		}
		seen[name] = true
		v, ok := d.Header.Vars[name]
		if !ok {
			return nil, &errs.Error{Code: errs.UndefinedVariable, Line: 1, Col: 1}
		}
		if s, ok := v.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
			name = s[1:]
			continue
		}
		return v, nil
	}
}

// ResolveVars resolves every @-string VALUE in a record in place — quoted or
// open, by design references in any string form (io-test-cases FINDINGS #3).
// Keys stay literal. Returns the first resolution error.
func ResolveVars(v any, defs *Definitions) *errs.Error {
	return resolveVarsAt(v, defs, nil)
}

// resolveVarsAt substitutes @-references, following them INTO the values they
// substitute — a variable's value may hold references of its own, and leaving
// those alone made io-go accept `~ @r: {{[@0]}}` with an undefined @0 that the
// reference rejects (oracle-confirmed 2026-09-06).
//
// `seen` carries the variable names on the current path, which is what makes
// that safe. A self-referential variable (`~ @r: {r: @r}`) is reported as
// invalid-definition — the same answer SchemaOf gives a self-referential $ref
// chain — and the check happens BEFORE the substitution, deliberately:
// assigning first and detecting after would leave the variable's own value
// pointing at itself, and the header writer would then never terminate. The
// first version of this did exactly that and died on the stack; rule 10 says a
// designated code, never a crash.
func resolveVarsAt(v any, defs *Definitions, seen map[string]bool) *errs.Error {
	sub := func(s string, set func(any)) (bool, *errs.Error) {
		if !strings.HasPrefix(s, "@") || len(s) <= 1 {
			return false, nil
		}
		name := s[1:]
		if seen[name] {
			return true, &errs.Error{Code: errs.InvalidDefinition, Line: 1, Col: 1}
		}
		r, verr := defs.Var(name)
		if verr != nil {
			return true, verr
		}
		set(r)
		if seen == nil {
			seen = map[string]bool{}
		}
		seen[name] = true
		verr = resolveVarsAt(r, defs, seen)
		delete(seen, name)
		return true, verr
	}

	switch x := v.(type) {
	case *core.Object:
		for i := range x.Members {
			mv := x.Members[i].Value
			if s, ok := mv.(string); ok {
				if done, verr := sub(s, func(r any) { x.Members[i].Value = r }); done {
					if verr != nil {
						return verr
					}
					continue
				}
			}
			if verr := resolveVarsAt(mv, defs, seen); verr != nil {
				return verr
			}
		}
	case []any:
		for i, e := range x {
			if s, ok := e.(string); ok {
				if done, verr := sub(s, func(r any) { x[i] = r }); done {
					if verr != nil {
						return verr
					}
					continue
				}
			}
			if verr := resolveVarsAt(e, defs, seen); verr != nil {
				return verr
			}
		}
	}
	return nil
}
