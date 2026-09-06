// Example 07 — one request, several entity types.
//
// This is the thing Internet Object does that JSON has no answer for: a single
// document carrying employees, alerts and stats, each section bound to its OWN
// schema and validated as itself. A JSON equivalent needs an envelope object,
// and nothing checks that `alerts` really holds alerts.
//
// The receiver takes the parts it wants, typed, with io.SectionAs[T].
package main

import (
	"fmt"

	io "github.com/maniartech/InternetObject-go"
)

// One request from a service to a dashboard.
const payload = `~ $Employee: {name: string, age: {int, min: 0}}
~ $Alert:    {level: {string, choices: [info, warn, error]}, msg: string}
~ $Stat:     {key: string, value: number}
--- employees: $Employee
~ Alice, 30
~ Bob, 41
--- alerts: $Alert
~ warn, disk nearly full
~ error, replica lagging
--- stats: $Stat
~ uptime, 99.9
~ rps, 1420.5
`

type Employee struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
}

type Alert struct {
	Level string `io:"level"`
	Msg   string `io:"msg"`
}

type Stat struct {
	Key   string  `io:"key"`
	Value float64 `io:"value"`
}

func main() {
	doc, err := io.Parse(payload)
	if err != nil {
		panic(err)
	}

	// What arrived, without knowing the shape in advance.
	fmt.Println("sections received:")
	for _, s := range doc.Sections() {
		fmt.Printf("  %-10s schema %-10s %d records\n", s.Name(), "$"+s.SchemaName(), s.Len())
	}

	// Take each entity type, typed. Each was validated against its own schema
	// during Parse — an alert with level "critical" would have failed there,
	// not here.
	employees, err := io.SectionAs[Employee](doc, "employees")
	if err != nil {
		panic(err)
	}
	alerts, err := io.SectionAs[Alert](doc, "alerts")
	if err != nil {
		panic(err)
	}
	stats, err := io.SectionAs[Stat](doc, "stats")
	if err != nil {
		panic(err)
	}

	fmt.Printf("\n%d employees: %v\n", len(employees), employees)
	fmt.Printf("%d alerts:    %v\n", len(alerts), alerts)
	fmt.Printf("%d stats:     %v\n", len(stats), stats)

	// A section the sender did not send is an ERROR, not an empty slice:
	// "not sent" and "sent none" are different facts.
	if _, err := io.SectionAs[Stat](doc, "incidents"); err != nil {
		fmt.Printf("\nasking for a section that is not there:\n  %v\n", err)
	}

	// And the mistake this API exists to prevent: a multi-section document
	// cannot be flattened into one slice by accident.
	var wrong []Employee
	if err := io.Unmarshal(payload, &wrong); err != nil {
		fmt.Printf("\nflattening a multi-section document is refused:\n  %v\n", err)
	}
}
