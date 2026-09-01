package conformance

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The stream comparator. Every case runs under THREE chunkings — whole,
// per-line, per-byte — and all three must produce the identical item
// sequence, because transport chunk boundaries carry no meaning. The
// per-byte run is what exercises multibyte decoding and split frames.
func RunStreamCase(row SuiteRow) []string {
	want := expectedStream(row.Expected)
	var problems []string
	for _, strategy := range []string{"whole", "per-line", "per-byte"} {
		got := runStream(row, strategy)
		if !value.Equal(got, want) {
			problems = append(problems, fmt.Sprintf("[%s]\n     expected=%s\n     actual  =%s",
				strategy, Show(want), Show(got)))
		}
	}
	return problems
}

func runStream(row SuiteRow, strategy string) *value.Object {
	r := document.NewReader(document.StreamOptions{
		Definitions:   row.Definitions,
		DefaultSchema: sigil(row.DefaultSchema),
	})
	var items []document.Item
	switch strategy {
	case "whole":
		items = append(items, r.Feed([]byte(row.Input))...)
	case "per-line":
		rest := row.Input
		for rest != "" {
			i := strings.IndexByte(rest, '\n')
			if i < 0 {
				items = append(items, r.Feed([]byte(rest))...)
				break
			}
			items = append(items, r.Feed([]byte(rest[:i+1]))...)
			rest = rest[i+1:]
		}
	default: // per-byte
		for i := 0; i < len(row.Input); i++ {
			items = append(items, r.Feed([]byte{row.Input[i]})...)
		}
	}
	last, fatal := r.Close()
	items = append(items, last...)

	arr := make([]any, len(items))
	for i, it := range items {
		o := &value.Object{}
		o.Members = append(o.Members, value.Member{Key: "kind", Value: it.Kind})
		o.Members = append(o.Members, value.Member{Key: "recordIndex", Value: float64(it.RecordIndex)})
		if it.SchemaName != "" {
			o.Members = append(o.Members, value.Member{Key: "schemaName", Value: strings.TrimPrefix(it.SchemaName, "$")})
		}
		if it.Kind == "record" {
			o.Members = append(o.Members, value.Member{Key: "value", Value: it.Value})
		} else if it.Err != nil {
			o.Members = append(o.Members, value.Member{Key: "error", Value: errObj(it.Err)})
		}
		arr[i] = o
	}
	out := &value.Object{}
	out.Members = append(out.Members, value.Member{Key: "items", Value: arr})
	var f any
	if fatal != nil {
		f = errObj(fatal)
	}
	out.Members = append(out.Members, value.Member{Key: "fatal", Value: f})
	return out
}

func errObj(e *document.ItemError) *value.Object {
	return &value.Object{Members: []value.Member{
		{Key: "category", Value: e.Category},
		{Key: "code", Value: e.Code},
	}}
}

// expectedStream normalizes the case's expected block: items lacking a
// schemaName stay without one, and a missing/null fatal is nil.
func expectedStream(v any) *value.Object {
	obj, ok := v.(*value.Object)
	if !ok {
		return &value.Object{}
	}
	out := &value.Object{}
	items, _ := fieldOf(obj, "items")
	if items == nil {
		items = []any{}
	}
	out.Members = append(out.Members, value.Member{Key: "items", Value: items})
	fatal, _ := fieldOf(obj, "fatal")
	out.Members = append(out.Members, value.Member{Key: "fatal", Value: fatal})
	return out
}

func fieldOf(o *value.Object, key string) (any, bool) {
	if i := o.Find(key); i >= 0 {
		return o.Members[i].Value, true
	}
	return nil, false
}

func sigil(name string) string {
	if name == "" {
		return ""
	}
	return "$" + name
}
