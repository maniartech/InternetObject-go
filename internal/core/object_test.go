package core_test

import (
	"testing"

	"github.com/maniartech/InternetObject-go/internal/core"
)

func keyed() *core.Object {
	return core.NewObject(3).Append("name", "Alice").Append("age", 30.0)
}

func TestObjectReadsByKeyAndByPosition(t *testing.T) {
	o := keyed()
	if v, ok := o.Get("name"); !ok || v != "Alice" {
		t.Errorf("Get(name) = %v, %v", v, ok)
	}
	if _, ok := o.Get("nope"); ok {
		t.Error("Get reported a key that is not there")
	}
	if !o.Has("age") || o.Has("nope") {
		t.Error("Has disagrees with Get")
	}
	if v, ok := o.At(1); !ok || v != 30.0 {
		t.Errorf("At(1) = %v, %v", v, ok)
	}
	if o.KeyAt(0) != "name" {
		t.Errorf("KeyAt(0) = %q", o.KeyAt(0))
	}
	// Out of range is REPORTED, never panicked - callers index these with
	// numbers that came from data.
	for _, i := range []int{-1, 99} {
		if _, ok := o.At(i); ok {
			t.Errorf("At(%d) claimed to succeed", i)
		}
		if o.KeyAt(i) != "" {
			t.Errorf("KeyAt(%d) = %q, want empty", i, o.KeyAt(i))
		}
		if o.SetAt(i, 1) {
			t.Errorf("SetAt(%d) claimed to succeed", i)
		}
		if o.DeleteAt(i) {
			t.Errorf("DeleteAt(%d) claimed to succeed", i)
		}
	}
}

// A positional member has no key of its own, so a keyed lookup must not find
// it: `~ Alice, 30` has no "name", however much a schema would say otherwise.
func TestPositionalMembersAreNotFoundByKey(t *testing.T) {
	o := core.NewObject(0).AppendValue("Alice").AppendValue(30.0)
	if _, ok := o.Get("name"); ok {
		t.Error("a positional member answered a keyed lookup")
	}
	if o.Find("") != -1 {
		t.Error("a positional member matched the empty key")
	}
	if got := len(o.Keys()); got != 0 {
		t.Errorf("Keys() = %d, want 0 for an all-positional object", got)
	}
	if o.Len() != 2 {
		t.Errorf("Len() = %d, want 2", o.Len())
	}
	if o.KeyAt(0) != "" {
		t.Errorf("KeyAt of a positional member = %q, want empty", o.KeyAt(0))
	}
}

// An edit must not reorder a document: Set on an existing key replaces in
// place, and only a genuinely new key goes to the end.
func TestSetReplacesInPlaceAndAppendsWhenNew(t *testing.T) {
	o := keyed()
	o.Set("name", "Bob")
	if o.Len() != 2 {
		t.Fatalf("Set added a member for an existing key: len = %d", o.Len())
	}
	if o.KeyAt(0) != "name" {
		t.Errorf("Set moved the member: KeyAt(0) = %q", o.KeyAt(0))
	}
	if v, _ := o.Get("name"); v != "Bob" {
		t.Errorf("Set did not replace: %v", v)
	}
	o.Set("city", "Pune")
	if o.KeyAt(2) != "city" {
		t.Errorf("a new key did not go to the end: %v", o.Keys())
	}
}

func TestDeleteRemovesOnlyTheNamedMember(t *testing.T) {
	o := keyed().Append("city", "Pune")
	if !o.Delete("age") {
		t.Fatal("Delete reported nothing to remove")
	}
	if o.Delete("age") {
		t.Error("Delete removed the same member twice")
	}
	if got := o.Keys(); len(got) != 2 || got[0] != "name" || got[1] != "city" {
		t.Errorf("keys after delete = %v", got)
	}
}

func TestAllIteratesInOrderAndCanStopEarly(t *testing.T) {
	o := keyed().AppendValue("extra")
	var keys []string
	for k := range o.All() {
		keys = append(keys, k)
	}
	// A positional member yields "" as its key.
	if len(keys) != 3 || keys[0] != "name" || keys[1] != "age" || keys[2] != "" {
		t.Errorf("All() keys = %q", keys)
	}
	n := 0
	for range o.All() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("All() ignored an early break: %d iterations", n)
	}
}

// Clone owns its member slice but SHARES the values, which is the contract
// callers have to be able to rely on in both directions.
func TestCloneSeparatesStructureAndSharesValues(t *testing.T) {
	nested := core.NewObject(1).Append("n", 1.0)
	o := keyed().Append("nested", nested)

	c := o.Clone()
	c.Set("name", "Bob")
	c.Delete("age")
	if v, _ := o.Get("name"); v != "Alice" {
		t.Error("editing the clone changed the original's members")
	}
	if !o.Has("age") {
		t.Error("deleting from the clone removed from the original")
	}

	shared, _ := c.Get("nested")
	shared.(*core.Object).Set("n", 2.0)
	got, _ := nested.Get("n")
	if got != 2.0 {
		t.Errorf("nested value was copied, not shared: %v", got)
	}

	if cl := (&core.Object{}).Clone(); cl.Members != nil {
		t.Error("cloning an empty object invented a slice")
	}
}

func TestNewObjectSizing(t *testing.T) {
	if o := core.NewObject(-1); o.Len() != 0 || o.Members != nil {
		t.Error("a negative capacity should give a bare object")
	}
	if o := core.NewObject(4); cap(o.Members) != 4 {
		t.Errorf("cap = %d, want 4", cap(o.Members))
	}
}
