package internetobject

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
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
	// Simple shapes decode straight from token spans, with no value tree
	// built at all (ADR 0007). Anything else — and anything the lazy path is
	// not certain about — takes the general path below.
	if took, err := unmarshalLazy(src, v); took {
		return err
	}
	return bindDoc(document.Parse(src), v)
}

// bindDoc binds a loaded document into v — the shared tail of Unmarshal and
// UnmarshalWith, so the two differ only in which schema validated the load.
func bindDoc(doc *document.Doc, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &UnmarshalError{Path: "$", Msg: "target must be a non-nil pointer"}
	}
	elem := rv.Elem()
	// A row fault does not fail the load when the section it came from binds
	// to a tolerant Collection[T] — that type exists to absorb it (ADR 0004
	// D4). Any other fault still fails, and so does a row fault in a section
	// bound strictly, which is why this asks per SECTION rather than globally.
	if len(doc.Errors) > 0 && !absorbedByCollections(doc, elem) {
		return toErrorList(doc.Errors)
	}
	switch {
	case elem.Kind() == reflect.Slice && isStructElem(elem.Type().Elem()):
		// A document carrying SEVERAL sections carries several entity types,
		// and flattening them into one slice binds an Alert into an Employee
		// as a zero value with no error at all — silent corruption, on the
		// format's own headline capability. Name the sections instead.
		if n := len(doc.Sections); n > 1 {
			return &UnmarshalError{Path: "$", Msg: fmt.Sprintf(
				"document has %d sections (%s); unmarshal into a struct with "+
					"section-named fields, or take one with io.SectionAs[T]",
				n, sectionNames(doc))}
		}
		records := allRecords(doc)
		out := reflect.MakeSlice(elem.Type(), len(records), len(records))
		for i, rec := range records {
			if err := bindInto(out.Index(i), rec, rootPath.record(i)); err != nil {
				return err
			}
		}
		elem.Set(out)
		return nil

	case elem.Kind() == reflect.Struct && !isModelStruct(elem.Type()):
		// A struct whose `io` tags name sections binds the WHOLE document:
		// one field per section, no ceremony. Chosen only when a tag actually
		// names a section the document has, so a record struct is unaffected.
		fields, err := sectionBinding(elem.Type(), doc)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			return bindSections(doc, elem, fields)
		}
		records := allRecords(doc)
		if len(records) != 1 {
			return &UnmarshalError{Path: "$",
				Msg: fmt.Sprintf("document holds %d records; unmarshal into a slice", len(records))}
		}
		return bindInto(elem, records[0], rootPath)

	default:
		return setValue(elem, doc.Project(), rootPath)
	}
}

// UnmarshalError is a binding fault: the value at Path cannot be stored in
// the target's Go type. Wire-level faults return as the ErrorList instead.
type UnmarshalError struct {
	Path string
	Msg  string
}

func (e *UnmarshalError) Error() string { return e.Path + ": " + e.Msg }

// sectionNames lists a document's section names for an error message.
func sectionNames(doc *document.Doc) string {
	names := make([]string, 0, len(doc.Sections))
	for _, sec := range doc.Sections {
		n := sec.Name
		if n == "" {
			n = DefaultSectionName
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// allRecords collects every record in document order.
func allRecords(doc *document.Doc) []*core.Object {
	var out []*core.Object
	for _, sec := range doc.Sections {
		for _, rec := range sec.Records {
			if obj, ok := rec.(*core.Object); ok {
				out = append(out, obj)
			}
		}
	}
	return out
}

// bindInto binds one record to a struct value (through pointers).
func bindInto(rv reflect.Value, rec *core.Object, at pathAt) error {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	plan, err := planFor(rv.Type())
	if err != nil {
		return &UnmarshalError{Path: at.String(), Msg: err.Error()}
	}
	return bindStruct(rv, rec, plan, at)
}

// bindStruct maps a record's members onto struct fields: keyed members by
// member name, positional members by position (the schema-less form). Members
// with no matching field are ignored, like encoding/json.
func bindStruct(rv reflect.Value, rec *core.Object, plan *structPlan, at pathAt) error {
	// One pass over the record, no per-record maps: a keyed member finds its
	// field through the plan's name index (built once per type), a positional
	// one through its own position. Binding runs per record, so the two maps
	// this used to build were the hottest allocation in the decode path.
	for i, m := range rec.Members {
		if m.Absent {
			continue
		}
		fi := -1
		if m.Positional || m.Key == strconv.Itoa(i) {
			if i < len(plan.fields) {
				fi = i
			}
		} else if j, ok := plan.byName[m.Key]; ok {
			fi = j
		}
		if fi < 0 {
			continue // a member with no field: ignored, as encoding/json does
		}
		f := plan.fields[fi]
		if err := setValue(rv.FieldByIndex(f.index), m.Value, at.member(f.name)); err != nil {
			return err
		}
	}
	return nil
}

// setValue stores one wire value into a Go value, converting where the
// conversion is exact and refusing where it is not.
func setValue(rv reflect.Value, v any, at pathAt) error {
	if v == nil {
		rv.SetZero() // null: pointers become nil, everything else its zero
		return nil
	}
	if ev, ok := v.(core.ErrorValue); ok {
		return &UnmarshalError{Path: at.String(), Msg: "value carries the deferred error " + string(ev.Code)}
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
				return typeMismatch(at, v, t)
			}
			rv.Set(reflect.ValueOf(*big.NewInt(int64(x))))
			return nil
		}
		return typeMismatch(at, v, t)
	case t == decimalType:
		if d, ok := v.(Decimal); ok {
			rv.Set(reflect.ValueOf(d))
			return nil
		}
		return typeMismatch(at, v, t)
	case t == timeType:
		if tm, ok := v.(time.Time); ok {
			rv.Set(reflect.ValueOf(tm.UTC()))
			return nil
		}
		return typeMismatch(at, v, t)
	case t == bytesType:
		if b, ok := v.([]byte); ok {
			rv.SetBytes(append([]byte(nil), b...))
			return nil
		}
		return typeMismatch(at, v, t)
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
				if err := setValue(out.Index(i), e, at.elem(i)); err != nil {
					return err
				}
			}
			rv.Set(out)
			return nil
		}
	case reflect.Map:
		if obj, ok := v.(*core.Object); ok && t.Key().Kind() == reflect.String {
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
				if err := setValue(ev, m.Value, at.deeper().member(key)); err != nil {
					return err
				}
				out.SetMapIndex(reflect.ValueOf(key), ev)
			}
			rv.Set(out)
			return nil
		}
	case reflect.Struct:
		if obj, ok := v.(*core.Object); ok {
			return bindInto(rv, obj, at.deeper())
		}
	}
	return typeMismatch(at, v, t)
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

func typeMismatch(at pathAt, v any, t reflect.Type) error {
	return &UnmarshalError{Path: at.String(), Msg: fmt.Sprintf("cannot store %T in %s", v, t)}
}

// absorbedByCollections reports whether every fault the document carries
// belongs to a section this target binds tolerantly.
//
// It leans on the per-section error attribution the document records as it
// parses (ADR 0005 D7): without that, a fault could only be matched to its
// section by guessing from its path, and two sections report the same paths.
func absorbedByCollections(doc *document.Doc, elem reflect.Value) bool {
	if elem.Kind() != reflect.Struct || isModelStruct(elem.Type()) {
		return false
	}
	fields, err := sectionBinding(elem.Type(), doc)
	if err != nil || len(fields) == 0 {
		return false
	}
	accounted := 0
	for _, sec := range doc.Sections {
		secErrs := doc.SecErrors[sec]
		if len(secErrs) == 0 {
			continue
		}
		name := sec.Name
		if name == "" {
			name = DefaultSectionName
		}
		f, ok := fields[name]
		if !ok {
			return false // a fault in a section nothing binds
		}
		fv := elem.FieldByIndex(f.index)
		if !fv.CanAddr() {
			return false
		}
		if _, isColl := fv.Addr().Interface().(collectionBinder); !isColl {
			return false // that section binds strictly, so its fault stands
		}
		accounted += len(secErrs)
	}
	// A fault attributed to no section at all — a header or binding fault — is
	// never a row fault, so it is never absorbed.
	return accounted == len(doc.Errors)
}
