package internetobject

import (
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Struct marshaling (ADR 0003): Marshal derives an Internet Object schema
// from the struct type, writes it as the document header, and emits the data
// positionally — the format's leanness for free. The `io` struct tag follows
// encoding/json's grammar: `io:"name,omitempty"`, `io:"-"`. A pointer field
// is the nullable marker. `io:",date"` / `io:",time"` select a time.Time
// field's temporal kind.

// Marshal renders v as canonical Internet Object text.
//
// A struct marshals as a schema header plus one record; a slice of structs as
// a schema header plus a `~`-collection. Maps, scalars and other values
// marshal as a schema-less record.
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

// MarshalError is a binding fault found while marshaling: the field path and
// what went wrong. Wire-level faults never occur on marshal — the writer is
// total over what encode produces.
type MarshalError struct {
	Path string
	Msg  string
}

func (e *MarshalError) Error() string { return e.Path + ": " + e.Msg }

// ── field plans ────────────────────────────────────────────────────────────

type structPlan struct {
	fields   []fieldPlan
	byName   map[string]int // member name → index in fields, built once
	shape    *core.Object   // the derived schema shape, as parsed text would be
	compiled *schema.Schema // the shape, compiled once
	validate bool           // any field (own or nested) carries a `schema` tag
	fastOK   bool           // every member can be WRITTEN without the tree
	lazyOK   bool           // every member can be READ from a token span
}

// encKind is a field's wire shape, decided ONCE when the plan is built. The
// encoder switches on it instead of comparing reflect.Types per value, which
// the CPU profile showed as runtime.ifaceeq plus reflect.Elem on every member
// of every record (ADR 0006 F1: compile the shape, then execute it).
type encKind uint8

const (
	encOther encKind = iota
	encString
	encBool
	encInt
	encUint
	encFloat
	encBigInt
	encDecimal
	encTime
	encBytes
	encSlice
)

// encKindOf classifies a type once, at plan-build time.
func encKindOf(t reflect.Type) encKind {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case bigIntElemType:
		return encBigInt
	case decimalType:
		return encDecimal
	case timeType:
		return encTime
	case bytesType:
		return encBytes
	}
	switch t.Kind() {
	case reflect.String:
		return encString
	case reflect.Bool:
		return encBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return encUint
	case reflect.Float32, reflect.Float64:
		return encFloat
	case reflect.Slice, reflect.Array:
		return encSlice
	}
	return encOther
}

type fieldPlan struct {
	name     string
	index    []int // reflect index path (embedded fields included)
	at       int   // the single index when index has depth 1, else -1
	enc      encKind
	elem     encKind // for encSlice: the element's kind
	optional bool    // ,optional or ,omitempty: the member compiles as `name?`
	omitZero bool    // ,omitempty only: the zero value is left off the wire
	nullable bool    // pointer field
	kind     string
}

var planCache sync.Map // reflect.Type → *structPlan

func planFor(t reflect.Type) (*structPlan, error) {
	if p, ok := planCache.Load(t); ok {
		return p.(*structPlan), nil
	}
	p, err := buildPlan(t, map[reflect.Type]bool{})
	if err != nil {
		return nil, err
	}
	planCache.Store(t, p)
	return p, nil
}

func buildPlan(t reflect.Type, visiting map[reflect.Type]bool) (*structPlan, error) {
	if visiting[t] {
		return nil, &MarshalError{Path: t.String(), Msg: "recursive struct types are not supported"}
	}
	visiting[t] = true
	defer delete(visiting, t)

	p := &structPlan{shape: &core.Object{}}
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || f.Anonymous {
			continue // embedded structs contribute through their visible fields
		}
		name, opts, skip := parseTag(f)
		if skip {
			continue
		}
		fp := fieldPlan{
			name:  name,
			index: f.Index,
			at:    -1,
			// `optional` is the schema fact (the member may be absent — IO's
			// `name?`); `omitempty` is the json-familiar encoding behavior
			// (skip the zero value on output), which requires optionality.
			optional: opts["optional"] || opts["omitempty"],
			omitZero: opts["omitempty"],
			nullable: f.Type.Kind() == reflect.Pointer,
		}
		switch {
		case opts["date"]:
			fp.kind = "date"
		case opts["time"]:
			fp.kind = "time"
		}
		var ann any
		var err error
		if tag, ok := f.Tag.Lookup("schema"); ok {
			// The `schema` tag holds the member's IO type annotation verbatim
			// — exactly what a schema would carry after `name:`.
			ann, err = annotationShape(tag, t.String()+"."+f.Name)
			p.validate = true
		} else {
			ann, err = annotationFor(f.Type, fp.kind, visiting, &p.validate)
		}
		if err != nil {
			return nil, err
		}
		if isPlainMemberName(fp.name) {
			key := fp.name
			if fp.optional {
				key += "?"
			}
			if fp.nullable {
				key += "*"
			}
			p.shape.Members = append(p.shape.Members, core.Member{Key: key, Value: ann})
		} else {
			// A name needing quotes cannot carry the short markers; the flags
			// move into the object-form typedef.
			if fp.optional || fp.nullable {
				ann = objectFormWithFlags(ann, fp.optional, fp.nullable)
			}
			p.shape.Members = append(p.shape.Members, core.Member{Key: fp.name, Quoted: true, Value: ann})
		}
		if len(f.Index) == 1 {
			fp.at = f.Index[0] // the common case: one cheap field lookup
		}
		fp.enc = encKindOf(f.Type)
		if fp.enc == encSlice {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			fp.elem = encKindOf(ft.Elem())
		}
		p.fields = append(p.fields, fp)
	}
	p.byName = make(map[string]int, len(p.fields))
	for i, f := range p.fields {
		p.byName[f.name] = i
	}
	compiled, cerr := schema.Compile(p.shape, "")
	if cerr != nil {
		return nil, &MarshalError{Path: t.String(), Msg: "derived schema does not compile: " + cerr.Code}
	}
	p.compiled = compiled
	p.fastOK = fastEligible(t, p)
	p.lazyOK = lazyEligible(p)
	return p, nil
}

// annotationShape parses a `schema` struct tag: the member's type annotation
// in the format's own syntax (`{int, min: 0, max: 130}`, `[string]`, `$Ref`,
// `{string, choices: [a, b]}`). An unbraced constraint list is braced for
// convenience, so `schema:"int, min: 0"` also works. The shape is compiled
// immediately so a bad tag fails at the type's first use with the designated
// code.
func annotationShape(tag, fieldPath string) (any, error) {
	text := strings.TrimSpace(tag)
	if text == "" {
		return nil, &MarshalError{Path: fieldPath, Msg: "empty schema tag"}
	}
	if text[0] != '{' && text[0] != '[' && strings.ContainsRune(text, ',') {
		text = "{" + text + "}"
	}
	pdoc := parser.Parse("x: " + text)
	if len(pdoc.Errors) > 0 {
		return nil, &MarshalError{Path: fieldPath, Msg: "invalid schema tag: " + pdoc.Errors[0].Code}
	}
	var rec *core.Object
	if len(pdoc.Sections) == 1 && len(pdoc.Sections[0].Records) == 1 {
		rec, _ = pdoc.Sections[0].Records[0].(*core.Object)
	}
	if rec == nil || len(rec.Members) != 1 || rec.Members[0].Key != "x" {
		return nil, &MarshalError{Path: fieldPath, Msg: "invalid schema tag: not a single type annotation"}
	}
	shape := rec.Members[0].Value
	if _, cerr := schema.Compile(&core.Object{Members: []core.Member{{Key: "x", Value: shape}}}, ""); cerr != nil {
		return nil, &MarshalError{Path: fieldPath, Msg: "invalid schema tag: " + cerr.Code}
	}
	return shape, nil
}

func parseTag(f reflect.StructField) (name string, opts map[string]bool, skip bool) {
	tag := f.Tag.Get("io")
	if tag == "-" {
		return "", nil, true
	}
	name = f.Name
	opts = map[string]bool{}
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	}
	for _, o := range parts[1:] {
		opts[o] = true
	}
	return name, opts, false
}

// objectFormWithFlags rewrites a type annotation as the object-form typedef
// carrying explicit optional/"null" flags — the only spelling a QUOTED member
// name can use (quoted names never strip `?`/`*` markers).
func objectFormWithFlags(ann any, optional, nullable bool) *core.Object {
	out := &core.Object{}
	switch tv := ann.(type) {
	case string:
		out.Members = append(out.Members, core.Member{Positional: true, Value: tv})
	case []any:
		out.Members = append(out.Members,
			core.Member{Positional: true, Value: "array"})
		var elem any = "any"
		if len(tv) == 1 {
			elem = tv[0]
		}
		out.Members = append(out.Members, core.Member{Key: "of", Value: elem})
	case *core.Object:
		out.Members = append(out.Members,
			core.Member{Positional: true, Value: "object"},
			core.Member{Key: "schema", Value: tv})
	}
	if optional {
		out.Members = append(out.Members, core.Member{Key: "optional", Value: true})
	}
	if nullable {
		out.Members = append(out.Members, core.Member{Key: "null", Quoted: true, Value: true})
	}
	return out
}

// isPlainMemberName reports a name that survives the bare-key grammar with
// optional/null markers appended (markers are stripped from BARE keys only).
func isPlainMemberName(s string) bool {
	if s == "" || s == "*" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return !(s[0] >= '0' && s[0] <= '9')
}

// ── schema derivation ──────────────────────────────────────────────────────

var (
	bigIntType = reflect.TypeOf((*big.Int)(nil))
	// bigIntElemType is big.Int itself, hoisted out of the hot path: calling
	// bigIntType.Elem() per value showed up in the profile.
	bigIntElemType = reflect.TypeOf(big.Int{})
	decimalType    = reflect.TypeOf(Decimal{})
	timeType       = reflect.TypeOf(time.Time{})
	bytesType      = reflect.TypeOf([]byte(nil))
	anyType        = reflect.TypeOf((*any)(nil)).Elem()
)

// annotationFor derives the IO type annotation for one Go type — the value a
// parsed schema would hold in that member position. tagged is set when the
// subtree carries a `schema` tag anywhere, so the owning plan validates.
func annotationFor(t reflect.Type, kind string, visiting map[reflect.Type]bool, tagged *bool) (any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == bigIntType.Elem():
		return "bigint", nil
	case t == decimalType:
		return "decimal", nil
	case t == timeType:
		if kind != "" {
			return kind, nil
		}
		return "datetime", nil
	case t == bytesType, t == anyType:
		// Binary is a value-level fact with no schema type of its own, so the
		// derived schema admits it as `any`.
		return "any", nil
	}
	switch t.Kind() {
	case reflect.String:
		return "string", nil
	case reflect.Bool:
		return "bool", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int", nil
	case reflect.Float32, reflect.Float64:
		return "number", nil
	case reflect.Slice, reflect.Array:
		elem, err := annotationFor(t.Elem(), "", visiting, tagged)
		if err != nil {
			return nil, err
		}
		return []any{elem}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, &MarshalError{Path: t.String(), Msg: "map keys must be strings"}
		}
		elem, err := annotationFor(t.Elem(), "", visiting, tagged)
		if err != nil {
			return nil, err
		}
		return &core.Object{Members: []core.Member{{Key: "*", Value: elem}}}, nil
	case reflect.Struct:
		sub, err := buildPlan(t, visiting)
		if err != nil {
			return nil, err
		}
		if sub.validate {
			*tagged = true
		}
		return sub.shape, nil
	case reflect.Interface:
		return "any", nil
	}
	return nil, &MarshalError{Path: t.String(), Msg: "unsupported type"}
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

// pathAt names a position in the value being encoded or decoded WITHOUT
// building the string. Errors are rare and paths are only for errors, so
// every part travels separately and they are joined exactly once, in
// String(), when a fault is actually reported.
//
// This shape was arrived at by measurement, and the two failures are worth
// recording: a linked list of parent POINTERS escapes (+6,000 allocations on
// encode), and joining lazily at each level turns one join per record into
// one per member (+9,500 on decode). Carrying the four parts flat is what
// finally cost nothing — before it, path building was 99.4% of encode's
// remaining allocations, all of it discarded (ADR 0006 P1).
type pathAt struct {
	root  string // the enclosing path, "$" at the top
	rec   int    // record index within a collection, -1 when not in one
	name  string // member name, "" when none
	index int    // element index within the member, -1 when not an element
}

// rootPath is the document root: the parent of every top-level record.
var rootPath = pathAt{root: "$", rec: -1, index: -1}

func (p pathAt) String() string {
	// Sized once: the parts are short and this runs only on a fault.
	var b strings.Builder
	b.Grow(len(p.root) + len(p.name) + 12)
	b.WriteString(p.root)
	if p.rec >= 0 {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(p.rec))
		b.WriteByte(']')
	}
	if p.name != "" {
		b.WriteByte('.')
		b.WriteString(p.name)
	}
	if p.index >= 0 {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(p.index))
		b.WriteByte(']')
	}
	return b.String()
}

// record names the i-th record of a collection.
func (p pathAt) record(i int) pathAt { p.rec = i; return p }

// member names a member inside p.
func (p pathAt) member(name string) pathAt { p.name = name; p.index = -1; return p }

// elem names the i-th element of p's member.
func (p pathAt) elem(i int) pathAt { p.index = i; return p }

// deeper descends past what the flat form can express — a nested object or
// map — by joining ONCE and starting fresh. Only containers pay for this.
func (p pathAt) deeper() pathAt {
	return pathAt{root: p.String(), rec: -1, index: -1}
}

// intOverflowMsg and uintOverflowMsg are shared by both encode paths so the
// two report a refusal identically.
func intOverflowMsg(n int64) string {
	return fmt.Sprintf("%d overflows the int wire type (use *big.Int)", n)
}

func uintOverflowMsg(n uint64) string {
	return fmt.Sprintf("%d overflows the int wire type (use *big.Int)", n)
}

// maxSafeInt is the largest integer the number wire type holds exactly.
const maxSafeInt = 1 << 53

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
		plan, err := planFor(t)
		if err != nil {
			return nil, err
		}
		return encodeStruct(rv, plan, at.deeper())
	}
	return nil, &MarshalError{Path: at.String(), Msg: "unsupported type " + t.String()}
}

// isModelStruct reports the value-model structs, which marshal as VALUES, not
// as records with fields.
func isModelStruct(t reflect.Type) bool {
	return t == decimalType || t == timeType
}

func isStructElem(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && !isModelStruct(t)
}
