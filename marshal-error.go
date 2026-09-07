package internetobject

// MarshalError is a binding fault found while marshaling: the field path and
// what went wrong. Wire-level faults never occur on marshal — the writer is
// total over what encode produces.
type MarshalError struct {
	Path string
	Msg  string
}

func (e *MarshalError) Error() string { return e.Path + ": " + e.Msg }

// ── field plans ────────────────────────────────────────────────────────────
