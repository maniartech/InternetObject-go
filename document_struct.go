package internetobject

import (
	"fmt"
	"reflect"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// Binding a whole document to a struct — the no-ceremony form of what
// io.SectionAs does one section at a time:
//
//	type Dashboard struct {
//	    Employees []Employee `io:"employees"`
//	    Alerts    []Alert    `io:"alerts"`
//	    Stats     []Stat     `io:"stats"`
//	}
//	var d Dashboard
//	err := io.Unmarshal(payload, &d)
//
// Section ↔ field by `io` tag name (ADR 0004 D4). Sections the struct does not
// name are ignored, as encoding/json ignores unknown keys; fields the document
// does not carry stay zero.
//
// **How this is chosen, and why it cannot fire by accident.** Section binding
// is used only when at least one of the struct's `io` tags names a section the
// document ACTUALLY HAS. That is an explicit opt-in written by the caller — you
// do not name a field after a section unless you mean it — so a record struct
// keeps binding as a record, and adding a field never silently changes what an
// existing struct means. The alternative, guessing from field types, is how the
// bug this replaced got in.

// sectionBinding reports the document sections this struct names, keyed by
// section name, or nil when the struct names none of them.
func sectionBinding(t reflect.Type, doc *document.Doc) (map[string]fieldPlan, error) {
	plan, err := planFor(t)
	if err != nil {
		return nil, err
	}
	// Only a document with an EXPLICITLY NAMED section can bind by section, and
	// the unnamed one is never the reason to start.
	//
	// The unnamed section is NAMED "data" by the parser, so without this a
	// perfectly ordinary record struct with a member called `data` would take
	// the section path and fail — which is exactly what a generated type did,
	// caught by the generated-code corpus gate. A tag naming a section is only
	// an opt-in when the section was named on purpose, and only the writer's
	// own predicate can say whether it was.
	named := false
	for _, sec := range doc.Sections {
		if !document.IsDefaultSectionName(sec.Name) {
			named = true
			break
		}
	}
	if !named {
		return nil, nil
	}
	present := make(map[string]bool, len(doc.Sections))
	for _, sec := range doc.Sections {
		n := sec.Name
		if n == "" {
			n = DefaultSectionName
		}
		present[n] = true
	}
	var out map[string]fieldPlan
	for _, f := range plan.fields {
		if !present[f.name] {
			continue
		}
		if out == nil {
			out = make(map[string]fieldPlan, len(plan.fields))
		}
		out[f.name] = f
	}
	return out, nil
}

// bindSections binds each named section into the struct field that names it.
func bindSections(doc *document.Doc, elem reflect.Value, fields map[string]fieldPlan) error {
	for _, sec := range doc.Sections {
		name := sec.Name
		if name == "" {
			name = DefaultSectionName
		}
		f, ok := fields[name]
		if !ok {
			continue // a section this struct does not ask for
		}
		fv := elem.FieldByIndex(f.index)
		at := pathAt{root: "$." + name}

		switch fv.Kind() {
		case reflect.Slice:
			records := sectionRecords(sec)
			out := reflect.MakeSlice(fv.Type(), len(records), len(records))
			for i, rec := range records {
				if err := bindInto(out.Index(i), rec, at.record(i)); err != nil {
					return err
				}
			}
			fv.Set(out)

		case reflect.Struct, reflect.Pointer:
			records := sectionRecords(sec)
			if len(records) != 1 {
				return &UnmarshalError{Path: at.String(), Msg: fmt.Sprintf(
					"section holds %d records; bind it to a slice", len(records))}
			}
			if err := bindInto(fv, records[0], at); err != nil {
				return err
			}

		default:
			return &UnmarshalError{Path: at.String(), Msg: fmt.Sprintf(
				"a section binds to a slice or a struct, not %s", fv.Kind())}
		}
	}
	return nil
}

func sectionRecords(sec *parser.Section) []*value.Object {
	out := make([]*value.Object, 0, len(sec.Records))
	for _, rec := range sec.Records {
		if obj, ok := rec.(*value.Object); ok {
			out = append(out, obj)
		}
	}
	return out
}
