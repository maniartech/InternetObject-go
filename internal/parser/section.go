package parser

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Section is one data section.
type Section struct {
	Name       string // "" when unnamed
	SchemaName string // explicit `$ref` binding (sigil stripped), "" when none
	Collection bool   // the body is a `~`-collection
	Records    []any  // record values; a non-collection section has at most one
}

// renameDuplicateSections reports duplicate-section-name for every repeat and
// renames it by appending _2, _3, … to the ORIGINAL name, counting names
// already taken, per name and not per document (CONFORMANCE §8).
func (p *parser) renameDuplicateSections() {
	if len(p.doc.Sections) < 2 {
		return
	}
	seen := map[string]bool{}
	for _, sec := range p.doc.Sections {
		if !seen[sec.Name] {
			seen[sec.Name] = true
			continue
		}
		p.doc.Errors = append(p.doc.Errors, errs.Error{Code: errs.DuplicateSectionName, Line: 1, Col: 1})
		for n := 2; ; n++ {
			candidate := fmt.Sprintf("%s_%d", sec.Name, n)
			if !seen[candidate] {
				sec.Name = candidate
				seen[candidate] = true
				break
			}
		}
	}
}
