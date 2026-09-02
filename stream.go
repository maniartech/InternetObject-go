package internetobject

import (
	"io"
	"iter"

	"github.com/maniartech/InternetObject-go/internal/document"
)

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

// Stream reads Internet Object records from r incrementally, yielding one
// StreamItem per logical record in wire order. A recoverable record fault
// arrives as an item with Err set and iteration continues; a FATAL fault
// (an unknown schema selector, a read failure) ends iteration with a non-nil
// error on the final pair. Transport chunk boundaries are never semantic.
func Stream(r io.Reader, opts *StreamOptions) iter.Seq2[StreamItem, error] {
	var o StreamOptions
	if opts != nil {
		o = *opts
	}
	return func(yield func(StreamItem, error) bool) {
		ropts := document.StreamOptions{
			Definitions:   o.Definitions,
			DefaultSchema: o.DefaultSchema,
		}
		if o.Schema != nil {
			ropts.Schema = o.Schema.s
		}
		reader := document.NewReader(ropts)
		emit := func(items []document.Item) bool {
			for _, it := range items {
				si := StreamItem{Index: it.RecordIndex, SchemaName: it.SchemaName, Value: it.Value}
				if it.Err != nil {
					si.Err = &Error{Code: it.Err.Code, Line: 1, Col: 1}
					si.Value = nil
				}
				if !yield(si, nil) {
					return false
				}
			}
			return true
		}

		buf := make([]byte, 64*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if !emit(reader.Feed(buf[:n])) {
					return
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				yield(StreamItem{}, err)
				return
			}
		}
		items, fatal := reader.Close()
		if !emit(items) {
			return
		}
		if fatal != nil {
			yield(StreamItem{}, Error{Code: fatal.Code, Line: 1, Col: 1})
		}
	}
}
