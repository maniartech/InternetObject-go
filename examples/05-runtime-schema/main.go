// Attaching a schema at RUNTIME — no tags involved.
//
// Tags are design-time. When the schema lives somewhere else (a registry, a
// file, a remote service), remember that an Internet Object schema is itself
// Internet Object text: fetch it and hand it to the engine as the document's
// definitions. Works today with zero special API; the typed variants
// (UnmarshalWith / ValidateWith / AttachSchema) are ADR 0004 phase 2.
package main

import (
	"fmt"
	"strings"

	io "github.com/maniartech/InternetObject-go"
)

// Imagine this arrived over HTTP from your schema registry.
const fetched = `name: {string, minLen: 2}, age: {int, min: 0, max: 130}, role?: string`

// The struct carries NO schema tags — names only. Constraints come from the
// fetched schema; `io` tags remain the name-binding contract.
type Person struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
	Role string `io:"role,omitempty"`
}

func main() {
	// 1. Validate + bind incoming data against the fetched schema: the schema
	//    text becomes the header of the document being read.
	rows := "~ Alice, 30, admin\n~ Bob, 131"
	doc := fetched + "\n---\n" + rows

	var people []Person
	err := io.Unmarshal(doc, &people)
	fmt.Println("wire validation from the FETCHED schema:", err) // mismatched-max (Bob)

	// The dynamic route accumulates: faults listed, good rows still loaded.
	parsed, perr := io.Parse(doc)
	fmt.Println("dynamic route:", perr, "| records (incl. fault markers):", len(parsed.Value().([]any)))

	// 2. The same works for streaming: preload the fetched definitions.
	stream := "~ Cara, 27\n~ Dan, oops\n"
	opts := &io.StreamOptions{Definitions: "~ $person: {" + fetched + "}", DefaultSchema: "$person"}
	for item, err := range io.Stream(strings.NewReader(stream), opts) {
		if err != nil {
			panic(err)
		}
		if item.Err != nil {
			fmt.Printf("stream record %d: %s\n", item.Index, item.Err.Code)
			continue
		}
		rec := item.Value.(*io.Object)
		fmt.Printf("stream record %d ok: %v\n", item.Index, rec.Members[rec.Find("name")].Value)
	}

	// 3. A fetched schema is inspectable like any other.
	s, err := io.ParseSchema(fetched)
	if err != nil {
		panic(err)
	}
	fmt.Println("fetched schema, compiled and re-rendered:", s.String())
}
