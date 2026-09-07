package internetobject

import (
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/parser"
)

// Struct marshaling (ADR 0003): Marshal derives an Internet Object schema
// from the struct type, writes it as the document header, and emits the data
// positionally — the format's leanness for free. The `io` struct tag follows
// encoding/json's grammar: `io:"name,omitempty"`, `io:"-"`. A pointer field
// is the nullable marker. `io:",date"` / `io:",time"` select a time.Time
// field's temporal kind.

// Marshal renders v as canonical Internet Object text.
//
// A RECORD - a struct, a map with string keys, or an *Object - marshals as one
// row; a SLICE OF RECORDS as a `~`-collection. Only structs derive a schema
// header, since only they declare types; maps and Objects are written
// header-less. Scalars and other values marshal as a schema-less record.
//
// Dispatch is on the slice's ELEMENT TYPE, never on what the elements happen
// to hold: `[]map[string]any` is always a collection and `[]any` is always an
// array, so an empty slice writes the same shape as a full one.
func Marshal(v any) (string, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return "", &MarshalError{Path: "$", Msg: "cannot marshal a nil value"}
		}
		rv = rv.Elem()
	}

	// Simple types skip the intermediate tree entirely; the spelling rules are
	// the same shared helpers either way (see marshal_fast.go).
	if text, took, err := marshalFast(rv); took {
		return text, err
	}

	var pdoc *parser.Document
	switch {
	case rv.Kind() == reflect.Struct && !isModelStruct(rv.Type()):
		plan, err := planFor(rv.Type())
		if err != nil {
			return "", err
		}
		rec, err := encodeStruct(rv, plan, rootPath)
		if err != nil {
			return "", err
		}
		if plan.validate {
			if err := checkRecords(plan.compiled, []any{rec}); err != nil {
				return "", err
			}
		}
		pdoc = schemaDoc(plan.shape, &parser.Section{Name: "data", Records: []any{rec}})

	case rv.Kind() == reflect.Slice && isStructElem(rv.Type().Elem()):
		et := rv.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		plan, err := planFor(et)
		if err != nil {
			return "", err
		}
		sec := &parser.Section{Name: "data", Collection: true}
		for i := 0; i < rv.Len(); i++ {
			ev := rv.Index(i)
			path := rootPath.record(i)
			for ev.Kind() == reflect.Pointer {
				if ev.IsNil() {
					return "", &MarshalError{Path: path.String(), Msg: "a collection record cannot be nil"}
				}
				ev = ev.Elem()
			}
			rec, err := encodeStruct(ev, plan, path)
			if err != nil {
				return "", err
			}
			sec.Records = append(sec.Records, rec)
		}
		if plan.validate {
			if err := checkRecords(plan.compiled, sec.Records); err != nil {
				return "", err
			}
		}
		pdoc = schemaDoc(plan.shape, sec)

	// A slice of MAPS or OBJECTS is a collection too. Only a slice of structs
	// derives a schema, so this writes a header-less one - but a list of
	// records is a list of records however the records are spelled, and
	// writing it as an array inside a single row (what used to happen) does
	// not even survive Unmarshal, which correctly expects a collection.
	case rv.Kind() == reflect.Slice && isRecordType(rv.Type().Elem()):
		sec := &parser.Section{Name: "data", Collection: true}
		for i := 0; i < rv.Len(); i++ {
			path := rootPath.record(i)
			el := rv.Index(i)
			// A nil map encodes as an empty record and a nil pointer as no
			// record at all; in a collection BOTH are the absence of a row,
			// which the format cannot write. Refuse them the same way the
			// struct path does rather than emitting a silent empty row.
			if isNilRecord(el) {
				return "", &MarshalError{Path: path.String(), Msg: "a collection record cannot be nil"}
			}
			ev, err := encodeValue(el, "", path)
			if err != nil {
				return "", err
			}
			rec, ok := ev.(*core.Object)
			if !ok {
				return "", &MarshalError{Path: path.String(), Msg: "a collection record cannot be nil"}
			}
			sec.Records = append(sec.Records, rec)
		}
		pdoc = &parser.Document{Sections: []*parser.Section{sec}}

	default:
		ev, err := encodeValue(rv, "", rootPath)
		if err != nil {
			return "", err
		}
		rec, ok := ev.(*core.Object)
		if !ok {
			rec = &core.Object{Members: []core.Member{{Positional: true, Value: ev}}}
		}
		pdoc = &parser.Document{Sections: []*parser.Section{
			{Name: "data", Records: []any{rec}},
		}}
	}

	doc, cerr := document.NewUnvalidated(pdoc)
	if cerr != nil {
		return "", &MarshalError{Path: "$", Msg: "derived schema does not compile: " + cerr.Code}
	}
	return doc.String(), nil
}

func encodeStruct(rv reflect.Value, plan *structPlan, at pathAt) (*core.Object, error) {
	out := &core.Object{Members: make([]core.Member, 0, len(plan.fields))}
	for _, f := range plan.fields {
		fv := rv.FieldByIndex(f.index)
		if f.omitZero && fv.IsZero() {
			continue
		}
		ev, err := encodeValue(fv, f.kind, at.member(f.name))
		if err != nil {
			return nil, err
		}
		out.Members = append(out.Members, core.Member{Key: f.name, Value: ev})
	}
	return out, nil
}

// encodeValue converts one Go value to the wire value model. Integer values
// beyond 2^53 are refused rather than silently rounded.
// encodeObject normalizes an Object a CALLER built: its member values are
// whatever Go types were stored, and the writer only speaks the format's own.
// Keys, order and positional-ness are preserved exactly.
func encodeObject(o *core.Object, at pathAt) (*core.Object, error) {
	out := &core.Object{Members: make([]core.Member, len(o.Members)), Line: o.Line, Col: o.Col}
	for i, m := range o.Members {
		out.Members[i] = m
		if m.Absent || m.Value == nil {
			continue
		}
		where := at.deeper().member(m.Key)
		if m.Positional {
			where = at.elem(i)
		}
		ev, err := encodeValue(reflect.ValueOf(m.Value), "", where)
		if err != nil {
			return nil, err
		}
		out.Members[i].Value = ev
	}
	return out, nil
}

func encodeValue(rv reflect.Value, kind string, at pathAt) (any, error) {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}
	t := rv.Type()
	switch {
	case t == bigIntType.Elem():
		bi := rv.Interface().(big.Int)
		return new(big.Int).Set(&bi), nil
	case t == decimalType:
		d := rv.Interface().(Decimal)
		coef := new(big.Int)
		if d.Coef != nil {
			coef.Set(d.Coef)
		}
		return Decimal{Coef: coef, Scale: d.Scale}, nil
	case t == timeType:
		// A temporal value IS a time.Time; the `,date` / `,time` tag chooses
		// the spelling, which the writer applies from the schema.
		return rv.Interface().(time.Time).UTC(), nil
	case t == bytesType:
		return append([]byte(nil), rv.Bytes()...), nil
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.String(), nil
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		if n > maxSafeInt || n < -maxSafeInt {
			return nil, &MarshalError{Path: at.String(), Msg: intOverflowMsg(n)}
		}
		return float64(n), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := rv.Uint()
		if n > maxSafeInt {
			return nil, &MarshalError{Path: at.String(), Msg: uintOverflowMsg(n)}
		}
		return float64(n), nil
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			ev, err := encodeValue(rv.Index(i), "", at.elem(i))
			if err != nil {
				return nil, err
			}
			out[i] = ev
		}
		return out, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, &MarshalError{Path: at.String(), Msg: "map keys must be strings"}
		}
		keys := make([]string, 0, rv.Len())
		for _, k := range rv.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys) // deterministic output
		out := &core.Object{}
		for _, k := range keys {
			ev, err := encodeValue(rv.MapIndex(reflect.ValueOf(k)), "", at.deeper().member(k))
			if err != nil {
				return nil, err
			}
			out.Members = append(out.Members, core.Member{Key: k, Value: ev})
		}
		return out, nil
	case reflect.Struct:
		// The format's OWN record type is a value, not a shape to reflect
		// over: its fields are representation. Normalize its members instead,
		// so a Go int a caller stored reaches the writer as a number.
		if t == objectType {
			o := rv.Interface().(core.Object) // rv may not be addressable
			return encodeObject(&o, at)
		}
		plan, err := planFor(t)
		if err != nil {
			return nil, err
		}
		return encodeStruct(rv, plan, at.deeper())
	}
	return nil, &MarshalError{Path: at.String(), Msg: "unsupported type " + t.String()}
}

// isNilRecord reports an element that carries no record at all: a nil pointer,
// a nil map, or an interface holding either.
func isNilRecord(rv reflect.Value) bool {
	for rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map:
		return rv.IsNil()
	}
	return false
}

// schemaDoc assembles a document with the derived schema as its header.
func schemaDoc(shape *core.Object, sec *parser.Section) *parser.Document {
	header := &parser.Header{
		Schemas: map[string]any{"schema": shape},
		Defs:    []parser.HeaderDef{{Kind: parser.DefSchema, Key: "schema", Value: shape}},
	}
	return &parser.Document{Header: header, Sections: []*parser.Section{sec}}
}

// ── value encoding ─────────────────────────────────────────────────────────

// maxSafeInt is the largest integer the number wire type holds exactly.
const maxSafeInt = 1 << 53

// intOverflowMsg and uintOverflowMsg are shared by both encode paths so the
// two report a refusal identically.
func intOverflowMsg(n int64) string {
	return fmt.Sprintf("%d overflows the int wire type (use *big.Int)", n)
}

func uintOverflowMsg(n uint64) string {
	return fmt.Sprintf("%d overflows the int wire type (use *big.Int)", n)
}
