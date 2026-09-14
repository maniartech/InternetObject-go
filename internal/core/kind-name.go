package core

import (
	"fmt"
	"math/big"
	"time"
)

// KindName names the kind of a value-model value the way the format does —
// `object`, `number`, `datetime` — for messages a user reads. The Go type
// behind a value is representation: a message saying `*core.Object` names a
// package the user cannot import.
func KindName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case float64:
		return "number"
	case *big.Int:
		return "bigint"
	case Decimal:
		return "decimal"
	case time.Time:
		return "datetime"
	case []byte:
		return "binary"
	case []any:
		return "array"
	case *Object:
		return "object"
	}
	return fmt.Sprintf("%T", v) // not a value-model value; its Go type is all there is
}
