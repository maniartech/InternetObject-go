// Example 08 — building and streaming a document the format's own way.
//
// Examples 01-07 all READ. This one writes: a document assembled at runtime
// with no Go type describing it, and the same records pushed onto a stream.
// Between them they are the two halves of a link — a gateway that receives
// whatever it receives and forwards it.
package main

import (
	"bytes"
	"fmt"

	io "github.com/maniartech/InternetObject-go"
)

func main() {
	employee, err := io.ParseSchema("{name: string, age: {int, min: 0, max: 130}}")
	if err != nil {
		panic(err)
	}
	alert, err := io.ParseSchema("{level: {string, choices: [info, warn, error]}, msg: string}")
	if err != nil {
		panic(err)
	}

	// ── Building, with no struct in sight ───────────────────────────────────
	// The shape is decided here, at runtime. A record may be a map, an
	// *io.Object, or a struct — whatever the caller happens to hold.
	b := io.NewBuilder()
	b.Define("employee", employee).Define("alert", alert)

	emp := b.Section("employees", "employee")
	_ = emp.Add(map[string]any{"name": "Alice", "age": 30})
	_ = emp.Add(io.NewObject(2).Append("name", "Bob").Append("age", 41))

	// Validation happens HERE, at the call that caused the fault — so a
	// builder can never produce a document its own parser would reject.
	if err := emp.Add(map[string]any{"name": "X", "age": -5}); err != nil {
		fmt.Println("rejected at Add:", err)
	}

	_ = b.Section("alerts", "alert").Add(map[string]any{"level": "warn", "msg": "disk nearly full"})

	doc, err := b.Document()
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n--- built document ---\n%s\n", doc.String())

	// It is an ordinary document: it re-parses, and it projects to JSON.
	j, _ := doc.JSON(&io.JSONOptions{Indent: "  "})
	fmt.Printf("--- as JSON ---\n%s\n", j)

	// ── Streaming the same records ──────────────────────────────────────────
	// A stream carries records one at a time. Definitions go out once, ahead
	// of the first record, and a schema switch is emitted only where the shape
	// actually changes.
	var wire bytes.Buffer
	sm, err := io.NewStreamMarshaler(&wire, &io.StreamOptions{
		Definitions: "~ $employee: {name: string, age: int}\n~ $alert: {level: string, msg: string}",
	})
	if err != nil {
		panic(err)
	}
	_ = sm.MarshalAs(map[string]any{"name": "Alice", "age": 30}, "employee")
	_ = sm.MarshalAs(map[string]any{"name": "Bob", "age": 41}, "employee")
	_ = sm.MarshalAs(map[string]any{"level": "warn", "msg": "disk nearly full"}, "alert")
	if err := sm.Close(); err != nil {
		panic(err)
	}
	fmt.Printf("\n--- the wire ---\n%s\n", wire.String())

	// The other end of the link, in this same package.
	fmt.Println("--- read back ---")
	for item, err := range io.Stream(bytes.NewReader(wire.Bytes()), nil) {
		if err != nil {
			panic(err)
		}
		name, _ := item.Value.(*io.Object).Get("name")
		if name == nil {
			name, _ = item.Value.(*io.Object).Get("level")
		}
		fmt.Printf("  [%d] %-9s %v\n", item.Index, item.SchemaName, name)
	}

	// ── Tolerant reading ────────────────────────────────────────────────────
	// A feed is not a config file: one malformed row is no reason to drop the
	// rest. Collection[T] keeps what bound and reports what did not.
	type Employee struct {
		Name string `io:"name"`
		Age  int    `io:"age"`
	}
	var feed struct {
		Employees io.Collection[Employee] `io:"employees"`
	}
	src := "~ $E: {name: string, age: {int, min: 0}}\n--- employees: $E\n" +
		"~ Alice, 30\n~ Bob, notanint\n~ Cara, 27\n"
	if err := io.Unmarshal(src, &feed); err != nil {
		panic(err)
	}
	fmt.Printf("\n--- tolerant read ---\n  %d rows attempted, %d bound\n",
		feed.Employees.Len(), len(feed.Employees.Items()))
	for i, e := range feed.Employees.All() {
		fmt.Printf("  row %d: %v\n", i, e)
	}
	for _, e := range feed.Employees.Errors() {
		fmt.Printf("  row %d failed: %s\n", e.RecordIndex, e.Code)
	}
}
