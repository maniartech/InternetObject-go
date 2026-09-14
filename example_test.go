package internetobject_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	io "github.com/maniartech/InternetObject-go"
)

type Employee struct {
	Name string `io:"name"`
	Age  int    `io:"age" schema:"{int, min: 0, max: 130}"`
}

func ExampleMarshal() {
	staff := []Employee{{"Alice", 30}, {"Bob", 41}}
	text, err := io.Marshal(staff)
	if err != nil {
		panic(err)
	}
	fmt.Println(text)
	// Output:
	// name: string, age: {int, min:0, max:130}
	// ---
	// ~ Alice, 30
	// ~ Bob, 41
}

func ExampleUnmarshal() {
	const doc = "name: string, age: {int, min: 0, max: 130}\n---\n~ Alice, 30\n~ Bob, 41"
	var staff []Employee
	if err := io.Unmarshal(doc, &staff); err != nil {
		panic(err)
	}
	fmt.Println(staff)
	// Output:
	// [{Alice 30} {Bob 41}]
}

// Every fault is reported, each with its designated code and position.
func ExampleUnmarshal_validation() {
	const doc = "name: string, age: {int, min: 0, max: 130}\n---\n~ Alice, 300\n~ Bob, old"
	var staff []Employee
	err := io.Unmarshal(doc, &staff)

	var faults io.ErrorList
	if errors.As(err, &faults) {
		for _, f := range faults {
			fmt.Println(f.Code, f.Path, f.Line)
		}
	}
	fmt.Println(errors.Is(err, io.Error{Code: io.MismatchedMax}))
	// Output:
	// mismatched-max $[0].age 3
	// expected-integer $[1].age 4
	// true
}

func ExampleParse() {
	doc, err := io.Parse("~ $emp: {name: string, age: int}\n--- staff: $emp\n~ Alice, 30\n~ Bob, 41")
	if err != nil {
		panic(err)
	}
	staff := doc.Section("staff")
	fmt.Println(staff.Len(), staff.SchemaName())
	for _, rec := range staff.Records() {
		name, _ := rec.(*io.Object).Get("name")
		fmt.Println(name)
	}
	// Output:
	// 2 emp
	// Alice
	// Bob
}

func ExampleStream() {
	const wire = "name: string, age: int\n---\n~ Alice, 30\n~ Bob, old\n~ Carol, 52\n"
	for item, err := range io.Stream(strings.NewReader(wire), nil) {
		if err != nil {
			panic(err) // the stream itself failed
		}
		if item.Err != nil {
			fmt.Println(item.Index, "rejected:", item.Err.Code)
			continue
		}
		name, _ := item.Value.(*io.Object).Get("name")
		fmt.Println(item.Index, name)
	}
	// Output:
	// 0 Alice
	// 1 rejected: expected-integer
	// 2 Carol
}

// A Collection keeps the rows that bind and reports the rest, instead of
// failing the whole document.
func ExampleCollection() {
	var doc struct {
		Staff io.Collection[Employee] `io:"staff"`
	}
	err := io.Unmarshal("--- staff\n~ name: Alice, age: 30\n~ name: Bob, age: 1.5\n", &doc)
	if err != nil {
		panic(err)
	}
	for _, e := range doc.Staff.Items() {
		fmt.Println(e.Name)
	}
	for _, f := range doc.Staff.Errors() {
		fmt.Println(f.Code, f.Path)
	}
	// Output:
	// Alice
	// invalid-object $.staff[1].age
}

func ExampleBuilder() {
	emp, err := io.ParseSchema("name: string, age: {int, min: 0}")
	if err != nil {
		panic(err)
	}
	var b io.Builder
	b.Define("emp", emp)
	staff := b.Section("staff", "emp")
	if err := staff.Add(map[string]any{"name": "Alice", "age": 30}); err != nil {
		panic(err)
	}
	// A record the schema refuses is reported at the call, and not added.
	fmt.Println(staff.Add(map[string]any{"name": "Bob", "age": -1}))

	doc, err := b.Document()
	if err != nil {
		panic(err)
	}
	fmt.Println(doc)
	// Output:
	// mismatched-min at $[1].age
	// ~ $emp: {name: string, age: {int, min:0}}
	//
	// --- staff: $emp
	// ~ Alice, 30
}

// Definitions compiled once, shared by a publisher and its subscribers, so the
// wire carries data alone.
func ExampleDefinitions_Stream() {
	defs, err := io.ParseDefinitions("~ $emp: {name: string, age: {int, min: 0}}\n~ $schema: $emp")
	if err != nil {
		panic(err)
	}
	const wire = "---\n~ Alice, 30\n~ Bob, -1\n"
	for item, err := range defs.Stream(strings.NewReader(wire), nil) {
		if err != nil {
			panic(err)
		}
		if item.Err != nil {
			fmt.Println(item.Index, "rejected:", item.Err.Code)
			continue
		}
		name, _ := item.Value.(*io.Object).Get("name")
		fmt.Println(item.Index, name)
	}
	// Output:
	// 0 Alice
	// 1 rejected: mismatched-min
}

func ExampleNewStreamMarshaler() {
	var buf bytes.Buffer
	w, err := io.NewStreamMarshaler(&buf, nil)
	if err != nil {
		panic(err)
	}
	for _, e := range []Employee{{"Alice", 30}, {"Bob", 41}} {
		if err := w.Marshal(e); err != nil {
			panic(err)
		}
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	fmt.Print(buf.String())
	// Output:
	// ---
	// ~ name: Alice, age: 30
	// ~ name: Bob, age: 41
}

// A schema compiled once, from anywhere, applied to data that carries none.
func ExampleUnmarshalWith() {
	s, err := io.ParseSchema("name: string, age: {int, min: 0}")
	if err != nil {
		panic(err)
	}
	var e Employee
	if err := io.UnmarshalWith("Alice, 30", &e, s); err != nil {
		panic(err)
	}
	fmt.Println(e)
	// Output:
	// {Alice 30}
}
