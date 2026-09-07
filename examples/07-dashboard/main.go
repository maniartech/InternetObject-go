// Example 07 — one request, several entity types.
//
// This is the thing Internet Object does that JSON has no answer for: a single
// document carrying employees, alerts and stats, each section bound to its OWN
// schema and validated as itself. A JSON equivalent needs an envelope object,
// and nothing checks that `alerts` really holds alerts.
package main

import (
	"fmt"

	io "github.com/maniartech/InternetObject-go"
)

// One request from a service to a dashboard.
const payload = `~ $employee: {name: string, age: {int, min: 0, max: 130}}
~ $alert:    {level: {string, choices: [info, warn, error]}, msg: string}
~ $stat:     {key: string, value: number}
--- employees: $employee
~ Alice, 30
~ Bob, 41
--- alerts: $alert
~ warn, disk nearly full
~ error, replica lagging
--- stats: $stat
~ uptime, 99.9
~ rps, 1420.5
`

type Employee struct {
	Name string `io:"name" schema:"{string, minLen: 2}"`
	Age  int    `io:"age"  schema:"{int, min: 0, max: 130}"`
}

type Alert struct {
	Level string `io:"level"`
	Msg   string `io:"msg"`
}

type Stat struct {
	Key   string  `io:"key"`
	Value float64 `io:"value"`
}

// Dashboard is the WHOLE document: one field per section, named by its io tag.
type Dashboard struct {
	Employees []Employee `io:"employees"`
	Alerts    []Alert    `io:"alerts"`
	Stats     []Stat     `io:"stats"`
}

func main() {
	// ── The no-ceremony path ────────────────────────────────────────────────
	// The document binds straight into the struct. Every section was validated
	// against its own schema on the way in, so a bad row never reaches here.
	var d Dashboard
	if err := io.Unmarshal(payload, &d); err != nil {
		panic(err)
	}
	fmt.Printf("employees %v\nalerts    %v\nstats     %v\n", d.Employees, d.Alerts, d.Stats)

	// Validation is native to the struct too — the `schema` tags above.
	fmt.Printf("\nValidate(dashboard)          -> %v\n", io.Validate(d))
	bad := Dashboard{Employees: []Employee{{Name: "X", Age: -5}}}
	fmt.Printf("Validate(negative age, X)     -> %v\n", io.Validate(bad))

	// ── Or take one entity type at a time ───────────────────────────────────
	doc, err := io.Parse(payload)
	if err != nil {
		panic(err)
	}
	fmt.Println("\nsections received:")
	for _, s := range doc.Sections() {
		fmt.Printf("  %-10s schema $%-9s %d records\n", s.Name(), s.SchemaName(), s.Len())
	}

	employees, err := io.SectionAs[Employee](doc, "employees")
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nSectionAs[Employee] -> %v\n", employees)

	// A section the sender did not send is an ERROR, not an empty slice:
	// "not sent" and "sent none" are different facts.
	if _, err := io.SectionAs[Stat](doc, "incidents"); err != nil {
		fmt.Printf("\nasking for a section that is not there:\n  %v\n", err)
	}

	// And the mistake this API exists to prevent: a multi-section document
	// cannot be flattened into one SLICE by accident. Bind a struct instead.
	var wrong []Employee
	if err := io.Unmarshal(payload, &wrong); err != nil {
		fmt.Printf("\nflattening a multi-section document is refused:\n  %v\n", err)
	}
}
