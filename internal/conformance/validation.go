package conformance

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The validation comparator: a case carries a schema fragment and a data
// fragment, composed into one document exactly as the reference runner does —
// the schema as the reserved default `$schema`, the data as the body.
func RunValidationCase(row SuiteRow) []string {
	src := "~ $schema: { " + row.Schema + " }\n---\n" + row.Input + "\n"
	doc := document.Load(src)
	var codes []string
	for _, e := range doc.Errors {
		codes = append(codes, e.Code)
	}

	var problems []string
	if !stringsEqual(codes, row.ErrorCodes) {
		problems = append(problems, fmt.Sprintf("codes  expected=%v  actual=%v", row.ErrorCodes, codes))
	}
	if len(row.ErrorCodes) == 0 {
		var actual any
		if len(codes) == 0 {
			actual = doc.Project()
		}
		if !value.Equal(actual, row.Expected) {
			problems = append(problems, fmt.Sprintf("value  expected=%s  actual=%s", Show(row.Expected), Show(actual)))
		}
	}
	return problems
}
