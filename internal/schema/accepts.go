package schema

// Accepts reports whether val passes md's whole per-member check — choices,
// type, pattern, lengths, bounds, multipleOf — exactly as validation would
// judge it in a record, with no definitions in scope.
//
// It exists for the fast paths (SPEC 0003 §5.2), which never report a fault:
// they ask this, and on false they decline and the general path reports.
// Asking the validator itself, rather than re-checking constraints on the Go
// value, is the point: a second statement of a constraint rule is the bug
// class this port has found most often. val must be in the value model's
// representation — a string, a float64 for any number, a bool.
func (md *MemberDef) Accepts(val any) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, fault := r.(valFail); !fault {
				panic(r)
			}
			ok = false
		}
	}()
	validateMember(val, true, md, NoDefs{})
	return true
}

// Constrained reports whether md declares anything a value of its type must
// additionally satisfy: a constraint or a set of choices.
func (md *MemberDef) Constrained() bool {
	return len(md.Constraints) > 0 || md.Choices != nil
}

// Standalone reports whether a value can be judged against md by Accepts
// alone: no default to supply for an absent value, no union to try, no nested
// or referenced schema to descend into.
func (md *MemberDef) Standalone() bool {
	return !md.HasDefault && md.AnyOf == nil && md.Schema == nil && md.SchemaRef == ""
}
