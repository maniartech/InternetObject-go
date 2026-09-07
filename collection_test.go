package internetobject_test

import (
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

type Ev struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
}

const evDoc = `~ $E: {name: string, age: {int, min: 0}}
--- events: $E
~ Alice, 30
~ Bob, notanint
~ Cara, 27
~ Dan, -5
`

// The two ways to read a collection, and why both exist.
func TestStrictSliceFailsWhereToleranceSucceeds(t *testing.T) {
	var strict struct {
		Events []Ev `io:"events"`
	}
	if err := io.Unmarshal(evDoc, &strict); err == nil {
		t.Error("[]T must fail the whole load on a bad row")
	}

	var tol struct {
		Events io.Collection[Ev] `io:"events"`
	}
	if err := io.Unmarshal(evDoc, &tol); err != nil {
		t.Fatalf("Collection[T] must absorb row faults, got %v", err)
	}
	c := &tol.Events
	if c.Len() != 4 {
		t.Errorf("Len() = %d, want 4 rows attempted", c.Len())
	}
	items := c.Items()
	if len(items) != 2 || items[0].Name != "Alice" || items[1].Name != "Cara" {
		t.Errorf("Items() = %v", items)
	}
	if !c.HasErrors() {
		t.Error("HasErrors() = false")
	}
	errs := c.Errors()
	if len(errs) != 2 {
		t.Fatalf("Errors() = %v", errs)
	}
	if errs[0].Code != io.ExpectedInteger || errs[0].RecordIndex != 1 {
		t.Errorf("first fault = %v at row %d", errs[0].Code, errs[0].RecordIndex)
	}
	if errs[1].Code != io.MismatchedMin || errs[1].RecordIndex != 3 {
		t.Errorf("second fault = %v at row %d", errs[1].Code, errs[1].RecordIndex)
	}
	// Every row is accounted for exactly once.
	if len(items)+len(errs) != c.Len() {
		t.Errorf("%d bound + %d failed != %d attempted", len(items), len(errs), c.Len())
	}
}

// All yields DOCUMENT indices, so a row and its fault can be lined up.
func TestAllYieldsDocumentIndices(t *testing.T) {
	var d struct {
		Events io.Collection[Ev] `io:"events"`
	}
	if err := io.Unmarshal(evDoc, &d); err != nil {
		t.Fatal(err)
	}
	var idx []int
	for i, e := range d.Events.All() {
		idx = append(idx, i)
		if e.Name == "" {
			t.Error("All yielded a zero value")
		}
	}
	if len(idx) != 2 || idx[0] != 0 || idx[1] != 2 {
		t.Errorf("All() indices = %v, want [0 2]", idx)
	}
	if v, ok := d.Events.At(2); !ok || v.Name != "Cara" {
		t.Errorf("At(2) = %v, %v", v, ok)
	}
	if _, ok := d.Events.At(1); ok {
		t.Error("At(1) returned a row that failed to bind")
	}
	// Early break is honoured.
	n := 0
	for range d.Events.All() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("All ignored an early break: %d", n)
	}
}

// A collection keeps its section's schema, so rows added later are held to the
// same contract as the rows that arrived.
func TestAddValidatesAgainstTheSectionSchema(t *testing.T) {
	var d struct {
		Events io.Collection[Ev] `io:"events"`
	}
	if err := io.Unmarshal(evDoc, &d); err != nil {
		t.Fatal(err)
	}
	if d.Events.Schema() == nil {
		t.Fatal("the collection did not keep its section's schema")
	}
	if err := d.Events.Add(Ev{"Eve", 22}); err != nil {
		t.Errorf("a valid row was rejected: %v", err)
	}
	if err := d.Events.Add(Ev{"Bad", -1}); err == nil {
		t.Error("a row violating the schema was accepted")
	}
	if got := len(d.Events.Items()); got != 3 {
		t.Errorf("Items() = %d, want the 2 bound plus 1 added", got)
	}
}

// A row fault is absorbed only by the section that binds tolerantly. One in a
// section bound strictly still fails the load, which is why absorption is
// decided per section rather than per document.
func TestOnlyTheTolerantSectionAbsorbs(t *testing.T) {
	const src = `~ $E: {name: string, age: {int, min: 0}}
--- good: $E
~ Alice, 30
--- bad: $E
~ Bob, notanint
`
	var mixed struct {
		Good io.Collection[Ev] `io:"good"`
		Bad  []Ev              `io:"bad"`
	}
	if err := io.Unmarshal(src, &mixed); err == nil {
		t.Error("a fault in a strictly-bound section must still fail the load")
	}

	var both struct {
		Good io.Collection[Ev] `io:"good"`
		Bad  io.Collection[Ev] `io:"bad"`
	}
	if err := io.Unmarshal(src, &both); err != nil {
		t.Errorf("both sections tolerant, so nothing should fail: %v", err)
	}
	if both.Bad.Len() != 1 || !both.Bad.HasErrors() {
		t.Errorf("the bad section did not absorb: len=%d errs=%v", both.Bad.Len(), both.Bad.Errors())
	}
	if both.Good.HasErrors() {
		t.Error("the good section reported a fault")
	}
}

// A header fault is not a row fault and is never absorbed.
func TestHeaderFaultsAreNeverAbsorbed(t *testing.T) {
	var d struct {
		Events io.Collection[Ev] `io:"events"`
	}
	if err := io.Unmarshal("~ $E: {name: nosuchtype}\n--- events: $E\n~ Alice\n", &d); err == nil {
		t.Error("a header fault was absorbed by a Collection")
	}
}

// A clean document leaves the collection with no errors at all.
func TestCleanCollection(t *testing.T) {
	var d struct {
		Events io.Collection[Ev] `io:"events"`
	}
	err := io.Unmarshal("~ $E: {name: string, age: int}\n--- events: $E\n~ A, 1\n~ B, 2\n", &d)
	if err != nil {
		t.Fatal(err)
	}
	if d.Events.Len() != 2 || d.Events.HasErrors() || d.Events.Errors() != nil {
		t.Errorf("clean load: len=%d errs=%v", d.Events.Len(), d.Events.Errors())
	}
}

// The zero value and a nil receiver read as empty rather than panicking.
func TestNilCollectionIsInert(t *testing.T) {
	var c *io.Collection[Ev]
	if c.Len() != 0 || c.Items() != nil || c.Errors() != nil || c.HasErrors() || c.Schema() != nil {
		t.Error("a nil Collection should read as empty")
	}
	if _, ok := c.At(0); ok {
		t.Error("At on a nil Collection reported a row")
	}
	for range c.All() {
		t.Error("All on a nil Collection yielded a row")
	}
	if err := c.Add(Ev{}); err == nil {
		t.Error("Add on a nil Collection should fail")
	}
	// The zero value is usable: no schema, so Add just appends.
	var z io.Collection[Ev]
	if err := z.Add(Ev{"A", 1}); err != nil {
		t.Errorf("Add to a zero Collection: %v", err)
	}
	if z.Len() != 1 {
		t.Errorf("zero Collection Len = %d", z.Len())
	}
}

// Items and Errors hand out copies, so a caller cannot reach into the
// collection's own slices.
func TestCollectionHandsOutCopies(t *testing.T) {
	var d struct {
		Events io.Collection[Ev] `io:"events"`
	}
	if err := io.Unmarshal(evDoc, &d); err != nil {
		t.Fatal(err)
	}
	items := d.Events.Items()
	items[0].Name = "clobbered"
	if again := d.Events.Items(); again[0].Name != "Alice" {
		t.Error("Items() shares the collection's slice")
	}
	errs := d.Events.Errors()
	errs[0].Code = "clobbered"
	if again := d.Events.Errors(); again[0].Code != io.ExpectedInteger {
		t.Error("Errors() shares the collection's slice")
	}
}
