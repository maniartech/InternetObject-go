package conformance

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/streaming"
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
		if !core.Equal(got, want) {
			problems = append(problems, fmt.Sprintf("[%s]\n     expected=%s\n     actual  =%s",
				strategy, Show(want), Show(got)))
		}
	}
	return problems
}

func runStream(row SuiteRow, strategy string) *core.Object {
	r := streaming.NewReader(streaming.StreamOptions{
		Definitions:   row.Definitions,
		DefaultSchema: sigil(row.DefaultSchema),
	})
	var items []streaming.Item
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
		o := &core.Object{}
		o.Members = append(o.Members, core.Member{Key: "kind", Value: it.Kind})
		o.Members = append(o.Members, core.Member{Key: "recordIndex", Value: float64(it.RecordIndex)})
		if it.SchemaName != "" {
			o.Members = append(o.Members, core.Member{Key: "schemaName", Value: strings.TrimPrefix(it.SchemaName, "$")})
		}
		if it.Kind == "record" {
			o.Members = append(o.Members, core.Member{Key: "value", Value: it.Value})
		} else if it.Err != nil {
			o.Members = append(o.Members, core.Member{Key: "error", Value: errObj(it.Err)})
		}
		arr[i] = o
	}
	out := &core.Object{}
	out.Members = append(out.Members, core.Member{Key: "items", Value: arr})
	var f any
	if fatal != nil {
		f = errObj(fatal)
	}
	out.Members = append(out.Members, core.Member{Key: "fatal", Value: f})
	return out
}

func errObj(e *streaming.ItemError) *core.Object {
	return &core.Object{Members: []core.Member{
		{Key: "category", Value: e.Category},
		// The corpus states codes as plain strings, so the adapter hands
		// over a string. core.Equal would otherwise compare a Code
		// against a string and find them different.
		{Key: "code", Value: string(e.Code)},
	}}
}

// expectedStream normalizes the case's expected block: items lacking a
// schemaName stay without one, and a missing/null fatal is nil.
func expectedStream(v any) *core.Object {
	obj, ok := v.(*core.Object)
	if !ok {
		return &core.Object{}
	}
	out := &core.Object{}
	items, _ := fieldOf(obj, "items")
	if items == nil {
		items = []any{}
	}
	out.Members = append(out.Members, core.Member{Key: "items", Value: items})
	fatal, _ := fieldOf(obj, "fatal")
	out.Members = append(out.Members, core.Member{Key: "fatal", Value: fatal})
	return out
}

func fieldOf(o *core.Object, key string) (any, bool) {
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
