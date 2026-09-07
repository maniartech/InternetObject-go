package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// A map and an *Object are records too, so they validate — but only against a
// schema the caller supplies, because neither declares any types of its own.
func TestValidateAcceptsMapsAndObjects(t *testing.T) {
	s, err := io.ParseSchema("{name: string, age: {int, min: 0}}")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		v    any
		ok   bool
	}{
		{"map ok", map[string]any{"name": "A", "age": 3}, true},
		{"map bad", map[string]any{"name": "A", "age": -1}, false},
		{"map missing", map[string]any{"name": "A"}, false},
		{"object ok", io.NewObject(2).Append("name", "A").Append("age", 3), true},
		{"object bad", io.NewObject(2).Append("name", "A").Append("age", -1), false},
		{"slice of maps ok", []map[string]any{{"name": "A", "age": 1}}, true},
		{"slice of maps bad", []map[string]any{{"name": "A", "age": 1}, {"name": "B", "age": -1}}, false},
	} {
		err := io.ValidateWith(tc.v, s)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}

	// Without a schema there is nothing to check against, and saying so beats
	// silently passing everything.
	err = io.Validate(map[string]any{"name": "A"})
	if err == nil {
		t.Error("Validate on a map with no schema should say what is missing")
	}
	if !strings.Contains(err.Error(), "ValidateWith") {
		t.Errorf("the message should name the way forward: %v", err)
	}

	// A nil record in a slice is refused rather than skipped.
	if err := io.ValidateWith([]map[string]any{nil}, s); err == nil {
		t.Error("a nil record was accepted")
	}
}
