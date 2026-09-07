// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

// Member is one object member. A positional member has no key of its own —
// its identity is its index.
type Member struct {
	Key        string
	Quoted     bool // the key (or positional string value) was written quoted
	Positional bool // no key was written
	Absent     bool // an empty comma slot: a positional hole with no value
	Value      any
	// Line, Col locate this member's VALUE, 1-based — where a validation
	// fault about it is reported. Zero when the member was not parsed from
	// source (built by a marshaler, or an empty comma slot).
	Line, Col int32
}
