// Runtime schemas: parse the schema ONCE, then apply the compiled object to
// documents, values and streams.
//
// Tags are design-time. When the schema lives elsewhere — a registry, a file,
// a remote service, or another document — fetch it, compile it with
// ParseSchema, and hand that *io.Schema to the `With` functions. Nothing is
// re-parsed per call, and schema text is never concatenated with data text.
package main

import (
	"fmt"
	"strings"

	io "github.com/maniartech/InternetObject-go"
)

// Imagine this arrived over HTTP from your schema registry, once, at startup.
const fetched = `name: {string, minLen: 2}, age: {int, min: 0, max: 130}, role?: string`

// The struct carries NO schema tags — `io` tags only name the members.
// Types and constraints come from the compiled schema.
type Person struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
	Role string `io:"role,omitempty"`
}

func main() {
	// Compile once. Reuse everywhere; safe to keep in a package var.
	schema, err := io.ParseSchema(fetched)
	if err != nil {
		panic(err)
	}

	// 1. Bind + validate incoming data that carries NO header of its own.
	var people []Person
	err = io.UnmarshalWith("~ Alice, 30, admin\n~ Bob, 131", &people, schema)
	fmt.Println("UnmarshalWith:", err) // mismatched-max (Bob's age)

	err = io.UnmarshalWith("~ Alice, 30, admin\n~ Bob, 25", &people, schema)
	fmt.Printf("loaded %d people, first = %s\n", len(people), people[0].Name)

	// 2. Validate a Go value against the runtime schema — no tags involved.
	fmt.Println("ValidateWith(ok):", io.ValidateWith(Person{Name: "Cara", Age: 27}, schema))
	fmt.Println("ValidateWith(bad):", io.ValidateWith(Person{Name: "X", Age: 200}, schema))

	// 3. Write with it: validated, header emitted from the compiled schema.
	text, err := io.MarshalWith(people, schema)
	if err != nil {
		panic(err)
	}
	fmt.Println("--- MarshalWith ---")
	fmt.Println(text)

	// 4. Parse dynamically under the runtime schema (no structs at all).
	doc, err := io.ParseWith("~ Dana, 41", schema)
	fmt.Println("ParseWith:", err, "records:", len(doc.Records()))

	// 5. Stream under it — every record validated, no header in the stream.
	for item, err := range io.Stream(strings.NewReader("~ Eve, 22\n~ Fay, oops\n"),
		&io.StreamOptions{Schema: schema}) {
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

	// A schema can equally be lifted out of another document's header, or
	// derived from a Go type — all three produce the same *io.Schema.
	other, _ := io.Parse("~ $person: {name: string}\n--- $person\n~ Gil")
	if s, err := other.SchemaOf("person"); err == nil {
		fmt.Println("schema lifted from a document:", s.String())
	}
}
