package conformance

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The roundtrip comparator: three properties per case, each catching what the
// others cannot — canonical OUTPUT (two conforming writers must agree on
// spelling), VALUE preservation (a writer that loses data raises no error),
// and IDEMPOTENCE (text that drifts on every rewrite).
func RunRoundtripCase(row SuiteRow) []string {
	var problems []string

	doc := document.Parse(row.Input)
	if len(doc.Errors) > 0 {
		problems = append(problems, fmt.Sprintf("input does not parse: %v", doc.Errors))
		return problems
	}

	produced := doc.String()
	expected, _ := row.Output.(string)
	if produced != expected {
		problems = append(problems, fmt.Sprintf("output\n     expected=%q\n     actual  =%q", expected, produced))
	}

	back := document.Parse(produced)
	if len(back.Errors) > 0 {
		problems = append(problems, fmt.Sprintf("output does not re-parse: %v", back.Errors))
		return problems
	}
	if !value.Equal(doc.Project(), back.Project()) {
		problems = append(problems, fmt.Sprintf("value changed\n     in =%s\n     out=%s",
			Show(doc.Project()), Show(back.Project())))
	} else if again := back.String(); again != produced {
		problems = append(problems, fmt.Sprintf("not idempotent:\n     first =%q\n     second=%q", produced, again))
	}
	return problems
}
