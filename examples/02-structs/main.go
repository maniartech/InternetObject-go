// Native structs, the encoding/json way: Marshal derives the schema from the
// struct type and writes the data positionally; Unmarshal validates against
// the document's schema and binds — and also binds schema-less records
// positionally by field order.
package main

import (
	"fmt"

	io "github.com/maniartech/InternetObject-go"
)

type Address struct {
	City string `io:"city"`
	Zip  string `io:"zip,omitempty"`
}

type Person struct {
	Name  string   `io:"name"`
	Age   int      `io:"age"`
	Email string   `io:"email,omitempty"` // optional AND zero left off the wire
	Score int      `io:"score,optional"`  // optional, but zero IS written
	Nick  *string  `io:"nick"`            // pointer = nullable: nil ⇄ N
	Tags  []string `io:"tags,omitempty"`
	Home  Address  `io:"home"`
}

func main() {
	nick := "Ally"
	people := []Person{
		{Name: "Alice", Age: 30, Nick: &nick, Home: Address{City: "Pune", Zip: "411001"}},
		{Name: "Bob", Age: 25, Email: "bob@x.io", Tags: []string{"a", "b"}, Home: Address{City: "Goa"}},
	}

	text, err := io.Marshal(people)
	if err != nil {
		panic(err)
	}
	fmt.Println(text)
	// name: string, age: int, email?: string, score?: int, nick*: string, ...
	// ---
	// ~ Alice, 30, , 0, Ally, , {Pune, 411001}
	// ~ Bob, 25, bob@x.io, 0, N, [a, b], {Goa}

	var back []Person
	if err := io.Unmarshal(text, &back); err != nil {
		panic(err)
	}
	fmt.Println("\nround-tripped:", back[0].Name, "and", back[1].Name)
	fmt.Println("nil nick survived:", back[1].Nick == nil)

	// Schema-less binding: positional by field order, keyed by name.
	var p Person
	if err := io.Unmarshal("Grace, 41", &p); err != nil {
		panic(err)
	}
	fmt.Println("schema-less:", p.Name, p.Age)
}
