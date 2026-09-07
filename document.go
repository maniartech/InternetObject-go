package internetobject

import (
	"github.com/maniartech/InternetObject-go/internal/document"
)

// Document is one parsed, validated Internet Object document.
type Document struct {
	doc *document.Doc
}

// Parse parses and validates src.
//
// The returned error is non-nil whenever the document carries faults (an
// ErrorList); the Document is still returned, holding every record that
// survived — the format's accumulate-and-continue promise.
func Parse(src string) (*Document, error) {
	doc := document.Parse(src)
	return &Document{doc: doc}, toErrorList(doc.Errors)
}

// Value returns the document's projected live value: an *Object, a []any of
// records, a scalar, or nil for an empty document. Positional members carry
// their index as a numeric-string key.
//
// The projection is a VIEW, not a copy. Projecting only drops absent slots and
// numbers unkeyed members, so wherever it would change nothing — which is
// every schema-validated record — the document's own values are returned as
// they are. Mutating what Value returns can therefore change what String
// writes; copy first if you need the two independent.
func (d *Document) Value() any {
	return d.doc.Project()
}

// Errors returns the document's accumulated faults, in order.
func (d *Document) Errors() []Error {
	out := make([]Error, len(d.doc.Errors))
	for i, e := range d.doc.Errors {
		out[i] = toError(e)
	}
	return out
}

// String renders the document as canonical Internet Object text. The output
// always re-parses to the same value, and writing it again yields the same
// text.
func (d *Document) String() string {
	return d.doc.String()
}
