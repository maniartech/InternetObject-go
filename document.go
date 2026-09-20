package internetobject

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
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

// TextOptions tunes how a document is written. A nil *TextOptions and the zero
// value mean the same thing, so callers that want the defaults pass nil.
type TextOptions struct {
	// SkipErrors writes the records that survived instead of refusing the
	// document, dropping every record that failed — a file that is
	// deliberately missing data, so ask for it only when that is what you
	// mean. What it writes re-parses, unless the document's own header or
	// schema is itself unwritable (docs/OPEN-QUESTIONS.md #9 and #10): the
	// header is written verbatim, and only RECORDS are skipped.
	//
	// It skips RECORDS. A fault that abandoned the load — a fatal parse, a bad
	// header, a broken schema binding — left the rest of the document
	// unvalidated, so there is nothing to skip and the document is refused
	// anyway.
	SkipErrors bool
}

// Text renders the document as canonical Internet Object text. The output
// re-parses to the same value, and writing it again yields the same text.
//
// A document that carries ANY fault is REFUSED, with [ForbiddenErrorNode]: a
// projection may describe errors — [Document.Value] and [Document.JSON] both
// embed the failed record — but a file must not contain them. Parsing
// accumulates faults and returns the document anyway, so without this rule a
// caller who parsed tolerantly and saved the result would write a silently
// truncated file. Set [TextOptions.SkipErrors] to write the survivors instead.
//
// The question asked is the document's FAULT LIST, not the shape of its
// records. A failed record becomes an error node, but a malformed literal does
// not — it stays in place as an unwritable value, and a fault in the header
// leaves no marked record at all. Both write corrupt text the reader cannot
// read back, so both must be refused, and only the fault list sees them.
// A nil or zero Document writes the empty document and reports no error, as
// [Document.JSON] writes "null" for one.
func (d *Document) Text(opts *TextOptions) (string, error) {
	if d == nil || d.doc == nil {
		return "", nil
	}
	// SkipErrors drops failed RECORDS, so it can only rescue faults whose
	// record actually left the document. A fault that abandoned the load, or
	// one whose record is still there holding a value nothing can spell, has
	// nothing to skip — writing anyway emits text the reader cannot read back.
	if e, refuse := d.refusal(opts); refuse {
		// Category is left to errs.CategoryOf, which states it once for every
		// code; setting it here would be a second statement of the same fact,
		// free to drift.
		return "", toError(errs.Error{
			Code:        errs.ForbiddenErrorNode,
			Path:        e.Path,
			RecordIndex: e.RecordIndex,
			Line:        e.Line,
			Col:         e.Col,
		})
	}
	return d.doc.String(), nil
}

// refusal reports the fault that stops this document being written, if any.
func (d *Document) refusal(opts *TextOptions) (errs.Error, bool) {
	if len(d.doc.Errors) == 0 {
		return errs.Error{}, false
	}
	if opts == nil || !opts.SkipErrors {
		return d.doc.Errors[0], true
	}
	for _, e := range d.doc.Errors {
		if !e.Recovered {
			return e, true
		}
	}
	return errs.Error{}, false
}

// MarshalText implements [encoding.TextMarshaler], so a Document travels
// through anything that writes a text form. It is [Document.Text] with the
// default options, and refuses a faulted document for the same reason.
func (d *Document) MarshalText() ([]byte, error) {
	s, err := d.Text(nil)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// String renders the document as canonical Internet Object text, for display.
//
// A [fmt.Stringer] cannot fail, so a document that [Document.Text] would refuse
// renders as a short note naming the fault instead. Use Text to handle that as
// an error, or to write the survivors with [TextOptions.SkipErrors].
//
// The note opens a brace it never closes, so it is a SYNTAX ERROR and cannot be
// mistaken for a document — which is the whole point of not returning the
// survivors here. Without it the note parses: a fault carrying no position
// renders as `<internetobject: forbidden-error-node>`, and `key: value` is
// valid Internet Object, so it would read back as a one-member record.
func (d *Document) String() string {
	s, err := d.Text(nil)
	if err != nil {
		return "{<internetobject: " + noteSafe.Replace(err.Error()) + ">"
	}
	return s
}

// noteSafe strips what a String note's message must not carry. The note's
// unclosed brace is what makes it a syntax error, and a member NAME reaches the
// message through the fault's path — a name may be quoted, so it may contain
// the `}` that would close it, or a `#` that comments away everything after.
// Measured 2026-09-20: without this, `~ $schema: {"a} #": int}` produced a note
// that parsed as an ordinary record.
var noteSafe = strings.NewReplacer("}", "�", "#", "�", "\n", " ", "\r", " ")
