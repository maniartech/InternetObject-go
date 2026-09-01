package value

import (
	"bytes"
	"math"
	"math/big"
	"sort"
)

// Equal implements the corpus's structural equality (CONFORMANCE §4, reduced
// to the reference runner's neutral spelling):
//
//   - objects compare by KEY SET, order ignored — member order in a projected
//     value is a host-language artifact, and where order is semantic the
//     serializer suite asserts it as text;
//   - temporals compare as instants (millisecond precision); the kind is not
//     part of this comparison, matching the runner's ISO-string reduction;
//   - NaN equals NaN (the comparison is structural, not IEEE);
//   - bigints, decimals, numbers and strings are distinct types — 1, 1n and
//     "1" are three different values.
//
// It expects PROJECTED values: every object member keyed.
func Equal(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(x) && math.IsNaN(y) {
			return true
		}
		return x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case *big.Int:
		y, ok := b.(*big.Int)
		return ok && x.Cmp(y) == 0
	case Decimal:
		y, ok := b.(Decimal)
		return ok && x.Scale == y.Scale && x.Coef.Cmp(y.Coef) == 0
	case []byte:
		y, ok := b.([]byte)
		return ok && bytes.Equal(x, y)
	case Temporal:
		y, ok := b.(Temporal)
		return ok && x.T.UnixMilli() == y.T.UnixMilli()
	case ErrorNode:
		y, ok := b.(ErrorNode)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !Equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case *Object:
		y, ok := b.(*Object)
		if !ok || len(x.Members) != len(y.Members) {
			return false
		}
		xk, yk := sortedKeys(x), sortedKeys(y)
		for i := range xk {
			if xk[i] != yk[i] {
				return false
			}
			if !Equal(x.Members[memberIndex(x, xk[i])].Value, y.Members[memberIndex(y, yk[i])].Value) {
				return false
			}
		}
		return true
	}
	return false
}

func sortedKeys(o *Object) []string {
	keys := make([]string, len(o.Members))
	for i := range o.Members {
		keys[i] = o.Members[i].Key
	}
	sort.Strings(keys)
	return keys
}

func memberIndex(o *Object, key string) int {
	for i := range o.Members {
		if o.Members[i].Key == key {
			return i
		}
	}
	return -1
}
