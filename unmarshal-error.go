package internetobject

// UnmarshalError is a binding fault: the value at Path cannot be stored in
// the target's Go type. Wire-level faults return as the ErrorList instead.
type UnmarshalError struct {
	Path string
	Msg  string
}

func (e *UnmarshalError) Error() string { return e.Path + ": " + e.Msg }
