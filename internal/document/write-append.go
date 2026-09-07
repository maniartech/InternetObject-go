package document

import (
	"time"
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

// AppendKey appends an object key, quoted only when it must be.
func AppendKey(dst []byte, key string) []byte { return appendObjectKey(dst, key) }
