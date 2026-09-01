// Package value is the Internet Object value model: the concrete Go types a
// parsed document decodes into, the ONE predicate that separates value types
// from records, and the corpus's structural equality.
//
// The closed set of value types:
//
//	nil        null
//	bool       boolean
//	float64    number (including NaN and ±Inf)
//	*big.Int   bigint — every digit past 2^53 survives
//	Decimal    decimal — scale is part of the value (1.50m ≠ 1.5m)
//	string     string
//	[]byte     binary
//	Temporal   date | time | datetime — the kind stays distinct
//	*Object    an ordered key/value record
//	[]any      an array
package value

import (
	"math/big"
	"time"
)

// Object is an ordered collection of members. Order is preserved end to end;
// comparison ignores it (see Equal), serialization does not.
type Object struct {
	Members []Member

	// EmptySlots counts commas that separated nothing (leading, doubled or
	// trailing) when this object was parsed. Data projection tolerates them;
	// schema compilation reports empty-memberdef when any are present.
	EmptySlots int
}

// Member is one object member. A positional member has no key of its own —
// its identity is its index.
type Member struct {
	Key        string
	Quoted     bool // the key was written quoted, so it is literal
	Positional bool // no key was written
	Value      any
}

// Find returns the index of the first keyed member with the given key, or -1.
func (o *Object) Find(key string) int {
	for i := range o.Members {
		if !o.Members[i].Positional && o.Members[i].Key == key {
			return i
		}
	}
	return -1
}

// Decimal is an exact fixed-point value: Coef × 10^-Scale. The scale is part
// of the value — 1.50m is coefficient 150, scale 2, and is distinct from
// 1.5m in serialized form.
type Decimal struct {
	Coef  *big.Int
	Scale int
}

// TemporalKind distinguishes the three temporal literal kinds, which stay
// distinct all the way out — a date is not a datetime with a zero time.
type TemporalKind uint8

const (
	KindDate TemporalKind = iota
	KindTime
	KindDateTime
)

// Temporal is a date, time or datetime value.
type Temporal struct {
	T    time.Time
	Kind TemporalKind
}

// ErrorNode marks a record that failed to parse or validate inside a
// collection: the fault was reported and the surrounding records survived.
type ErrorNode struct {
	Code string
}

// IsScalar is THE record-versus-value decision, made once. It lists the value
// types explicitly and lets "record" be what is left, so the next value type
// added to the format extends this list instead of being silently walked as a
// record (the highest-yield trap in PORTING-NOTES).
func IsScalar(v any) bool {
	switch v.(type) {
	case nil, bool, float64, string, *big.Int, Decimal, []byte, Temporal:
		return true
	}
	return false
}
