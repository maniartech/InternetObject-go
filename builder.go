package internetobject

import (
	"reflect"
	"strconv"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Builder assembles a document by hand, for the case a Go type cannot express:
// sections whose shape is decided at runtime — a gateway fanning several
// entity types into one response, a migration writing whatever it reads.
//
//	b := io.NewBuilder()
//	b.Define("Employee", empSchema)
//	emp := b.Section("employees", "Employee")
//	if err := emp.Add(map[string]any{"name": "Alice", "age": 30}); err != nil { … }
//	doc, err := b.Document()
//	text := doc.String()
//
// A record is VALIDATED WHEN IT IS ADDED, so a fault is reported at the call
// that caused it rather than at the end with a path to decode. A builder can
// therefore never produce a document its own parser would reject.
//
// A Builder is single-owner and NOT safe for concurrent use — it is the one
// mutable thing in this package, and it exists to be filled in and finished.
// Everything it produces (a *Document, a *Schema) is immutable and shareable.
type Builder struct {
	header   *parser.Header
	defs     *document.Definitions
	sections []*SectionBuilder
	byName   map[string]*SectionBuilder
	rows     int // records added, across all sections
	err      error
}

// SectionBuilder collects the records of one section.
type SectionBuilder struct {
	b   *Builder
	sec *parser.Section
	sch *schema.Schema
}

// NewBuilder returns an empty document to fill in.
func NewBuilder() *Builder {
	h := &parser.Header{Schemas: map[string]any{}, Vars: map[string]any{}}
	return &Builder{
		header: h,
		defs:   document.NewDefinitions(h),
		byName: map[string]*SectionBuilder{},
	}
}

// NewBuilderFrom starts from a parsed document, CLONING it, so the original
// stays the immutable value every reader of it expects.
//
// This is how a parsed document is edited: read it, clone it, change the
// clone, write that. Mutating what Value or Records returns is a programming
// error — those are views of the document's own objects.
func NewBuilderFrom(doc *Document) *Builder {
	b := NewBuilder()
	if doc == nil || doc.doc == nil {
		return b
	}
	if h := doc.doc.Header; h != nil {
		for _, def := range h.Defs {
			switch def.Kind {
			case parser.DefSchema:
				b.header.Schemas[def.Key] = def.Value
			case parser.DefVar:
				b.header.Vars[def.Key] = def.Value
			}
			b.header.Upsert(def)
		}
		b.header.Inline = h.Inline
	}
	for _, sec := range doc.doc.Sections {
		name := sec.Name
		if document.IsDefaultSectionName(name) {
			name = ""
		}
		sb := b.Section(name, sec.SchemaName)
		if b.err != nil {
			return b
		}
		sb.sec.Collection = sec.Collection
		for _, rec := range sec.Records {
			if obj, ok := rec.(*core.Object); ok {
				sb.sec.Records = append(sb.sec.Records, obj.Clone())
				b.rows++
			}
		}
	}
	return b
}

// Define adds a named schema to the header, so sections can bind to it by name
// and records can reference it.
//
// It is refused once any record has been added: a definition is only
// meaningful against the records validated with it, so changing one underneath
// them would leave a document whose rows no longer match its own header.
func (b *Builder) Define(name string, s *Schema) *Builder {
	if b.err != nil {
		return b
	}
	if s == nil || s.s == nil {
		b.fail("Define(" + name + "): the schema is nil")
		return b
	}
	if b.rows > 0 {
		b.fail("Define(" + name + "): definitions cannot change once records have been added")
		return b
	}
	name = trimSigil(name, '$')
	b.header.Schemas[name] = s.s
	b.header.Upsert(parser.HeaderDef{Kind: parser.DefSchema, Key: name, Value: s.s})
	b.defs = document.NewDefinitions(b.header)
	return b
}

// Var adds an `@variable` to the header. Like Define, it is refused once
// records exist.
func (b *Builder) Var(name string, v any) *Builder {
	if b.err != nil {
		return b
	}
	if b.rows > 0 {
		b.fail("Var(" + name + "): definitions cannot change once records have been added")
		return b
	}
	name = trimSigil(name, '@')
	b.header.Vars[name] = v
	b.header.Upsert(parser.HeaderDef{Kind: parser.DefVar, Key: name, Value: v})
	b.defs = document.NewDefinitions(b.header)
	return b
}

// Section opens a section, binding it to a named schema. An empty name is the
// document's single unnamed section; an empty schema name leaves the section
// unvalidated and header-less.
//
// Opening the same name twice returns the section already opened, so records
// can be added to it in more than one place.
func (b *Builder) Section(name, schemaName string) *SectionBuilder {
	if sb, ok := b.byName[name]; ok {
		return sb
	}
	sb := &SectionBuilder{b: b, sec: &parser.Section{
		Name:       name,
		SchemaName: trimSigil(schemaName, '$'),
		Collection: true,
	}}
	if sb.sec.Name == "" {
		sb.sec.Name = DefaultSectionName
	}
	if sb.sec.SchemaName != "" {
		s, cerr := b.defs.SchemaOf(sb.sec.SchemaName)
		if cerr != nil {
			b.fail("Section(" + name + "): no schema named $" + sb.sec.SchemaName)
			return sb
		}
		sb.sch = s
	}
	b.sections = append(b.sections, sb)
	b.byName[name] = sb
	return sb
}

// Add appends one record — a struct, a map with string keys, or an *Object —
// validating it against the section's schema first. A record that does not
// satisfy the schema is REJECTED and not added.
func (s *SectionBuilder) Add(v any) error {
	if s.b.err != nil {
		return s.b.err
	}
	rec, err := recordOf(v)
	if err != nil {
		return err
	}
	if s.sch != nil {
		validated, verrs := schema.ValidateRecordAt(rec, s.sch, s.b.defs, true, "$["+strconv.Itoa(len(s.sec.Records))+"]")
		if len(verrs) > 0 {
			return toErrorList(verrs)
		}
		rec = validated
	}
	s.sec.Records = append(s.sec.Records, rec)
	s.b.rows++
	return nil
}

// Len is the number of records added to this section.
func (s *SectionBuilder) Len() int { return len(s.sec.Records) }

// Document finishes the build. The result is an ordinary *Document: immutable,
// shareable, and renderable with String.
func (b *Builder) Document() (*Document, error) {
	if b.err != nil {
		return nil, b.err
	}
	pdoc := &parser.Document{Header: b.header}
	for _, sb := range b.sections {
		pdoc.Sections = append(pdoc.Sections, sb.sec)
	}
	if len(pdoc.Sections) == 0 {
		pdoc.Sections = append(pdoc.Sections,
			&parser.Section{Name: DefaultSectionName, Collection: true})
	}
	doc, cerr := document.NewUnvalidated(pdoc)
	if cerr != nil {
		return nil, toErrorList([]errs.Error{*cerr})
	}
	return &Document{doc: doc}, nil
}

// String is Document().String(), for the common case of building text.
// A build error is returned as the empty string; call Document to see it.
func (b *Builder) String() string {
	doc, err := b.Document()
	if err != nil {
		return ""
	}
	return doc.String()
}

// Err reports the first fault the builder hit, or nil.
func (b *Builder) Err() error { return b.err }

func (b *Builder) fail(msg string) {
	if b.err == nil {
		b.err = &MarshalError{Path: "$", Msg: msg}
	}
}

// recordOf turns a value into the record the writer wants, reusing the
// marshaler's own encoder so a builder and a Marshal agree about what a struct
// or a map becomes.
func recordOf(v any) (*core.Object, error) {
	if obj, ok := v.(*core.Object); ok {
		if obj == nil {
			return nil, &MarshalError{Path: "$", Msg: "Add: the record is nil"}
		}
		return encodeObject(obj, rootPath)
	}
	if v == nil {
		return nil, &MarshalError{Path: "$", Msg: "Add: the record is nil"}
	}
	ev, err := encodeValue(reflect.ValueOf(v), "", rootPath)
	if err != nil {
		return nil, err
	}
	obj, ok := ev.(*core.Object)
	if !ok {
		return nil, &MarshalError{Path: "$", Msg: "Add: a record must be a struct, a map or an *Object"}
	}
	return obj, nil
}
