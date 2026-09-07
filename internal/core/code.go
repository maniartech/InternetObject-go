// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

// Code is a designated error code: stable, kebab-case, and the conformance
// contract. Messages are informational and are never asserted anywhere.
//
// It lives here, in the value model, because ErrorNode carries one — a failed
// record is a VALUE, so its code is part of the value model too. Keeping it
// here is also what lets the public package name the codes as constants
// without core learning anything about the packages above it.
//
// Codes follow the frozen <predicate>-<subject> grammar. Adding one is a
// change to the format, not an implementation decision.
type Code string

func (c Code) String() string { return string(c) }
