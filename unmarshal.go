package internetobject

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// Unmarshal parses src and stores the result in the value pointed to by v:
// a *struct for a single record, a *[]T for a collection, or *map[string]any
// / *any for the dynamic projection.
//
// A document with a schema header validates as usual — designated-code faults
// return as the ErrorList and nothing is stored. A schema-less record binds
// positionally by field order and by key for named members, so
// `Unmarshal("Alice, 30", &p)` works without a header.
func Unmarshal(src string, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &UnmarshalError{Path: "$", Msg: "target must be a non-nil pointer"}
	}
	doc := document.Load(src)
	if len(doc.Errors) > 0 {
		return toErrorList(doc.Errors)
	}

	elem := rv.Elem()
	switch {
	case elem.Kind() == reflect.Slice && isStructElem(elem.Type().Elem()):
		records := allRecords(doc)
		out := reflect.MakeSlice(elem.Type(), len(records), len(records))
		for i, rec := range records {
			if err := bindInto(out.Index(i), rec, fmt.Sprintf("$[%d]", i)); err != nil {
				return err
			}
		}
		elem.Set(out)
		return nil

	case elem.Kind() == reflect.Struct && !isModelStruct(elem.Type()):
		records := allRecords(doc)
		if len(records) != 1 {
			return &UnmarshalError{Path: "$",
				Msg: fmt.Sprintf("document holds %d records; unmarshal into a slice", len(records))}
		}
		return bindInto(elem, records[0], "$")

	default:
		return setValue(elem, doc.Project(), "$")
	}
}

// UnmarshalError is a binding fault: the value at Path cannot be stored in
// the target's Go type. Wire-level faults return as the ErrorList instead.
type UnmarshalError struct {
	Path string
	Msg  string
}

func (e *UnmarshalError) Error() string { return e.Path + ": " + e.Msg }

// allRecords collects every record in document order.
func allRecords(doc *document.Doc) []*value.Object {
	var out []*value.Object
	for _, sec := range doc.Sections {
		for _, rec := range sec.Records {
			if obj, ok := rec.(*value.Object); ok {
				out = append(out, obj)
			}
		}
	}
	return out
}

// bindInto binds one record to a struct value (through pointers).
func bindInto(rv reflect.Value, rec *value.Object, path string) error {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	plan, err := planFor(rv.Type())
	if err != nil {
		return &UnmarshalError{Path: path, Msg: err.Error()}
	}
	return bindStruct(rv, rec, plan, path)
}

// bindStruct maps a record's members onto struct fields: keyed members by
// member name, positional members by position (the schema-less form). Members
// with no matching field are ignored, like encoding/json.
func bindStruct(rv reflect.Value, rec *value.Object, plan *structPlan, path string) error {
	byName := map[string]any{}
	byPos := map[int]any{}
	for i, m := range rec.Members {
		switch {
		case m.Absent:
		case m.Positional || m.Key == strconv.Itoa(i):
			byPos[i] = m.Value
		default:
			byName[m.Key] = m.Value
		}
	}
	for i, f := range plan.fields {
		mv, ok := byName[f.name]
		if !ok {
			mv, ok = byPos[i]
		}
		if !ok {
			continue // absent: the field keeps its zero value
		}
		if err := setValue(rv.FieldByIndex(f.index), mv, path+"."+f.name); err != nil {
			return err
		}
	}
	return nil
}

// setValue stores one wire value into a Go value, converting where the
// conversion is exact and refusing where it is not.
func setValue(rv reflect.Value, v any, path string) error {
	if v == nil {
		rv.SetZero() // null: pointers become nil, everything else its zero
		return nil
	}
	if ev, ok := v.(value.ErrorValue); ok {
		return &UnmarshalError{Path: path, Msg: "value carries the deferred error " + ev.Code}
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	t := rv.Type()

	switch {
	case t == bigIntType.Elem():
		switch x := v.(type) {
		case *big.Int:
			rv.Set(reflect.ValueOf(*new(big.Int).Set(x)))
			return nil
		case float64:
			if x != math.Trunc(x) || math.Abs(x) > maxSafeInt {
				return typeMismatch(path, v, t)
			}
			rv.Set(reflect.ValueOf(*big.NewInt(int64(x))))
			return nil
		}
		return typeMismatch(path, v, t)
	case t == decimalType:
		if d, ok := v.(Decimal); ok {
			rv.Set(reflect.ValueOf(d))
			return nil
		}
		return typeMismatch(path, v, t)
	case t == timeType:
		if tm, ok := v.(Temporal); ok {
			rv.Set(reflect.ValueOf(tm.T.UTC()))
			return nil
		}
		return typeMismatch(path, v, t)
	case t == temporalType:
		if tm, ok := v.(Temporal); ok {
			rv.Set(reflect.ValueOf(tm))
			return nil
		}
		return typeMismatch(path, v, t)
	case t == bytesType:
		if b, ok := v.([]byte); ok {
			rv.SetBytes(append([]byte(nil), b...))
			return nil
		}
		return typeMismatch(path, v, t)
	case t == anyType:
		rv.Set(reflect.ValueOf(v))
		return nil
	}

	switch rv.Kind() {
	case reflect.String:
		if s, ok := v.(string); ok {
			rv.SetString(s)
			return nil
		}
	case reflect.Bool:
		if b, ok := v.(bool); ok {
			rv.SetBool(b)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, ok := integralOf(v); ok && !rv.OverflowInt(n) {
			rv.SetInt(n)
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, ok := integralOf(v); ok && n >= 0 && !rv.OverflowUint(uint64(n)) {
			rv.SetUint(uint64(n))
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if f, ok := v.(float64); ok {
			rv.SetFloat(f)
			return nil
		}
	case reflect.Slice:
		if arr, ok := v.([]any); ok {
			out := reflect.MakeSlice(t, len(arr), len(arr))
			for i, e := range arr {
				if err := setValue(out.Index(i), e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
			rv.Set(out)
			return nil
		}
	case reflect.Map:
		if obj, ok := v.(*value.Object); ok && t.Key().Kind() == reflect.String {
			out := reflect.MakeMapWithSize(t, len(obj.Members))
			for i, m := range obj.Members {
				if m.Absent {
					continue
				}
				key := m.Key
				if m.Positional || key == "" {
					key = strconv.Itoa(i)
				}
				ev := reflect.New(t.Elem()).Elem()
				if err := setValue(ev, m.Value, path+"."+key); err != nil {
					return err
				}
				out.SetMapIndex(reflect.ValueOf(key), ev)
			}
			rv.Set(out)
			return nil
		}
	case reflect.Struct:
		if obj, ok := v.(*value.Object); ok {
			return bindInto(rv, obj, path)
		}
	}
	return typeMismatch(path, v, t)
}

// integralOf extracts an exact int64 from the numeric wire types.
func integralOf(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		if x != math.Trunc(x) || math.IsInf(x, 0) || math.IsNaN(x) {
			return 0, false
		}
		return int64(x), true
	case *big.Int:
		if x.IsInt64() {
			return x.Int64(), true
		}
	}
	return 0, false
}

func typeMismatch(path string, v any, t reflect.Type) error {
	return &UnmarshalError{Path: path, Msg: fmt.Sprintf("cannot store %T in %s", v, t)}
}
