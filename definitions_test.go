package internetobject_test

import (
	"strings"
	"sync"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

const defsHeader = `~ $Employee: {name: string, age: {int, min: 0}}
~ $Alert: {level: string, msg: string}
~ @region: apac
~ $schema: $Employee`

func mustDefs(t *testing.T) *io.Definitions {
	t.Helper()
	d, err := io.ParseDefinitions(defsHeader)
	if err != nil {
		t.Fatalf("ParseDefinitions: %v", err)
	}
	return d
}

func TestDefinitionsReadsItsHeader(t *testing.T) {
	d := mustDefs(t)
	if got := d.Names(); len(got) != 4 || got[0] != "Employee" {
		t.Errorf("Names() = %v", got)
	}
	if d.Len() != 4 {
		t.Errorf("Len() = %d", d.Len())
	}
	// A name may be written with or without its sigil.
	for _, n := range []string{"Employee", "$Employee"} {
		if d.Schema(n) == nil {
			t.Errorf("Schema(%q) = nil", n)
		}
	}
	if d.Schema("Nope") != nil {
		t.Error("Schema of an undefined name should be nil")
	}
	for _, n := range []string{"region", "@region"} {
		if v, ok := d.Var(n); !ok || v != "apac" {
			t.Errorf("Var(%q) = %v, %v", n, v, ok)
		}
	}
	if _, ok := d.Var("nope"); ok {
		t.Error("Var of an undefined name reported ok")
	}
	if d.Default() == nil {
		t.Error("Default() = nil; the header names $schema")
	}
	// The rendered header re-parses to the same definitions.
	back, err := io.ParseDefinitions(d.String())
	if err != nil {
		t.Fatalf("the rendered header does not re-parse: %v\n%s", err, d.String())
	}
	if len(back.Names()) != len(d.Names()) {
		t.Errorf("round trip changed the definitions: %v vs %v", back.Names(), d.Names())
	}
}

// The point of the type: the wire carries only data.
func TestDefinitionsParseHeaderlessData(t *testing.T) {
	d := mustDefs(t)
	doc, err := d.Parse("---\n~ Alice, 30\n~ Bob, 41")
	if err != nil {
		t.Fatalf("headerless data did not bind: %v", err)
	}
	if len(doc.Records()) != 2 {
		t.Fatalf("got %d records", len(doc.Records()))
	}
	rec := doc.Records()[0].(*io.Object)
	if v, ok := rec.Get("name"); !ok || v != "Alice" {
		t.Errorf("member not bound by the preloaded schema: %v", v)
	}
	// Validation still applies — a preloaded schema is a schema.
	if _, err := d.Parse("---\n~ Carl, nope"); err == nil {
		t.Error("a bad row passed against the preloaded schema")
	} else {
		var list io.ErrorList
		if !asErrorList(err, &list) || !list.Has(io.ExpectedInteger) {
			t.Errorf("got %v, want expected-integer", err)
		}
	}
}

// io-specs: a document's own header wins over preloaded definitions.
func TestInStreamDefinitionsOverridePreloaded(t *testing.T) {
	d := mustDefs(t)
	doc, err := d.Parse("~ $Employee: {name: string, nick: string}\n--- $Employee\n~ Dee, dee")
	if err != nil {
		t.Fatalf("%v", err)
	}
	rec := doc.Records()[0].(*io.Object)
	if _, ok := rec.Get("nick"); !ok {
		t.Errorf("the document's own $Employee did not win: %v", rec.Keys())
	}
	// A name the document does NOT redefine still resolves to the preloaded one.
	doc2, err := d.Parse("~ $Other: {x: int}\n--- $Alert\n~ warn, disk full")
	if err != nil {
		t.Fatalf("a preloaded name did not resolve beside a local one: %v", err)
	}
	if len(doc2.Records()) != 1 {
		t.Errorf("got %d records", len(doc2.Records()))
	}
}

func TestDefinitionsStream(t *testing.T) {
	d := mustDefs(t)
	var got []string
	for item, err := range d.Stream(strings.NewReader("---\n~ Eve, 22\n~ Frank, bad\n"), nil) {
		if err != nil {
			t.Fatalf("fatal: %v", err)
		}
		if item.Err != nil {
			got = append(got, "err:"+string(item.Err.Code))
			continue
		}
		v, _ := item.Value.(*io.Object).Get("name")
		got = append(got, v.(string))
	}
	if len(got) != 2 || got[0] != "Eve" || got[1] != "err:expected-integer" {
		t.Errorf("stream items = %v", got)
	}
}

// The type exists to be compiled once and shared, so this is contract, not
// an optimisation: -race must find nothing.
func TestDefinitionsAreSafeToShare(t *testing.T) {
	d := mustDefs(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, err := d.Parse("---\n~ Alice, 30"); err != nil {
					t.Errorf("concurrent Parse: %v", err)
					return
				}
				_ = d.Schema("Employee")
				_, _ = d.Var("region")
				_ = d.Names()
				_ = d.String()
			}
		}()
	}
	wg.Wait()
}

// A header that cannot compile fails at ParseDefinitions, not on the
// thousandth payload.
func TestBrokenHeaderFailsUpFront(t *testing.T) {
	for _, src := range []string{
		"~ $P: {n: nosuchtype}",
		"~ $P: {n: string",
		"~ $A: $B",
	} {
		if d, err := io.ParseDefinitions(src); err == nil {
			t.Errorf("%q compiled to %v; expected an error", src, d.Names())
		}
	}
}

// A document exposes its own header the same way.
func TestDocumentDefinitionsView(t *testing.T) {
	doc, err := io.Parse(defsHeader + "\n---\n~ Alice, 30")
	if err != nil {
		t.Fatal(err)
	}
	d := doc.Definitions()
	if d.Schema("Alert") == nil {
		t.Error("the document's own $Alert is not visible")
	}
	if v, ok := doc.Var("region"); !ok || v != "apac" {
		t.Errorf("Document.Var = %v, %v", v, ok)
	}
	if _, ok := doc.Var("nope"); ok {
		t.Error("Document.Var reported an undefined name")
	}
}

// The zero value and a nil receiver answer rather than panicking — a caller
// may hold one before it is set.
func TestNilDefinitionsAreInert(t *testing.T) {
	var d *io.Definitions
	if d.Schema("x") != nil || d.Default() != nil || d.Len() != 0 || d.Names() != nil {
		t.Error("a nil Definitions should read as empty")
	}
	if _, ok := d.Var("x"); ok {
		t.Error("a nil Definitions has no variables")
	}
	if got := d.String(); got != "" {
		t.Errorf("nil String() = %q", got)
	}
	if _, err := d.Parse("---\n~ 1"); err != nil {
		t.Errorf("a nil Definitions should still parse: %v", err)
	}
}

// A document's own definition may be an alias of a shared one. Resolving the
// document's names before the shared definitions were in scope reported these
// as undefined-schema (review of SPEC 0004 A1, 2026-09-14).
func TestLocalAliasOfASharedDefinition(t *testing.T) {
	defs, err := io.ParseDefinitions("~ $base: {name: string}")
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		"~ $person: $base\n--- $person\n~ Alice\n",
		"~ $schema: $base\n---\n~ Alice\n",
	} {
		doc, err := defs.Parse(src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if !strings.Contains(doc.String(), "Alice") {
			t.Errorf("%q wrote %q", src, doc.String())
		}
	}
}

// Definitions.Stream keeps its definitions in force when the caller also
// preloads header text: they are layered, the caller's text above them. The
// definitions used to be skipped whenever opts.Definitions was set, so these
// records were read with no schema at all (2026-09-14).
func TestDefinitionsStreamLayersUnderCallerDefinitions(t *testing.T) {
	defs, err := io.ParseDefinitions("~ $emp: {name: string, age: int}\n~ $schema: $emp")
	if err != nil {
		t.Fatal(err)
	}
	opts := &io.StreamOptions{Definitions: "~ $alert: {level: string}"}
	var got []string
	for item, err := range defs.Stream(strings.NewReader("---\n~ Alice, 30\n~ Bob, old\n--- $alert\n~ warn\n"), opts) {
		if err != nil {
			t.Fatal(err)
		}
		if item.Err != nil {
			got = append(got, "err:"+string(item.Err.Code))
			continue
		}
		obj := item.Value.(*io.Object)
		if v, ok := obj.Get("name"); ok {
			got = append(got, v.(string))
		} else if v, ok := obj.Get("level"); ok {
			got = append(got, v.(string))
		} else {
			got = append(got, "unbound")
		}
	}
	if want := "Alice err:expected-integer warn"; strings.Join(got, " ") != want {
		t.Errorf("items = %v, want %s", got, want)
	}
}

// A lookup returns the same *Schema every time, so the header it renders for
// MarshalWith is rendered once, not once per lookup (2026-09-14).
func TestDefinitionsHandOutOneSchemaPerName(t *testing.T) {
	defs, err := io.ParseDefinitions("~ $emp: {name: string, age: int}\n~ $schema: $emp")
	if err != nil {
		t.Fatal(err)
	}
	if defs.Schema("emp") == nil || defs.Schema("emp") != defs.Schema("$emp") {
		t.Error("two lookups of $emp returned different values")
	}
	if defs.Default() == nil || defs.Default() != defs.Schema("emp") {
		t.Error("Default and the $emp it aliases returned different values")
	}
	rec := struct {
		Name string `io:"name"`
		Age  int    `io:"age"`
	}{"Ann", 30}
	if _, err := io.MarshalWith(rec, defs.Schema("emp")); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(50, func() {
		if _, err := io.MarshalWith(rec, defs.Schema("emp")); err != nil {
			t.Fatal(err)
		}
	})
	direct := defs.Schema("emp")
	want := testing.AllocsPerRun(50, func() {
		if _, err := io.MarshalWith(rec, direct); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > want {
		t.Errorf("MarshalWith through a lookup spends %.0f allocations, %.0f with the schema held", allocs, want)
	}
}

// String writes no separator: empty definitions are "", and a header holding
// only a bare schema expression still round-trips.
func TestDefinitionsStringWritesNoSeparator(t *testing.T) {
	for _, src := range []string{"", "---", "name: string, age: int", "~ $p: {name: string}"} {
		d, err := io.ParseDefinitions(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		text := d.String()
		if strings.Contains(text, "---") {
			t.Errorf("%q rendered %q, with a separator", src, text)
		}
		back, err := io.ParseDefinitions(text)
		if err != nil {
			t.Fatalf("%q -> %q does not re-parse: %v", src, text, err)
		}
		if back.Len() != d.Len() || (back.Default() == nil) != (d.Default() == nil) {
			t.Errorf("%q -> %q lost definitions", src, text)
		}
	}
}

// The default a stream binds to, from the most specific source down: the
// stream's own header, then preloaded definitions (text, then compiled), then
// StreamOptions.DefaultSchema. A bare schema expression counts as a default at
// its own layer, so a later expression overrides an earlier `$schema`.
func TestStreamDefaultSchemaPrecedence(t *testing.T) {
	defs, err := io.ParseDefinitions("~ $emp: {name: string, age: int}\n~ $schema: $emp\n~ $alt: {level: string}")
	if err != nil {
		t.Fatal(err)
	}
	firstKey := func(seq func(func(io.StreamItem, error) bool)) string {
		for item, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if item.Err != nil {
				return "err:" + string(item.Err.Code)
			}
			obj := item.Value.(*io.Object)
			if obj.Len() == 0 {
				return "empty"
			}
			return obj.Members[0].Key
		}
		return "none"
	}
	cases := []struct {
		name, wire string
		opts       *io.StreamOptions
		want       string
	}{
		{"compiled $schema", "---\n~ Ann, 30\n", nil, "name"},
		{"in-stream expression beats compiled $schema", "code: string\n---\n~ x\n", nil, "code"},
		{"compiled $schema beats DefaultSchema", "---\n~ Ann, 30\n", &io.StreamOptions{DefaultSchema: "$alt"}, "name"},
		{"preloaded text $schema beats compiled", "---\n~ warn\n",
			&io.StreamOptions{Definitions: "~ $schema: {level: string}"}, "level"},
		{"in-stream expression beats preloaded text $schema", "code: string\n---\n~ x\n",
			&io.StreamOptions{Definitions: "~ $schema: {level: string}"}, "code"},
	}
	for _, c := range cases {
		if got := firstKey(defs.Stream(strings.NewReader(c.wire), c.opts)); got != c.want {
			t.Errorf("%s: first member %q, want %q", c.name, got, c.want)
		}
	}
	// Plain Stream, no compiled layer: an in-stream expression beats a
	// preloaded `$schema`, and DefaultSchema applies only when nothing else does.
	if got := firstKey(io.Stream(strings.NewReader("code: string\n---\n~ x\n"),
		&io.StreamOptions{Definitions: "~ $schema: {level: string}"})); got != "code" {
		t.Errorf("plain Stream: first member %q, want code", got)
	}
	if got := firstKey(io.Stream(strings.NewReader("---\n~ warn\n"),
		&io.StreamOptions{Definitions: "~ $alt: {level: string}", DefaultSchema: "$alt"})); got != "level" {
		t.Errorf("DefaultSchema fallback: first member %q, want level", got)
	}
}
