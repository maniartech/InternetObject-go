package internetobject

import (
	"encoding/base64"
	"math/big"
	"os"
	"reflect"
	"time"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/value"
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
// member is a scalar or a slice of scalars, and whose type declares no
// `schema` constraints (those need the tree for validation anyway). Anything
// else falls back.

// fastEligible reports whether a plan's every field can be written directly.
// Computed once per type, in the plan.
func fastEligible(t reflect.Type, plan *structPlan) bool {
	if plan.validate {
		return false // constraints need the tree to validate against
	}
	for _, f := range plan.fields {
		ft := t.FieldByIndex(f.index).Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if !fastScalarType(ft) {
			if ft.Kind() != reflect.Slice || !fastScalarType(ft.Elem()) {
				return false
			}
		}
	}
	return true
}

func fastScalarType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case bigIntType.Elem(), decimalType, temporalType, timeType, bytesType:
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
func appendFastRecord(dst []byte, rv reflect.Value, plan *structPlan, at pathAt) ([]byte, error) {
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
		k := ""
		switch kind {
		case "date", "time":
			k = kind
		default:
			k = "datetime"
		}
		return document.AppendTemporalValue(dst, value.Temporal{T: rv.Interface().(time.Time).UTC()}, k), nil
	case temporalType:
		return document.AppendTemporalValue(dst, rv.Interface().(value.Temporal), ""), nil
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
// byte-identical; it is read once, not per call.
var noFastPath = os.Getenv("IO_NO_FAST_PATH") != ""

// marshalFast renders v without the intermediate tree, or reports notFast so
// the caller uses the general path.
func marshalFast(rv reflect.Value) (string, bool, error) {
	if noFastPath || os.Getenv("IO_NO_FAST_PATH") != "" {
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

	header := document.SchemaText(plan.compiled)
	n := 1
	if collection {
		n = rv.Len()
	}
	dst := make([]byte, 0, len(header)+8+48*n)
	dst = append(dst, header...)
	dst = append(dst, '\n', '-', '-', '-', '\n')

	if !collection {
		if dst, err = appendFastRecord(dst, rv, plan, rootPath); err != nil {
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
		if dst, err = appendFastRecord(dst, ev, plan, path); err != nil {
			return "", true, err
		}
	}
	return string(dst), true, nil
}
