package internetobject

import (
	"io"
	"iter"
	"strconv"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/streaming"
)

// Stream reads Internet Object records from r incrementally, yielding one
// StreamItem per logical record in wire order. A recoverable record fault
// arrives as an item with Err set and iteration continues; a FATAL fault
// (an unknown schema selector, a read failure) ends iteration with a non-nil
// error on the final pair. Transport chunk boundaries are never semantic.
func Stream(r io.Reader, opts *StreamOptions) iter.Seq2[StreamItem, error] {
	return stream(r, opts, nil)
}

// stream is Stream with a compiled header in scope beneath everything opts and
// the stream declare — how Definitions.Stream shares its compiled schemas
// instead of rendering them back to text for every stream to parse again.
func stream(r io.Reader, opts *StreamOptions, parent *document.Frozen) iter.Seq2[StreamItem, error] {
	var o StreamOptions
	if opts != nil {
		o = *opts
	}
	return func(yield func(StreamItem, error) bool) {
		ropts := streaming.StreamOptions{
			Definitions:   o.Definitions,
			DefaultSchema: o.DefaultSchema,
			Parent:        parent,
		}
		if o.Schema != nil {
			ropts.Schema = o.Schema.s
		}
		reader := streaming.NewReader(ropts)
		emit := func(items []streaming.Item) bool {
			for _, it := range items {
				si := StreamItem{Index: it.RecordIndex, SchemaName: it.SchemaName, Value: it.Value}
				if it.Err != nil {
					// The reader already classified this fault; carrying the
					// category is a spec MUST (io-specs/streaming/error-model)
					// and it used to be dropped here (ADR 0005 D1).
					si.Err = &Error{
						Code: it.Err.Code, Category: it.Err.Category,
						Path:        "$[" + strconv.Itoa(it.RecordIndex) + "]",
						RecordIndex: it.RecordIndex,
					}
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
			// The reader records no position for a fatal stream fault, so none
			// is reported; it used to claim 1:1.
			yield(StreamItem{}, toError(errs.Error{Code: fatal.Code, Category: fatal.Category}))
		}
	}
}
