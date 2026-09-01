// Package internetobject implements the Internet Object data-interchange
// format (https://internetobject.org): a schema-first, lean, human-readable
// successor to JSON.
//
// Parsing accumulates errors rather than failing fast — Parse returns the
// document AND an error: the error lists every fault (stable kebab-case
// codes with positions), while the document still holds every record that
// survived. Values decode to a precise model: numbers are float64, bigints
// *big.Int, decimals keep their scale (1.50m is not 1.5m), temporals keep
// their kind (date, time, datetime), binary is []byte.
//
// This implementation passes the complete shared conformance corpus
// (io-test-cases): tokenizer, parser, schema, validation, serializer,
// document, streaming and regression suites.
package internetobject

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/schema"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The live value model, shared with the pipeline by construction.
type (
	// Object is an ordered key/value record.
	Object = value.Object
	// Member is one member of an Object.
	Member = value.Member
	// Decimal is an exact fixed-point value; its scale is part of the value.
	Decimal = value.Decimal
	// Temporal is a date, time or datetime; the kind stays distinct.
	Temporal = value.Temporal
)

// Error is one accumulated fault: a designated code and a 1-based position.
// Codes — never messages — are the conformance contract.
type Error struct {
	Code string
	Line int
	Col  int
}

func (e Error) Error() string {
	return fmt.Sprintf("%s at %d:%d", e.Code, e.Line, e.Col)
}

// ErrorList is every fault a document carried, in order. It is the error
// value Parse and ParseSchema return.
type ErrorList []Error

func (l ErrorList) Error() string {
	parts := make([]string, len(l))
	for i, e := range l {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

func toErrorList(es []errs.Error) error {
	if len(es) == 0 {
		return nil
	}
	out := make(ErrorList, len(es))
	for i, e := range es {
		out[i] = Error{Code: e.Code, Line: int(e.Line), Col: int(e.Col)}
	}
	return out
}

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
	doc := document.Load(src)
	return &Document{doc: doc}, toErrorList(doc.Errors)
}

// Value returns the document's projected live value: an *Object, a []any of
// records, a scalar, or nil for an empty document. Positional members carry
// their index as a numeric-string key.
func (d *Document) Value() any {
	return d.doc.Project()
}

// Errors returns the document's accumulated faults, in order.
func (d *Document) Errors() []Error {
	out := make([]Error, len(d.doc.Errors))
	for i, e := range d.doc.Errors {
		out[i] = Error{Code: e.Code, Line: int(e.Line), Col: int(e.Col)}
	}
	return out
}

// String renders the document as canonical Internet Object text. The output
// always re-parses to the same value, and writing it again yields the same
// text.
func (d *Document) String() string {
	return d.doc.Write()
}

// Schema is a compiled schema definition.
type Schema struct {
	s *schema.Schema
}

// ParseSchema compiles a schema definition string, e.g.
// "name: string, age: {int, min: 0}". Compilation fails fast: the error is
// an ErrorList holding the one designated fault.
func ParseSchema(def string) (*Schema, error) {
	s, cerr := document.CompileSchemaString(def)
	if cerr != nil {
		return nil, ErrorList{{Code: cerr.Code, Line: int(cerr.Line), Col: int(cerr.Col)}}
	}
	return &Schema{s: s}, nil
}

// MemberNames returns the schema's member names in declaration order.
func (s *Schema) MemberNames() []string {
	names := make([]string, 0, len(s.s.Names))
	for _, n := range s.s.Names {
		if n != "*" {
			names = append(names, n)
		}
	}
	return names
}

// Open reports whether the schema accepts undeclared members.
func (s *Schema) Open() bool { return s.s.Open != nil }
