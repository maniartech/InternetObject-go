package internetobject

// MarshalError is a fault found while marshaling: the field path, what went
// wrong, and — when the fault came from somewhere else, such as the writer a
// StreamMarshaler writes to — the underlying error, which errors.Is and
// errors.As reach through Unwrap. Wire-level faults never occur on marshal —
// the writer is total over what encode produces.
type MarshalError struct {
	Path string
	Msg  string
	Err  error // the underlying cause, or nil
}

func (e *MarshalError) Error() string {
	if e.Err != nil {
		return e.Path + ": " + e.Msg + ": " + e.Err.Error()
	}
	return e.Path + ": " + e.Msg
}

// Unwrap returns the underlying cause, or nil.
func (e *MarshalError) Unwrap() error { return e.Err }

// ── field plans ────────────────────────────────────────────────────────────
