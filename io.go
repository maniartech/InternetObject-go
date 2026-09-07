// Package internetobject implements the Internet Object data-interchange
// format (https://internetobject.org): a schema-first, lean, human-readable
// successor to JSON.
//
// Parsing accumulates errors rather than failing fast — Parse returns the
// document AND an error: the error lists every fault (stable kebab-case
// codes with positions), while the document still holds every record that
// survived. Values decode to a precise model: numbers are float64, bigints
// *big.Int, decimals keep their scale (1.50m is not 1.5m), temporals keep
// their kind (date, time, datetime), binary is []byte.
//
// This implementation passes the complete shared conformance corpus
// (io-test-cases): tokenizer, parser, schema, validation, serializer,
// document, streaming and regression suites.
//
// # Vocabulary
//
// Every name in this package follows one rule, so the verb tells you what
// kind of thing you get back:
//
//	Marshal / Unmarshal   Go value  ⇄ IO text   (like encoding/json)
//	Parse    / String     IO text   ⇄ this package's own types,
//	                      Document and Schema   (like url.Parse / URL.String)
//	Stream                incremental reading, one record at a time
//	Validate              check a value; produce no text
//
// Two suffixes modify any of them without changing the verb:
//
//	…With(…, s *Schema)   the same operation against an explicitly supplied
//	                      schema instead of a derived or embedded one
//	…As[T](…)             the same operation producing Go values of type T
//
// So Parse gives you a *Document to navigate, Unmarshal fills your struct,
// and ParseWith / UnmarshalWith are those two against a runtime schema. There
// is no Load, Read, Decode or Write in the public surface: one verb per
// direction, everywhere.
package internetobject

import (
	"github.com/maniartech/InternetObject-go/internal/core"
)

// The live value model, shared with the pipeline by construction.
type (
	// Object is an ordered key/value record.
	Object = core.Object
	// Member is one member of an Object.
	Member = core.Member
	// ErrorItem stands in for a record that failed, INSIDE projected data:
	// the row keeps its position and carries this marker instead of a value,
	// so the good records around it are untouched. Test for it with IsError.
	ErrorItem = core.ErrorNode
)

// NewObject returns an empty ordered record, sized for cap members. It is the
// starting point for building a document by hand rather than from a Go struct.
func NewObject(cap int) *Object { return core.NewObject(cap) }

// TimeAnchor is the date a time-of-day carries. The format has no bare clock
// type, so `t"14:30"` is this date at that clock — the reference's convention,
// and what makes two implementations agree on the instant.
var TimeAnchor = core.TimeAnchor

// IsError reports whether a projected value is a failed record.
//
// This is a TYPE check, never a property check: a schema may legitimately
// declare a member called "__error" or "code", so data must never be able to
// impersonate a failure (ADR 0005 D4).
func IsError(v any) bool {
	_, ok := v.(ErrorItem)
	return ok
}
