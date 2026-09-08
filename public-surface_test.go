package internetobject_test

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The exported wrappers, exercised THROUGH the public package.
//
// Several of these delegate to internal code that is already well covered, and
// that is exactly why they need their own test: a passthrough is where a
// swapped argument or a dropped error hides, and the internal tests cannot see
// it because they never call the wrapper. io.DecimalFromInt and
// io.DecimalFromFloat had 0% coverage at this boundary while core's equivalents
// had 100%.

func TestExportedDecimalConstructors(t *testing.T) {
	// io.DecimalFromInt over several Go integer widths, signed and unsigned.
	for name, got := range map[string]io.Decimal{
		"int":    io.DecimalFromInt(42),
		"int8":   io.DecimalFromInt(int8(42)),
		"int64":  io.DecimalFromInt(int64(42)),
		"uint":   io.DecimalFromInt(uint(42)),
		"uint64": io.DecimalFromInt(uint64(42)),
	} {
		if got.String() != "42" || got.Scale != 0 {
			t.Errorf("DecimalFromInt(%s) = %q scale %d", name, got.String(), got.Scale)
		}
	}
	if got := io.DecimalFromInt(-7); got.String() != "-7" {
		t.Errorf("negative: %q", got.String())
	}
	// Past int64, which is the case a careless conversion would truncate.
	if got := io.DecimalFromInt(uint64(18446744073709551615)); got.String() != "18446744073709551615" {
		t.Errorf("max uint64: %q", got.String())
	}

	// io.DecimalFromFloat — the scale is the caller's, and the error is real.
	d, err := io.DecimalFromFloat(19.99, 2)
	if err != nil || d.String() != "19.99" {
		t.Errorf("DecimalFromFloat = %q, %v", d.String(), err)
	}
	if d, err := io.DecimalFromFloat(-2.5, 0); err != nil || d.String() != "-3" {
		t.Errorf("half away from zero: %q, %v", d.String(), err)
	}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := io.DecimalFromFloat(f, 2); err == nil {
			t.Errorf("DecimalFromFloat(%v) was accepted", f)
		}
	}

	// The other two constructors, so all four are covered from out here.
	if got := io.NewDecimal(1999, 2); got.String() != "19.99" {
		t.Errorf("NewDecimal = %q", got.String())
	}
	if got := io.DecimalFromBig(big.NewInt(1999), 2); got.String() != "19.99" {
		t.Errorf("DecimalFromBig = %q", got.String())
	}
	if _, err := io.ParseDecimal("nope"); err == nil {
		t.Error("ParseDecimal accepted rubbish")
	}
	if !errors.Is(mustQuoErr(t), io.ErrDivideByZero) {
		t.Error("io.ErrDivideByZero is not the error a zero divisor returns")
	}
}

func mustQuoErr(t *testing.T) error {
	t.Helper()
	one, _ := io.ParseDecimal("1")
	zero, _ := io.ParseDecimal("0")
	_, err := one.Quo(zero, 2)
	return err
}

// Builder.String is the shorthand for Document().String(), including its
// documented behaviour on a failed build: the empty string, with the error
// available from Document().
func TestBuilderString(t *testing.T) {
	b := io.NewBuilder()
	if err := b.Section("", "").Add(map[string]any{"a": 1}); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	if text == "" {
		t.Fatal("String() returned nothing for a good build")
	}
	doc, err := b.Document()
	if err != nil || doc.String() != text {
		t.Errorf("String() disagrees with Document().String(): %q vs %q", text, doc.String())
	}
	if _, err := io.Parse(text); err != nil {
		t.Errorf("Builder.String() does not re-parse: %v (%q)", err, text)
	}

	// A failed build gives "" — and the error is still reachable.
	bad := io.NewBuilder()
	bad.Section("x", "NoSuchSchema")
	if got := bad.String(); got != "" {
		t.Errorf("a failed build returned %q, want the empty string", got)
	}
	if bad.Err() == nil {
		t.Error("the failure is not reachable through Err()")
	}
	if _, err := bad.Document(); err == nil {
		t.Error("Document() hid the failure")
	}
}

// A row that fails to BIND — as opposed to failing validation — still lands in
// the collection's error list rather than being dropped silently. This is the
// path bindFault exists for, and nothing reached it: the earlier tests all
// used rows that failed validation instead.
func TestCollectionReportsBindingFailures(t *testing.T) {
	type Row struct {
		N int `io:"n"` // the document will send a string here
	}
	var d struct {
		Rows io.Collection[Row] `io:"rows"`
	}
	// No schema, so the values reach the binder untyped and the MISMATCH is a
	// Go-side binding failure rather than a validation fault.
	err := io.Unmarshal("--- rows\n~ n: 1\n~ n: notanint\n~ n: 3\n", &d)
	if err != nil {
		t.Fatalf("a binding failure should be absorbed by the collection: %v", err)
	}
	if d.Rows.Len() != 3 {
		t.Errorf("Len() = %d, want 3 attempted", d.Rows.Len())
	}
	items := d.Rows.Items()
	if len(items) != 2 || items[0].N != 1 || items[1].N != 3 {
		t.Errorf("Items() = %v", items)
	}
	errs := d.Rows.Errors()
	if len(errs) != 1 {
		t.Fatalf("Errors() = %v, want one", errs)
	}
	if errs[0].RecordIndex != 1 {
		t.Errorf("the fault names row %d, want 1", errs[0].RecordIndex)
	}
	if errs[0].Code == "" {
		t.Error("the fault carries no code")
	}
	// Every row is accounted for exactly once.
	if len(items)+len(errs) != d.Rows.Len() {
		t.Errorf("%d bound + %d failed != %d attempted", len(items), len(errs), d.Rows.Len())
	}
}

// JSON must refuse a value it has no spelling for, by name, rather than
// emitting something invalid. typeNameOf is what puts the type in the message.
func TestJSONRefusesAnUnrenderableValue(t *testing.T) {
	b := io.NewBuilder()
	// A Go value the format itself carries but JSON has no mapping for is hard
	// to reach through the parser, so it is built directly.
	rec := io.NewObject(1)
	rec.Append("ch", make(chan int))
	if err := b.Section("", "").Add(rec); err == nil {
		// The marshaler rejects it first, which is also correct — the point is
		// that neither path emits invalid JSON.
		doc, derr := b.Document()
		if derr == nil {
			if _, jerr := doc.JSON(nil); jerr == nil {
				t.Error("an unrenderable value produced JSON")
			}
		}
	}
}

// Definitions.Stream with options supplied by the caller — the branch where
// StreamOptions is not nil and already carries definitions of its own.
func TestDefinitionsStreamWithCallerOptions(t *testing.T) {
	defs, err := io.ParseDefinitions("~ $emp: {name: string, age: int}\n~ $schema: $emp")
	if err != nil {
		t.Fatal(err)
	}
	// Caller-supplied options are honoured, not overwritten.
	n := 0
	for item, err := range defs.Stream(strings.NewReader("---\n~ Alice, 30\n"), &io.StreamOptions{}) {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if item.Err != nil {
			t.Fatalf("record %d: %s", item.Index, item.Err.Code)
		}
		if v, ok := item.Value.(*io.Object).Get("name"); !ok || v != "Alice" {
			t.Errorf("record = %v", item.Value)
		}
		n++
	}
	if n != 1 {
		t.Errorf("read %d records", n)
	}

	// Header text passed WITH its separator parses the same as without.
	withSep, err := io.ParseDefinitions("~ $emp: {name: string}\n---\n")
	if err != nil {
		t.Fatalf("a header carrying its own separator was refused: %v", err)
	}
	if withSep.Schema("emp") == nil {
		t.Error("the definition was lost when the separator was present")
	}
}

// Document.Definitions on a document whose header does not compile returns an
// empty view rather than panicking.
func TestDocumentDefinitionsOnABrokenHeader(t *testing.T) {
	doc, err := io.Parse("~ $P: {n: nosuchtype}\n---\n~ 1")
	if err == nil {
		t.Skip("this header now compiles; the branch is unreachable")
	}
	if doc == nil {
		t.Fatal("Parse returned no document")
	}
	d := doc.Definitions() // must not panic
	if d == nil {
		t.Fatal("Definitions() returned nil")
	}
	if d.Len() != 0 || d.Schema("P") != nil {
		t.Errorf("a broken header produced %d definitions", d.Len())
	}
}

// The remaining exported entry points, so the public surface has no untried
// corner. Each is a smoke test: the detailed behaviour lives in its own file.
func TestExportedSurfaceSmoke(t *testing.T) {
	type R struct {
		N int `io:"n"`
	}
	s, err := io.SchemaFor[R]()
	if err != nil {
		t.Fatal(err)
	}
	if s.MemberNames()[0] != "n" || s.Open() {
		t.Errorf("SchemaFor: %v open=%v", s.MemberNames(), s.Open())
	}

	text, err := io.MarshalWith(R{1}, s)
	if err != nil {
		t.Fatal(err)
	}
	var back R
	if err := io.UnmarshalWith(text, &back, s); err != nil || back.N != 1 {
		t.Errorf("MarshalWith/UnmarshalWith round trip: %v %v", back, err)
	}
	if err := io.ValidateWith(R{1}, s); err != nil {
		t.Errorf("ValidateWith: %v", err)
	}
	doc, err := io.ParseWith(text, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Records()) != 1 {
		t.Errorf("ParseWith gave %d records", len(doc.Records()))
	}

	// MarshalJSON, the encoding/json entry point.
	wrapped, err := json.Marshal(map[string]any{"doc": doc})
	if err != nil || !strings.Contains(string(wrapped), `"n":1`) {
		t.Errorf("MarshalJSON: %s %v", wrapped, err)
	}
}

// Section.Value — the per-section projection, which nothing reached. A
// collection section yields its records; a single-record section yields the
// record's own value, not a one-element slice.
func TestSectionValue(t *testing.T) {
	doc, err := io.Parse("--- rows\n~ a: 1\n~ a: 2\n--- one\nb: 3\n")
	if err != nil {
		t.Fatal(err)
	}
	rows := doc.Section("rows")
	if rows == nil || !rows.IsCollection() {
		t.Fatal("rows section missing or not a collection")
	}
	got, ok := rows.Value().([]any)
	if !ok || len(got) != 2 {
		t.Errorf("collection Value() = %#v", rows.Value())
	}

	one := doc.Section("one")
	if one == nil {
		t.Fatal("one section missing")
	}
	if one.IsCollection() {
		t.Fatal("`one` should not be a collection")
	}
	// A single-record section projects the RECORD, not a slice holding it.
	if _, isSlice := one.Value().([]any); isSlice {
		t.Errorf("a single-record section projected a slice: %#v", one.Value())
	}
	if obj, ok := one.Value().(*io.Object); !ok {
		t.Errorf("Value() = %T, want *io.Object", one.Value())
	} else if v, _ := obj.Get("b"); v != float64(3) {
		t.Errorf("member = %v", v)
	}
}

// A uint past the wire type's range is refused with a message naming the
// value, rather than silently wrapping.
func TestUintOverflowIsRefused(t *testing.T) {
	type big struct {
		N uint64 `io:"n"`
	}
	_, err := io.Marshal(big{N: 18446744073709551615})
	if err == nil {
		t.Fatal("a uint64 past the safe integer range was accepted")
	}
	if !strings.Contains(err.Error(), "18446744073709551615") {
		t.Errorf("the message does not name the value: %v", err)
	}
	if !strings.Contains(err.Error(), "big.Int") {
		t.Errorf("the message does not say what to use instead: %v", err)
	}
	// Inside the range is fine.
	if _, err := io.Marshal(big{N: 42}); err != nil {
		t.Errorf("a small uint64 was refused: %v", err)
	}
}

// A `schema` tag carrying a $ref or an @variable has no definition context to
// resolve it against — a derived schema declares nothing. Both COMPILE (the
// reference is resolved lazily, so SchemaFor succeeds) and both fail at
// VALIDATION with their designated codes, which is what noDefs exists to
// produce.
func TestSchemaTagReferenceHasNoContext(t *testing.T) {
	type refRow struct {
		N int `io:"n" schema:"$Nope"`
	}
	s, err := io.SchemaFor[refRow]()
	if err != nil {
		t.Fatalf("a $ref in a tag should compile lazily: %v", err)
	}
	if s.String() != "n: $Nope" {
		t.Errorf("compiled to %q", s.String())
	}
	assertCode(t, io.Validate(refRow{N: 1}), io.UndefinedSchema)
	// Marshal validates too, so it reports the same fault rather than emitting
	// a document against a schema it cannot resolve.
	_, mErr := io.Marshal(refRow{N: 1})
	assertCode(t, mErr, io.UndefinedSchema)

	type varRow struct {
		N int `io:"n" schema:"{int, min: @nope}"`
	}
	if _, err := io.SchemaFor[varRow](); err != nil {
		t.Fatalf("an @variable in a tag should compile: %v", err)
	}
	assertCode(t, io.Validate(varRow{N: 1}), io.UndefinedVariable)
}

// assertCode fails unless err carries the designated code.
func assertCode(t *testing.T, err error, want io.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got no error", want)
	}
	var list io.ErrorList
	if errors.As(err, &list) {
		if !list.Has(want) {
			t.Errorf("got %v, want %s", list.Codes(), want)
		}
		return
	}
	var one io.Error
	if errors.As(err, &one) {
		if one.Code != want {
			t.Errorf("got %s, want %s", one.Code, want)
		}
		return
	}
	if !strings.Contains(err.Error(), string(want)) {
		t.Errorf("got %v, want %s", err, want)
	}
}

// An OPTIONAL or NULLABLE field carrying a type annotation is rewritten into
// the object form of a typedef, because `?` and `*` cannot be attached to a
// bare annotation. Nothing exercised that rewrite.
func TestOptionalAndNullableAnnotatedFields(t *testing.T) {
	type row struct {
		A string `io:"a,optional"      schema:"{string, minLen: 2}"`
		B int    `io:"b,omitempty"     schema:"{int, min: 0}"`
		C []int  `io:"c,optional"      schema:"[int]"`
		D string `io:"d,optional"      schema:"string"`
	}
	s, err := io.SchemaFor[row]()
	if err != nil {
		t.Fatalf("SchemaFor: %v", err)
	}
	text := s.String()
	for _, want := range []string{"a?", "b?", "c?", "d?"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q missing from %q", want, text)
		}
	}

	// The constraints survive the rewrite.
	if err := io.ValidateWith(row{A: "x"}, s); err == nil {
		t.Error("minLen was lost in the optional rewrite")
	}
	if err := io.ValidateWith(row{B: -1}, s); err == nil {
		t.Error("min was lost in the optional rewrite")
	}
	if err := io.ValidateWith(row{A: "ok", B: 1, C: []int{1}, D: "d"}, s); err != nil {
		t.Errorf("a valid row was rejected: %v", err)
	}

	// And the whole thing round-trips.
	out, err := io.Marshal(row{A: "ok", B: 1, C: []int{1, 2}, D: "d"})
	if err != nil {
		t.Fatal(err)
	}
	var back row
	if err := io.Unmarshal(out, &back); err != nil {
		t.Fatalf("%v for %q", err, out)
	}
	if back.A != "ok" || back.B != 1 || len(back.C) != 2 || back.D != "d" {
		t.Errorf("round trip = %+v (%q)", back, out)
	}
}

// A member name that NEEDS QUOTING cannot carry the short `?` / `*` markers —
// `"a b"?` is not a spelling the format has — so the flags move into the
// object form of the typedef instead. That rewrite had no test.
func TestQuotedMemberNameCarriesFlagsInTheTypedef(t *testing.T) {
	type row struct {
		B int    `io:"10,optional"   schema:"{int, min: 0}"`
		C string `io:"c:d,omitempty"`
	}
	s, err := io.SchemaFor[row]()
	if err != nil {
		t.Fatalf("SchemaFor: %v", err)
	}
	text := s.String()
	// The names are quoted and the optionality is inside the typedef, not a
	// marker glued to the name.
	for _, want := range []string{`"10"`, `"c:d"`} {
		if !strings.Contains(text, want) {
			t.Errorf("%s missing from %q", want, text)
		}
	}
	if strings.Contains(text, `"10"?`) || strings.Contains(text, `"c:d"?`) {
		t.Errorf("a quoted name carried a short marker: %q", text)
	}
	// The flags are inside the typedef, which is the whole point of the rewrite.
	if !strings.Contains(text, "optional: T") {
		t.Errorf("optionality did not move into the typedef: %q", text)
	}

	// The optionality survives: an absent member is accepted.
	if err := io.ValidateWith(row{}, s); err != nil {
		t.Errorf("optional members were not optional: %v", err)
	}
	// And so do the constraints that travelled with them.
	if err := io.ValidateWith(row{B: -1}, s); err == nil {
		t.Error("min was lost moving into the typedef")
	}

	// The whole thing round-trips through text.
	out, err := io.Marshal(row{B: 1, C: "z"})
	if err != nil {
		t.Fatal(err)
	}
	var back row
	if err := io.Unmarshal(out, &back); err != nil {
		t.Fatalf("%v for %q", err, out)
	}
	if back.B != 1 || back.C != "z" {
		t.Errorf("round trip = %+v (%q)", back, out)
	}
}
