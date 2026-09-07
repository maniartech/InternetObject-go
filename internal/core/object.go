// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

import "iter"

// Object is an ordered collection of members. Order is preserved end to end;
// comparison ignores it (see Equal), serialization does not.
//
// Members is exported because the pipeline walks it directly and must not pay
// for accessors, but a CALLER should reach for the methods: they are the only
// thing that keeps keyed and positional members straight, and getting that
// wrong is the easiest mistake this type allows.
//
// Lookup is a LINEAR SCAN, deliberately. The reference carries a key→index map
// on every object; here that would be one map allocation per record parsed, on
// the hottest path there is, to speed up a scan over the handful of members a
// record actually has. Measured on this port's own corpus the scan wins
// outright. An object with hundreds of members would invert that — index it
// yourself if you build one.
type Object struct {
	Members []Member
	// Line, Col locate the object's first token, 1-based. An absence fault
	// (a member that is missing entirely) has no value to point at and is
	// reported here instead — the reference does the same (ADR 0005 D2).
	Line, Col int32
}

// NewObject returns an empty object, sized for cap members.
func NewObject(cap int) *Object {
	if cap <= 0 {
		return &Object{}
	}
	return &Object{Members: make([]Member, 0, cap)}
}

// Find returns the index of the first keyed member with the given key, or -1.
//
// POSITIONAL MEMBERS ARE NOT CANDIDATES: `~ Alice, 30` has no keys at all, and
// a lookup for "name" must not match the first value just because a schema
// would have named it that. Bind through a schema if you want that.
func (o *Object) Find(key string) int {
	for i := range o.Members {
		if !o.Members[i].Positional && o.Members[i].Key == key {
			return i
		}
	}
	return -1
}

// Len is the number of members, positional and absent slots included.
func (o *Object) Len() int { return len(o.Members) }

// Get returns the value of a keyed member. The second result reports whether
// the key was present, which is what distinguishes an absent member from one
// explicitly written null.
func (o *Object) Get(key string) (any, bool) {
	if i := o.Find(key); i >= 0 {
		return o.Members[i].Value, true
	}
	return nil, false
}

// Has reports whether a keyed member exists.
func (o *Object) Has(key string) bool { return o.Find(key) >= 0 }

// At returns the value of the member at index i, by POSITION — the way a
// positional record is read. Out of range is reported, never panicked.
func (o *Object) At(i int) (any, bool) {
	if i < 0 || i >= len(o.Members) {
		return nil, false
	}
	return o.Members[i].Value, true
}

// KeyAt returns the key of the member at index i, or "" when that member is
// positional, absent, or the index is out of range.
func (o *Object) KeyAt(i int) string {
	if i < 0 || i >= len(o.Members) || o.Members[i].Positional {
		return ""
	}
	return o.Members[i].Key
}

// Set assigns a keyed member, IN PLACE if the key is already there and
// appended otherwise, so a document's member order survives an edit. It
// returns the object so assignments can be chained.
func (o *Object) Set(key string, v any) *Object {
	if i := o.Find(key); i >= 0 {
		o.Members[i].Value = v
		o.Members[i].Absent = false
		return o
	}
	o.Members = append(o.Members, Member{Key: key, Value: v})
	return o
}

// SetAt replaces the value at index i, leaving its key and position alone, and
// reports whether the index existed.
func (o *Object) SetAt(i int, v any) bool {
	if i < 0 || i >= len(o.Members) {
		return false
	}
	o.Members[i].Value = v
	o.Members[i].Absent = false
	return true
}

// Append adds a keyed member WITHOUT checking for a duplicate, which is what
// makes it O(1) — use Set unless you have already established the key is new.
// The format forbids duplicate keys, so an object built this way will be
// rejected as `duplicate-member` when written and read back.
func (o *Object) Append(key string, v any) *Object {
	o.Members = append(o.Members, Member{Key: key, Value: v})
	return o
}

// AppendValue adds a POSITIONAL member: a value with no key of its own, whose
// identity is its index. This is what `~ Alice, 30` is made of.
func (o *Object) AppendValue(v any) *Object {
	o.Members = append(o.Members, Member{Positional: true, Value: v})
	return o
}

// Delete removes the first keyed member with this key and reports whether
// there was one. Later members shift down, so positional indices change.
func (o *Object) Delete(key string) bool {
	i := o.Find(key)
	if i < 0 {
		return false
	}
	return o.DeleteAt(i)
}

// DeleteAt removes the member at index i and reports whether it existed.
func (o *Object) DeleteAt(i int) bool {
	if i < 0 || i >= len(o.Members) {
		return false
	}
	o.Members = append(o.Members[:i], o.Members[i+1:]...)
	return true
}

// Keys returns the keys of the keyed members, in order. Positional members
// contribute nothing, so len(Keys()) can be less than Len().
func (o *Object) Keys() []string {
	out := make([]string, 0, len(o.Members))
	for i := range o.Members {
		if !o.Members[i].Positional {
			out = append(out, o.Members[i].Key)
		}
	}
	return out
}

// All iterates every member in order, yielding "" as the key of a positional
// one. Use it with range-over-func:
//
//	for key, val := range obj.All() { ... }
func (o *Object) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for i := range o.Members {
			key := ""
			if !o.Members[i].Positional {
				key = o.Members[i].Key
			}
			if !yield(key, o.Members[i].Value) {
				return
			}
		}
	}
}

// Clone returns an object with its own member slice, so adding or removing
// members cannot affect the original. Member VALUES are shared: a nested
// *Object reached through either one is the same object.
func (o *Object) Clone() *Object {
	c := &Object{Line: o.Line, Col: o.Col}
	if o.Members != nil {
		c.Members = make([]Member, len(o.Members))
		copy(c.Members, o.Members)
	}
	return c
}
