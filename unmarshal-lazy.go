package internetobject

import (
	"math"
	"os"
	"reflect"
	"sync/atomic"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
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

// noLazy forces the general path. It is an atomic flag rather than a plain
// variable read once at init, because the differential test must flip it
// INSIDE a test run (export_test.go, WithTreeDecode).
//
// It used to be `var noLazy = os.Getenv("IO_NO_LAZY") != ""`, read once, while
// the test flipped the environment with t.Setenv. The flip never reached this
// flag, so TestLazyMatchesTreePath and FuzzLazyMatchesTreePath compared the
// lazy path against ITSELF from the commit that introduced them (f559ec6) until
// 2026-09-14. Proved by sabotage: with every bound string corrupted, seven
// ordinary tests failed and both differential tests passed.
var noLazy atomic.Bool

func init() { noLazy.Store(os.Getenv("IO_NO_LAZY") != "") }

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
	if noLazy.Load() {
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
		if bindFramed(tmp, recs[0], f, plan) != nil {
			return false, nil // the general path decodes it, and reports any fault
		}
		elem.Set(tmp)
		return true, nil
	}

	out := reflect.MakeSlice(elem.Type(), len(recs), len(recs))
	for i := range recs {
		if bindFramed(out.Index(i), recs[i], f, plan) != nil {
			return false, nil
		}
	}
	elem.Set(out)
	return true, nil
}

// bindFramed decodes one framed record into a struct value, or declines.
//
// It never REPORTS a fault. A record that is not a clean, complete match — a
// type mismatch, an unknown or repeated member, a required member that never
// arrived — returns errUnsupportedLazy, and the general path re-decodes the
// document and reports every fault itself (ADR 0007, amended 2026-09-14).
//
// That is the fix for a validation bypass this path shipped with. It bound only
// the members PRESENT, so `~ Alice` against a five-member schema decoded
// silently into Age 0, Score 0, Active false, where the general path reports
// missing-value four times. An empty slot (`Alice,,1.5`) did the same for one
// member. It went unseen because the differential test meant to compare the
// two paths never actually switched paths (see noLazy).
//
// Reporting faults here instead would mean re-implementing the validator's
// accumulation rules — every fault, in order, across records — which is a second
// copy of a rule: the shape of bug this port keeps finding. Faults are the rare
// case; decoding them twice costs nothing that matters.
func bindFramed(rv reflect.Value, rec parser.RawRecord, f *document.Framed, plan *structPlan) error {
	if len(plan.fields) > 64 {
		return errUnsupportedLazy // `seen` is a 64-bit mask
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	names, defs := document.SchemaMemberDefs(f.Schema)

	var seen uint64
	var keyed, positional bool
	for i, m := range rec.Members {
		if i >= len(plan.fields) {
			return errUnsupportedLazy
		}
		// A record mixing keyed and positional members has ordering rules of its
		// own (`age: 0, name: A, 0, T` is unexpected-positional-member). Rather
		// than copy them, decline, and let the general path apply them. Found by
		// the differential fuzzer the moment it could see this path, 2026-09-14.
		if m.Positional() {
			positional = true
		} else {
			keyed = true
		}
		if keyed && positional {
			return errUnsupportedLazy
		}
		j := i
		if !m.Positional() {
			// A keyed member may name any declared member, in any order.
			k, known := plan.byName[m.Key]
			if !known {
				return errUnsupportedLazy
			}
			j = k
		} else if m.Absent {
			continue // an empty slot: required-ness is checked below
		}
		if seen&(1<<j) != 0 {
			return errUnsupportedLazy // the same member twice
		}
		seen |= 1 << j
		if e := bindMember(rv.FieldByIndex(plan.fields[j].index), m, f, defs[names[j]]); e != nil {
			return e
		}
	}

	// Every declared member that never arrived — omitted at the end, or left as
	// an empty slot — must be optional. (IsSimpleSchema already excludes
	// defaults, so there is no value to fill in.)
	for j := range plan.fields {
		if seen&(1<<j) == 0 {
			if md := defs[names[j]]; md == nil || !md.Optional {
				return errUnsupportedLazy
			}
		}
	}
	return nil
}

// bindMember decodes one framed member into one field, checking the schema's
// declared type as it goes.
func bindMember(field reflect.Value, m parser.RawMember, f *document.Framed, md *schema.MemberDef) error {

	s := f.Stream
	tok := s.Tokens[m.Tok]

	// Null first: it is legal only where the schema allows it.
	if m.Kind == tokenizer.KindNull {
		if md != nil && !md.Null {
			return errUnsupportedLazy
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
			return errUnsupportedLazy
		}
	case "int":
		if m.Kind != tokenizer.KindNumber {
			return errUnsupportedLazy
		}
		if n := s.Number(tok); n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return errUnsupportedLazy
		}
	case "number":
		if m.Kind != tokenizer.KindNumber {
			return errUnsupportedLazy
		}
	case "bool":
		if m.Kind != tokenizer.KindBoolean {
			return errUnsupportedLazy
		}
	case "array":
		if m.Kind != tokenizer.KindBracketOpen {
			return errUnsupportedLazy
		}
	}

	// The decode IS the store: nothing is boxed on the way.
	switch field.Kind() {
	case reflect.String:
		if m.Kind != tokenizer.KindString {
			return errUnsupportedLazy
		}
		v := s.StringValue(tok)
		if core.IsVariableRef(v) {
			// A reference, not text: the general path resolves it, or reports
			// undefined-variable. Binding it literally accepted `@0` as the string
			// "@0" (differential fuzzer, 2026-09-14).
			return errUnsupportedLazy
		}
		field.SetString(v)
		return nil
	case reflect.Bool:
		if m.Kind != tokenizer.KindBoolean {
			return errUnsupportedLazy
		}
		field.SetBool(s.Bool(tok))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if m.Kind != tokenizer.KindNumber {
			return errUnsupportedLazy
		}
		n := s.Number(tok)
		if n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return errUnsupportedLazy
		}
		if field.OverflowInt(int64(n)) {
			return errUnsupportedLazy
		}
		field.SetInt(int64(n))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if m.Kind != tokenizer.KindNumber {
			return errUnsupportedLazy
		}
		n := s.Number(tok)
		if n != math.Trunc(n) || n < 0 || math.IsInf(n, 0) || math.IsNaN(n) {
			return errUnsupportedLazy
		}
		if field.OverflowUint(uint64(n)) {
			return errUnsupportedLazy
		}
		field.SetUint(uint64(n))
		return nil
	case reflect.Float32, reflect.Float64:
		if m.Kind != tokenizer.KindNumber {
			return errUnsupportedLazy
		}
		field.SetFloat(s.Number(tok))
		return nil
	case reflect.Slice:
		if m.Kind != tokenizer.KindBracketOpen {
			return errUnsupportedLazy
		}
		return bindFramedArray(field, m, f, md)
	}
	return errUnsupportedLazy
}

// bindFramedArray decodes a bracketed span into a slice, re-framing the
// interior on demand.
func bindFramedArray(field reflect.Value, m parser.RawMember, f *document.Framed, md *schema.MemberDef) error {

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
		if err := bindMember(out.Index(i), e, f, of); err != nil {
			return err
		}
	}
	field.Set(out)
	return nil
}

// errUnsupportedLazy makes the caller fall back to the general path, which
// decodes the document again and reports any fault. It never reaches a user:
// this path does not report faults of its own.
var errUnsupportedLazy = &UnmarshalError{Path: "$", Msg: "unsupported by the lazy path"}
