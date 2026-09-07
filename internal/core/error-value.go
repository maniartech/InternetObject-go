// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

// ErrorValue is a malformed VALUE literal (a bad datetime, bigint, decimal or
// number) whose error is deferred rather than fatal: parsing continues, and
// the fault surfaces either as the recorded code (no schema) or as the typed
// member's own expected-* code (a schema masks it — reference behavior,
// io-test-cases ISSUE-23).
type ErrorValue struct {
	Code string
	Line int32
	Col  int32
}
