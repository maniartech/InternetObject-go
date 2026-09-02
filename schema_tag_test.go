package internetobject_test

import (
	"reflect"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

type user struct {
	Name string `io:"name" schema:"{string, minLen: 2, maxLen: 50}"`
	Age  int    `io:"age"  schema:"int, min: 0, max: 130"` // unbraced convenience form
	Role string `io:"role,omitempty" schema:"{string, choices: [admin, user]}"`
}

func TestSchemaTagInHeader(t *testing.T) {
	text, err := io.Marshal(user{Name: "Alice", Age: 30, Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	want := "name: {string, minLen:2, maxLen:50}, age: {int, min:0, max:130}, " +
		"role?: {string, choices:[\"admin\", \"user\"]}\n---\nAlice, 30, admin"
	if text != want {
		t.Fatalf("Marshal:\n got %q\nwant %q", text, want)
	}
	var back user
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != "Alice" || back.Age != 30 || back.Role != "admin" {
		t.Fatalf("round trip: %+v", back)
	}
}

func TestMarshalValidatesConstraints(t *testing.T) {
	_, err := io.Marshal(user{Name: "A", Age: 30}) // name too short
	var list io.ErrorList
	if !errorsAs(err, &list) || list[0].Code != "mismatched-min-len" {
		t.Fatalf("want mismatched-min-len, got %v", err)
	}

	_, err = io.Marshal([]user{
		{Name: "Alice", Age: -1},              // min
		{Name: "Bob", Age: 25, Role: "ghost"}, // choices
	})
	if !errorsAs(err, &list) || len(list) != 2 ||
		list[0].Code != "mismatched-min" || list[1].Code != "mismatched-choice" {
		t.Fatalf("want [mismatched-min mismatched-choice], got %v", err)
	}
}

func TestUnmarshalValidatesConstraints(t *testing.T) {
	text, err := io.Marshal(user{Name: "Alice", Age: 30})
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(text, "Alice, 30", "Alice, 131", 1)
	var back user
	uerr := io.Unmarshal(bad, &back)
	var list io.ErrorList
	if !errorsAs(uerr, &list) || list[0].Code != "mismatched-max" {
		t.Fatalf("want mismatched-max from the wire, got %v", uerr)
	}
}

func TestValidateAfterMutation(t *testing.T) {
	u := user{Name: "Alice", Age: 30}
	if err := io.Validate(u); err != nil {
		t.Fatalf("valid value: %v", err)
	}
	u.Age = -5
	err := io.Validate(&u) // pointer works too
	var list io.ErrorList
	if !errorsAs(err, &list) || list[0].Code != "mismatched-min" {
		t.Fatalf("want mismatched-min, got %v", err)
	}
	u.Age = 40
	if err := io.Validate([]user{u, {Name: "Bob", Age: 1}}); err != nil {
		t.Fatalf("valid slice: %v", err)
	}
}

// Untagged types validate against the derived schema too (types only).
func TestValidateUntaggedType(t *testing.T) {
	if err := io.Validate(person{Name: "A", Age: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaFor(t *testing.T) {
	s, err := io.SchemaFor[user]()
	if err != nil {
		t.Fatal(err)
	}
	text := s.String()
	if !strings.Contains(text, "min:0") || !strings.Contains(text, "role?") {
		t.Fatalf("SchemaFor rendering: %q", text)
	}
	// The rendering is itself a valid schema definition.
	if _, err := io.ParseSchema(text); err != nil {
		t.Fatalf("SchemaFor output does not re-parse: %v\n%q", err, text)
	}
	if got := s.MemberNames(); !reflect.DeepEqual(got, []string{"name", "age", "role"}) {
		t.Fatalf("MemberNames = %v", got)
	}
}

// `optional` marks the member `?` in the schema but still writes zero values;
// `omitempty` implies optional AND leaves the zero value off the wire.
func TestOptionalVersusOmitempty(t *testing.T) {
	type rec struct {
		A string `io:"a,optional"`
		B string `io:"b,omitempty"`
	}
	text, err := io.Marshal(rec{})
	if err != nil {
		t.Fatal(err)
	}
	want := "a?: string, b?: string\n---\n\"\""
	if text != want {
		t.Fatalf("got %q want %q", text, want)
	}
	var back rec
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	if back != (rec{}) {
		t.Fatalf("round trip: %+v", back)
	}
}

func TestBadSchemaTag(t *testing.T) {
	type broken struct {
		X int `io:"x" schema:"nosuchtype"`
	}
	_, err := io.Marshal(broken{})
	if err == nil || !strings.Contains(err.Error(), "unknown-type") {
		t.Fatalf("want unknown-type, got %v", err)
	}
	type malformed struct {
		X int `io:"x" schema:"int, min:"`
	}
	if _, err := io.Marshal(malformed{}); err == nil ||
		!strings.Contains(err.Error(), "schema tag") {
		t.Fatalf("malformed tag: %v", err)
	}
}

// A schema tag on a nested struct member replaces the derived sub-schema.
func TestSchemaTagNestedValidation(t *testing.T) {
	type inner struct {
		N int `io:"n" schema:"{int, min: 10}"`
	}
	type outer struct {
		In inner `io:"in"`
	}
	if err := io.Validate(outer{In: inner{N: 12}}); err != nil {
		t.Fatal(err)
	}
	err := io.Validate(outer{In: inner{N: 3}})
	var list io.ErrorList
	if !errorsAs(err, &list) || list[0].Code != "mismatched-min" {
		t.Fatalf("nested constraint: %v", err)
	}
	// Marshal enforces it automatically because a nested field is tagged.
	if _, err := io.Marshal(outer{In: inner{N: 3}}); err == nil {
		t.Fatal("Marshal must refuse a value violating nested constraints")
	}
}
