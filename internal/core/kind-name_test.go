package core

import (
	"math/big"
	"testing"
	"time"
)

func TestKindName(t *testing.T) {
	for want, v := range map[string]any{
		"null": nil, "string": "a", "bool": true, "number": 1.5,
		"bigint": big.NewInt(1), "decimal": Decimal{}, "datetime": time.Time{},
		"binary": []byte{1}, "array": []any{}, "object": &Object{}, "int": 3,
	} {
		if got := KindName(v); got != want {
			t.Errorf("KindName(%#v) = %q, want %q", v, got, want)
		}
	}
}
