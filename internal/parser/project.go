package parser

import (
	"strconv"

	"github.com/maniartech/InternetObject-go/internal/value"
)

// Project reduces a parsed document to its plain value model — the
// counterpart of the reference implementation's toJSON/toObject projection,
// which the parse-kind corpus suites assert:
//
//   - a keyed member projects under its key; a positional member under its
//     index as a numeric-string key ("0", "1", …);
//   - a record whose sole content was one braced object IS that object; any
//     other bare value is the member named "0" (handled at parse);
//   - a collection section projects to an array of records;
//   - multiple sections project to an object keyed by section name; a single
//     section drops the wrapper;
//   - a header with plain definitions wraps everything as {header, data};
//     one carrying only $schemas/@variables projects data alone;
//   - an empty document projects to nil.
func (d *Document) Project() any {
	data := d.projectSections()
	if d.Header != nil && d.Header.Plain != nil && len(d.Header.Plain.Members) > 0 {
		return &value.Object{Members: []value.Member{
			{Key: "header", Value: projectValue(d.Header.Plain)},
			{Key: "data", Value: data},
		}}
	}
	return data
}

func (d *Document) projectSections() any {
	switch len(d.Sections) {
	case 0:
		return nil
	case 1:
		return sectionValue(d.Sections[0])
	}
	out := &value.Object{}
	for _, sec := range d.Sections {
		out.Members = append(out.Members, value.Member{Key: sec.Name, Value: sectionValue(sec)})
	}
	return out
}

func sectionValue(sec *Section) any {
	if sec.Collection {
		// sec.Records is already a []any; when every record projects to
		// itself, it IS the projection.
		for i, r := range sec.Records {
			if pr, changed := project(r); changed {
				arr := make([]any, len(sec.Records))
				copy(arr, sec.Records[:i])
				arr[i] = pr
				for j := i + 1; j < len(sec.Records); j++ {
					arr[j] = projectValue(sec.Records[j])
				}
				return arr
			}
		}
		return sec.Records
	}
	if len(sec.Records) == 0 {
		return nil
	}
	return projectValue(sec.Records[0])
}

// ProjectValue projects one value: objects get every member keyed (positional
// and empty-quoted keys become index keys), recursively.
func ProjectValue(v any) any { return projectValue(v) }

func projectValue(v any) any {
	out, _ := project(v)
	return out
}

// project returns v's projection and whether that projection DIFFERS from v.
//
// The projection is copy-on-write. Projecting only ever does three things —
// drop absent slots, number positional and empty keys, recurse — so a value
// with no absent slot, no unkeyed member and no changed child projects to
// itself, and is returned as itself instead of being cloned.
//
// That case is not a corner: it is every schema-validated record. `assemble`
// emits one keyed, present member per declared slot, so the old code deep-
// cloned an entire validated document to produce a value-identical copy. It
// was the dynamic path's third full materialization of every record (parse
// builds one, validation assembles a second), and ~20% of its allocated bytes
// on a path where the GC already accounts for roughly half the CPU time.
//
// The tradeoff is aliasing: Document.Value() and Records() now hand back the
// document's own objects, so mutating a projection can change what String()
// writes. Leaf values ([]byte, *big.Int, Decimal.Coef) were always shared this
// way — this widens that to containers, and is documented on the public
// methods.
func project(v any) (any, bool) {
	switch x := v.(type) {
	case *value.Object:
		for i, m := range x.Members {
			pv, changed := project(m.Value)
			if m.Absent || m.Positional || m.Key == "" || changed {
				return projectObjectFrom(x, i, pv), true
			}
		}
		return x, false
	case []any:
		for i, e := range x {
			if pe, changed := project(e); changed {
				out := make([]any, len(x))
				copy(out, x[:i])
				out[i] = pe
				for j := i + 1; j < len(x); j++ {
					out[j] = projectValue(x[j])
				}
				return out, true
			}
		}
		return x, false
	default:
		return v, false
	}
}

// projectObjectFrom builds the projection of x, given that every member before
// `at` projects to itself and that member `at` projects to pv.
func projectObjectFrom(x *value.Object, at int, pv any) *value.Object {
	out := &value.Object{
		Line: x.Line, Col: x.Col,
		Members: make([]value.Member, 0, len(x.Members)),
	}
	out.Members = append(out.Members, x.Members[:at]...)
	for i := at; i < len(x.Members); i++ {
		m := x.Members[i]
		if m.Absent {
			continue // an empty comma slot projects nothing
		}
		val := pv
		if i != at {
			val = projectValue(m.Value)
		}
		key := m.Key
		if m.Positional || key == "" {
			key = strconv.Itoa(i)
		}
		out.Members = append(out.Members,
			value.Member{Key: key, Value: val, Line: m.Line, Col: m.Col})
	}
	return out
}
