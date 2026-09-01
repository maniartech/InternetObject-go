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
		arr := make([]any, len(sec.Records))
		for i, r := range sec.Records {
			arr[i] = projectValue(r)
		}
		return arr
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
	switch x := v.(type) {
	case *value.Object:
		out := &value.Object{Members: make([]value.Member, 0, len(x.Members))}
		for i, m := range x.Members {
			if m.Absent {
				continue // an empty comma slot projects nothing
			}
			key := m.Key
			if m.Positional || key == "" {
				key = strconv.Itoa(i)
			}
			out.Members = append(out.Members, value.Member{Key: key, Value: projectValue(m.Value)})
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = projectValue(e)
		}
		return out
	default:
		return v
	}
}
