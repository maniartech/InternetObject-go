package parser

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// Document is one parsed Internet Object document.
type Document struct {
	Header   *Header
	Sections []*Section
	Errors   []errs.Error
}
