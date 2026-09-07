// Package streaming implements the record protocol over an incremental byte
// stream: `~` introduces each logical record, the FIRST `---` terminates the
// header, and later `---` frames switch the schema context.
//
// Chunk boundaries are never semantic - the same input split any way yields an
// identical item sequence, because the framer keeps its scan state across feeds
// and every frame goes through the same path a one-record document uses.
package streaming

import (
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// StreamOptions seed a reader before any stream bytes arrive.
type StreamOptions struct {
	// Definitions is preloaded header text (the part before a `---`);
	// in-stream definitions override matching keys.
	Definitions string
	// DefaultSchema is the fallback default-schema name, with its $ sigil.
	DefaultSchema string
	// Schema is an already-compiled schema every record is validated
	// against. It outranks both the in-stream header and DefaultSchema
	// (ADR 0004 D5) and is never re-parsed.
	Schema *schema.Schema
}

// Reader consumes a stream incrementally. Feed returns the items each chunk
// completes; Close flushes the final frame and returns the terminal fatal
