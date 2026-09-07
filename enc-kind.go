package internetobject

import (
	"reflect"
)

// encKind is a field's wire shape, decided ONCE when the plan is built. The
// encoder switches on it instead of comparing reflect.Types per value, which
// the CPU profile showed as runtime.ifaceeq plus reflect.Elem on every member
// of every record (ADR 0006 F1: compile the shape, then execute it).
type encKind uint8

const (
	encOther encKind = iota
	encString
	encBool
	encInt
	encUint
	encFloat
	encBigInt
	encDecimal
	encTime
	encBytes
	encSlice
)

// encKindOf classifies a type once, at plan-build time.
func encKindOf(t reflect.Type) encKind {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case bigIntElemType:
		return encBigInt
	case decimalType:
		return encDecimal
	case timeType:
		return encTime
	case bytesType:
		return encBytes
	}
	switch t.Kind() {
	case reflect.String:
		return encString
	case reflect.Bool:
		return encBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return encUint
	case reflect.Float32, reflect.Float64:
		return encFloat
	case reflect.Slice, reflect.Array:
		return encSlice
	}
	return encOther
}
