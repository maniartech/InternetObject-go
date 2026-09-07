package internetobject

import (
	"iter"
	"reflect"
)

// Collection is a section's rows bound to T, keeping the ones that bound
// ALONGSIDE the faults of the ones that did not.
//
// This is the format's accumulate-and-continue rule at the row level, and it
// is the difference between the two ways to read a collection:
//
//	[]T             strict — any bad row fails the whole Unmarshal
//	Collection[T]   tolerant — good rows arrive, bad rows are reported
//
// Which one you want depends on the data. A config file should be strict: a
// single bad row means the file is wrong. A feed of a million events should
// not be, because one malformed event is not a reason to drop the other
// 999,999.
//
//	var d struct {
//	    Events io.Collection[Event] `io:"events"`
//	}
//	io.Unmarshal(text, &d)
//	for i, e := range d.Events.All() { … }   // i is the DOCUMENT row index
//	for _, err := range d.Events.Errors() { … }
//
// The indices in Errors and All are positions in the document, so a fault and
// the row it belongs to can always be lined up.
type Collection[T any] struct {
	items  []T
	at     []int // items[k] came from document row at[k]
	errors []Error
	schema *Schema
	n      int // rows attempted, both kinds
}

// Len is the number of rows ATTEMPTED — bound and failed together — so it is
// the number of rows the document actually carried.
func (c *Collection[T]) Len() int {
	if c == nil {
		return 0
	}
	return c.n
}

// Items returns the rows that bound, in document order. It never contains a
// zero value standing in for a failure; a row that failed is in Errors.
func (c *Collection[T]) Items() []T {
	if c == nil {
		return nil
	}
	out := make([]T, len(c.items))
	copy(out, c.items)
	return out
}

// Errors returns the faults of the rows that did not bind, each carrying the
// document path of the row it came from.
func (c *Collection[T]) Errors() []Error {
	if c == nil || len(c.errors) == 0 {
		return nil
	}
	out := make([]Error, len(c.errors))
	copy(out, c.errors)
	return out
}

// HasErrors reports whether any row failed.
func (c *Collection[T]) HasErrors() bool { return c != nil && len(c.errors) > 0 }

// All iterates the rows that bound, yielding each row's index IN THE DOCUMENT
// rather than its position among the survivors — so an index here and an index
// in an error mean the same thing.
func (c *Collection[T]) All() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		if c == nil {
			return
		}
		for k, v := range c.items {
			if !yield(c.at[k], v) {
				return
			}
		}
	}
}

// At returns the row that bound at document index i.
func (c *Collection[T]) At(i int) (T, bool) {
	var zero T
	if c == nil {
		return zero, false
	}
	for k, at := range c.at {
		if at == i {
			return c.items[k], true
		}
	}
	return zero, false
}

// Add appends a row, VALIDATING it against the collection's schema first when
// it has one. A row that does not satisfy the schema is rejected and not
// added — the same rule the Builder follows, for the same reason.
//
// A collection produced by Unmarshal carries its section's schema, so rows
// added afterwards are held to the same contract as the rows that arrived.
func (c *Collection[T]) Add(v T) error {
	if c == nil {
		return &MarshalError{Path: "$", Msg: "Add on a nil Collection"}
	}
	if c.schema != nil {
		if err := ValidateWith(v, c.schema); err != nil {
			return err
		}
	}
	c.items = append(c.items, v)
	c.at = append(c.at, c.n)
	c.n++
	return nil
}

// Schema is the schema rows are validated against, or nil.
func (c *Collection[T]) Schema() *Schema {
	if c == nil {
		return nil
	}
	return c.schema
}

// collectionBinder is how the section binder fills a Collection[T] without
// reflecting into its unexported fields: the concrete type knows its own T, so
// it does the work and the binder only has to recognise it.
//
// The fields stay unexported deliberately — the type's invariant is that
// items, at and errors stay in step — and bindFrom is the one place that
// invariant is established.
type collectionBinder interface {
	bindFrom(sec *Section, at pathAt) error
}

// bindFrom fills the collection from a section, keeping the rows that bind and
// recording the faults of the rest.
func (c *Collection[T]) bindFrom(sec *Section, at pathAt) error {
	c.items, c.at, c.errors = nil, nil, nil
	c.n = sec.Len()
	c.schema = sec.Schema()

	for i, rec := range sec.sec.Records {
		// A row that already failed validation carries its marker; report that
		// rather than trying to bind a value which is not there.
		if item, isErr := rec.(ErrorItem); isErr {
			c.errors = append(c.errors, Error{
				Code: item.Code, Category: item.Category, Path: item.Path,
				RecordIndex: i, Line: int(item.Line), Col: int(item.Col),
			})
			continue
		}
		obj, ok := rec.(*Object)
		if !ok {
			c.errors = append(c.errors, Error{
				Code: InvalidObject, Category: CategoryValidation,
				Path: at.record(i).String(), RecordIndex: i,
			})
			continue
		}
		var v T
		if err := bindInto(reflect.ValueOf(&v).Elem(), obj, at.record(i)); err != nil {
			c.errors = append(c.errors, bindFault(err, at.record(i).String(), i))
			continue
		}
		c.items = append(c.items, v)
		c.at = append(c.at, i)
	}
	return nil
}

// bindFault turns a binding error into the Error a collection reports, keeping
// the designated code where there is one.
func bindFault(err error, path string, index int) Error {
	if list, ok := err.(ErrorList); ok && len(list) > 0 {
		e := list[0]
		e.RecordIndex = index
		if e.Path == "" {
			e.Path = path
		}
		return e
	}
	if e, ok := err.(Error); ok {
		e.RecordIndex = index
		return e
	}
	// An UnmarshalError is a Go-side type mismatch, not a format fault; it has
	// no designated code, so it reports as the general one with its message
	// preserved by the path.
	return Error{
		Code: InvalidObject, Category: CategoryValidation,
		Path: path, RecordIndex: index,
	}
}
