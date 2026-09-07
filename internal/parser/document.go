package parser

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Document is one parsed Internet Object document.
//
// A document is an error COLLECTOR as much as a value: parsing accumulates and
// continues, so a document routinely holds valid records BESIDE the faults that
// spoiled the others. Errors is every fault in document order; SecErrors says
// which section each one came from.
type Document struct {
	Header   *Header
	Sections []*Section
	Errors   []errs.Error

	// SecErrors attributes faults to the section they came from.
	//
	// The flat Errors list cannot do that job alone: two sections both report
	// `$[1]` for their second record, so nothing in the error itself
	// distinguishes them. Attribution is recorded HERE, at the moment the fault
	// is raised, rather than re-derived afterwards from the error markers left
	// in the records - which would count every validation fault twice, once as
	// the error and once as its marker.
	SecErrors map[*Section][]errs.Error
}

// AddSectionError records a fault against BOTH the flat list and its section,
// so the two can never disagree about what went wrong or where.
func (d *Document) AddSectionError(sec *Section, es ...errs.Error) {
	if len(es) == 0 {
		return
	}
	d.Errors = append(d.Errors, es...)
	if sec == nil {
		return
	}
	if d.SecErrors == nil {
		d.SecErrors = map[*Section][]errs.Error{}
	}
	d.SecErrors[sec] = append(d.SecErrors[sec], es...)
}
