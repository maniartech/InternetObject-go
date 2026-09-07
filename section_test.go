package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

const dashboardDoc = `~ $Employee: {name: string, age: int}
~ $Alert: {level: string, msg: string}
--- employees: $Employee
~ Alice, 30
~ Bob, 41
--- alerts: $Alert
~ warn, disk full
`

type secEmployee struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
}
type secAlert struct {
	Level string `io:"level"`
	Msg   string `io:"msg"`
}

// A document carrying several entity types must not flatten into one slice.
//
// It used to, silently: []Employee came back with four rows, two of them alerts
// and stats bound as ZERO-VALUE employees, and a nil error. No corpus case can
// catch that — the corpus gates parsing, and binding into Go structs is this
// port's own business.
func TestMultiSectionDoesNotFlattenSilently(t *testing.T) {
	var rows []secEmployee
	err := io.Unmarshal(dashboardDoc, &rows)
	if err == nil {
		t.Fatalf("multi-section document flattened into %d rows with no error: %+v", len(rows), rows)
	}
	for _, want := range []string{"2 sections", "employees", "alerts", "SectionAs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got %q", want, err)
		}
	}
}

func TestSectionAccessors(t *testing.T) {
	doc, err := io.Parse(dashboardDoc)
	if err != nil {
		t.Fatal(err)
	}
	// Two: a header followed directly by `--- name:` creates no empty default
	// section.
	if secs := doc.Sections(); len(secs) != 2 {
		names := make([]string, len(secs))
		for i, s := range secs {
			names[i] = s.Name()
		}
		t.Fatalf("got %d sections %v, want 2", len(secs), names)
	}
	emp := doc.Section("employees")
	if emp == nil {
		t.Fatal("employees section missing")
	}
	if emp.SchemaName() != "Employee" {
		t.Errorf("schema name = %q, want Employee", emp.SchemaName())
	}
	if emp.Len() != 2 || !emp.IsCollection() {
		t.Errorf("employees: len=%d collection=%v", emp.Len(), emp.IsCollection())
	}
	if emp.Schema() == nil {
		t.Error("employees section reports no schema")
	}
	if doc.Section("nope") != nil {
		t.Error("Section() invented a section")
	}
}

func TestSectionAs(t *testing.T) {
	doc, err := io.Parse(dashboardDoc)
	if err != nil {
		t.Fatal(err)
	}
	emps, err := io.SectionAs[secEmployee](doc, "employees")
	if err != nil {
		t.Fatal(err)
	}
	if len(emps) != 2 || emps[0].Name != "Alice" || emps[1].Age != 41 {
		t.Fatalf("employees = %+v", emps)
	}
	alerts, err := io.SectionAs[secAlert](doc, "alerts")
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Level != "warn" {
		t.Fatalf("alerts = %+v", alerts)
	}

	// A section that was not sent is an error, not an empty slice: "not sent"
	// and "sent none" are different facts.
	if _, err := io.SectionAs[secAlert](doc, "incidents"); err == nil {
		t.Error("a missing section returned no error")
	}
}

// The single-section case must be untouched by the multi-section guard.
func TestSingleSectionStillUnmarshals(t *testing.T) {
	const one = "name: string, age: int\n---\n~ Alice, 30\n~ Bob, 41"
	var rows []secEmployee
	if err := io.Unmarshal(one, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "Alice" {
		t.Fatalf("rows = %+v", rows)
	}
}

// The no-ceremony form: a struct whose io tags name sections binds the whole
// document, one field per section.
func TestDocumentStructBinding(t *testing.T) {
	type dash struct {
		Employees []secEmployee `io:"employees"`
		Alerts    []secAlert    `io:"alerts"`
	}
	var d dash
	if err := io.Unmarshal(dashboardDoc, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Employees) != 2 || d.Employees[0].Name != "Alice" {
		t.Errorf("employees = %+v", d.Employees)
	}
	if len(d.Alerts) != 1 || d.Alerts[0].Level != "warn" {
		t.Errorf("alerts = %+v", d.Alerts)
	}
}

// A section the struct does not name is ignored, as encoding/json ignores an
// unknown key; a field the document does not carry stays zero.
func TestDocumentStructPartial(t *testing.T) {
	type onlyAlerts struct {
		Alerts  []secAlert    `io:"alerts"`
		Missing []secEmployee `io:"nowhere"`
	}
	var d onlyAlerts
	if err := io.Unmarshal(dashboardDoc, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Alerts) != 1 {
		t.Errorf("alerts = %+v", d.Alerts)
	}
	if d.Missing != nil {
		t.Errorf("a field naming no section should stay zero, got %+v", d.Missing)
	}
}

// Section binding must not fire for a RECORD struct. The rule is that a tag
// has to name a section the document ACTUALLY HAS, so adding a field can never
// silently change what an existing struct means.
func TestRecordStructUnaffectedBySectionBinding(t *testing.T) {
	const one = "name: string, age: int\n---\nAlice, 30"
	var e secEmployee
	if err := io.Unmarshal(one, &e); err != nil {
		t.Fatal(err)
	}
	if e.Name != "Alice" || e.Age != 30 {
		t.Fatalf("record = %+v", e)
	}
}

// Validation is native to the document struct: the element types' `schema`
// tags are enforced by io.Validate.
func TestDocumentStructValidates(t *testing.T) {
	type tagged struct {
		Name string `io:"name" schema:"{string, minLen: 2}"`
	}
	type dash struct {
		Employees []tagged `io:"employees"`
	}
	if err := io.Validate(dash{Employees: []tagged{{Name: "Alice"}}}); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	err := io.Validate(dash{Employees: []tagged{{Name: "X"}}})
	if err == nil {
		t.Fatal("a member violating its schema tag was accepted")
	}
	if !strings.Contains(err.Error(), "mismatched-min-len") {
		t.Errorf("error = %v, want mismatched-min-len", err)
	}
}

// A Document is an ERROR COLLECTOR as much as a value: it holds the records
// that survived alongside markers for the ones that did not, and each section
// reports its own faults.
//
// The flat document list cannot do that job alone - two sections both report
// `$[1]` for their second record, so the section is the only thing that can say
// which is which. io-js2 draws the same line (src/core/section.ts: `errors`),
// and its own error objects carry no path at all, only a position.
func TestSectionCollectsItsOwnErrors(t *testing.T) {
	nlSec := string(rune(10))
	doc := "~ $Emp: {name: string, age: int}" + nlSec +
		"~ $Alert: {level: string}" + nlSec +
		"--- employees: $Emp" + nlSec +
		"~ Alice, 30" + nlSec +
		"~ Bob, oops" + nlSec +
		"--- alerts: $Alert" + nlSec +
		"~ warn" + nlSec +
		"~ 42"

	d, err := io.Parse(doc)
	if err == nil {
		t.Fatal("expected faults")
	}
	if got := len(d.Errors()); got != 2 {
		t.Fatalf("document should collect both faults, got %d", got)
	}

	for _, tc := range []struct {
		section string
		code    io.Code
	}{
		{"employees", "expected-integer"},
		{"alerts", "expected-string"},
	} {
		sec := d.Section(tc.section)
		if sec == nil {
			t.Fatalf("%s section missing", tc.section)
		}
		if !sec.HasErrors() {
			t.Errorf("%s: HasErrors() = false", tc.section)
		}
		es := sec.Errors()
		if len(es) != 1 {
			t.Fatalf("%s: got %d errors, want 1 (%v)", tc.section, len(es), es)
		}
		if es[0].Code != tc.code {
			t.Errorf("%s: code = %s, want %s", tc.section, es[0].Code, tc.code)
		}
		// The valid row is still there, next to the marker.
		if sec.Len() != 2 {
			t.Errorf("%s: lost a record, len = %d", tc.section, sec.Len())
		}
		if io.IsError(sec.Records()[0]) {
			t.Errorf("%s: the GOOD row was marked as an error", tc.section)
		}
		if !io.IsError(sec.Records()[1]) {
			t.Errorf("%s: the bad row carries no marker", tc.section)
		}
	}
}

// A clean section reports nothing, so len(Errors()) == 0 is the test for
// "this section loaded cleanly".
func TestCleanSectionHasNoErrors(t *testing.T) {
	d, err := io.Parse(dashboardDoc)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range d.Sections() {
		if s.HasErrors() || len(s.Errors()) != 0 {
			t.Errorf("clean section %q reports %v", s.Name(), s.Errors())
		}
	}
}

// Every route into a section's error list must attribute: parse recovery,
// variable resolution, schema validation and deferred literal faults alike.
// A fault counted twice is as wrong as one lost, so the totals are checked
// against the document's own flat list.
func TestEveryFaultRouteAttributesToItsSection(t *testing.T) {
	nl := string(rune(10))
	for _, tc := range []struct {
		name, src string
		code      io.Code
		section   string
	}{
		{
			name:    "parse recovery",
			src:     "--- a" + nl + "~ 1" + nl + "~ {q:" + nl + "--- b" + nl + "~ 2",
			code:    "expected-value",
			section: "a",
		},
		{
			name:    "deferred literal",
			src:     "--- a" + nl + "~ 1" + nl + "~ 0B" + nl + "--- b" + nl + "~ 2",
			code:    "invalid-number",
			section: "a",
		},
		{
			name:    "schema validation",
			src:     "~ $S: {n: int}" + nl + "--- a: $S" + nl + "~ 1" + nl + "~ x" + nl + "--- b: $S" + nl + "~ 2",
			code:    "expected-integer",
			section: "a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := io.Parse(tc.src)
			if err == nil {
				t.Fatal("expected a fault")
			}
			total := 0
			for _, s := range d.Sections() {
				es := s.Errors()
				total += len(es)
				if s.Name() != tc.section {
					if len(es) != 0 {
						t.Errorf("section %q should be clean, got %v", s.Name(), es)
					}
					continue
				}
				if len(es) != 1 || es[0].Code != tc.code {
					t.Fatalf("section %q: got %v, want one %s", s.Name(), es, tc.code)
				}
			}
			// Not double counted, and not lost: the section lists account for
			// exactly the document's faults.
			if total != len(d.Errors()) {
				t.Errorf("section totals = %d, document = %d", total, len(d.Errors()))
			}
		})
	}
}
