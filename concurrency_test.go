package internetobject_test

import (
	"strings"
	"sync"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// A parsed Document, a Definitions and a Schema are documented as safe to share
// across goroutines. This holds every read method to that under -race, on a
// document that exercises the lazily-resolved parts: named schemas, a `$ref`
// alias, a nested reference, a variable, and lookups of names that do not exist
// (whose failure used to be memoized with an unguarded map write).
func TestSharedValuesAreSafeForConcurrentUse(t *testing.T) {
	const src = "~ @min: 2\n" +
		"~ $addr: {city: string}\n" +
		"~ $person: {name: {string, minLen: @min}, home: $addr}\n" +
		"~ $alias: $person\n" +
		"--- people: $person\n" +
		"~ Alice, {Paris}\n" +
		"~ Bob, {Rome}\n"
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defs := doc.Definitions()
	sch := doc.Schema()

	// A bare schema expression bound by several sections is compiled on first
	// use and published atomically; shared definitions are consulted as the
	// parent of every document parsed against them.
	inline, err := io.Parse("name: string, age: int\n--- a\n~ A, 1\n--- b\n~ B, 2")
	if err != nil {
		t.Fatal(err)
	}
	shared, err := io.ParseDefinitions("~ $base: {name: string}")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, _ = doc.SchemaOf("person")
				_, _ = doc.SchemaOf("alias")
				_, _ = doc.SchemaOf("missing")
				_, _ = doc.Var("min")
				_, _ = doc.Var("missing")
				_ = doc.String()
				_, _ = doc.JSON(nil)
				_ = doc.Value()
				_ = doc.Records()
				_ = doc.Errors()
				if s := doc.Section("people"); s != nil {
					_ = s.Value()
					_ = s.Schema()
				}
				_ = defs.Schema("addr")
				_ = defs.Schema("missing")
				_, _ = defs.Var("min")
				_ = defs.Names()
				_ = defs.String()
				_ = sch.String()
				_ = sch.MemberNames()
				_ = inline.String()
				_ = inline.Section("a").Schema()
				_ = inline.Section("b").Value()
				if d, err := shared.Parse("~ $person: $base\n--- $person\n~ Ann\n"); err != nil {
					t.Error(err)
				} else {
					_ = d.String()
				}
				for _, err := range shared.Stream(strings.NewReader("--- $base\n~ Bo\n"), nil) {
					if err != nil {
						t.Error(err)
					}
				}
			}
		}()
	}
	wg.Wait()
}
