package internetobject_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The conformance corpus asserts error CODES only — in every implementation.
// Positions, paths, categories and record indexes are therefore ungated
// anywhere, which is exactly how this port drifted to a hardcoded 1:1 at 13
// sites while staying green. These tests are the only
// gate that exists for them; treat a failure here as a real defect, not a
// brittle assertion.

const errDoc = "name: string, age: {int, min: 0, max: 130}\n" +
	"---\n" +
	"~ Alice, 30\n" + // line 3, good
	"~ Bob, oops\n" + // line 4, expected-integer at the value
	"~ Cara, -5\n" + // line 5, mismatched-min at the value
	"~ Dan, 200" // line 6, mismatched-max at the value

func TestErrorsCarryPositionPathAndIndex(t *testing.T) {
	_, err := io.Parse(errDoc)
	var list io.ErrorList
	if !errors.As(err, &list) || len(list) != 3 {
		t.Fatalf("want 3 faults, got %v", err)
	}

	want := []io.Error{
		{Code: "expected-integer", Category: "validation", Path: "$[1].age", RecordIndex: 1, Line: 4, Col: 8},
		{Code: "mismatched-min", Category: "validation", Path: "$[2].age", RecordIndex: 2, Line: 5, Col: 9},
		{Code: "mismatched-max", Category: "validation", Path: "$[3].age", RecordIndex: 3, Line: 6, Col: 8},
	}
	for i, w := range want {
		if list[i] != w {
			t.Errorf("fault %d:\n got %+v\nwant %+v", i, list[i], w)
		}
	}
}

// A syntax fault keeps the position the tokenizer computed, and is classified
// by where it arose rather than by how the code is spelled.
func TestSyntaxFaultPositionAndCategory(t *testing.T) {
	_, err := io.Parse("---\n~ {a: 1\n")
	var list io.ErrorList
	if !errors.As(err, &list) || len(list) == 0 {
		t.Fatalf("want a syntax fault, got %v", err)
	}
	if list[0].Category != "syntax" {
		t.Errorf("category = %q, want syntax (%+v)", list[0].Category, list[0])
	}
	if list[0].Line != 2 {
		t.Errorf("line = %d, want 2 (%+v)", list[0].Line, list[0])
	}
}

// A deferred literal error already carries a real position from the
// tokenizer; validation must not overwrite it with a fabricated one.
func TestDeferredLiteralKeepsItsPosition(t *testing.T) {
	_, err := io.Parse("---\n~ x: 0B\n")
	var list io.ErrorList
	if !errors.As(err, &list) || len(list) == 0 {
		t.Fatalf("want invalid-number, got %v", err)
	}
	if list[0].Line != 2 || list[0].Col == 0 {
		t.Errorf("position lost: %+v", list[0])
	}
}

// An absence fault has no value to point at and is reported at the record —
// the reference does the same.
func TestMissingMemberReportsAtTheRecord(t *testing.T) {
	_, err := io.Parse("name: string, age: int\n---\n~ Alice\n~ Bob, 5")
	var list io.ErrorList
	if !errors.As(err, &list) || list[0].Code != "missing-value" {
		t.Fatalf("want missing-value, got %v", err)
	}
	if list[0].Line != 3 || list[0].Path != "$[0].age" || list[0].RecordIndex != 0 {
		t.Errorf("absence fault not located: %+v", list[0])
	}
}

// A faulted row keeps its place in BOTH projections and carries the same
// marker; neither may quietly turn it into nil.
func TestFaultedRowMarkerInBothProjections(t *testing.T) {
	doc, err := io.Parse(errDoc)
	if err == nil {
		t.Fatal("expected faults")
	}
	records := doc.Records()
	if len(records) != 4 {
		t.Fatalf("collection lost rows: %d", len(records))
	}
	if io.IsError(records[0]) {
		t.Error("row 0 is valid and must not be marked")
	}
	item, ok := records[1].(io.ErrorItem)
	if !ok {
		t.Fatalf("row 1 should be an ErrorItem, got %T", records[1])
	}
	if item.Code != "expected-integer" || item.Path != "$[1].age" || item.Line != 4 {
		t.Errorf("marker not populated: %+v", item)
	}

	// Value() must agree with Records() on every row.
	values := doc.Value().([]any)
	if len(values) != len(records) {
		t.Fatalf("Value() has %d rows, Records() %d", len(values), len(records))
	}
	for i := range values {
		if io.IsError(values[i]) != io.IsError(records[i]) {
			t.Errorf("row %d: Value() and Records() disagree about failure", i)
		}
	}
}

// IsError is a type check, so data can never impersonate a failure.
func TestDataCannotForgeAnError(t *testing.T) {
	doc, err := io.Parse("---\n~ __error: T, code: expected-integer, category: validation")
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range doc.Records() {
		if io.IsError(r) {
			t.Fatalf("row %d: data forged an error marker", i)
		}
	}
}

func TestErrorListHelpers(t *testing.T) {
	_, err := io.Parse(errDoc)
	var list io.ErrorList
	if !errors.As(err, &list) {
		t.Fatalf("err is not an ErrorList: %T", err)
	}
	// The constants are the point: a typo is now a build error rather than a
	// comparison that is quietly always false.
	want := []io.Code{io.ExpectedInteger, io.MismatchedMin, io.MismatchedMax}
	if got := list.Codes(); !slices.Equal(got, want) {
		t.Errorf("Codes() = %v, want %v", got, want)
	}
	if !list.Has(io.MismatchedMin) || list.Has("nope") {
		t.Error("Has() is wrong")
	}
	if !errors.Is(err, io.Error{Code: "mismatched-min"}) {
		t.Error("errors.Is should match a code carried by the list")
	}
	if errors.Is(err, io.Error{Code: "unknown-member"}) {
		t.Error("errors.Is matched a code the list does not carry")
	}
	// The message names the path, so a log line locates the fault.
	if !strings.Contains(list[0].Error(), "$[1].age") {
		t.Errorf("Error() = %q, want the path in it", list[0].Error())
	}
}

// Streaming carries the category the reader computed (a spec MUST) and the
// record's own index.
func TestStreamErrorCarriesCategoryAndIndex(t *testing.T) {
	src := "~ $P: {n: string, a: int}\n--- $P\n~ Alice, 30\n~ Bob, oops\n"
	var seen int
	for item, err := range io.Stream(strings.NewReader(src), nil) {
		if err != nil {
			t.Fatalf("fatal: %v", err)
		}
		if item.Err == nil {
			continue
		}
		seen++
		if item.Err.Category != "validation" {
			t.Errorf("category = %q, want validation", item.Err.Category)
		}
		if item.Err.RecordIndex != 1 || item.Err.Path != "$[1]" {
			t.Errorf("index/path wrong: %+v", *item.Err)
		}
	}
	if seen != 1 {
		t.Fatalf("expected one faulted record, saw %d", seen)
	}
}

// Nested members report their own path, not just the record's.
func TestNestedMemberPath(t *testing.T) {
	// Two members, so the lone-object absorption rule does not apply and the
	// braced value really is the nested object.
	_, err := io.Parse("id: int, home: {city: string, zip: {string, minLen: 5}}\n---\n~ 1, {Pune, \"41\"}")
	var list io.ErrorList
	if !errors.As(err, &list) || len(list) == 0 {
		t.Fatalf("want a fault, got %v", err)
	}
	if list[0].Code != "mismatched-min-len" {
		t.Fatalf("unexpected fault: %+v", list[0])
	}
	if !strings.Contains(list[0].Path, "zip") {
		t.Errorf("path = %q, want it to name the nested member", list[0].Path)
	}
	if list[0].Line != 3 {
		t.Errorf("line = %d, want 3 (%+v)", list[0].Line, list[0])
	}
}

// A named schema is compiled EAGERLY, whether or not anything references it.
//
// io-go used to compile them lazily, so `~ $Draft: {title: nosuchtype}` that
// nothing referenced parsed CLEAN while the reference rejects it with
// unknown-type. That leniency was also the cause of a writer bug: writeHeader
// compiled a definition in order to write it, found it broken, and silently
// DROPPED it — so a valid-looking document lost data on save. Compiling here
// removes both at the source.
func TestNamedSchemaCompilesEvenIfUnreferenced(t *testing.T) {
	_, err := io.Parse("~ $Draft: {title: nosuchtype}" + nlHdr2 +
		"~ $schema: {id: int}" + nlHdr2 + "---" + nlHdr2 + "~ 1")
	var list io.ErrorList
	if !errors.As(err, &list) || !list.Has("unknown-type") {
		t.Fatalf("an unreferenced broken definition must not parse clean, got %v", err)
	}

	// A sound one survives the write, and writing is idempotent.
	good := "~ $Draft: {title: string}" + nlHdr2 + "~ $schema: {id: int}" + nlHdr2 + "---" + nlHdr2 + "~ 1"
	doc, err := io.Parse(good)
	if err != nil {
		t.Fatalf("%q: %v", good, err)
	}
	first := doc.String()
	if !strings.Contains(first, "$Draft") {
		t.Fatalf("the definition was dropped: %q", first)
	}
	back, err := io.Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	if second := back.String(); second != first {
		t.Errorf("not idempotent: %q vs %q", first, second)
	}
}

var nlHdr2 = string(rune(10))

// Every Error the package returns carries the fields its type documents: a
// Category, and RecordIndex -1 outside a collection. Four entry points used to
// build Errors by hand and left Category empty and RecordIndex 0, and a
// Collection fault reported `$.people[0][0]` at 0:0 (2026-09-14).
func TestEveryErrorKeepsTheDocumentedContract(t *testing.T) {
	firstOf := func(err error) io.Error {
		t.Helper()
		var list io.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			return list[0]
		}
		var e io.Error
		if !errors.As(err, &e) {
			t.Fatalf("not an io.Error: %#v", err)
		}
		return e
	}
	outside := map[string]error{}
	_, outside["ParseSchema"] = io.ParseSchema("a: nosuchtype")
	doc, err := io.Parse("~ $p: {name: string}\n--- $p\n~ a")
	if err != nil {
		t.Fatal(err)
	}
	_, outside["Document.SchemaOf"] = doc.SchemaOf("missing")
	_, outside["ParseWith(nil)"] = io.ParseWith("a", nil)
	for _, streamErr := range io.Stream(strings.NewReader("~ $p: {name: string}\n--- $nope\n~ a\n"), nil) {
		if streamErr != nil {
			outside["Stream"] = streamErr
		}
	}
	if len(outside) != 4 {
		t.Fatalf("an entry point produced no error: %v", outside)
	}
	for name, err := range outside {
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		e := firstOf(err)
		if e.Category == "" || e.RecordIndex != -1 {
			t.Errorf("%s: %+v, want a Category and RecordIndex -1", name, e)
		}
	}

	var d struct {
		People io.Collection[person] `io:"people"`
	}
	if err := io.Unmarshal("--- people\n~ name: Ann, age: 1.5\n~ name: Bob, age: 2\n", &d); err != nil {
		t.Fatal(err)
	}
	errs := d.People.Errors()
	if len(errs) != 1 {
		t.Fatalf("collection faults: %v", errs)
	}
	e := errs[0]
	if e.Path != "$.people[0].age" || e.RecordIndex != 0 || e.Line != 2 || e.Category == "" {
		t.Errorf("collection fault = %+v, want path $.people[0].age at the record's line 2", e)
	}
}

// An Error without a source position — a record built in Go — prints none.
func TestErrorWithoutAPositionPrintsNone(t *testing.T) {
	for want, e := range map[string]io.Error{
		"mismatched-min at $[1].age":       {Code: io.MismatchedMin, Path: "$[1].age"},
		"mismatched-min":                   {Code: io.MismatchedMin},
		"mismatched-min at $[1].age (4:8)": {Code: io.MismatchedMin, Path: "$[1].age", Line: 4, Col: 8},
		"mismatched-min at 4:8":            {Code: io.MismatchedMin, Line: 4, Col: 8},
	} {
		if got := e.Error(); got != want {
			t.Errorf("%+v.Error() = %q, want %q", e, got, want)
		}
	}
}
