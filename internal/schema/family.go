package schema

import (
	"math/big"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/errs"
)

// The registered type names. A name outside this set is unknown-type — unless
// the spec reserves it for a future version, which is reserved-type: a
// format-level rejection, not a typo. (uint64/float32/float64 are registered
// and compile; their rejection happens at validation. int64 is not registered
// at all, so it reports reserved-type here.)
var registeredTypes = map[string]bool{
	"string": true, "email": true, "url": true,
	"number": true, "int": true, "uint": true, "float": true,
	"int8": true, "int16": true, "int32": true,
	"uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
	"bigint": true, "decimal": true,
	"bool":     true,
	"datetime": true, "date": true, "time": true,
	"any": true, "array": true, "object": true,
}

var reservedTypes = map[string]bool{
	"int64": true, "uint64": true, "float32": true, "float64": true,
}

// unusableTypeCode names why a type name cannot be used: reserved for a
// future version, or no such type at all.
func unusableTypeCode(name string) errs.Code {
	if reservedTypes[name] {
		return errs.ReservedType
	}
	return errs.UnknownType
}

// The type families, shared by compile-time constraint checks and validation.
type family uint8

const (
	famAny family = iota
	famString
	famNumber
	famBigInt
	famDecimal
	famBool
	famTemporal
	famArray
	famObject
)

func familyOf(typeName string) family {
	switch typeName {
	case "any":
		return famAny
	case "string", "email", "url":
		return famString
	case "bigint":
		return famBigInt
	case "decimal":
		return famDecimal
	case "bool":
		return famBool
	case "datetime", "date", "time":
		return famTemporal
	case "array":
		return famArray
	case "object":
		return famObject
	}
	return famNumber
}

// ValueFitsType reports whether v is a legal value for the named type. It is
// the same family rule the compiler enforces, exported so the WRITER can ask
// before it substitutes something that would not compile back.
func ValueFitsType(typeName string, v any) (ok bool) {
	ok = true
	expectFamily(typeName, v, func(_ errs.Code, pass bool) {
		if !pass {
			ok = false
		}
	})
	return ok
}

// expectFamily checks one value against a type's family — the shared rule
// behind min/max/multipleOf/default and every element of choices.
func expectFamily(typeName string, v any, expect func(errs.Code, bool)) {
	{
		switch familyOf(typeName) {
		case famString:
			_, ok := v.(string)
			expect(errs.ExpectedString, ok)
		case famBigInt:
			_, ok := v.(*big.Int)
			expect(errs.ExpectedBigInt, ok)
		case famDecimal:
			_, ok := v.(core.Decimal)
			expect(errs.ExpectedDecimal, ok)
		case famTemporal:
			_, ok := v.(time.Time)
			expect(errs.ExpectedDateTime, ok)
		case famBool:
			_, ok := v.(bool)
			expect(errs.ExpectedBoolean, ok)
		case famArray:
			_, ok := v.([]any)
			expect(errs.ExpectedArray, ok)
		case famObject:
			_, ok := v.(*core.Object)
			expect(errs.InvalidObject, ok)
		case famNumber:
			_, ok := v.(float64)
			expect(errs.ExpectedNumber, ok)
		}
	}
}
