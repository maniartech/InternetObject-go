// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

// Object is an ordered collection of members. Order is preserved end to end;
// comparison ignores it (see Equal), serialization does not.
type Object struct {
	Members []Member
	// Line, Col locate the object's first token, 1-based. An absence fault
	// (a member that is missing entirely) has no value to point at and is
	// reported here instead — the reference does the same (ADR 0005 D2).
	Line, Col int32
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
