// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

// ErrorNode marks a record that failed to parse or validate inside a
// collection: the fault was reported and the surrounding records survived.
type ErrorNode struct {
	Code        Code
	Category    string
	Path        string
	RecordIndex int
	Line, Col   int32
}
