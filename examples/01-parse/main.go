// Parse a document, read its values, and write it back canonically.
//
// The one behavior to internalize: errors ACCUMULATE. A faulted record does
// not abort the parse — the error list carries every designated code, and the
// document still holds every record that survived.
package main

import (
	"fmt"
	"log"

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
	if name, ok := first.Get("name"); ok {
		fmt.Println("first name:", name)
	}

	// Text is the canonical writer: its output re-parses to the same value, and
	// writing it again changes nothing.
	//
	// It REFUSES this document, though, because a record in it failed: a
	// projection may describe errors, but a file must not contain them, so a
	// document parsed tolerantly cannot quietly be saved as a truncated file.
	if _, err := doc.Text(nil); err != nil {
		fmt.Println("--- refused, as it should be ---")
		fmt.Println(err)
	}

	// Asking for the survivors is explicit, and says what you are giving up.
	text, err := doc.Text(&io.TextOptions{SkipErrors: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("--- canonical form, failed records dropped ---")
	fmt.Println(text)
}
