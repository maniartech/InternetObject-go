package main

import (
	"encoding/json"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The same record three ways: the generated guarded type, the hand-written
// tagged struct the engine binds by reflection, and encoding/json.
type tagged struct {
	Name   string   `io:"name"   json:"name"`
	Age    int      `io:"age"    json:"age"`
	Email  string   `io:"email"  json:"email"`
	Active bool     `io:"active" json:"active"`
	Score  float64  `io:"score"  json:"score"`
	Tags   []string `io:"tags"   json:"tags"`
}

var (
	benchTagged = tagged{"Alice", 30, "alice@example.com", true, 99.5, []string{"admin"}}
	benchGen, _ = NewPerson("Alice", 30, "alice@example.com", true, 99.5, []string{"admin"})
	benchIOText string
	benchJSON   string
)

func init() {
	benchIOText, _ = benchGen.Marshal()
	b, _ := json.Marshal(benchTagged)
	benchJSON = string(b)
}

func BenchmarkGenMarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := benchGen.Marshal(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTaggedMarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := io.Marshal(benchTagged); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONMarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(benchTagged); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenUnmarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var p Person
		if err := p.Unmarshal(benchIOText); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTaggedUnmarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var t tagged
		if err := io.Unmarshal(benchIOText, &t); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var t tagged
		if err := json.Unmarshal([]byte(benchJSON), &t); err != nil {
			b.Fatal(err)
		}
	}
}

// Isolates what the generated Marshal adds over calling the engine directly
// with the same schema: the plain-twin copy, and nothing else.
func BenchmarkMarshalWithDirect(b *testing.B) {
	s, err := PersonSchemaOf()
	if err != nil {
		b.Fatal(err)
	}
	p := benchGen.plain()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := io.MarshalWith(p, s); err != nil {
			b.Fatal(err)
		}
	}
}

// How much of that is re-rendering the SCHEMA HEADER, which is a compile-time
// constant for a generated type and is re-emitted on every single call?
func BenchmarkHeaderShare(b *testing.B) {
	s, _ := PersonSchemaOf()
	full, _ := io.MarshalWith(benchGen.plain(), s)
	b.Logf("output is %d bytes; header is %d of them", len(full), len(full)-len(lastLine(full)))
	b.ReportAllocs()
	for b.Loop() {
		_ = s.String() // just the header text the writer must produce each call
	}
}

func lastLine(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return s[i+1:]
		}
	}
	return s
}
