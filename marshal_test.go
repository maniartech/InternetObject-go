package internetobject_test

import (
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	io "github.com/maniartech/InternetObject-go"
)

type person struct {
	Name   string   `io:"name"`
	Age    int      `io:"age"`
	Email  string   `io:"email,omitempty"`
	Tags   []string `io:"tags,omitempty"`
	secret string   //lint:ignore U1000 unexported fields must be ignored
}

func TestMarshalCollection(t *testing.T) {
	people := []person{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25, Email: "bob@x.io", Tags: []string{"a", "b"}},
	}
	out, err := io.Marshal(people)
	if err != nil {
		t.Fatal(err)
	}
	want := "name: string, age: int, email?: string, tags?: [string]\n---\n" +
		"~ Alice, 30\n~ Bob, 25, bob@x.io, [a, b]"
	if out != want {
		t.Fatalf("Marshal:\n got %q\nwant %q", out, want)
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	in := []person{
		{Name: "Alice, Jr.", Age: 30, Tags: []string{"x---y", "null"}},
		{Name: "Bob", Age: 25, Email: "bob@x.io"},
	}
	text, err := io.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var back []person
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatalf("Unmarshal(%q): %v", text, err)
	}
	if !reflect.DeepEqual(in, back) {
		t.Fatalf("round trip:\n in  %#v\n out %#v\n text %q", in, back, text)
	}
}

func TestUnmarshalSingleAndSchemaless(t *testing.T) {
	var p person
	if err := io.Unmarshal("Alice, 30", &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Alice" || p.Age != 30 {
		t.Fatalf("positional bind: %+v", p)
	}
	p = person{}
	if err := io.Unmarshal("age: 41, name: Grace", &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Grace" || p.Age != 41 {
		t.Fatalf("keyed bind: %+v", p)
	}
}

func TestUnmarshalValidates(t *testing.T) {
	text := "name: string, age: int\n---\n~ Alice, notanumber"
	var got []person
	err := io.Unmarshal(text, &got)
	var list io.ErrorList
	if err == nil || !errorsAs(err, &list) || list[0].Code != "expected-integer" {
		t.Fatalf("want expected-integer ErrorList, got %v", err)
	}
}

type kitchen struct {
	B    bool           `io:"b"`
	F    float64        `io:"f"`
	U    uint16         `io:"u"`
	Big  *big.Int       `io:"big"`
	Dec  io.Decimal     `io:"dec"`
	When time.Time      `io:"when"`
	Day  time.Time      `io:"day,date"`
	Blob []byte         `io:"blob"`
	Meta map[string]int `io:"meta"`
	Any  any            `io:"any"`
	Ptr  *string        `io:"ptr"`
}

func TestMarshalKitchenSinkRoundTrip(t *testing.T) {
	s := "hi"
	in := kitchen{
		B:    true,
		F:    -2.5,
		U:    9,
		Big:  new(big.Int).Lsh(big.NewInt(1), 80),
		Dec:  io.Decimal{Coef: big.NewInt(150), Scale: 2},
		When: time.Date(2024, 1, 15, 14, 30, 45, 123e6, time.UTC),
		Day:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Blob: []byte{1, 2, 3},
		Meta: map[string]int{"b": 2, "a": 1},
		Any:  "text",
		Ptr:  &s,
	}
	text, err := io.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var back kitchen
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatalf("Unmarshal(%q): %v", text, err)
	}
	if !reflect.DeepEqual(in, back) {
		t.Fatalf("round trip:\n in  %#v\n out %#v\n text %q", in, back, text)
	}

	// nil pointer marshals as N and comes back nil
	in.Ptr = nil
	text, err = io.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	back = kitchen{}
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	if back.Ptr != nil {
		t.Fatalf("nil pointer round trip: %v (text %q)", *back.Ptr, text)
	}
}

type nested struct {
	ID   int  `io:"id"`
	Home addr `io:"home"`
}

type addr struct {
	City string `io:"city"`
	Zip  string `io:"zip,omitempty"`
}

func TestMarshalNestedStruct(t *testing.T) {
	in := nested{ID: 7, Home: addr{City: "Pune", Zip: "411001"}}
	text, err := io.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "home: {city: string, zip?: string}") {
		t.Fatalf("nested schema missing: %q", text)
	}
	var back nested
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, back) {
		t.Fatalf("round trip: %#v via %q", back, text)
	}
}

type embedded struct {
	addr
	Name string `io:"name"`
}

func TestMarshalEmbedded(t *testing.T) {
	in := embedded{addr: addr{City: "Pune"}, Name: "A"}
	text, err := io.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var back embedded
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, back) {
		t.Fatalf("embedded round trip: %#v via %q", back, text)
	}
}

func TestMarshalErrors(t *testing.T) {
	if _, err := io.Marshal(struct {
		C chan int `io:"c"`
	}{}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("chan: %v", err)
	}
	if _, err := io.Marshal(struct {
		N int64 `io:"n"`
	}{N: 1 << 60}); err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("overflow: %v", err)
	}
	type selfRef struct {
		Next *selfRef `io:"next"`
	}
	if _, err := io.Marshal(selfRef{}); err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Fatalf("recursion: %v", err)
	}
}

func TestUnmarshalErrors(t *testing.T) {
	var p person
	if err := io.Unmarshal("name: string\n---\n~ A\n~ B", &p); err == nil ||
		!strings.Contains(err.Error(), "records") {
		t.Fatalf("multi-record into struct: %v", nilOr(err))
	}
	if err := io.Unmarshal("T", &p.Age); err == nil {
		t.Fatal("non-pointer-compatible bind must fail")
	}
	var n int
	if err := io.Unmarshal("3.5", &n); err == nil || !strings.Contains(err.Error(), "cannot store") {
		t.Fatalf("fractional into int: %v", nilOr(err))
	}
}

func TestUnmarshalDynamicTargets(t *testing.T) {
	var v any
	if err := io.Unmarshal("~ a: 1\n~ b: 2", &v); err != nil {
		t.Fatal(err)
	}
	if arr, ok := v.([]any); !ok || len(arr) != 2 {
		t.Fatalf("dynamic: %#v", v)
	}
}

func TestMarshalConcurrent(t *testing.T) {
	in := person{Name: "P", Age: 1}
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 200; j++ {
				text, err := io.Marshal(in)
				if err == nil {
					var p person
					err = io.Unmarshal(text, &p)
				}
				if err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkMarshal(b *testing.B) {
	people := make([]person, 100)
	for i := range people {
		people[i] = person{Name: "Person", Age: i, Email: "p@x.io"}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := io.Marshal(people); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	people := make([]person, 100)
	for i := range people {
		people[i] = person{Name: "Person", Age: i, Email: "p@x.io"}
	}
	text, err := io.Marshal(people)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var back []person
		if err := io.Unmarshal(text, &back); err != nil {
			b.Fatal(err)
		}
	}
}

// errorsAs avoids importing errors alongside the io alias clash in examples.
func errorsAs(err error, target *io.ErrorList) bool {
	l, ok := err.(io.ErrorList)
	if ok {
		*target = l
	}
	return ok
}

func nilOr(err error) any {
	if err == nil {
		return "<nil>"
	}
	return err
}
