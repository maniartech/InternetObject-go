package internetobject

import (
	"encoding/base64"
	"errors"
	"math/big"
	"os"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// The direct encode path (ADR 0006 roadmap item 5).
//
// The general path builds a *value.Object tree and hands it to the writer.
// That tree costs one interface box per scalar member — 47% of encode's
// allocations — for values the writer consumes immediately and discards. When
// a type is simple enough, this path walks the struct straight into the
// output buffer instead.
//
// It is NOT a second implementation of the format. Every spelling decision —
// when a string is quoted, how a number or temporal is written, when a key
// needs quotes — is made by the exported helpers in internal/document, the
// same ones the tree path uses. What is duplicated here is the *traversal*,
// which is mechanical. `TestFastPathMatchesTreePath` and its fuzz target hold
// the two byte-identical, so a divergence is a test failure rather than a
// silent wire change.
//
// The path is taken only for a struct (or slice of structs) whose every
// member is a scalar or a slice of scalars. A type whose `schema` tags must be
// validated is taken too when the fast encoder can ask the validator every
// question the tree would (fastCheck); a value the validator refuses makes
// it decline, and the tree path reports. Anything else falls back.

// fastEligible reports whether a plan's every field can be written directly,
// and for a type that validates records what to ask about each (plan.checks).
// Computed once per type, in the plan.
func fastEligible(t reflect.Type, plan *structPlan) bool {
	var checks []fieldCheck
	if plan.validate {
		checks = make([]fieldCheck, len(plan.fields))
	}
	for i := range plan.fields {
		f := &plan.fields[i]
		declared := t.FieldByIndex(f.index).Type
		ft := declared
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if !fastScalarType(ft) {
			if ft.Kind() != reflect.Slice || !fastScalarType(ft.Elem()) {
				return false
			}
		}
		if plan.validate {
			c, ok := fastCheck(f, declared, plan.compiled.Defs[f.name])
			if !ok {
				return false
			}
			checks[i] = c
		}
	}
	plan.checks = checks // only for a plan the fast encoder takes
	return true
}

// fieldCheck is what the fast encoder asks schema.Accepts about one field of a
// type that validates: the field's value, and each element of a slice field.
// A nil definition means there is nothing to ask.
type fieldCheck struct {
	value, elem *schema.MemberDef
}

// fastCheck works out what to ask the validator about one field so that the
// fast encoder refuses exactly what validating the record would, or reports
// that Accepts alone cannot tell. It can when the field's definition md is
// the one the field's Go type derives — a tag that adds constraints, not one
// that changes the type (`schema:"string"` on an int must still reach the
// tree, which reports expected-string) — with nothing that needs more than the
// value: no default, union or schema, no constraint on an array as a whole,
// and no `optional: false` on a field `omitempty` may leave out.
//
// What must be asked is anything a value of the derived type can still fail:
// a constraint; nil, where null is not declared; and ANY string, because the
// validator reads an `@`-string as a variable reference, which a record with
// no definitions cannot resolve. That last one shipped as a bypass in review
// (SPEC 0003 §5.2): an unconstrained name "@x" was written, where validation
// reports undefined-variable.
func fastCheck(f *fieldPlan, declared reflect.Type, md *schema.MemberDef) (fieldCheck, bool) {
	var c fieldCheck
	if md == nil || !md.Standalone() || (f.omitZero && !md.Optional) {
		return c, false
	}
	derived, err := annotationFor(declared, f.kind, map[reflect.Type]bool{}, new(bool))
	if err != nil {
		return c, false
	}
	nilable := declared.Kind() == reflect.Pointer
	switch d := derived.(type) {
	case string:
		if md.Type != d || md.Of != nil || (md.Constrained() && !acceptsKind(f.enc)) {
			return c, false
		}
		if md.Constrained() || nilable || f.enc == encString {
			c.value = md
		}
	case []any:
		elem, ok := d[0].(string)
		of := md.Of
		if !ok || md.Type != "array" || len(md.Constraints) > 0 || of == nil || of.Type != elem ||
			of.Of != nil || !of.Standalone() || (of.Constrained() && !acceptsKind(f.elem)) {
			return c, false
		}
		if nilable {
			c.value = md // a nil slice pointer is judged; a non-nil one has nothing more
		}
		st := declared
		for st.Kind() == reflect.Pointer {
			st = st.Elem()
		}
		if of.Constrained() || st.Elem().Kind() == reflect.Pointer || f.elem == encString {
			c.elem = of
		}
	default:
		return c, false
	}
	return c, true
}

// acceptsKind reports the kinds whose constrained value fastAccepts can box. A
// bool is not among them: no bool typedef carries a constraint, and a tag that
// makes one (`{any, choices: [T]}`) changes the type, which fastCheck refuses.
func acceptsKind(k encKind) bool {
	switch k {
	case encString, encInt, encUint, encFloat:
		return true
	}
	return false
}

// fastAccepts reports whether rv satisfies md, boxed exactly as encodeValue
// boxes it for the tree's validation: a string, a float64 for every number,
// nil for a nil pointer.
func fastAccepts(rv reflect.Value, k encKind, md *schema.MemberDef) bool {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return md.Accepts(nil)
		}
		rv = rv.Elem()
	}
	if k == encString && core.IsVariableRef(rv.String()) {
		return false // a reference the record's empty definitions cannot resolve
	}
	if !md.Constrained() {
		return true
	}
	switch k {
	case encString:
		return md.Accepts(rv.String())
	case encInt:
		return md.Accepts(float64(rv.Int()))
	case encUint:
		return md.Accepts(float64(rv.Uint()))
	case encFloat:
		return md.Accepts(rv.Float())
	}
	return false // fastCheck admits no other constrained kind
}

// errFastDecline makes marshalFast hand the value to the tree path, which
// validates it and reports the fault. It never reaches a caller.
var errFastDecline = errors.New("the fast encoder declines this value")

func fastScalarType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case bigIntType.Elem(), decimalType, timeType, bytesType:
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// appendFastRecord writes one struct as a record body: members in plan order,
// `omitempty` holes held back so trailing ones vanish — the same rule the
// tree writer applies.
//
// checks is passed separately from plan because what must be asked depends on
// the schema validated against, not only the type: Marshal passes the type's
// own (plan.checks), MarshalWith those for the schema it was given (§5.3).
func appendFastRecord(dst []byte, rv reflect.Value, plan *structPlan, checks []fieldCheck, at pathAt) ([]byte, error) {
	written, pending := 0, 0
	for i := range plan.fields {
		f := &plan.fields[i]
		var fv reflect.Value
		if f.at >= 0 {
			fv = rv.Field(f.at) // depth-1: no index walk
		} else {
			fv = rv.FieldByIndex(f.index)
		}
		if f.omitZero && fv.IsZero() {
			pending++ // a hole: only emitted if a later member follows
			continue
		}
		if checks != nil {
			c := checks[i]
			if c.value != nil && !fastAccepts(fv, f.enc, c.value) {
				return nil, errFastDecline
			}
			if c.elem != nil {
				sv := reflect.Indirect(fv)
				for j := 0; sv.IsValid() && j < sv.Len(); j++ {
					if !fastAccepts(sv.Index(j), f.elem, c.elem) {
						return nil, errFastDecline
					}
				}
			}
		}
		for ; pending > 0; pending-- {
			if written > 0 {
				dst = append(dst, ',', ' ')
			}
			written++
		}
		if written > 0 {
			dst = append(dst, ',', ' ')
		}
		written++
		var err error
		if dst, err = appendKind(dst, fv, f.enc, f.elem, f.kind, at.member(f.name)); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

// appendKind writes one value using the shape the plan already decided, so
// nothing re-derives a reflect.Type per record.
func appendKind(dst []byte, rv reflect.Value, k, elem encKind, tkind string, at pathAt) ([]byte, error) {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return append(dst, 'N'), nil
		}
		rv = rv.Elem()
	}
	switch k {
	case encString:
		return document.AppendString(dst, rv.String()), nil
	case encBool:
		if rv.Bool() {
			return append(dst, 'T'), nil
		}
		return append(dst, 'F'), nil
	case encInt:
		n := rv.Int()
		if n > maxSafeInt || n < -maxSafeInt {
			return nil, &MarshalError{Path: at.String(), Msg: intOverflowMsg(n)}
		}
		return document.AppendNumber(dst, float64(n)), nil
	case encUint:
		n := rv.Uint()
		if n > maxSafeInt {
			return nil, &MarshalError{Path: at.String(), Msg: uintOverflowMsg(n)}
		}
		return document.AppendNumber(dst, float64(n)), nil
	case encFloat:
		return document.AppendNumber(dst, rv.Float()), nil
	case encSlice:
		if rv.Type() == bytesType {
			break // []byte is binary, not an array of numbers
		}
		dst = append(dst, '[')
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			var err error
			if dst, err = appendKind(dst, rv.Index(i), elem, encOther, "", at.elem(i)); err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	}
	// The remaining kinds are rare enough to keep the reflective form.
	return appendFastValue(dst, rv, tkind, at)
}

// appendFastValue writes one Go value through the shared spelling helpers.
func appendFastValue(dst []byte, rv reflect.Value, kind string, at pathAt) ([]byte, error) {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return append(dst, 'N'), nil
		}
		rv = rv.Elem()
	}
	t := rv.Type()
	switch t {
	case bigIntType.Elem():
		bi := rv.Interface().(big.Int)
		return append(bi.Append(dst, 10), 'n'), nil
	case decimalType:
		d := rv.Interface().(Decimal)
		return append(append(dst, d.String()...), 'm'), nil
	case timeType:
		k := "datetime"
		if kind == "date" || kind == "time" {
			k = kind
		}
		return document.AppendTemporalValue(dst, rv.Interface().(time.Time).UTC(), k), nil
	case bytesType:
		dst = append(dst, 'b', '"')
		dst = base64.StdEncoding.AppendEncode(dst, rv.Bytes())
		return append(dst, '"'), nil
	}

	switch rv.Kind() {
	case reflect.String:
		return document.AppendString(dst, rv.String()), nil
	case reflect.Bool:
		if rv.Bool() {
			return append(dst, 'T'), nil
		}
		return append(dst, 'F'), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		if n > maxSafeInt || n < -maxSafeInt {
			return nil, &MarshalError{Path: at.String(), Msg: intOverflowMsg(n)}
		}
		return document.AppendNumber(dst, float64(n)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := rv.Uint()
		if n > maxSafeInt {
			return nil, &MarshalError{Path: at.String(), Msg: uintOverflowMsg(n)}
		}
		return document.AppendNumber(dst, float64(n)), nil
	case reflect.Float32, reflect.Float64:
		return document.AppendNumber(dst, rv.Float()), nil
	case reflect.Slice, reflect.Array:
		dst = append(dst, '[')
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			var err error
			if dst, err = appendFastValue(dst, rv.Index(i), "", at.elem(i)); err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	}
	return nil, &MarshalError{Path: at.String(), Msg: "unsupported type " + t.String()}
}

// noFastPath forces the general path. It exists for the differential test,
// which encodes the same value both ways and requires the two to be
// byte-identical (export_test.go, WithTreeEncode).
//
// An atomic flag, so a test can flip it mid-run without this path paying for
// os.Getenv — a lock and a map lookup — on every Marshal, which is what it did
// before 2026-09-14 to make the same flip work.
var noFastPath atomic.Bool

func init() { noFastPath.Store(os.Getenv("IO_NO_FAST_PATH") != "") }

// marshalFast renders v without the intermediate tree, or reports notFast so
// the caller uses the general path.
func marshalFast(rv reflect.Value) (string, bool, error) {
	if noFastPath.Load() {
		return "", false, nil
	}
	var et reflect.Type
	collection := false
	switch {
	case rv.Kind() == reflect.Struct && !isModelStruct(rv.Type()):
		et = rv.Type()
	case rv.Kind() == reflect.Slice && isStructElem(rv.Type().Elem()):
		et = rv.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		collection = true
	default:
		return "", false, nil
	}

	plan, err := planFor(et)
	if err != nil {
		return "", false, err
	}
	if !plan.fastOK {
		return "", false, nil
	}

	n := 1
	if collection {
		n = rv.Len()
	}
	dst := make([]byte, 0, len(plan.header)+8+48*n)
	dst = append(dst, plan.header...)

	if !collection {
		if dst, err = appendFastRecord(dst, rv, plan, plan.checks, rootPath); err != nil {
			if err == errFastDecline {
				return "", false, nil
			}
			return "", true, err
		}
		return string(dst), true, nil
	}
	for i := 0; i < rv.Len(); i++ {
		ev := rv.Index(i)
		path := rootPath.record(i)
		for ev.Kind() == reflect.Pointer {
			if ev.IsNil() {
				return "", true, &MarshalError{Path: path.String(), Msg: "a collection record cannot be nil"}
			}
			ev = ev.Elem()
		}
		if i > 0 {
			dst = append(dst, '\n')
		}
		dst = append(dst, '~', ' ')
		if dst, err = appendFastRecord(dst, ev, plan, plan.checks, path); err != nil {
			if err == errFastDecline {
				return "", false, nil
			}
			return "", true, err
		}
	}
	return string(dst), true, nil
}
