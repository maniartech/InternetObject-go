package internetobject

type fieldPlan struct {
	name     string
	index    []int // reflect index path (embedded fields included)
	at       int   // the single index when index has depth 1, else -1
	enc      encKind
	elem     encKind // for encSlice: the element's kind
	optional bool    // ,optional or ,omitempty: the member compiles as `name?`
	omitZero bool    // ,omitempty only: the zero value is left off the wire
	nullable bool    // pointer field
	kind     string
}
