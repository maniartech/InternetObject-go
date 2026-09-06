package internetobject

import (
	"fmt"
	"reflect"

	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Sections — the core type behind a document that carries several entity types
// at once, which is the format's own reason for existing. A dashboard response
// is one document with `--- employees`, `--- alerts` and `--- stats`, each bound
// to its own schema, and the receiver takes the part it wants.
//
// Modelled on io-js2's IOSection/IODocument, adapted rather than transliterated:
//
//	TypeScript                        Go
//	doc.sections.get(name)            doc.Section(name)
//	doc.sections.getAt(i) / .length   doc.Sections()  — a slice IS the collection
//	section.name / .schemaName        s.Name() / s.SchemaName()
//	section.data                      s.Records() (always a slice) / s.Value()
//	loadCollection<T>(section)        io.SectionAs[T](doc, name)  — generics
//
// The one place Go should NOT follow TypeScript is `section.data`, which is a
// union of collection-or-object-or-null. Records() is always a slice, so a
// caller writes one loop rather than a type switch.

// DefaultSectionName is the name an unnamed section answers to, matching the
// reference implementation. `--- ` with no name and `--- data` are the same
// section to Section().
const DefaultSectionName = "data"

// Section is one section of a parsed document: its name, the schema it was
// validated against, and its records.
type Section struct {
	sec  *parser.Section
	sch  *schema.Schema
	docp *Document
}

// Name reports the section's name, or DefaultSectionName for the unnamed one.
func (s *Section) Name() string {
	if s.sec.Name == "" {
		return DefaultSectionName
	}
	return s.sec.Name
}

// SchemaName reports the `--- name: $Schema` selector, empty when the section
// bound to the document's default schema instead.
func (s *Section) SchemaName() string { return s.sec.SchemaName }

// Schema returns the compiled schema this section was validated against, or nil.
func (s *Section) Schema() *Schema {
	if s.sch == nil {
		return nil
	}
	return newSchema(s.sch)
}

// IsCollection reports whether the section was written as a collection (`~`
// rows) rather than a single bare record.
func (s *Section) IsCollection() bool { return s.sec.Collection }

// Len is the number of records in the section.
func (s *Section) Len() int { return len(s.sec.Records) }

// Records returns the section's records as live values — ALWAYS a slice, with
// one element for a bare record. Faulted rows keep their place and carry their
// marker; test one with IsError.
//
// As with Document.Value, these are a VIEW of the document's own records.
func (s *Section) Records() []any {
	out := make([]any, 0, len(s.sec.Records))
	for _, rec := range s.sec.Records {
		out = append(out, parser.ProjectValue(rec))
	}
	return out
}

// Value returns the section's data the way the document projects it: a []any
// for a collection, the record itself for a bare section, nil when empty.
func (s *Section) Value() any {
	if s.sec.Collection {
		return s.Records()
	}
	if len(s.sec.Records) == 0 {
		return nil
	}
	return parser.ProjectValue(s.sec.Records[0])
}

// Sections returns every section in document order.
func (d *Document) Sections() []*Section {
	out := make([]*Section, 0, len(d.doc.Sections))
	for _, sec := range d.doc.Sections {
		out = append(out, &Section{sec: sec, sch: d.doc.SecSchemas[sec], docp: d})
	}
	return out
}

// Section returns the named section, or nil when the document has none.
// DefaultSectionName matches the unnamed section.
func (d *Document) Section(name string) *Section {
	for _, sec := range d.doc.Sections {
		n := sec.Name
		if n == "" {
			n = DefaultSectionName
		}
		if n == name {
			return &Section{sec: sec, sch: d.doc.SecSchemas[sec], docp: d}
		}
	}
	return nil
}

// SectionAs binds one section's records into a slice of T — the typed way to
// take one entity type out of a document carrying several.
//
//	employees, err := io.SectionAs[Employee](doc, "employees")
//	alerts, err := io.SectionAs[Alert](doc, "alerts")
//
// This is the Go form of the `…As[T]` suffix (ADR 0004 D0): the same read,
// producing Go values of type T. A missing section is an error rather than an
// empty slice, because "the sender did not send it" and "the sender sent none"
// are different facts and silently merging them is how dashboards lie.
func SectionAs[T any](d *Document, name string) ([]T, error) {
	if d == nil {
		return nil, &UnmarshalError{Path: "$", Msg: "nil document"}
	}
	sec := d.Section(name)
	if sec == nil {
		have := make([]string, 0, len(d.doc.Sections))
		for _, s := range d.Sections() {
			have = append(have, s.Name())
		}
		return nil, &UnmarshalError{Path: "$." + name,
			Msg: fmt.Sprintf("no such section; the document has %v", have)}
	}
	out := make([]T, len(sec.sec.Records))
	for i, rec := range sec.sec.Records {
		obj, ok := rec.(*Object)
		if !ok {
			return nil, &UnmarshalError{
				Path: fmt.Sprintf("$.%s[%d]", name, i),
				Msg:  "record did not survive validation; read it with Records() and test IsError",
			}
		}
		if err := bindInto(reflect.ValueOf(&out[i]).Elem(), obj, rootPath.record(i)); err != nil {
			return nil, err
		}
	}
	return out, nil
}
