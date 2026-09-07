// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

import (
	"math/big"
	"time"
)

// IsScalar is THE record-versus-value decision, made once. It lists the value
// types explicitly and lets "record" be what is left, so the next value type
// added to the format extends this list instead of being silently walked as a
// record (the highest-yield trap in PORTING-NOTES).
func IsScalar(v any) bool {
	switch v.(type) {
	case nil, bool, float64, string, *big.Int, Decimal, []byte, time.Time:
		return true
	}
	return false
}
