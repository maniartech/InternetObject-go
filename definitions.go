package internetobject

import (
	goio "io"
	"iter"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Definitions is a header compiled once: the named schemas, the `@variables`
// and the default schema a document's `---` binds to, all resolved up front.
//
// It exists for the case the format is built for — a schema agreed out of band
// between a publisher and a subscriber, so the wire carries only data. Compile
// it once at startup and reuse it for every payload:
//
//	defs, err := io.ParseDefinitions(headerText)
//	doc, err := defs.Parse(payload)
//	for item, err := range defs.Stream(conn, nil) { … }
//
// A *Definitions is IMMUTABLE and safe to use from many goroutines at once.
// That is the difference between this and the resolver a parsed document
// carries internally: that one memoizes as it goes, so it belongs to one
// document on one goroutine, while this one has no work left to do.
//
// PRECEDENCE, as io-specs requires: a document's own header wins. A name it
// defines shadows the same name here, its own `$schema` replaces this default,
// and where it defines neither, these stay in force.
type Definitions struct {
	frozen *document.Frozen
}

// ParseDefinitions compiles header text — the part of a document before its
// first `---`, with or without the separator — into a reusable value.
//
// Every definition is compiled NOW, so a header that cannot compile is an
// error here rather than a surprise on the thousandth payload.
func ParseDefinitions(src string) (*Definitions, error) {
	// A header is parsed as the header of an empty document; adding the
	// separator lets a caller pass the text either way.
	doc := document.Parse(headerSource(src))
	if len(doc.Errors) > 0 {
		return nil, toErrorList(doc.Errors)
	}
	f, cerr := document.Freeze(doc.Header)
	if cerr != nil {
		return nil, toErrorList([]errs.Error{*cerr})
	}
	return &Definitions{frozen: f}, nil
}

// Schema returns a named schema, or nil when the header does not define one.
// The name may be written with or without its `$`.
func (d *Definitions) Schema(name string) *Schema {
	if d == nil || d.frozen == nil {
		return nil
	}
	if s, ok := d.frozen.Schemas[trimSigil(name, '$')]; ok {
		return &Schema{s: s}
	}
	return nil
}

// Default is the schema a document's unnamed section binds to — the header's
// `$schema`, or its bare schema expression. It is nil when the header declares
// neither.
func (d *Definitions) Default() *Schema {
	if d == nil || d.frozen == nil || d.frozen.Default == nil {
		return nil
	}
	return &Schema{s: d.frozen.Default}
}

// Var returns a variable's value. The name may be written with or without
// its `@`.
func (d *Definitions) Var(name string) (any, bool) {
	if d == nil || d.frozen == nil {
		return nil, false
	}
	v, ok := d.frozen.Vars[trimSigil(name, '@')]
	return v, ok
}

// Names returns every definition's name in the order the header wrote them,
// sigils stripped.
func (d *Definitions) Names() []string {
	if d == nil || d.frozen == nil {
		return nil
	}
	out := make([]string, len(d.frozen.Names))
	copy(out, d.frozen.Names)
	return out
}

// Len is the number of definitions.
func (d *Definitions) Len() int {
	if d == nil || d.frozen == nil {
		return 0
	}
	return len(d.frozen.Names)
}

// String renders the definitions as canonical Internet Object header text —
// the part BEFORE the `---`, with no separator — so it can be sent to a peer
// that reads it back with ParseDefinitions, or handed to
// StreamOptions.Definitions.
func (d *Definitions) String() string {
	if d == nil || d.frozen == nil || d.frozen.Header == nil {
		return "---"
	}
	return document.NewUnvalidatedHeader(d.frozen.Header).String()
}

// Parse reads a document with these definitions in scope, so the text need
// carry no header of its own. A header it does carry takes precedence.
func (d *Definitions) Parse(src string) (*Document, error) {
	var frozen *document.Frozen
	if d != nil {
		frozen = d.frozen
	}
	doc := document.ParseWithDefs(src, frozen)
	pub := &Document{doc: doc}
	if len(doc.Errors) > 0 {
		return pub, toErrorList(doc.Errors)
	}
	return pub, nil
}

// Stream reads records from r with these definitions in scope — the shared,
// out-of-band deployment mode, where the wire carries only data sections.
//
// In-stream definitions still override these, and an in-stream `$schema`
// replaces this default; where the stream declares neither, these stay in
// force (io-specs/streaming/schema-and-state.md).
func (d *Definitions) Stream(r goio.Reader, opts *StreamOptions) iter.Seq2[StreamItem, error] {
	o := StreamOptions{}
	if opts != nil {
		o = *opts
	}
	// The reader takes preloaded definitions as header TEXT. Rendering this
	// header back is exact — it is the text it was compiled from — and keeps
	// one statement of the precedence rule rather than a second one here.
	if o.Definitions == "" && d != nil && d.frozen != nil && d.frozen.Header != nil {
		o.Definitions = d.String()
	}
	return Stream(r, &o)
}

// Definitions returns a read-only view of this document's own header.
//
// It is a VIEW: it shares the document's compiled schemas rather than copying
// them, and like the document itself it must not be mutated.
func (doc *Document) Definitions() *Definitions {
	f, cerr := document.Freeze(doc.doc.Header)
	if cerr != nil {
		return &Definitions{}
	}
	return &Definitions{frozen: f}
}

// Var resolves one of the document header's `@variables`.
func (doc *Document) Var(name string) (any, bool) {
	v, err := doc.doc.Defs.Var(trimSigil(name, '@'))
	if err != nil {
		return nil, false
	}
	return v, true
}

// headerSource makes header text parseable on its own: a caller may pass it
// with or without the `---` that ends it.
func headerSource(src string) string {
	for i := 0; i+2 < len(src)+1 && i < len(src); i++ {
		if src[i] == '-' && i+2 < len(src) && src[i+1] == '-' && src[i+2] == '-' {
			return src // it already carries a separator; parse as written
		}
	}
	return src + "\n---\n"
}

// trimSigil drops a leading `$` or `@` so a caller may write a name either way.
func trimSigil(name string, sigil byte) string {
	if len(name) > 0 && name[0] == sigil {
		return name[1:]
	}
	return name
}
