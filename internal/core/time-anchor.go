// Package core holds the format's own value model: the types every stage of
// the pipeline passes to the next.
//
// One type per file, deliberately. These are the nouns of the format — an
// Object is the item in a COLLECTION, a Member is one of its members — and a
// reader looking for one should not have to scroll past four others.
package core

import (
	"time"
)

// TimeAnchor is the date a time-of-day carries: the format has no bare clock,
// so `t"14:30"` is this date at that clock — the reference's convention, and
// what makes two implementations agree on the instant.
var TimeAnchor = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
