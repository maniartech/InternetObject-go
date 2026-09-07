// Package streaming implements the record protocol over an incremental byte
// stream: `~` introduces each logical record, the FIRST `---` terminates the
// header, and later `---` frames switch the schema context.
//
// Chunk boundaries are never semantic - the same input split any way yields an
// identical item sequence, because the framer keeps its scan state across feeds
// and every frame goes through the same path a one-record document uses.
package streaming

import "github.com/maniartech/InternetObject-go/internal/core"

// ItemError is a record error's wire form.
type ItemError struct {
	Category string // syntax | validation | general | stream
	Code     core.Code
}
