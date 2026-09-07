package internetobject

import (
	"bufio"
	goio "io"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// StreamMarshaler writes records onto a stream, one at a time, framed the way
// [Stream] reads them.
//
//	sm, err := io.NewStreamMarshaler(conn, &io.StreamOptions{Schema: s})
//	for _, rec := range records {
//	    if err := sm.Marshal(rec); err != nil { … }
//	}
//	err = sm.Close()
//
// It is the other half of streaming: without it this package could consume a
// stream but not produce one, so it could not sit on both ends of a link.
//
// A record is VALIDATED BEFORE ANYTHING IS WRITTEN, and a record that fails is
// not emitted, so a stream never carries a row its own reader would reject.
//
// SEQUENTIAL BY PROTOCOL. io-specs is explicit that writer calls must be
// issued sequentially — v1 does not define concurrent-write framing — so this
// type is not safe for concurrent use, exactly like [bufio.Writer]. A caller
// fanning in from several goroutines holds the lock itself.
type StreamMarshaler struct {
	w        *bufio.Writer
	schema   *schema.Schema
	active   string // the schema name currently in force on the wire
	defs     string // header text written before the first record
	wroteHdr bool
	closed   bool
	buf      []byte // reused between records
	err      error
	// resolver is the header's namespace, so an `@variable` a record refers to
	// resolves as it would in a document. Nil only when the stream declares no
	// header at all, and validation then runs against the empty namespace.
	resolver *document.Definitions
}

// NewStreamMarshaler starts a stream on w.
//
// StreamOptions.Schema is the schema every record is validated against and
// written under; StreamOptions.Definitions is header text emitted before the
// first record, so a reader can resolve names the records refer to. Both are
// optional: with neither, records are written unvalidated and header-less.
//
// Nothing is written until the first Marshal or Close, so a stream that
// carries no records still emits a well-formed empty document.
func NewStreamMarshaler(w goio.Writer, opts *StreamOptions) (*StreamMarshaler, error) {
	if w == nil {
		return nil, &MarshalError{Path: "$", Msg: "NewStreamMarshaler: nil writer"}
	}
	sm := &StreamMarshaler{w: bufio.NewWriter(w)}
	if opts != nil {
		sm.defs = opts.Definitions
		if opts.Schema != nil {
			sm.schema = opts.Schema.s
			if sm.defs == "" {
				// The schema has to reach the reader, or it cannot bind the
				// positional records this writes. Rendering it as the header
				// is the same text MarshalWith would produce.
				sm.defs = document.SchemaText(opts.Schema.s)
			}
		}
	}
	if sm.defs != "" {
		// Parsed once, here, rather than per record: the header cannot change
		// for the life of a stream.
		hdoc := document.Parse(headerSource(sm.defs))
		if len(hdoc.Errors) > 0 {
			return nil, toErrorList(hdoc.Errors)
		}
		sm.resolver = hdoc.Defs
	}
	return sm, nil
}

// Marshal writes one record — a struct, a map with string keys, or an *Object.
//
// The record is validated against the stream's schema first; a record that
// fails is REJECTED and nothing is written for it, so the stream stays
// readable. The error is the same ErrorList a Parse would report.
func (m *StreamMarshaler) Marshal(v any) error {
	return m.marshalAs(v, "")
}

// MarshalAs writes one record under an explicitly named schema, switching the
// stream's schema context when it is not already the one in force.
//
// This is how a heterogeneous stream is written — alerts among employees —
// and it needs the name to be defined in the header this stream carries.
func (m *StreamMarshaler) MarshalAs(v any, schemaName string) error {
	if schemaName == "" {
		return &MarshalError{Path: "$", Msg: "MarshalAs: the schema name is empty"}
	}
	return m.marshalAs(v, trimSigil(schemaName, '$'))
}

func (m *StreamMarshaler) marshalAs(v any, name string) error {
	if m.err != nil {
		return m.err
	}
	if m.closed {
		return m.fail("Marshal after Close")
	}

	sch := m.schema
	if name != "" {
		// A named switch resolves against the header this stream declared;
		// a name the reader will not know is refused here rather than
		// producing a stream that dies on the other end.
		if m.resolver == nil {
			return m.fail("MarshalAs: this stream declares no definitions")
		}
		s, cerr := m.resolver.SchemaOf(name)
		if cerr != nil || s == nil {
			return m.fail("MarshalAs: no schema named $" + name + " in this stream's header")
		}
		sch = s
	}

	rec, err := recordOf(v)
	if err != nil {
		return err
	}
	if sch != nil {
		validated, verrs := schema.ValidateRecordAt(rec, sch, m.defsOrEmpty(), true, "$")
		if len(verrs) > 0 {
			return toErrorList(verrs) // nothing is written
		}
		rec = validated
	}

	if err := m.writeHeader(); err != nil {
		return err
	}
	// A schema switch is emitted only when the effective schema CHANGES, which
	// io-specs asks of a writer; re-announcing it per record would be legal
	// but wasteful, and would obscure where the shape actually changes.
	if name != m.active {
		m.active = name
		sep := "---\n"
		if name != "" {
			sep = "--- $" + name + "\n"
		}
		if _, err := m.w.WriteString(sep); err != nil {
			return m.fail(err.Error())
		}
	}

	m.buf = m.buf[:0]
	m.buf = append(m.buf, '~', ' ')
	m.buf = document.AppendRecord(m.buf, rec, sch)
	m.buf = append(m.buf, '\n')
	if _, err := m.w.Write(m.buf); err != nil {
		return m.fail(err.Error())
	}
	return nil
}

// writeHeader emits the header and its `---` terminator, at most once and
// before any record. io-specs requires the terminator even when the header is
// empty, so the legacy header-less form is never produced.
func (m *StreamMarshaler) writeHeader() error {
	if m.wroteHdr {
		return nil
	}
	m.wroteHdr = true
	if m.defs != "" {
		if _, err := m.w.WriteString(m.defs); err != nil {
			return m.fail(err.Error())
		}
		if m.defs[len(m.defs)-1] != '\n' {
			if _, err := m.w.WriteString("\n"); err != nil {
				return m.fail(err.Error())
			}
		}
	}
	// The first record's separator is written by marshalAs, which also handles
	// a named switch; leaving it there keeps one statement of that rule.
	m.active = "\x00" // a value no schema name can equal, so the first record separates
	return nil
}

// Flush writes any buffered records through to the underlying writer.
func (m *StreamMarshaler) Flush() error {
	if m.err != nil {
		return m.err
	}
	if err := m.w.Flush(); err != nil {
		return m.fail(err.Error())
	}
	return nil
}

// Close finishes the stream and flushes it. A stream that carried no records
// still emits its header and terminator, so the result is a well-formed empty
// document rather than nothing at all.
//
// Close is idempotent; the underlying writer is not closed.
func (m *StreamMarshaler) Close() error {
	if m.closed {
		return m.err
	}
	m.closed = true
	if m.err != nil {
		return m.err
	}
	if err := m.writeHeader(); err != nil {
		return err
	}
	if m.active == "\x00" {
		// Nothing was written, so the terminator has not been emitted yet.
		if _, err := m.w.WriteString("---\n"); err != nil {
			return m.fail(err.Error())
		}
	}
	return m.Flush()
}

func (m *StreamMarshaler) fail(msg string) error {
	if m.err == nil {
		m.err = &MarshalError{Path: "$", Msg: msg}
	}
	return m.err
}

// defsOrEmpty is the namespace records validate against: the stream's own
// header, or nothing when it declared none.
func (m *StreamMarshaler) defsOrEmpty() schema.Defs {
	if m.resolver == nil {
		return schema.NoDefs{}
	}
	return m.resolver
}
