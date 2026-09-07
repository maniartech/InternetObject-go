package internetobject

import (
	"reflect"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Validate checks a struct (or slice of structs) against the schema derived
// from its type — field types, `schema` tag constraints, optional/nullable
// flags. Faults return as the ErrorList with designated codes; nil means the
// value marshals to a document that satisfies its own schema.
//
// Go cannot intercept a plain field assignment, so this is the mutation
// story: mutate freely, call Validate when it matters — and Marshal runs the
// same check automatically whenever the type declares constraints, so an
// invalid value never reaches the wire.
func Validate(v any) error { return validateAgainst(v, nil) }

// ValidateWith validates v against an ALREADY COMPILED schema — one fetched
// from a registry, read from a file, or derived from another type — instead
// of the schema derived from v's own type and tags. `io` tags still name the
// members; the given schema owns types and constraints entirely (ADR 0004 D5:
// attached wins outright, never merges).
func ValidateWith(v any, s *Schema) error {
	if s == nil {
		return &MarshalError{Path: "$", Msg: "nil schema"}
	}
	return validateAgainst(v, s.s)
}

func validateAgainst(v any, override *schema.Schema) error {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return &MarshalError{Path: "$", Msg: "cannot validate a nil value"}
		}
		rv = rv.Elem()
	}
	switch {
	case rv.Kind() == reflect.Struct && !isModelStruct(rv.Type()):
		plan, err := planFor(rv.Type())
		if err != nil {
			return err
		}
		rec, err := encodeStruct(rv, plan, rootPath)
		if err != nil {
			return err
		}
		return checkRecords(schemaOr(override, plan), []any{rec})
	case rv.Kind() == reflect.Slice && isStructElem(rv.Type().Elem()):
		et := rv.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		plan, err := planFor(et)
		if err != nil {
			return err
		}
		var records []any
		for i := 0; i < rv.Len(); i++ {
			ev := rv.Index(i)
			for ev.Kind() == reflect.Pointer && !ev.IsNil() {
				ev = ev.Elem()
			}
			if ev.Kind() != reflect.Struct {
				return &MarshalError{Path: "$", Msg: "a collection record cannot be nil"}
			}
			rec, err := encodeStruct(ev, plan, rootPath)
			if err != nil {
				return err
			}
			records = append(records, rec)
		}
		return checkRecords(schemaOr(override, plan), records)
	// A MAP or an *Object is a record too, and the marshaler already knows how
	// to encode one (isRecordType). What it does NOT have is a derived schema:
	// a map declares no types, so there is nothing to check it against unless
	// the caller supplies one.
	case isRecordType(rv.Type()):
		if override == nil {
			return &MarshalError{Path: "$", Msg: "a map or an Object declares no types, " +
				"so it has nothing to validate against; use ValidateWith with a schema"}
		}
		rec, err := encodeValue(rv, "", rootPath)
		if err != nil {
			return err
		}
		obj, ok := rec.(*core.Object)
		if !ok {
			return &MarshalError{Path: "$", Msg: "cannot validate a nil record"}
		}
		return checkRecords(override, []any{obj})

	case rv.Kind() == reflect.Slice && isRecordType(rv.Type().Elem()):
		if override == nil {
			return &MarshalError{Path: "$", Msg: "a map or an Object declares no types, " +
				"so it has nothing to validate against; use ValidateWith with a schema"}
		}
		var records []any
		for i := 0; i < rv.Len(); i++ {
			el := rv.Index(i)
			if isNilRecord(el) {
				return &MarshalError{Path: rootPath.record(i).String(),
					Msg: "a collection record cannot be nil"}
			}
			rec, err := encodeValue(el, "", rootPath.record(i))
			if err != nil {
				return err
			}
			records = append(records, rec)
		}
		return checkRecords(override, records)
	}
	return &MarshalError{Path: "$", Msg: "Validate takes a struct, a map, an Object, or a slice of those"}
}

// schemaOr picks the explicitly given schema over the type-derived one.
func schemaOr(override *schema.Schema, plan *structPlan) *schema.Schema {
	if override != nil {
		return override
	}
	return plan.compiled
}

// checkRecords validates encoded records against a compiled schema,
// accumulating every fault. Used by Validate/ValidateWith always, and by
// Marshal whenever the plan carries `schema`-tag constraints.
func checkRecords(s *schema.Schema, records []any) error {
	var all []errs.Error
	for _, rec := range records {
		// CheckRecord, not ValidateRecord: this caller wants the faults, and
		// the assembled record it used to build was discarded on every call.
		all = append(all, schema.CheckRecord(rec.(*core.Object), s, noDefs{}, true)...)
	}
	return toErrorList(all)
}

// noDefs is the definition context of a derived schema: it declares nothing,
// so a `$Ref` or `@var` in a schema tag resolves to its designated error.
type noDefs struct{}

func (noDefs) SchemaOf(string) (*schema.Schema, *errs.Error) {
	return nil, &errs.Error{Code: errs.UndefinedSchema, Line: 1, Col: 1}
}

func (noDefs) Var(string) (any, *errs.Error) {
	return nil, &errs.Error{Code: errs.UndefinedVariable, Line: 1, Col: 1}
}

// SchemaFor returns the Internet Object schema derived from T: field types,
// `io` names and markers, and `schema` tag constraints — the schema Marshal
// writes as the document header.
func SchemaFor[T any]() (*Schema, error) {
	t := reflect.TypeFor[T]()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || isModelStruct(t) {
		return nil, &MarshalError{Path: t.String(), Msg: "SchemaFor takes a struct type"}
	}
	plan, err := planFor(t)
	if err != nil {
		return nil, err
	}
	return newSchema(plan.compiled), nil
}

// String renders the schema in canonical Internet Object syntax — valid as a
// schema-only document header, and re-parses to the same schema.
func (s *Schema) String() string {
	return document.SchemaText(s.s)
}
