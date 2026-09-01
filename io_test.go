package internetobject_test

import (
	"errors"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

func TestParseRoundTrip(t *testing.T) {
	src := "~ $schema: {name: string, age: int}\n---\n~ Alice, 30\n~ Bob, 25"
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	records, ok := doc.Value().([]any)
	if !ok || len(records) != 2 {
		t.Fatalf("Value() = %#v", doc.Value())
	}
	rec := records[0].(*io.Object)
	if i := rec.Find("name"); i < 0 || rec.Members[i].Value != "Alice" {
		t.Fatalf("record 0 = %#v", rec)
	}

	out := doc.String()
	back, err := io.Parse(out)
	if err != nil {
		t.Fatalf("re-parse of %q: %v", out, err)
	}
	if back.String() != out {
		t.Fatalf("not idempotent: %q then %q", out, back.String())
	}
}

func TestParseAccumulatesAndContinues(t *testing.T) {
	doc, err := io.Parse("~ $schema: {a: int}\n---\n~ 1\n~ x\n~ 3")
	if err == nil {
		t.Fatal("want an error for the faulted record")
	}
	var list io.ErrorList
	if !errors.As(err, &list) || len(list) != 1 || list[0].Code != "expected-integer" {
		t.Fatalf("err = %v", err)
	}
	records := doc.Value().([]any)
	if len(records) != 3 {
		t.Fatalf("surviving records: %#v", records)
	}
}

func TestParseSchema(t *testing.T) {
	s, err := io.ParseSchema("name: string, age?: {int, min: 0}, *")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.MemberNames(); len(got) != 2 || got[0] != "name" || got[1] != "age" {
		t.Fatalf("MemberNames = %v", got)
	}
	if !s.Open() {
		t.Fatal("schema should be open")
	}
	if _, err := io.ParseSchema("x: nosuchtype"); err == nil ||
		!strings.Contains(err.Error(), "unknown-type") {
		t.Fatalf("want unknown-type, got %v", err)
	}
}

func TestStream(t *testing.T) {
	src := "~ $P: {n:string, a:int}\n--- $P\n~ Alice, 30\n~ Bob, oops\n~ Cara, 27\n"
	var values, faults int
	for item, err := range io.Stream(strings.NewReader(src), nil) {
		if err != nil {
			t.Fatalf("fatal: %v", err)
		}
		if item.Err != nil {
			faults++
			if item.Err.Code != "expected-integer" {
				t.Fatalf("record %d: %v", item.Index, item.Err)
			}
			continue
		}
		if item.SchemaName != "$P" {
			t.Fatalf("schemaName = %q", item.SchemaName)
		}
		values++
	}
	if values != 2 || faults != 1 {
		t.Fatalf("values=%d faults=%d", values, faults)
	}
}
