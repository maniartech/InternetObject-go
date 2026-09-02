package internetobject

import (
	"fmt"
	"reflect"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// Runtime schemas (ADR 0004 D5). A schema is data: it may be derived from Go
// types and `schema` tags at compile time, carried in the document's own
// header, or — as here — parsed ONCE from anywhere (a registry, a file, a
// remote service) and then applied to many documents and values.
//
// The `With` functions take that already-compiled *Schema. It wins outright
// over both the document's header and the type's tags; `io` tags still name
// the members. Nothing is re-parsed per call, and the data text is never
// concatenated with schema text.

// ParseWith parses src and validates every record against s instead of
// whatever the document's own header would bind. Definitions in the header
// (@variables, $refs) are still read, so records may reference them.
//
// Like Parse, the error lists every fault and the document still holds the
// records that survived.
func ParseWith(src string, s *Schema) (*Document, error) {
	if s == nil {
		return nil, ErrorList{{Code: "invalid-schema", Line: 1, Col: 1}}
	}
	doc := document.LoadWith(src, s.s)
	return &Document{doc: doc}, toErrorList(doc.Errors)
}

// UnmarshalWith parses src, validates it against the already-compiled schema
// s, and stores the result in the value pointed to by v.
//
// This is the runtime-schema counterpart of Unmarshal: the wire text needs no
// header of its own, and v's `schema` tags (if any) are not consulted — s is
// the single authority for types and constraints.
func UnmarshalWith(src string, v any, s *Schema) error {
	if s == nil {
		return &UnmarshalError{Path: "$", Msg: "nil schema"}
	}
	return bindDoc(document.LoadWith(src, s.s), v)
}

// MarshalWith renders v as an Internet Object document using s as the schema:
// the records are validated against it, it is written as the document header,
// and members are emitted positionally in ITS order.
func MarshalWith(v any, s *Schema) (string, error) {
	if s == nil {
		return "", &MarshalError{Path: "$", Msg: "nil schema"}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return "", &MarshalError{Path: "$", Msg: "cannot marshal a nil value"}
		}
		rv = rv.Elem()
	}

	sec := &parser.Section{Name: "data"}
	switch {
	case rv.Kind() == reflect.Struct && !isModelStruct(rv.Type()):
		plan, err := planFor(rv.Type())
		if err != nil {
			return "", err
		}
		rec, err := encodeStruct(rv, plan, "$")
		if err != nil {
			return "", err
		}
		sec.Records = []any{rec}

	case rv.Kind() == reflect.Slice && isStructElem(rv.Type().Elem()):
		et := rv.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		plan, err := planFor(et)
		if err != nil {
			return "", err
		}
		sec.Collection = true
		for i := 0; i < rv.Len(); i++ {
			ev := rv.Index(i)
			path := fmt.Sprintf("$[%d]", i)
			for ev.Kind() == reflect.Pointer {
				if ev.IsNil() {
					return "", &MarshalError{Path: path, Msg: "a collection record cannot be nil"}
				}
				ev = ev.Elem()
			}
			rec, err := encodeStruct(ev, plan, path)
			if err != nil {
				return "", err
			}
			sec.Records = append(sec.Records, rec)
		}

	default:
		return "", &MarshalError{Path: "$", Msg: "MarshalWith takes a struct or a slice of structs"}
	}

	if err := checkRecords(s.s, sec.Records); err != nil {
		return "", err
	}
	pdoc := &parser.Document{Sections: []*parser.Section{sec}}
	return document.NewWithSchema(pdoc, s.s).Write(), nil
}

// Schema returns the schema a section of the parsed document was validated
// against, or nil when it had none.
func (d *Document) Schema() *Schema {
	for _, sec := range d.doc.Sections {
		if s := d.doc.SecSchemas[sec]; s != nil {
			return &Schema{s: s}
		}
	}
	return nil
}

// SchemaOf returns a named schema defined in the document's header — the way
// to lift a `$Person` definition out of one document and reuse it against
// others.
func (d *Document) SchemaOf(name string) (*Schema, error) {
	s, cerr := d.doc.Defs.SchemaOf(name)
	if cerr != nil {
		return nil, ErrorList{{Code: cerr.Code, Line: int(cerr.Line), Col: int(cerr.Col)}}
	}
	return &Schema{s: s}, nil
}

// Records returns the document's records as live values, faulted ones
// included (as their error markers) — the dynamic counterpart of Unmarshal.
func (d *Document) Records() []any {
	var out []any
	for _, sec := range d.doc.Sections {
		for _, rec := range sec.Records {
			if _, bad := rec.(value.ErrorNode); bad {
				out = append(out, nil)
				continue
			}
			out = append(out, parser.ProjectValue(rec))
		}
	}
	return out
}
