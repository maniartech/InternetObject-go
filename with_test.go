package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The runtime schema: compiled once, reused. The struct has NO schema tags.
const registrySchema = `name: {string, minLen: 2}, age: {int, min: 0, max: 130}, role?: string`

type runtimePerson struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
	Role string `io:"role,omitempty"`
}

func mustSchema(t *testing.T, def string) *io.Schema {
	t.Helper()
	s, err := io.ParseSchema(def)
	if err != nil {
		t.Fatalf("ParseSchema(%q): %v", def, err)
	}
	return s
}

func TestUnmarshalWithHeaderlessData(t *testing.T) {
	s := mustSchema(t, registrySchema)

	var people []runtimePerson
	if err := io.UnmarshalWith("~ Alice, 30, admin\n~ Bob, 25", &people, s); err != nil {
		t.Fatal(err)
	}
	if len(people) != 2 || people[0].Name != "Alice" || people[1].Age != 25 {
		t.Fatalf("bound: %+v", people)
	}

	// The runtime schema's constraints are enforced on the wire.
	err := io.UnmarshalWith("~ Alice, 131", &people, s)
	var list io.ErrorList
	if !errorsAs(err, &list) || list[0].Code != "mismatched-max" {
		t.Fatalf("want mismatched-max, got %v", err)
	}
}

func TestValidateWith(t *testing.T) {
	s := mustSchema(t, registrySchema)
	if err := io.ValidateWith(runtimePerson{Name: "Cara", Age: 27}, s); err != nil {
		t.Fatalf("valid value rejected: %v", err)
	}
	err := io.ValidateWith(runtimePerson{Name: "X", Age: 200}, s)
	var list io.ErrorList
	if !errorsAs(err, &list) || len(list) != 2 {
		t.Fatalf("want two faults, got %v", err)
	}
	// A slice validates too.
	if err := io.ValidateWith([]runtimePerson{{Name: "Ann", Age: 1}}, s); err != nil {
		t.Fatal(err)
	}
	if err := io.ValidateWith(runtimePerson{}, nil); err == nil {
		t.Fatal("nil schema must error")
	}
}

func TestMarshalWithRoundTrip(t *testing.T) {
	s := mustSchema(t, registrySchema)
	in := []runtimePerson{{Name: "Alice", Age: 30, Role: "admin"}, {Name: "Bob", Age: 25}}

	text, err := io.MarshalWith(in, s)
	if err != nil {
		t.Fatal(err)
	}
	// The header is the runtime schema, written from the compiled object.
	if !strings.HasPrefix(text, "name: {string, minLen:2}, age: {int, min:0, max:130}, role?: string\n---\n") {
		t.Fatalf("header not written from the schema: %q", text)
	}
	// It re-parses on its own (the header travels with the data)...
	var viaHeader []runtimePerson
	if err := io.Unmarshal(text, &viaHeader); err != nil {
		t.Fatalf("self-describing round trip: %v\n%q", err, text)
	}
	// ...and through the runtime schema again.
	var viaSchema []runtimePerson
	if err := io.UnmarshalWith(text, &viaSchema, s); err != nil {
		t.Fatal(err)
	}
	for _, got := range [][]runtimePerson{viaHeader, viaSchema} {
		if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
			t.Fatalf("round trip: %+v", got)
		}
	}

	// MarshalWith validates before writing.
	if _, err := io.MarshalWith(runtimePerson{Name: "A", Age: 5}, s); err == nil {
		t.Fatal("MarshalWith must refuse a value the schema rejects")
	}
}

// An attached schema outranks the document's own header (ADR 0004 D5).
func TestRuntimeSchemaOutranksHeader(t *testing.T) {
	strict := mustSchema(t, "name: {string, minLen: 2}, age: {int, min: 18}")
	// The document's header would allow age 5; the attached schema does not.
	doc := "name: string, age: int\n---\n~ Alice, 5"

	var people []runtimePerson
	err := io.UnmarshalWith(doc, &people, strict)
	var list io.ErrorList
	if !errorsAs(err, &list) || list[0].Code != "mismatched-min" {
		t.Fatalf("attached schema must win, got %v", err)
	}
	// Without it, the header governs and the same document is fine.
	if err := io.Unmarshal(doc, &people); err != nil {
		t.Fatalf("header route: %v", err)
	}
}

func TestParseWithAndSchemaLifting(t *testing.T) {
	s := mustSchema(t, registrySchema)
	doc, err := io.ParseWith("~ Dana, 41", s)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Records(); len(got) != 1 {
		t.Fatalf("records: %#v", got)
	}
	if doc.Schema() == nil {
		t.Fatal("Schema() should report the schema that validated the load")
	}

	// A schema defined in one document can be lifted and reused on others.
	src, err := io.Parse("~ $person: {name: {string, minLen: 2}, age: int}\n--- $person\n~ Gil, 20")
	if err != nil {
		t.Fatal(err)
	}
	lifted, err := src.SchemaOf("person")
	if err != nil {
		t.Fatal(err)
	}
	var p runtimePerson
	if err := io.UnmarshalWith("Hana, 33", &p, lifted); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Hana" || p.Age != 33 {
		t.Fatalf("lifted-schema bind: %+v", p)
	}
	if err := io.UnmarshalWith("X, 33", &p, lifted); err == nil {
		t.Fatal("lifted schema must carry its constraints")
	}
	if _, err := src.SchemaOf("nope"); err == nil {
		t.Fatal("unknown schema name must error")
	}
}

func TestStreamWithRuntimeSchema(t *testing.T) {
	s := mustSchema(t, registrySchema)
	var ok, faults int
	for item, err := range io.Stream(strings.NewReader("~ Eve, 22\n~ Fay, oops\n~ Gus, 200\n"),
		&io.StreamOptions{Schema: s}) {
		if err != nil {
			t.Fatalf("fatal: %v", err)
		}
		if item.Err != nil {
			faults++
			continue
		}
		ok++
	}
	if ok != 1 || faults != 2 { // "oops" is not an int; 200 exceeds max
		t.Fatalf("ok=%d faults=%d", ok, faults)
	}
}

// A schema derived from a Go type is the same kind of value, so it can drive
// the With functions for a DIFFERENT type with matching member names.
func TestSchemaForFeedsWith(t *testing.T) {
	s, err := io.SchemaFor[user]() // user has schema tags (see schema_tag_test.go)
	if err != nil {
		t.Fatal(err)
	}
	type plain struct {
		Name string `io:"name"`
		Age  int    `io:"age"`
	}
	if err := io.ValidateWith(plain{Name: "Alice", Age: 30}, s); err != nil {
		t.Fatalf("derived schema on a plain type: %v", err)
	}
	if err := io.ValidateWith(plain{Name: "A", Age: 30}, s); err == nil {
		t.Fatal("derived constraints must still apply")
	}
}

// A schema that absorbs into itself must terminate (the reference
// stack-overflows here — FINDINGS #14), while legitimate recursion still
// validates: real nesting consumes a level of data per step.
func TestRecursiveSchemasTerminate(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want io.Code
	}{
		{"~ $P: {A: $P}\n--- $P\n~ $P: 0", "unknown-member"},
		{"~ $P: {A: $Q}\n~ $Q: {A: $P}\n--- $P\n~ {x: 1}", "unknown-member"},
	} {
		_, err := io.Parse(tc.src)
		var list io.ErrorList
		if !errorsAs(err, &list) || list[0].Code != tc.want {
			t.Fatalf("%q: want %s, got %v", tc.src, tc.want, err)
		}
	}
	if _, err := io.Parse("~ $P: {A*: $P}\n--- $P\n~ {A: {A: N}}"); err != nil {
		t.Fatalf("legitimate recursion must still validate: %v", err)
	}
}

func BenchmarkUnmarshalWith(b *testing.B) {
	s, err := io.ParseSchema(registrySchema)
	if err != nil {
		b.Fatal(err)
	}
	var rows strings.Builder
	for i := 0; i < 100; i++ {
		rows.WriteString("~ Person, 30, admin\n")
	}
	text := rows.String()
	b.ReportAllocs()
	for b.Loop() {
		var people []runtimePerson
		if err := io.UnmarshalWith(text, &people, s); err != nil {
			b.Fatal(err)
		}
	}
}
