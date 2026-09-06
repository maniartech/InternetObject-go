package internetobject

import (
	"math"
	"os"
	"reflect"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// Lazy decoding (ADR 0007 phase 2).
//
// The general path builds a boxed value tree, validates it into a second
// tree, then binds that into the caller's struct. This path never builds
// either: the header is parsed once for its schema, the data records are
// FRAMED (token spans, no values), and each member is decoded straight into
// its Go field. A decoded string is a substring of the source, so binding one
// allocates nothing at all.
//
// The type check and the decode are the same step: if a member's token is not
// a number and the schema declares `int`, that IS `expected-integer`, reported
// at the token's own line and column.
//
// It is taken only when every part is simple enough to be certain about — a
// struct or slice of structs, a schema of plain typed members, no
// constraints, defaults, choices, references or variables. Everything else
// falls back to the path that has always run, so the fallback is the
// specification and this is an optimization of it. `IO_NO_LAZY=1` forces the
// fallback, which is how the differential test holds the two identical.

var noLazy = os.Getenv("IO_NO_LAZY") != ""

// lazyEligible reports whether every field can be decoded straight from a
// token span. It is DECODE eligibility, deliberately narrower than the encode
// path's: writing a temporal or a decimal is a formatting question, reading
// one into a Go field is a conversion the general path already owns. A type
// this declines never enters the lazy path at all, so no half-bound value can
// escape it.
func lazyEligible(plan *structPlan) bool {
	for i := range plan.fields {
		switch plan.fields[i].enc {
		case encString, encBool, encInt, encUint, encFloat:
		case encSlice:
			switch plan.fields[i].elem {
			case encString, encBool, encInt, encUint, encFloat:
			default:
				return false
			}
		default:
			return false
		}
	}
	return len(plan.fields) > 0
}

// unmarshalLazy binds src into v without building a value tree. took reports
// whether it handled the call at all.
func unmarshalLazy(src string, v any) (took bool, err error) {
	if noLazy {
		return false, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false, nil
	}
	elem := rv.Elem()

	var et reflect.Type
	collection := false
	switch {
	case elem.Kind() == reflect.Slice && isStructElem(elem.Type().Elem()):
		et, collection = elem.Type().Elem(), true
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
	case elem.Kind() == reflect.Struct && !isModelStruct(elem.Type()):
		et = elem.Type()
	default:
		return false, nil
	}

	plan, perr := planFor(et)
	if perr != nil || !plan.lazyOK {
		return false, nil // only kinds this path decodes directly
	}

	f, ok := document.ParseFramed(src)
	if !ok || f.Schema == nil || !document.IsSimpleSchema(f.Schema) {
		return false, nil
	}
	names, defs := document.SchemaMemberDefs(f.Schema)
	if len(names) != len(plan.fields) {
		return false, nil // shapes differ: let the general path decide
	}
	for i, n := range names {
		if n != plan.fields[i].name {
			return false, nil
		}
	}
	_ = defs

	// Bind into a temporary and publish only on success, so a fallback can
	// never leave the caller's value half-written.
	recs := f.Raw.Records
	if !collection {
		if len(recs) != 1 {
			return false, nil
		}
		tmp := reflect.New(elem.Type()).Elem()
		if e := bindFramed(tmp, recs[0], f, plan, 0); e != nil {
			if e == errUnsupportedLazy {
				return false, nil // the general path takes it
			}
			return true, e
		}
		elem.Set(tmp)
		return true, nil
	}

	out := reflect.MakeSlice(elem.Type(), len(recs), len(recs))
	for i := range recs {
		if e := bindFramed(out.Index(i), recs[i], f, plan, i); e != nil {
			if e == errUnsupportedLazy {
				return false, nil
			}
			return true, e
		}
	}
	elem.Set(out)
	return true, nil
}

// bindFramed decodes one framed record into a struct value.
func bindFramed(rv reflect.Value, rec parser.RawRecord, f *document.Framed,
	plan *structPlan, recIndex int) error {

	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	names, defs := document.SchemaMemberDefs(f.Schema)

	for i, m := range rec.Members {
		if i >= len(plan.fields) {
			return lazyFault(errs.UnknownMember, f, m, recIndex, "")
		}
		if !m.Positional() {
			// A keyed member may name any declared member, in any order.
			j, known := plan.byName[m.Key]
			if !known {
				return lazyFault(errs.UnknownMember, f, m, recIndex, m.Key)
			}
			if e := bindMember(rv.FieldByIndex(plan.fields[j].index), m, f,
				defs[names[j]], recIndex, names[j]); e != nil {
				return e
			}
			continue
		}
		if m.Absent {
			continue // an empty slot leaves the field at its zero value
		}
		if e := bindMember(rv.FieldByIndex(plan.fields[i].index), m, f,
			defs[names[i]], recIndex, names[i]); e != nil {
			return e
		}
	}
	return nil
}

// bindMember decodes one framed member into one field, checking the schema's
// declared type as it goes.
func bindMember(field reflect.Value, m parser.RawMember, f *document.Framed,
	md *schema.MemberDef, recIndex int, name string) error {

	s := f.Stream
	tok := s.Tokens[m.Tok]

	// Null first: it is legal only where the schema allows it.
	if m.Kind == tokenizer.KindNull {
		if md != nil && !md.Null {
			return lazyFault(errs.ForbiddenNull, f, m, recIndex, name)
		}
		field.SetZero()
		return nil
	}

	declared := ""
	if md != nil {
		declared = md.Type
	}

	switch declared {
	case "string":
		if m.Kind != tokenizer.KindString {
			return lazyFault(errs.ExpectedString, f, m, recIndex, name)
		}
	case "int":
		if m.Kind != tokenizer.KindNumber {
			return lazyFault(errs.ExpectedInteger, f, m, recIndex, name)
		}
		if n := s.Number(tok); n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return lazyFault(errs.ExpectedInteger, f, m, recIndex, name)
		}
	case "number":
		if m.Kind != tokenizer.KindNumber {
			return lazyFault(errs.ExpectedNumber, f, m, recIndex, name)
		}
	case "bool":
		if m.Kind != tokenizer.KindBoolean {
			return lazyFault(errs.ExpectedBoolean, f, m, recIndex, name)
		}
	case "array":
		if m.Kind != tokenizer.KindBracketOpen {
			return lazyFault(errs.ExpectedArray, f, m, recIndex, name)
		}
	}

	// The decode IS the store: nothing is boxed on the way.
	switch field.Kind() {
	case reflect.String:
		if m.Kind != tokenizer.KindString {
			return lazyFault(errs.ExpectedString, f, m, recIndex, name)
		}
		field.SetString(s.StringValue(tok))
		return nil
	case reflect.Bool:
		if m.Kind != tokenizer.KindBoolean {
			return lazyFault(errs.ExpectedBoolean, f, m, recIndex, name)
		}
		field.SetBool(s.Bool(tok))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if m.Kind != tokenizer.KindNumber {
			return lazyFault(errs.ExpectedInteger, f, m, recIndex, name)
		}
		n := s.Number(tok)
		if n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return lazyFault(errs.ExpectedInteger, f, m, recIndex, name)
		}
		if field.OverflowInt(int64(n)) {
			return lazyFault(errs.OutOfRangeInteger, f, m, recIndex, name)
		}
		field.SetInt(int64(n))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if m.Kind != tokenizer.KindNumber {
			return lazyFault(errs.ExpectedInteger, f, m, recIndex, name)
		}
		n := s.Number(tok)
		if n != math.Trunc(n) || n < 0 || math.IsInf(n, 0) || math.IsNaN(n) {
			return lazyFault(errs.ExpectedInteger, f, m, recIndex, name)
		}
		if field.OverflowUint(uint64(n)) {
			return lazyFault(errs.OutOfRangeInteger, f, m, recIndex, name)
		}
		field.SetUint(uint64(n))
		return nil
	case reflect.Float32, reflect.Float64:
		if m.Kind != tokenizer.KindNumber {
			return lazyFault(errs.ExpectedNumber, f, m, recIndex, name)
		}
		field.SetFloat(s.Number(tok))
		return nil
	case reflect.Slice:
		if m.Kind != tokenizer.KindBracketOpen {
			return lazyFault(errs.ExpectedArray, f, m, recIndex, name)
		}
		return bindFramedArray(field, m, f, md, recIndex, name)
	}
	return errUnsupportedLazy
}

// bindFramedArray decodes a bracketed span into a slice, re-framing the
// interior on demand.
func bindFramedArray(field reflect.Value, m parser.RawMember, f *document.Framed,
	md *schema.MemberDef, recIndex int, name string) error {

	elems, ok := parser.FrameSpan(f.Stream, m.Tok+1, m.End-1)
	if !ok {
		return errUnsupportedLazy
	}
	out := reflect.MakeSlice(field.Type(), len(elems), len(elems))
	var of *schema.MemberDef
	if md != nil {
		of = md.Of
	}
	for i, e := range elems {
		if err := bindMember(out.Index(i), e, f, of, recIndex, name); err != nil {
			return err
		}
	}
	field.Set(out)
	return nil
}

// errUnsupportedLazy makes the caller fall back rather than report; it never
// reaches a user.
var errUnsupportedLazy = &UnmarshalError{Path: "$", Msg: "unsupported by the lazy path"}

// lazyFault builds a designated wire fault positioned at the offending token.
func lazyFault(code string, f *document.Framed, m parser.RawMember, recIndex int, name string) error {
	// An EMPTY member — a trailing comma slot, as in `a,b,c` against a schema
	// of two — has no token of its own, and its Tok is one PAST the end. It
	// used to index out of range and panic, which rule 10 forbids outright: a
	// designated code, never a host-runtime crash. Found by fuzzing the lazy
	// path against the tree path with `name,age,score,active,tags---,,,,,`.
	line, col := int32(1), int32(1)
	if toks := f.Stream.Tokens; len(toks) > 0 {
		i := int(m.Tok)
		if i < 0 || i >= len(toks) {
			i = len(toks) - 1 // the nearest real position we have
		}
		line, col = toks[i].Line, toks[i].Col
	}
	at := rootPath.record(recIndex)
	if name != "" {
		at = at.member(name)
	}
	path := at.String()
	return ErrorList{{
		Code: code, Category: errs.CategoryOf(code), Path: path,
		RecordIndex: recIndex, Line: int(line), Col: int(col),
	}}
}
