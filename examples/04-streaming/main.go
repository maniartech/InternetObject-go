// Streaming: records arrive as an iterator while bytes arrive from any
// io.Reader. A faulted record is an item with Err set and the stream
// CONTINUES; transport chunk boundaries are never semantic.
package main

import (
	"fmt"
	"strings"

	stdio "io"

	io "github.com/maniartech/InternetObject-go"
)

const wire = `~ $person: {name: string, age: int}
--- $person
~ Alice, 30
~ Bob, oops
~ Cara, 27
`

func main() {
	run := func(label string, r stdio.Reader) {
		fmt.Println(label)
		for item, err := range io.Stream(r, nil) {
			if err != nil {
				fmt.Println("  FATAL:", err) // e.g. an unknown schema selector
				return
			}
			if item.Err != nil {
				fmt.Printf("  record %d faulted: %s (stream continues)\n", item.Index, item.Err.Code)
				continue
			}
			rec := item.Value.(*io.Object)
			v, _ := rec.Get("name")
			name, _ := v.(string)
			fmt.Printf("  record %d ok: %s (schema %s)\n", item.Index, name, item.SchemaName)
		}
	}

	// Whole buffer at once, and byte-by-byte: identical item sequences.
	run("one chunk:", strings.NewReader(wire))
	run("per byte:", stdio.MultiReader(oneByteReaders(wire)...))
}

func oneByteReaders(s string) []stdio.Reader {
	out := make([]stdio.Reader, len(s))
	for i := range s {
		out[i] = strings.NewReader(s[i : i+1])
	}
	return out
}
