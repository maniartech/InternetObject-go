package gen

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// A generated type shares no memory with its callers: mutating what a getter
// returned, or what was passed to the constructor or a setter, never changes
// the record. Its fields are unexported so that nothing writes past
// validation, and handing out the stored slice undid that (SPEC 0004 A11).
//
// It compiles and runs a small generated package, so -short skips it.
func TestGeneratedTypeSharesNoMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles generated code")
	}
	code, _, err := Generate("aliasing", "Rec",
		"tags: [string], ids: [bigint], big: bigint, nick?: string, when?: datetime")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeModule(t, dir)
	write(t, filepath.Join(dir, "rec.go"), code)
	write(t, filepath.Join(dir, "rec_test.go"), []byte(`package aliasing

import (
	"math/big"
	"testing"
	"time"
)

func TestNoSharedMemory(t *testing.T) {
	tags := []string{"a", "b"}
	ids := []*big.Int{big.NewInt(1)}
	bi := big.NewInt(7)
	nick := "Al"
	when := time.Unix(0, 0).UTC()
	r, err := NewRec(tags, ids, bi, &nick, &when)
	if err != nil {
		t.Fatal(err)
	}
	before, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	// What the caller passed in.
	tags[0], nick = "changed", "changed"
	ids[0].SetInt64(99)
	bi.SetInt64(99)
	when = when.Add(time.Hour)
	// What the getters handed out.
	r.Tags()[1] = "changed"
	r.Ids()[0].SetInt64(99)
	r.Big().SetInt64(99)
	*r.Nick() = "changed"
	*r.When() = time.Time{}
	// What a setter was given.
	later := []string{"x"}
	if err := r.SetTags(later); err != nil {
		t.Fatal(err)
	}
	later[0] = "changed"

	after, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if r.Tags()[0] != "x" || r.Ids()[0].Int64() != 1 || r.Big().Int64() != 7 ||
		*r.Nick() != "Al" || !r.When().Equal(time.Unix(0, 0)) {
		t.Fatalf("the record changed through shared memory:\n before %q\n after  %q", before, after)
	}
}
`))
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated type shares memory:\n%s", trim(string(out)))
	}
}

// Member names that collide with what the constructor uses compile, or are
// refused with a reason — never emitted as code that does not build.
func TestGeneratedTypeSurvivesAwkwardMemberNames(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles generated code")
	}
	code, _, err := Generate("awkward", "Rec", "slices: [string], big: bigint, time?: datetime, io: string")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Generate("awkward", "Rec", "recClonePtr?: string"); err == nil {
		t.Error("a member spelled like a generated helper was accepted")
	}
	dir := t.TempDir()
	writeModule(t, dir)
	write(t, filepath.Join(dir, "rec.go"), code)
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated code does not build:\n%s\n%s", trim(string(out)), code)
	}
}
