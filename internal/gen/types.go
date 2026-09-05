package gen

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/schema"
)

// goType maps one compiled member definition onto the Go type a generated
// field takes.
//
// The mapping is the ONLY place this tool decides anything about the format,
// and it decides nothing the engine does not already: every name here is a
// registered type name from internal/schema, and the Go type is the one the
// existing binder already reads and writes for it. A type this function
// declines never reaches the template, so the generator refuses a schema
// rather than emitting code that binds it wrongly.
func goType(md *schema.MemberDef) (string, error) {
	if md.SchemaRef != "" {
		return "", fmt.Errorf("a $-reference (%s) is not generated yet", md.SchemaRef)
	}
	if md.AnyOf != nil {
		return "", fmt.Errorf("anyOf is not generated yet")
	}

	var base string
	switch md.Type {
	case "string", "email", "url":
		base = "string"
	case "bool":
		base = "bool"
	case "number", "float":
		base = "float64"
	case "int":
		base = "int"
	case "int8", "int16", "int32":
		base = md.Type
	case "uint", "uint8", "uint16", "uint32":
		base = md.Type
	case "bigint":
		base = "*big.Int"
	case "decimal":
		base = "io.Decimal"
	case "date", "time", "datetime":
		base = "time.Time"
	case "any":
		base = "any"
	case "array":
		if md.Of == nil {
			return "", fmt.Errorf("an array with no element type is not generated yet")
		}
		elem, err := goType(md.Of)
		if err != nil {
			return "", fmt.Errorf("array element: %w", err)
		}
		if strings.HasPrefix(elem, "*") && md.Of.Type != "bigint" {
			return "", fmt.Errorf("an array of optional elements is not generated yet")
		}
		// A slice is already nil-able, so an optional/null array does not also
		// become a pointer — `[]T` covers both absent and null.
		return "[]" + elem, nil
	case "object":
		return "", fmt.Errorf("a nested object schema is not generated yet")
	default:
		// Includes the reserved names (int64, uint64, float32, float64), which
		// the compiler itself rejects, so reaching here means a new registered
		// type has been added without teaching this table about it.
		return "", fmt.Errorf("type %q has no Go mapping", md.Type)
	}

	// Optional or null becomes a pointer: it is how the engine already spells
	// "absent or null" for a struct field, and it is what lets a generated
	// getter report the difference instead of flattening it to a zero value.
	if (md.Optional || md.Null) && base != "any" && !strings.HasPrefix(base, "*") {
		base = "*" + base
	}
	return base, nil
}

// imports reports the import paths a generated file needs for these fields.
func imports(fields []field) []string {
	need := map[string]bool{}
	for _, f := range fields {
		switch {
		case strings.Contains(f.GoType, "time.Time"):
			need["time"] = true
		case strings.Contains(f.GoType, "big.Int"):
			need["math/big"] = true
		}
	}
	out := make([]string, 0, len(need))
	for _, p := range []string{"math/big", "time"} { // deterministic order
		if need[p] {
			out = append(out, p)
		}
	}
	return out
}
