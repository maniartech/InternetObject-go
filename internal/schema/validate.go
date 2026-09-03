package schema

import (
	"math"
	"math/big"
	"regexp"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The validation stage: (compiled schema + parsed record) → validated value
// or designated error codes. The structure mirrors the reference
// implementation's observable behavior, derived from its object processor:
//
//   - a BARE record surfaces only its prevailing error (fail-fast); a
//     COLLECTION record accumulates one error per faulted member;
//   - membership faults (a surplus or duplicate member, a positional member
//     after a keyed one) abort the record and PREVAIL over accumulated
//     member errors;
//   - each member's own validation is fail-fast: common checks (absence →
//     default/optional/missing-value; null → nullability; choices) come
//     before the type check, constraints after it;
//   - the validated record carries schema members in SCHEMA order, then
//     undeclared extras in arrival order.

// Defs resolves names during validation: compiled schemas by (sigil-less)
// name, and variables.
type Defs interface {
	SchemaOf(name string) (*Schema, *errs.Error)
	// Var resolves a variable by sigil-less name; the error is
	// undefined-variable for a missing one, invalid-definition for a cycle.
	Var(name string) (any, *errs.Error)
}

// absent marks a member that legitimately produced no value (optional, no
// default).
type absentType struct{}

var absent = absentType{}

type valFail struct{ err errs.Error }

// memberSlot is one declared member's validation state. The three facts live
// in ONE slice so a record costs a single allocation here — three parallel
// slices cost three, and the two maps this replaced cost two plus hashing
// (ADR 0006 P2).
type memberSlot struct {
	val       any
	filled    bool // a value was produced (absent members leave this false)
	processed bool // this member has been dealt with; a second key is a duplicate
}

// vfail raises a member fault. It carries NO position: the recover sites in
// validateObject know which member and record the fault belongs to and stamp
// it there (ADR 0005 D2), which is why every validation error used to report
// a fabricated 1:1.
func vfail(code string) {
	panic(valFail{errs.Error{Code: code, RecordIndex: -1}})
}

// ValidateRecord validates one record. accumulate selects the collection
// discipline (all member faults, in order) over the bare-record one (only the
// prevailing fault). The validated record is returned when there are no
// errors.
func ValidateRecord(rec *value.Object, s *Schema, defs Defs, accumulate bool) (*value.Object, []errs.Error) {
	return ValidateRecordAt(rec, s, defs, accumulate, "$")
}

// ValidateRecordAt is ValidateRecord with the structural path this record
// occupies, so faults report where they are (ADR 0005 D3): "$" for a bare
// record, "$[2]" for the third record of a collection.
func ValidateRecordAt(rec *value.Object, s *Schema, defs Defs, accumulate bool, path string) (*value.Object, []errs.Error) {
	out, acc, fatal := validateObject(rec, s, defs, path)
	switch {
	case fatal != nil && accumulate:
		return nil, append(acc, *fatal)
	case fatal != nil:
		return nil, []errs.Error{*fatal}
	case len(acc) > 0 && accumulate:
		return nil, acc
	case len(acc) > 0:
		return nil, acc[:1]
	}
	return out, nil
}

// validateObject implements the record/object algorithm. It returns the
// validated object, the accumulated member errors, and the fatal membership
// error (which aborted processing) if any.
func validateObject(rec *value.Object, s *Schema, defs Defs, path string) (out *value.Object, acc []errs.Error, fatal *errs.Error) {
	// Validated members: schema-order slots first, then extras by arrival.
	// Slots are addressed by POSITION, not by name — the schema is compiled
	// and its member positions are fixed, so two maps per record became two
	// slices (ADR 0006 P2). Small enough to stay on the stack for typical
	// records.
	slots := make([]memberSlot, len(s.Names))
	var extras []value.Member

	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(valFail)
			if !ok {
				panic(r)
			}
			e := locate(f.err, path, "", rec, nil)
			fatal = &e
			out = nil
		}
	}()

	// try validates one member, converting a member-level failure into an
	// accumulated error. idx is the member's schema position, or -1 for an
	// undeclared member, whose value the caller places in extras itself.
	try := func(idx int, name string, m *value.Member, f func() any) (out any, ok bool) {
		defer func() {
			if r := recover(); r != nil {
				fail, isFail := r.(valFail)
				if !isFail {
					panic(r)
				}
				acc = append(acc, locate(fail.err, path, name, rec, m))
				if idx >= 0 {
					slots[idx].processed = true
				}
				out, ok = nil, false
			}
		}()
		v := f()
		if v == (any)(absent) {
			return nil, false
		}
		if idx >= 0 {
			slots[idx] = memberSlot{val: v, filled: true, processed: true}
		}
		return v, true
	}

	// fillMissing gives every unbound schema member its absence treatment:
	// default, optional, or missing-value. lookup lets a keyed member found in
	// the record supply the value (the normal path); the absorption path
	// consumed the whole record already and must not read it twice.
	fillMissing := func(lookup bool) {
		for idx, name := range s.Names {
			if (name == "*" && isWildcardDef(s)) || slots[idx].processed {
				continue
			}
			md := s.Defs[name]
			val, present := any(nil), false
			var mp *value.Member
			if lookup {
				if i := rec.Find(name); i >= 0 {
					val, present = rec.Members[i].Value, true
					mp = &rec.Members[i]
				}
			}
			try(idx, name, mp, func() any { return validateMember(val, present, md, defs) })
		}
	}

	// The lone-object absorption rule (io-test-cases ISSUE-15): when the
	// record's first member is KEYED with a name the schema does not declare,
	// the record cannot be the record itself, so the WHOLE record is the value
	// of the first schema member. Open schemas are excluded (an undeclared key
	// is a legal extra there) unless they declare exactly one REAL member —
	// the `*` wildcard entry is openness, not a member, and never absorbs.
	declared := len(s.Names)
	if isWildcardDef(s) {
		declared--
	}
	if len(rec.Members) > 0 && declared > 0 && (s.Open == nil || declared == 1) &&
		!absorptionLoops(rec.Members[0].Key, s, defs) {
		fm := rec.Members[0]
		if !fm.Positional && s.Defs[fm.Key] == nil && fm.Key != "*" {
			name0 := s.Names[0]
			try(0, name0, &rec.Members[0], func() any { return validateMember(rec, true, s.Defs[name0], defs) })
			slots[0].processed = true
			fillMissing(false)
			return assemble(rec, s, slots, extras), acc, nil
		}
	}

	// Positional members map onto schema names by index.
	i := 0
	positional := true
	for ; i < len(s.Names); i++ {
		name := s.Names[i]
		if name == "*" && isWildcardDef(s) {
			break // the wildcard is openness, not a member
		}
		md := s.Defs[name]
		if i < len(rec.Members) {
			m := rec.Members[i]
			if !m.Positional {
				positional = false
				break
			}
			if m.Absent {
				// an empty comma slot: the member holds its position, absent
				if md.Optional && !md.HasDefault {
					continue
				}
				try(i, name, nil, func() any { return validateMember(nil, false, md, defs) })
				continue
			}
			try(i, name, &rec.Members[i], func() any { return validateMember(m.Value, true, md, defs) })
		} else {
			// entirely missing — absence treatment, but leave an optional
			// member unprocessed so a later keyed value may still fill it
			if md.Optional && !md.HasDefault {
				continue
			}
			try(i, name, nil, func() any { return validateMember(nil, false, md, defs) })
		}
	}

	// Surplus positional values: fatal against a closed schema; under an open
	// one each is validated like any undeclared member (the wildcard's def or
	// bare `any`), so @-references resolve and a typed wildcard constrains —
	// raw pass-through here skipped both (fuzzer-found, oracle-pinned).
	if positional {
		for ; i < len(rec.Members); i++ {
			m := rec.Members[i]
			if !m.Positional {
				break
			}
			if m.Absent {
				continue // a trailing hole carries no information
			}
			if s.Open == nil {
				vfail(errs.UnknownMember)
			}
			md := undeclaredMemberDef("", s.Open)
			mv := m.Value
			func() {
				defer func() {
					if r := recover(); r != nil {
						f, ok := r.(valFail)
						if !ok {
							panic(r)
						}
						acc = append(acc, f.err)
					}
				}()
				extras = append(extras, value.Member{Positional: true, Value: validateMember(mv, true, md, defs)})
			}()
		}
	}

	// Remaining members must be keyed.
	for ; i < len(rec.Members); i++ {
		m := rec.Members[i]
		if m.Positional {
			vfail(errs.UnexpectedPositionalMember)
		}
		name := m.Key
		idx, declared := s.Index[name]
		if declared && slots[idx].processed {
			vfail(errs.DuplicateMember)
		}
		if !declared && hasExtra(extras, name) {
			vfail(errs.DuplicateMember)
		}
		md := s.Defs[name]
		if name == "*" && isWildcardDef(s) {
			declared = false
			// The `*` entry is OPENNESS, not a member named `*`. A data key
			// that happens to be `*` is an ordinary extra: it must keep its
			// arrival position, not be hoisted into schema order ahead of
			// positional members — which produced a record the writer could
			// only spell as unparseable `"*": 0, 0` (found by the byte fuzzer;
			// the reference keeps arrival order).
			md, declared = nil, false
		}
		if md == nil {
			if s.Open == nil {
				vfail(errs.UnknownMember)
			}
			md = undeclaredMemberDef(name, s.Open)
			mv := m.Value
			if v, ok := try(-1, name, &rec.Members[i], func() any {
				return validateMember(mv, true, md, defs)
			}); ok {
				extras = append(extras, value.Member{Key: name, Value: v})
			}
			continue
		}
		mv := m.Value
		try(idx, name, &rec.Members[i], func() any { return validateMember(mv, true, md, defs) })
	}

	fillMissing(true)
	return assemble(rec, s, slots, extras), acc, nil
}

// absorptionLoops reports whether applying the lone-object absorption rule
// could never terminate. Absorption hands the WHOLE record down to the first
// declared member without consuming anything, so if the chain of "first
// member's schema" cycles before some schema on it declares the record's own
// first key (which is what stops absorption), the record would be absorbed
// forever — `~ $P: {A: $P}` fed `{$P: 0}`, which stack-overflows the
// reference implementation (docs/FINDINGS.md #14).
//
// Skipping absorption here is not an invented rule: the record then takes the
// ordinary path and reports the fault it actually has (unknown-member), which
// is what absorption exists to override only when it can succeed. Legitimate
// recursive schemas are untouched — real nesting consumes a level of data per
// step and terminates on its own.
func absorptionLoops(key string, s *Schema, defs Defs) bool {
	seen := map[*Schema]bool{}
	for cur := s; cur != nil; {
		if seen[cur] {
			return true
		}
		seen[cur] = true

		declared := len(cur.Names)
		if isWildcardDef(cur) {
			declared--
		}
		// Absorption stops here — the key is declared, there is nothing to
		// absorb into, or this schema does not absorb at all.
		if declared == 0 || cur.Defs[key] != nil || !(cur.Open == nil || declared == 1) {
			return false
		}
		md := cur.Defs[cur.Names[0]]
		if md == nil {
			return false
		}
		next := md.Schema
		if next == nil && md.SchemaRef != "" {
			resolved, cerr := defs.SchemaOf(strings.TrimPrefix(md.SchemaRef, "$"))
			if cerr != nil {
				return false // a dangling reference is reported on the normal path
			}
			next = resolved
		}
		cur = next
	}
	return false
}

// locate fills in a fault's structural context: the path it occurred at, and
// the position of the value it is about. A fault with no value to point at —
// a member that is missing entirely — is reported at the record, exactly as
// the reference does (ADR 0005 D2). A position already set by the tokenizer
// (a deferred literal error) is never overwritten.
func locate(e errs.Error, path, name string, rec *value.Object, m *value.Member) errs.Error {
	if e.Path == "" {
		e.Path = path
		if name != "" && name != "*" {
			e.Path = path + "." + name
		}
	}
	if e.Category == "" {
		e.Category = errs.CategoryOf(e.Code)
	}
	if e.Line == 0 {
		if m != nil && m.Line != 0 {
			e.Line, e.Col = m.Line, m.Col
		} else if rec != nil {
			e.Line, e.Col = rec.Line, rec.Col
		}
	}
	return e
}

// isWildcardDef reports whether the "*" entry in Names is the typed-open
// wildcard (its def IS s.Open) rather than a literal quoted "*" member.
func isWildcardDef(s *Schema) bool {
	o, ok := s.Open.(*MemberDef)
	return ok && o == s.Defs["*"]
}

// assemble builds the validated object: declared members in schema order,
// then extras in arrival order.
func assemble(rec *value.Object, s *Schema, slots []memberSlot, extras []value.Member) *value.Object {
	// A validated record is ALWAYS a fresh object, never the parsed one with
	// its members renamed. Reusing it saves two allocations per record and
	// was tried: the absorption rule can make a record a member of itself, or
	// of an ancestor, so writing validated values back builds a reference
	// cycle that the writer then walks forever. The byte fuzzer found both
	// shapes (`$P: {A: $P}` and the mutual `B: {B}`) within seconds. Detecting
	// the cycle safely costs more than the two allocations are worth; the
	// right fix for the tree's cost is not to build a tree at all on the
	// decode path (ADR 0006 P3), not to alias this one.
	n := len(extras)
	for i := range slots {
		if slots[i].filled {
			n++
		}
	}
	out := &value.Object{Members: make([]value.Member, 0, n)}
	for i, name := range s.Names {
		if slots[i].filled {
			out.Members = append(out.Members, value.Member{Key: name, Value: slots[i].val})
		}
	}
	out.Members = append(out.Members, extras...)
	return out
}

// hasExtra reports whether an undeclared member with this name was already
// accepted. Extras are few, so a scan beats a map — and the parser already
// rejects duplicate keys within a parsed record, leaving only hand-built
// objects to reach this.
func hasExtra(extras []value.Member, name string) bool {
	for i := range extras {
		if !extras[i].Positional && extras[i].Key == name {
			return true
		}
	}
	return false
}

// undeclaredMemberDef is THE definition an undeclared member gets under an
// open or untyped container: the wildcard's own def when one was declared,
// otherwise `any` WITH null allowed — an untyped container constrains
// nothing, nullability included (PORTING-NOTES rule 8).
func undeclaredMemberDef(name string, open any) *MemberDef {
	if md, ok := open.(*MemberDef); ok {
		clone := *md
		clone.Name = name
		return &clone
	}
	return &MemberDef{Name: name, Type: "any", Null: true}
}

// ── one member ─────────────────────────────────────────────────────────────

// validateMember runs the full check sequence for one member value. It
// panics (valFail) on the first fault; returns absent for a legitimately
// missing optional member.
func validateMember(val any, present bool, md *MemberDef, defs Defs) any {
	// Resolution: an @-string is a variable reference (the reference resolves
	// these before every other check).
	if s, ok := val.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
		v, verr := defs.Var(s[1:])
		if verr != nil {
			panic(valFail{*verr})
		}
		val = v
	}

	// Absence: default → optional → missing-value. A default is NOT
	// re-validated against the member's own constraints.
	if !present {
		if md.HasDefault {
			return resolveRef(md.Default, defs)
		}
		if md.Optional {
			return absent
		}
		vfail(errs.MissingValue)
	}

	// Null: the * flag (or "null": T) is what allows it.
	if val == nil {
		if md.Null {
			return nil
		}
		vfail(errs.ForbiddenNull)
	}

	// Choices, before the type check.
	if md.Choices != nil {
		found := false
		for _, c := range md.Choices {
			if value.Equal(val, resolveRef(c, defs)) {
				found = true
				break
			}
		}
		if !found {
			vfail(errs.MismatchedChoice)
		}
	}

	switch md.Type {
	case "uint64", "float32", "float64":
		// Registered so they compile, reserved so a value is rejected here.
		vfail(errs.ReservedType)
	}

	switch familyOf(md.Type) {
	case famString:
		return validateString(val, md)
	case famNumber:
		return validateNumber(val, md, defs)
	case famBigInt:
		return validateBigInt(val, md, defs)
	case famDecimal:
		return validateDecimal(val, md, defs)
	case famBool:
		if _, ok := val.(bool); !ok {
			vfail(errs.ExpectedBoolean)
		}
		return val
	case famTemporal:
		return validateTemporal(val, md, defs)
	case famArray:
		return validateArray(val, md, defs)
	case famObject:
		return validateObjectMember(val, md, defs)
	default: // any
		if ev, ok := val.(value.ErrorValue); ok {
			panic(valFail{errs.Error{Code: ev.Code, Line: ev.Line, Col: ev.Col}}) // a deferred malformed literal surfaces as itself
		}
		if md.AnyOf != nil {
			for _, alt := range md.AnyOf {
				if v, ok := tryAlternative(val, alt, defs); ok {
					return v
				}
			}
			vfail(errs.MismatchedAnyOf)
		}
		return val
	}
}

// tryAlternative validates val against one anyOf alternative, reporting
// success instead of failing.
func tryAlternative(val any, md *MemberDef, defs Defs) (v any, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isFail := r.(valFail); !isFail {
				panic(r)
			}
			v, ok = nil, false
		}
	}()
	return validateMember(val, true, md, defs), true
}

// resolveRef resolves an @-reference in a constraint value (a choice, bound
// or default may name a variable).
func resolveRef(v any, defs Defs) any {
	if s, ok := v.(string); ok && strings.HasPrefix(s, "@") && len(s) > 1 {
		r, verr := defs.Var(s[1:])
		if verr != nil {
			panic(valFail{*verr})
		}
		return r
	}
	return v
}

func validateString(val any, md *MemberDef) any {
	s, ok := val.(string)
	if !ok {
		vfail(errs.ExpectedString)
	}
	switch md.Type {
	case "string":
		if pat, ok := md.Constraints["pattern"].(string); ok {
			re := md.re
			if re == nil {
				var err error
				flags := ""
				if f, ok := md.Constraints["flags"].(string); ok && strings.Contains(f, "i") {
					flags = "(?i)"
				}
				re, err = regexp.Compile(flags + pat)
				if err != nil {
					vfail(errs.MismatchedPattern)
				}
				md.re = re
			}
			if !re.MatchString(s) {
				vfail(errs.MismatchedPattern)
			}
		}
	case "email":
		if !emailRe.MatchString(s) {
			vfail(errs.InvalidEmail)
		}
	case "url":
		if !urlRe.MatchString(s) {
			vfail(errs.InvalidURL)
		}
	}
	n := -1
	length := func() int {
		if n < 0 {
			n = 0
			for range s {
				n++ // length is measured in code points, never bytes or UTF-16 units
			}
		}
		return n
	}
	if l, ok := md.Constraints["len"].(float64); ok && float64(length()) != l {
		vfail(errs.MismatchedLen)
	}
	if l, ok := md.Constraints["maxLen"].(float64); ok && float64(length()) > l {
		vfail(errs.MismatchedMaxLen)
	}
	if l, ok := md.Constraints["minLen"].(float64); ok && float64(length()) < l {
		vfail(errs.MismatchedMinLen)
	}
	// Return the ORIGINAL interface value rather than re-boxing: the caller
	// already holds this value boxed, and `return s` allocates a fresh
	// interface for a value validation did not change (ADR 0006 P7).
	return val
}

// The email and url expressions, ported from the reference implementation.
// Both are deliberately UNANCHORED — a substring match accepts (which is why
// `a@b@c.com` passes), exactly as the reference behaves.
var (
	emailRe = regexp.MustCompile(`(?:[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*|"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*")@(?:(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?|\[(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?|[a-z0-9-]*[a-z0-9]:(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21-\x5a\x53-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])+)\])`)
	urlRe   = regexp.MustCompile(`(([A-Za-z]{3,9}:(?://)?)(?:[\-;:&=\+\$,\w]+@)?[A-Za-z0-9\.\-]+|(?:www\.|[\-;:&=\+\$,\w]+@)[A-Za-z0-9\.\-]+)((?:/[\+~%/\.\w\-_]*)?\??(?:[\-\+=&;%@\.\w_]*)#?(?:[\.\!/\\\w]*))?`)
)

// integerTypes are the schema types whose values must be whole numbers.
var integerTypes = map[string]bool{
	"int": true, "uint": true,
	"int8": true, "int16": true, "int32": true,
	"uint8": true, "uint16": true, "uint32": true,
}

// intrinsic bounds of the sized types (nil = unbounded on that side)
func typeBounds(t string) (min, max float64, bounded bool) {
	switch t {
	case "int8":
		return -128, 127, true
	case "int16":
		return -32768, 32767, true
	case "int32":
		return -2147483648, 2147483647, true
	case "uint8":
		return 0, 255, true
	case "uint16":
		return 0, 65535, true
	case "uint32":
		return 0, 4294967295, true
	case "uint":
		return 0, math.Inf(1), true
	}
	return 0, 0, false
}

func validateNumber(val any, md *MemberDef, defs Defs) any {
	expected := errs.ExpectedNumber
	if integerTypes[md.Type] {
		expected = errs.ExpectedInteger
	}
	f, ok := val.(float64)
	if !ok {
		vfail(expected)
	}
	if integerTypes[md.Type] && f != math.Trunc(f) {
		vfail(errs.ExpectedInteger)
	}
	// declared bounds first (the author's constraint), the type's own after
	if m, ok := numBound(md, "min", defs); ok && f < m {
		vfail(errs.MismatchedMin)
	}
	if m, ok := numBound(md, "max", defs); ok && f > m {
		vfail(errs.MismatchedMax)
	}
	if lo, hi, bounded := typeBounds(md.Type); bounded && (f < lo || f > hi) {
		vfail(errs.OutOfRangeInteger)
	}
	if m, ok := numBound(md, "multipleOf", defs); ok && math.Mod(f, m) != 0 {
		vfail(errs.MismatchedMultipleOf)
	}
	return val // the original box; see validateString
}

func numBound(md *MemberDef, key string, defs Defs) (float64, bool) {
	v, ok := md.Constraints[key]
	if !ok {
		return 0, false
	}
	f, ok := resolveRef(v, defs).(float64)
	if !ok {
		vfail(errs.ExpectedNumber)
	}
	return f, true
}

func validateBigInt(val any, md *MemberDef, defs Defs) any {
	b, ok := val.(*big.Int)
	if !ok {
		vfail(errs.ExpectedBigInt)
	}
	bound := func(key string) (*big.Int, bool) {
		v, ok := md.Constraints[key]
		if !ok {
			return nil, false
		}
		bb, ok := resolveRef(v, defs).(*big.Int)
		if !ok {
			vfail(errs.ExpectedBigInt)
		}
		return bb, true
	}
	if m, ok := bound("min"); ok && b.Cmp(m) < 0 {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && b.Cmp(m) > 0 {
		vfail(errs.MismatchedMax)
	}
	if m, ok := bound("multipleOf"); ok {
		if m.Sign() == 0 || new(big.Int).Mod(b, m).Sign() != 0 {
			vfail(errs.MismatchedMultipleOf)
		}
	}
	return val // the original box; see validateString
}

func validateDecimal(val any, md *MemberDef, defs Defs) any {
	d, ok := val.(value.Decimal)
	if !ok {
		vfail(errs.ExpectedDecimal)
	}
	if sc, ok := md.Constraints["scale"].(float64); ok && float64(d.Scale) != sc {
		vfail(errs.MismatchedScale)
	}
	if pr, ok := md.Constraints["precision"].(float64); ok && float64(decimalDigits(d)) > pr {
		vfail(errs.MismatchedPrecision)
	}
	bound := func(key string) (value.Decimal, bool) {
		v, ok := md.Constraints[key]
		if !ok {
			return value.Decimal{}, false
		}
		dd, ok := resolveRef(v, defs).(value.Decimal)
		if !ok {
			vfail(errs.ExpectedDecimal)
		}
		return dd, true
	}
	if m, ok := bound("min"); ok && cmpDecimal(d, m) < 0 {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && cmpDecimal(d, m) > 0 {
		vfail(errs.MismatchedMax)
	}
	if m, ok := bound("multipleOf"); ok && !decimalMultiple(d, m) {
		vfail(errs.MismatchedMultipleOf)
	}
	return val // the original box; see validateString
}

// decimalDigits counts a decimal's significant digits (its precision).
func decimalDigits(d value.Decimal) int {
	s := new(big.Int).Abs(d.Coef).String()
	if s == "0" {
		return 1
	}
	return len(s)
}

// cmpDecimal compares two decimals numerically, aligning scales.
func cmpDecimal(a, b value.Decimal) int {
	av, bv := a.Coef, b.Coef
	if a.Scale < b.Scale {
		av = new(big.Int).Mul(av, pow10(b.Scale-a.Scale))
	} else if b.Scale < a.Scale {
		bv = new(big.Int).Mul(bv, pow10(a.Scale-b.Scale))
	}
	return av.Cmp(bv)
}

func decimalMultiple(d, m value.Decimal) bool {
	dv, mv := d.Coef, m.Coef
	if d.Scale < m.Scale {
		dv = new(big.Int).Mul(dv, pow10(m.Scale-d.Scale))
	} else if m.Scale < d.Scale {
		mv = new(big.Int).Mul(mv, pow10(d.Scale-m.Scale))
	}
	if mv.Sign() == 0 {
		return false
	}
	return new(big.Int).Mod(dv, mv).Sign() == 0
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func validateTemporal(val any, md *MemberDef, defs Defs) any {
	expected := errs.ExpectedDateTime
	switch md.Type {
	case "date":
		expected = errs.ExpectedDate
	case "time":
		expected = errs.ExpectedTime
	}
	t, ok := val.(value.Temporal)
	if !ok {
		// the three temporal kinds are interchangeable at the type check;
		// anything else — including a deferred malformed literal — is not
		vfail(expected)
	}
	bound := func(key string) (value.Temporal, bool) {
		v, ok := md.Constraints[key]
		if !ok {
			return value.Temporal{}, false
		}
		tt, ok := resolveRef(v, defs).(value.Temporal)
		if !ok {
			vfail(errs.ExpectedDateTime)
		}
		return tt, true
	}
	if m, ok := bound("min"); ok && t.Time.UnixMilli() < m.Time.UnixMilli() {
		vfail(errs.MismatchedMin)
	}
	if m, ok := bound("max"); ok && t.Time.UnixMilli() > m.Time.UnixMilli() {
		vfail(errs.MismatchedMax)
	}
	return val // the original box; see validateString
}

func validateArray(val any, md *MemberDef, defs Defs) any {
	arr, ok := val.([]any)
	if !ok {
		vfail(errs.ExpectedArray)
	}
	if l, ok := md.Constraints["len"].(float64); ok && float64(len(arr)) != l {
		vfail(errs.MismatchedLen)
	}
	if l, ok := md.Constraints["maxLen"].(float64); ok && float64(len(arr)) > l {
		vfail(errs.MismatchedMaxLen)
	}
	if l, ok := md.Constraints["minLen"].(float64); ok && float64(len(arr)) < l {
		vfail(errs.MismatchedMinLen)
	}
	if md.Of == nil {
		return arr // an untyped array constrains nothing, nulls included
	}
	// Validate in place. The element validators return the value they were
	// given (they check, they do not transform), so a second slice would be a
	// copy of the first — one allocation per array, per record. Only when an
	// element genuinely changes (a default, a resolved @reference) is a value
	// written back, and it is written into the slice the parser already built,
	// which nothing else references once validation returns (ADR 0006 P3).
	for i, e := range arr {
		if v := validateMember(e, true, md.Of, defs); !sameValue(v, e) {
			arr[i] = v
		}
	}
	return arr
}

// sameValue reports whether validation handed back the identical interface
// value it was given — the common case, and cheaper than assuming it did not.
func sameValue(a, b any) bool {
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	}
	return false
}

func validateObjectMember(val any, md *MemberDef, defs Defs) any {
	// The reference resolves a `$ref` before the type check, so a dangling
	// reference reports undefined-schema even when the value is not an object.
	sch := md.Schema
	if sch == nil && md.SchemaRef != "" {
		s, cerr := defs.SchemaOf(strings.TrimPrefix(md.SchemaRef, "$"))
		if cerr != nil {
			panic(valFail{*cerr})
		}
		sch = s
	}
	obj, ok := val.(*value.Object)
	if !ok {
		vfail(errs.InvalidObject)
	}
	if sch == nil {
		return obj // a bare `object` member constrains nothing
	}
	v, acc, fatal := validateObject(obj, sch, defs, md.Path)
	if fatal != nil {
		panic(valFail{*fatal})
	}
	if len(acc) > 0 {
		panic(valFail{acc[0]})
	}
	return v
}
