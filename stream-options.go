package internetobject

// StreamOptions seed a streaming read before any bytes arrive.
type StreamOptions struct {
	// Definitions is preloaded header text (the part before a `---`);
	// in-stream definitions override matching keys.
	Definitions string
	// DefaultSchema is the fallback default-schema name, e.g. "$Person".
	DefaultSchema string
	// Schema is an already-compiled schema (from ParseSchema, SchemaFor, or
	// another document) that every record is validated against. It outranks
	// the in-stream header and DefaultSchema, and is never re-parsed — the
	// runtime-schema route for streams.
	Schema *Schema
}
