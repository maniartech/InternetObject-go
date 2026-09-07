package internetobject_test

import (
	"reflect"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The format has three record spellings - a struct, a map with string keys,
// and an *Object - and the marshaler used to know only one. The other two were
// written as an ARRAY inside a single row, which does not even survive
// Unmarshal: it correctly expects a collection.
func TestSliceOfRecordsIsACollection(t *testing.T) {
	obj := func() *io.Object {
		return io.NewObject(1).Append("name", "Alice")
	}
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"maps", []map[string]any{{"name": "Alice"}, {"name": "Bob"}}},
		{"objects", []*io.Object{obj(), obj()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, err := io.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			var back []map[string]any
			if err := io.Unmarshal(text, &back); err != nil {
				t.Fatalf("did not survive the round trip: %v\n%s", err, text)
			}
			if len(back) != 2 {
				t.Fatalf("got %d records, want 2: %s", len(back), text)
			}
			if back[0]["name"] != "Alice" {
				t.Errorf("first record = %v", back[0])
			}
			doc, err := io.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			if !doc.Sections()[0].IsCollection() {
				t.Errorf("not written as a collection: %q", text)
			}
		})
	}
}

// An Object is the format's OWN record type. Reflecting over it as if it were
// a user's struct emitted Members, Positional, Quoted, Line and Col into the
// document - the library leaking its representation into the text it produces.
func TestObjectMarshalsAsARecordNotAsItsRepresentation(t *testing.T) {
	o := io.NewObject(2).Append("name", "Alice").Append("age", 30)
	text, err := io.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"Members", "Positional", "Quoted", "Line", "Col"} {
		if contains(text, leaked) {
			t.Fatalf("internal field %q reached the output: %q", leaked, text)
		}
	}
	var back map[string]any
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	// A Go int a caller stored must reach the writer as the format's number.
	if !reflect.DeepEqual(back, map[string]any{"name": "Alice", "age": float64(30)}) {
		t.Errorf("round trip = %#v", back)
	}
}

// Positional members keep their position, and nested values are normalized
// just as deeply.
func TestObjectMarshalKeepsShape(t *testing.T) {
	inner := io.NewObject(1).Append("n", 1)
	o := io.NewObject(3).AppendValue("Alice").Append("age", 30).Append("meta", inner)
	text, err := io.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := io.Parse(text)
	if err != nil {
		t.Fatalf("%v for %q", err, text)
	}
	rec := doc.Records()[0].(*io.Object)
	if v, ok := rec.At(0); !ok || v != "Alice" {
		t.Errorf("positional member did not survive: %v (%q)", v, text)
	}
	if v, ok := rec.Get("age"); !ok || v != float64(30) {
		t.Errorf("age = %v (%q)", v, text)
	}
	nested, ok := rec.Get("meta")
	if !ok {
		t.Fatalf("nested object lost: %q", text)
	}
	if v, _ := nested.(*io.Object).Get("n"); v != float64(1) {
		t.Errorf("nested value not normalized: %v (%q)", v, text)
	}
}

// Dispatch is on the ELEMENT TYPE, never on what the elements happen to hold,
// so an empty slice writes the same shape as a full one.
func TestCollectionDispatchIsStatic(t *testing.T) {
	empty, err := io.Marshal([]map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var back []map[string]any
	if err := io.Unmarshal(empty, &back); err != nil {
		t.Errorf("an empty collection did not round-trip: %v (%q)", err, empty)
	}
	if len(back) != 0 {
		t.Errorf("empty collection came back with %d records", len(back))
	}
	// []any stays an ARRAY: it is genuinely ambiguous, and content sniffing
	// would make an empty slice write a different shape from a full one.
	arr, err := io.Marshal([]any{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if doc, err := io.Parse(arr); err == nil && doc.Sections()[0].IsCollection() {
		t.Errorf("[]any was written as a collection: %q", arr)
	}
}

// A nil record in a collection is refused rather than written as a hole,
// matching the struct path.
func TestNilRecordInACollectionIsRefused(t *testing.T) {
	if _, err := io.Marshal([]*io.Object{nil}); err == nil {
		t.Error("a nil Object record was accepted")
	}
	if _, err := io.Marshal([]map[string]any{nil}); err == nil {
		t.Error("a nil map record was accepted")
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
