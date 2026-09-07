package schema

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/errs"
)

// ── a typedef is a record, and every type declares its schema ───────────────
//
// `{string, minLen: 2}` is not special syntax: it is a RECORD, and the type it
// must satisfy is the type's own memberdef schema — an ordered list of keys and
// the value each takes. Order is the contract behind the positional form, which
// is why `{bool, F}` works at all: position 0 binds to `type`, position 1 to
// `default`, exactly as `~ Alice, 30` binds to `{name, age}`.
//
// This table is transcribed from io-js2's per-type schemas
// (io-js2/src/schema/types/*.ts), where it is normative. io-go previously
// carried the same information as key SETS plus a hand-written per-family
// switch — which threw the ORDER away, and with it the positional form
// entirely, and left two copies of one decision to drift apart.
type typedefMember struct {
	name string
	// valueType is what the key's value must be:
	//   "self"    the family's own type — default, min, max, multipleOf
	//   "[self]"  an array of the family's own type — choices
	//   ""        a SHAPE this pass does not type-check — of, schema, anyOf
	//   else      a concrete type name
	valueType string
}

// typedefSchemas is the assembly point: each type declares its own schema in its
// own file, exactly as io-js2 does in src/schema/types/.
var typedefSchemas = map[family][]typedefMember{
	famString:   stringTypedef,
	famNumber:   numberTypedef,
	famBigInt:   bigintTypedef,
	famDecimal:  decimalTypedef,
	famBool:     boolTypedef,
	famTemporal: temporalTypedef,
	famArray:    arrayTypedef,
	famObject:   objectTypedef,
	famAny:      anyTypedef,
}

// typedefSchemaFor returns the memberdef schema a typedef of this type must
// satisfy.
func typedefSchemaFor(typeName string) []typedefMember {
	return typedefSchemas[familyOf(typeName)]
}

// typedefKey returns the member a key names, or false when the type does not
// declare it — which is unknown-member.
func typedefKey(typeName, key string) (typedefMember, bool) {
	for _, m := range typedefSchemaFor(typeName) {
		if m.name == key {
			return m, true
		}
	}
	return typedefMember{}, false
}

// constraintKeys lists the per-family constraint keys each type accepts
// beyond the universal ones (type, optional, null, default). A key outside
// the set is unknown-member. Derived from the reference implementation's
// per-type memberdef schemas.
var (
	stringKeys   = keySet("choices", "pattern", "flags", "len", "minLen", "maxLen", "format", "escapeLines", "encloser")
	numberKeys   = keySet("choices", "min", "max", "multipleOf", "format")
	decimalKeys  = keySet("choices", "min", "max", "multipleOf", "precision", "scale")
	datetimeKeys = keySet("choices", "min", "max")
	boolKeys     = keySet()
	objectKeys   = keySet("schema")
	arrayKeys    = keySet("of", "len", "minLen", "maxLen")
	anyKeys      = keySet("choices", "anyOf", "isSchema")
)

func keySet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

func allowedKeys(typeName string) map[string]bool {
	switch typeName {
	case "string", "email", "url":
		return stringKeys
	case "decimal":
		return decimalKeys
	case "datetime", "date", "time":
		return datetimeKeys
	case "bool":
		return boolKeys
	case "object":
		return objectKeys
	case "array":
		return arrayKeys
	case "any":
		return anyKeys
	default: // the number family, bigint included
		return numberKeys
	}
}

// checkConstraintValue type-checks one constraint's VALUE against what the
// member's type expects — the reference validates the object-form typedef
// against a per-type memberdef schema, so `{int, default: notanumber}` is
// expected-number and `{bigint, multipleOf: 5}` is expected-bigint, at
// compile. An @-reference is resolved later and skipped here.
func checkConstraintValue(typeName, key string, v any) {
	if s, ok := v.(string); ok && strings.HasPrefix(s, "@") {
		return
	}
	expect := func(code string, ok bool) {
		if !ok {
			fail(code)
		}
	}
	switch key {
	case "len", "minLen", "maxLen", "precision", "scale":
		_, ok := v.(float64)
		expect(errs.ExpectedNumber, ok)
	case "pattern", "flags", "format", "encloser":
		_, ok := v.(string)
		expect(errs.ExpectedString, ok)
	case "escapeLines":
		_, ok := v.(bool)
		expect(errs.ExpectedBoolean, ok)
	case "choices":
		// Every choice must be a value of the member's own type. The elements
		// used to go unchecked, so `{string, choices: [0B]}` compiled with a
		// malformed literal inside it and the writer then emitted an EMPTY
		// element — text its own reader rejects. The reference reports
		// expected-string here; found by fuzzing, oracle-confirmed 2026-09-06.
		list, ok := v.([]any)
		expect(errs.ExpectedArray, ok)
		for _, e := range list {
			if es, isStr := e.(string); isStr && strings.HasPrefix(es, "@") {
				continue // a variable reference, resolved later
			}
			expectFamily(typeName, e, expect)
		}
	case "min", "max", "multipleOf", "default":
		expectFamily(typeName, v, expect)
	}
}
