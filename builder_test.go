package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

func empSchema(t *testing.T) *io.Schema {
	t.Helper()
	s, err := io.ParseSchema("{name: string, age: {int, min: 0, max: 130}}")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The gap this closes: a document whose sections are decided at runtime, which
// no Go type can express.
func TestBuilderMakesAMultiSectionDocument(t *testing.T) {
	alert, err := io.ParseSchema("{level: {string, choices: [info, warn, error]}, msg: string}")
	if err != nil {
		t.Fatal(err)
	}
	b := io.NewBuilder()
	b.Define("Employee", empSchema(t)).Define("Alert", alert)

	emp := b.Section("employees", "Employee")
	if err := emp.Add(map[string]any{"name": "Alice", "age": 30}); err != nil {
		t.Fatalf("map record: %v", err)
	}
	if err := emp.Add(io.NewObject(2).Append("name", "Bob").Append("age", 41)); err != nil {
		t.Fatalf("Object record: %v", err)
	}
	type E struct {
		Name string `io:"name"`
		Age  int    `io:"age"`
	}
	if err := emp.Add(E{"Cara", 27}); err != nil {
		t.Fatalf("struct record: %v", err)
	}
	if err := b.Section("alerts", "Alert").Add(map[string]any{"level": "warn", "msg": "disk"}); err != nil {
		t.Fatal(err)
	}

	doc, err := b.Document()
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	text := doc.String()

	// A builder must never produce a document its own parser rejects.
	back, err := io.Parse(text)
	if err != nil {
		t.Fatalf("the built document does not re-parse: %v\n%s", err, text)
	}
	if back.String() != text {
		t.Errorf("writing is not idempotent:\n%q\n%q", text, back.String())
	}
	if len(back.Sections()) != 2 {
		t.Fatalf("got %d sections", len(back.Sections()))
	}
	if got := back.Section("employees"); got == nil || got.Len() != 3 {
		t.Errorf("employees section = %v", got)
	}
	if got := back.Section("alerts"); got == nil || got.SchemaName() != "Alert" {
		t.Errorf("alerts section lost its binding")
	}
	// The records bound by name, so the schema really was applied.
	rec := back.Section("employees").Records()[0].(*io.Object)
	if v, ok := rec.Get("name"); !ok || v != "Alice" {
		t.Errorf("first employee = %v", rec.Keys())
	}
}

// Validation happens at Add, so the fault is reported at the call that caused
// it — and the bad record is not added.
func TestBuilderValidatesOnAdd(t *testing.T) {
	b := io.NewBuilder().Define("E", empSchema(t))
	sec := b.Section("", "E")
	if err := sec.Add(map[string]any{"name": "Alice", "age": 30}); err != nil {
		t.Fatal(err)
	}
	err := sec.Add(map[string]any{"name": "X", "age": -5})
	if err == nil {
		t.Fatal("a record violating the schema was accepted")
	}
	var list io.ErrorList
	if !asErrorList(err, &list) || !list.Has(io.MismatchedMin) {
		t.Errorf("got %v, want mismatched-min", err)
	}
	if sec.Len() != 1 {
		t.Errorf("the rejected record was added anyway: len = %d", sec.Len())
	}
	// A missing member is caught the same way.
	if err := sec.Add(map[string]any{"name": "Y"}); err == nil {
		t.Error("a record missing a required member was accepted")
	}
	// The document still holds only the good row.
	doc, err := b.Document()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Records()) != 1 {
		t.Errorf("document has %d records", len(doc.Records()))
	}
}

// A section with no schema accepts anything and writes header-less.
func TestBuilderWithoutASchema(t *testing.T) {
	b := io.NewBuilder()
	sec := b.Section("", "")
	if err := sec.Add(map[string]any{"anything": "goes", "n": 1}); err != nil {
		t.Fatal(err)
	}
	doc, err := b.Document()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc.String(), "$") {
		t.Errorf("a schema-less build wrote a header: %q", doc.String())
	}
	if _, err := io.Parse(doc.String()); err != nil {
		t.Errorf("does not re-parse: %v (%q)", err, doc.String())
	}
}

// A definition may not change under records already validated against it.
func TestDefinitionsFreezeOnceRecordsExist(t *testing.T) {
	b := io.NewBuilder().Define("E", empSchema(t))
	if err := b.Section("", "E").Add(map[string]any{"name": "Alice", "age": 30}); err != nil {
		t.Fatal(err)
	}
	if b.Define("F", empSchema(t)).Err() == nil {
		t.Error("Define after a record was accepted")
	}
	// The builder stays failed, so the mistake cannot be missed.
	if _, err := b.Document(); err == nil {
		t.Error("Document succeeded after a refused Define")
	}
}

func TestBuilderRejectsUnknownSchemaAndNilRecord(t *testing.T) {
	b := io.NewBuilder()
	b.Section("x", "NoSuch")
	if b.Err() == nil {
		t.Error("binding a section to an undefined schema was accepted")
	}
	b2 := io.NewBuilder()
	sec := b2.Section("", "")
	if err := sec.Add(nil); err == nil {
		t.Error("a nil record was accepted")
	}
	if err := sec.Add((*io.Object)(nil)); err == nil {
		t.Error("a nil *Object was accepted")
	}
	if err := sec.Add(42); err == nil {
		t.Error("a scalar was accepted as a record")
	}
}

// Editing a parsed document goes through a clone, so the original — which
// every reader treats as immutable — is untouched.
func TestNewBuilderFromClones(t *testing.T) {
	src := "~ $E: {name: string, age: int}\n--- employees: $E\n~ Alice, 30\n~ Bob, 41"
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	// Canonical output need not equal arbitrary input — it must re-parse and be
	// idempotent — so immutability is checked against the document's OWN text
	// before the builder touches it.
	before := doc.String()

	b := io.NewBuilderFrom(doc)
	if err := b.Section("employees", "E").Add(map[string]any{"name": "Cara", "age": 27}); err != nil {
		t.Fatal(err)
	}
	edited, err := b.Document()
	if err != nil {
		t.Fatal(err)
	}
	if got := edited.Section("employees").Len(); got != 3 {
		t.Errorf("edited copy has %d records, want 3", got)
	}
	if got := doc.Section("employees").Len(); got != 2 {
		t.Errorf("the ORIGINAL changed: %d records", got)
	}
	if doc.String() != before {
		t.Errorf("the original's text changed:\n%q\n%q", before, doc.String())
	}
	if _, err := io.Parse(edited.String()); err != nil {
		t.Errorf("the edited document does not re-parse: %v", err)
	}
}

// Opening the same section twice continues it rather than starting a second.
func TestSectionIsIdempotent(t *testing.T) {
	b := io.NewBuilder().Define("E", empSchema(t))
	b.Section("s", "E").Add(map[string]any{"name": "A", "age": 1})
	b.Section("s", "E").Add(map[string]any{"name": "B", "age": 2})
	doc, err := b.Document()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections()) != 1 {
		t.Fatalf("got %d sections", len(doc.Sections()))
	}
	if doc.Section("s").Len() != 2 {
		t.Errorf("got %d records", doc.Section("s").Len())
	}
}

// An empty build is still a valid document.
func TestEmptyBuild(t *testing.T) {
	doc, err := io.NewBuilder().Document()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Parse(doc.String()); err != nil {
		t.Errorf("an empty build does not re-parse: %v (%q)", err, doc.String())
	}
}

// Variables reach the header and resolve in the document that comes out.
func TestBuilderVariables(t *testing.T) {
	b := io.NewBuilder().Var("region", "apac")
	b.Section("", "").Add(map[string]any{"where": "@region"})
	doc, err := b.Document()
	if err != nil {
		t.Fatal(err)
	}
	back, err := io.Parse(doc.String())
	if err != nil {
		t.Fatalf("%v for %q", err, doc.String())
	}
	if v, ok := back.Var("region"); !ok || v != "apac" {
		t.Errorf("variable lost: %v %v (%q)", v, ok, doc.String())
	}
}
