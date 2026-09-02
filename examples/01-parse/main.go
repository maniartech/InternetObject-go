// Parse a document, read its values, and write it back canonically.
//
// The one behavior to internalize: errors ACCUMULATE. A faulted record does
// not abort the parse — the error list carries every designated code, and the
// document still holds every record that survived.
package main

import (
	"fmt"

	io "github.com/maniartech/InternetObject-go"
)

func main() {
	doc, err := io.Parse(`
~ $schema: {name: string, age: {int, min: 0}, active: bool}
---
~ Alice, 30, T
~ Bob, twenty, F
~ Cara, 27, T`)

	// err is an io.ErrorList: every fault, in order, with designated codes.
	fmt.Println("errors:", err) // expected-integer at ...

	// The document still holds the records that survived.
	records := doc.Value().([]any)
	fmt.Println("records loaded:", len(records))
	first := records[0].(*io.Object)
	if i := first.Find("name"); i >= 0 {
		fmt.Println("first name:", first.Members[i].Value)
	}

	// String() is the canonical writer: its output always re-parses to the
	// same value, and writing it again changes nothing.
	fmt.Println("--- canonical form ---")
	fmt.Println(doc.String())
}
