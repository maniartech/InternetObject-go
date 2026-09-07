package document

import (
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// The exported spellers, so the fast marshal path can emit one value without
// building a document around it.

func AppendString(dst []byte, s string) []byte { return appendAutoString(dst, s) }

// AppendNumber appends a float64 in IO spelling.
func AppendNumber(dst []byte, f float64) []byte { return appendIONumber(dst, f) }

// AppendTemporalValue appends a temporal under the declared kind ("" to infer).
func AppendTemporalValue(dst []byte, t time.Time, declared string) []byte {
	return appendTemporal(dst, t, declared)
}

// AppendRecord renders ONE record the way a document section would, so a
// stream writer frames records with the same code the document writer uses
// rather than a second, drifting copy of it.
//
// A nil schema writes the record's own keys; a schema writes it positionally,
// which is what makes a streamed record readable against the header the stream
// declared.
func AppendRecord(dst []byte, obj *core.Object, sch *schema.Schema) []byte {
	d := &Doc{Document: &parser.Document{}, soloSchema: sch}
	return d.appendRecord(dst, obj, sch)
}
