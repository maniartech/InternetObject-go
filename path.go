package internetobject

import (
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// pathAt names a position in the value being encoded or decoded WITHOUT
// building the string. Errors are rare and paths are only for errors, so
// every part travels separately and they are joined exactly once, in
// String(), when a fault is actually reported.
//
// This shape was arrived at by measurement, and the two failures are worth
// recording: a linked list of parent POINTERS escapes (+6,000 allocations on
// encode), and joining lazily at each level turns one join per record into
// one per member (+9,500 on decode). Carrying the four parts flat is what
// finally cost nothing — before it, path building was 99.4% of encode's
// remaining allocations, all of it discarded (ADR 0006 P1).
type pathAt struct {
	root  string // the enclosing path, "$" at the top
	rec   int    // record index within a collection, -1 when not in one
	name  string // member name, "" when none
	index int    // element index within the member, -1 when not an element
}

var (
	bigIntType = reflect.TypeOf((*big.Int)(nil))
	// bigIntElemType is big.Int itself, hoisted out of the hot path: calling
	// bigIntType.Elem() per value showed up in the profile.
	bigIntElemType = reflect.TypeOf(big.Int{})
	decimalType    = reflect.TypeOf(Decimal{})
	timeType       = reflect.TypeOf(time.Time{})
	bytesType      = reflect.TypeOf([]byte(nil))
	objectType     = reflect.TypeOf(Object{})
	anyType        = reflect.TypeOf((*any)(nil)).Elem()
)

// rootPath is the document root: the parent of every top-level record.
var rootPath = pathAt{root: "$", rec: -1, index: -1}

// record names the i-th record of a collection.
func (p pathAt) record(i int) pathAt { p.rec = i; return p }

// member names a member inside p.
func (p pathAt) member(name string) pathAt { p.name = name; p.index = -1; return p }

// elem names the i-th element of p's member.
func (p pathAt) elem(i int) pathAt { p.index = i; return p }

// deeper descends past what the flat form can express — a nested object or
// map — by joining ONCE and starting fresh. Only containers pay for this.
func (p pathAt) deeper() pathAt {
	return pathAt{root: p.String(), rec: -1, index: -1}
}

func (p pathAt) String() string {
	// Sized once: the parts are short and this runs only on a fault.
	var b strings.Builder
	b.Grow(len(p.root) + len(p.name) + 12)
	b.WriteString(p.root)
	if p.rec >= 0 {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(p.rec))
		b.WriteByte(']')
	}
	if p.name != "" {
		b.WriteByte('.')
		b.WriteString(p.name)
	}
	if p.index >= 0 {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(p.index))
		b.WriteByte(']')
	}
	return b.String()
}
