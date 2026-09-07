package schema

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// vfail raises a member fault. It carries NO position: the recover sites in
// validateObject know which member and record the fault belongs to and stamp
// it there (ADR 0005 D2), which is why every validation error used to report
// a fabricated 1:1.
func vfail(code errs.Code) {
	panic(valFail{errs.Error{Code: code, RecordIndex: -1}})
}

// ValidateRecord validates one record. accumulate selects the collection
// discipline (all member faults, in order) over the bare-record one (only the
// prevailing fault). The validated record is returned when there are no
// errors.
func ValidateRecord(rec *core.Object, s *Schema, defs Defs, accumulate bool) (*core.Object, []errs.Error) {
	return ValidateRecordAt(rec, s, defs, accumulate, "$")
}

// CheckRecord validates a record and returns ONLY its faults.
//
// It is ValidateRecord without the assembled result — which the callers that
// merely check (Validate, ValidateWith, and MarshalWith before it writes)
// discarded anyway, after paying for a whole second object and member slice per
// record. Nested objects are still assembled: a child's validated value is
// placed into its parent's slot, so only the TOP-level result is optional.
func CheckRecord(rec *core.Object, s *Schema, defs Defs, accumulate bool) []errs.Error {
	_, acc := validateAt(rec, s, defs, accumulate, "$", false)
	return acc
}

// ValidateRecordAt is ValidateRecord with the structural path this record
// occupies, so faults report where they are (ADR 0005 D3): "$" for a bare
// record, "$[2]" for the third record of a collection.
func ValidateRecordAt(rec *core.Object, s *Schema, defs Defs, accumulate bool, path string) (*core.Object, []errs.Error) {
	defs = defsOr(defs)
	return validateAt(rec, s, defs, accumulate, path, true)
}

func validateAt(rec *core.Object, s *Schema, defs Defs, accumulate bool, path string, wantOut bool) (*core.Object, []errs.Error) {
	defs = defsOr(defs)
	out, acc, fatal := validateObject(rec, s, defs, path, wantOut)
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
func validateObject(rec *core.Object, s *Schema, defs Defs, path string, wantOut bool) (out *core.Object, acc []errs.Error, fatal *errs.Error) {
	// Validated members: schema-order slots first, then extras by arrival.
	// Slots are addressed by POSITION, not by name — the schema is compiled
	// and its member positions are fixed, so two maps per record became two
	// slices (ADR 0006 P2). Small enough to stay on the stack for typical
	// records.
	slots := make([]memberSlot, len(s.Names))
	var extras []core.Member

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
	try := func(idx int, name string, m *core.Member, f func() any) (out any, ok bool) {
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
			var mp *core.Member
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
			if !wantOut {
				return nil, acc, nil
			}
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
				extras = append(extras, core.Member{Positional: true, Value: validateMember(mv, true, md, defs)})
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
				extras = append(extras, core.Member{Key: name, Value: v})
			}
			continue
		}
		mv := m.Value
		try(idx, name, &rec.Members[i], func() any { return validateMember(mv, true, md, defs) })
	}

	fillMissing(true)
	if !wantOut {
		return nil, acc, nil
	}
	return assemble(rec, s, slots, extras), acc, nil
}

// absorptionLoops reports whether applying the lone-object absorption rule
// could never terminate. Absorption hands the WHOLE record down to the first
// declared member without consuming anything, so if the chain of "first
// member's schema" cycles before some schema on it declares the record's own
// first key (which is what stops absorption), the record would be absorbed
// forever. A cyclic schema that is APPLIED to an object record — `~ $P: {A: $P}`
// with `--- $P` and `{x: 1}` — stack-overflows the reference implementation with
// an uncoded RangeError. Merely DECLARING the cycle is harmless: nothing
// recurses until the schema is reached, which is why the obvious-looking
// `{$P: 0}` does not reproduce it. Measured against the oracle 2026-09-04.
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
func locate(e errs.Error, path, name string, rec *core.Object, m *core.Member) errs.Error {
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
func assemble(rec *core.Object, s *Schema, slots []memberSlot, extras []core.Member) *core.Object {
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
	out := &core.Object{Members: make([]core.Member, 0, n)}
	for i, name := range s.Names {
		if slots[i].filled {
			out.Members = append(out.Members, core.Member{Key: name, Value: slots[i].val})
		}
	}
	out.Members = append(out.Members, extras...)
	return out
}

// hasExtra reports whether an undeclared member with this name was already
// accepted. Extras are few, so a scan beats a map — and the parser already
// rejects duplicate keys within a parsed record, leaving only hand-built
// objects to reach this.
func hasExtra(extras []core.Member, name string) bool {
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
			if core.Equal(val, resolveRef(c, defs)) {
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
		if ev, ok := val.(core.ErrorValue); ok {
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
