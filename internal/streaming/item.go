// Package streaming implements the record protocol over an incremental byte
// stream: `~` introduces each logical record, the FIRST `---` terminates the
// header, and later `---` frames switch the schema context.
//
// Chunk boundaries are never semantic - the same input split any way yields an
// identical item sequence, because the framer keeps its scan state across feeds
// and every frame goes through the same path a one-record document uses.
package streaming

// Item is one emitted stream item.
type Item struct {
	Kind        string // "record" or "record-error"
	RecordIndex int
	SchemaName  string // with the $ sigil, only when an explicit selector applied
	Value       any    // present on "record"
	Err         *ItemError
}
