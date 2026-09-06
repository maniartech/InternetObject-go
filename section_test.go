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
