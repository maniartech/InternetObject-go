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
//
// # Vocabulary
//
// Every name in this package follows one rule, so the verb tells you what
// kind of thing you get back:
//
//	Marshal / Unmarshal   Go value  ⇄ IO text   (like encoding/json)
//	Parse    / String     IO text   ⇄ this package's own types,
//	                      Document and Schema   (like url.Parse / URL.String)
//	Stream                incremental reading, one record at a time
//	Validate              check a value; produce no text
//
// Two suffixes modify any of them without changing the verb:
//
//	…With(…, s *Schema)   the same operation against an explicitly supplied
//	                      schema instead of a derived or embedded one
//	…As[T](…)             the same operation producing Go values of type T
//
// So Parse gives you a *Document to navigate, Unmarshal fills your struct,
// and ParseWith / UnmarshalWith are those two against a runtime schema. There
// is no Load, Read, Decode or Write in the public surface: one verb per
// direction, everywhere.
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
	// TemporalKind distinguishes the three temporal literals. A value KEEPS
	// the kind it carries end to end: a midnight datetime is not a date, and
	// a 1900-01-01 date is not a time of day.
	TemporalKind = value.TemporalKind
	// ErrorItem stands in for a record that failed, INSIDE projected data:
	// the row keeps its position and carries this marker instead of a value,
	// so the good records around it are untouched. Test for it with IsError.
	ErrorItem = value.ErrorNode
)

// The temporal kinds, so a caller can name the one a value carries.
const (
	KindDate     = value.KindDate
	KindTime     = value.KindTime
	KindDateTime = value.KindDateTime
)

// IsError reports whether a projected value is a failed record.
//
// This is a TYPE check, never a property check: a schema may legitimately
// declare a member called "__error" or "code", so data must never be able to
// impersonate a failure (ADR 0005 D4).
func IsError(v any) bool {
	_, ok := v.(ErrorItem)
	return ok
}

// Error is one accumulated fault: a designated code and a 1-based position.
// Codes — never messages — are the conformance contract.
type Error struct {
	// Code is the designated kebab-case code — the conformance contract.
	Code string
	// Category is derived from where the fault arose, never from the code's
	// spelling: "syntax", "validation", "stream" or "general".
	Category string
	// Path locates the fault structurally: "$" is the document root, "$[2]"
	// the third record of a collection, "$[2].age" a member of it.
	Path string
	// RecordIndex is the 0-based position of the failed record within its
	// collection, or -1 when the fault is not inside one.
	RecordIndex int
	// Line, Col are the 1-based position of the offending value.
	Line int
	Col  int
}

func (e Error) Error() string {
	if e.Path != "" && e.Path != "$" {
		return fmt.Sprintf("%s at %s (%d:%d)", e.Code, e.Path, e.Line, e.Col)
	}
	return fmt.Sprintf("%s at %d:%d", e.Code, e.Line, e.Col)
}

// Is reports whether target is an Error with the same code, so
// errors.Is(err, io.Error{Code: "mismatched-min"}) works on a single fault.
func (e Error) Is(target error) bool {
	t, ok := target.(Error)
	return ok && t.Code == e.Code
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

// Codes returns the designated codes in order — the conformance-relevant
// projection of the list.
func (l ErrorList) Codes() []string {
	out := make([]string, len(l))
	for i, e := range l {
		out[i] = e.Code
	}
	return out
}

// Has reports whether any fault carries the given code.
func (l ErrorList) Has(code string) bool {
	for _, e := range l {
		if e.Code == code {
			return true
		}
	}
	return false
}

// Is lets errors.Is(err, io.Error{Code: …}) match a list containing that code.
func (l ErrorList) Is(target error) bool {
	t, ok := target.(Error)
	return ok && l.Has(t.Code)
}

// Unwrap exposes the individual faults to errors.Is/As traversal.
func (l ErrorList) Unwrap() []error {
	out := make([]error, len(l))
	for i, e := range l {
		out[i] = e
	}
	return out
}

// toError is THE conversion from an internal fault to the public one, so
// every entry point reports the same fields.
func toError(e errs.Error) Error {
	cat := e.Category
	if cat == "" {
		cat = errs.CategoryOf(e.Code)
	}
	idx := e.RecordIndex
	if idx == 0 && e.Path == "" {
		idx = -1 // never parsed from a collection context
	}
	return Error{
		Code: e.Code, Category: cat, Path: e.Path,
		RecordIndex: idx, Line: int(e.Line), Col: int(e.Col),
	}
}

func toErrorList(es []errs.Error) error {
	if len(es) == 0 {
		return nil
	}
	out := make(ErrorList, len(es))
	for i, e := range es {
		out[i] = toError(e)
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
	doc := document.Parse(src)
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

// Schema is a compiled schema definition.
type Schema struct {
	s *schema.Schema
}

// ParseSchema compiles a schema definition string, e.g.
// "name: string, age: {int, min: 0}". Compilation fails fast: the error is
// an ErrorList holding the one designated fault.
func ParseSchema(def string) (*Schema, error) {
	s, cerr := document.ParseSchema(def)
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
