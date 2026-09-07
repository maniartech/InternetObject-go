package internetobject

// StreamItem is one record emitted by a streaming read.
type StreamItem struct {
	// Index is the record's zero-based, dense, stream-global index; failed
	// records consume one too.
	Index int
	// SchemaName carries the explicit `$Name` selector that validated this
	// record, with its sigil; empty when the default context applied.
	SchemaName string
	// Value is the record's live value; nil when Err is set.
	Value any
	// Err is the record's fault (recoverable — iteration continues).
	Err *Error
}
