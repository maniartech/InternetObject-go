package internetobject_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// Comparative benchmarks against encoding/json — the baseline every Go data
// format is measured against. The same logical data is expressed in both
// formats, decoded into the same Go types, so the numbers are comparable.
//
//	go test -bench 'Compare' -benchmem -run '^$' .

type benchPerson struct {
	Name   string   `io:"name"   json:"name"`
	Age    int      `io:"age"    json:"age"`
	Email  string   `io:"email"  json:"email"`
	Active bool     `io:"active" json:"active"`
	Score  float64  `io:"score"  json:"score"`
	Tags   []string `io:"tags"   json:"tags"`
}

const benchRows = 1000

var (
	benchData     []benchPerson
	benchIOText   string
	benchJSONText string
)

func init() {
	benchData = make([]benchPerson, benchRows)
	for i := range benchData {
		benchData[i] = benchPerson{
			Name:   fmt.Sprintf("Person %d", i),
			Age:    20 + i%60,
			Email:  fmt.Sprintf("person%d@example.com", i),
			Active: i%3 != 0,
			Score:  float64(i) * 1.5,
			Tags:   []string{"alpha", "beta"},
		}
	}
	var err error
	benchIOText, err = io.Marshal(benchData)
	if err != nil {
		panic(err)
	}
	b, err := json.Marshal(benchData)
	if err != nil {
		panic(err)
	}
	benchJSONText = string(b)
}

// ── decode into Go structs ─────────────────────────────────────────────────

func BenchmarkCompareUnmarshalStruct_IO(b *testing.B) {
	b.SetBytes(int64(len(benchIOText)))
	b.ReportAllocs()
	for b.Loop() {
		var out []benchPerson
		if err := io.Unmarshal(benchIOText, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareUnmarshalStruct_JSON(b *testing.B) {
	b.SetBytes(int64(len(benchJSONText)))
	b.ReportAllocs()
	for b.Loop() {
		var out []benchPerson
		if err := json.Unmarshal([]byte(benchJSONText), &out); err != nil {
			b.Fatal(err)
		}
	}
}

// ── encode from Go structs ─────────────────────────────────────────────────

func BenchmarkCompareMarshalStruct_IO(b *testing.B) {
	b.SetBytes(int64(len(benchIOText)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := io.Marshal(benchData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareMarshalStruct_JSON(b *testing.B) {
	b.SetBytes(int64(len(benchJSONText)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(benchData); err != nil {
			b.Fatal(err)
		}
	}
}

// ── decode into the dynamic model ──────────────────────────────────────────

func BenchmarkCompareParseDynamic_IO(b *testing.B) {
	b.SetBytes(int64(len(benchIOText)))
	b.ReportAllocs()
	for b.Loop() {
		doc, err := io.Parse(benchIOText)
		if err != nil {
			b.Fatal(err)
		}
		_ = doc.Value()
	}
}

func BenchmarkCompareParseDynamic_JSON(b *testing.B) {
	b.SetBytes(int64(len(benchJSONText)))
	b.ReportAllocs()
	for b.Loop() {
		var out any
		if err := json.Unmarshal([]byte(benchJSONText), &out); err != nil {
			b.Fatal(err)
		}
	}
}

// ── validation (a capability JSON does not have) ───────────────────────────

func BenchmarkCompareValidate_IO(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if err := io.Validate(benchData); err != nil {
			b.Fatal(err)
		}
	}
}

// ── single small record, the API-payload case ──────────────────────────────

var (
	oneIO     string
	oneJSON   string
	onePerson = benchPerson{Name: "Alice", Age: 30, Email: "alice@example.com",
		Active: true, Score: 99.5, Tags: []string{"admin"}}
)

func init() {
	var err error
	if oneIO, err = io.Marshal(onePerson); err != nil {
		panic(err)
	}
	b, err := json.Marshal(onePerson)
	if err != nil {
		panic(err)
	}
	oneJSON = string(b)
}

func BenchmarkCompareSmallUnmarshal_IO(b *testing.B) {
	b.SetBytes(int64(len(oneIO)))
	b.ReportAllocs()
	for b.Loop() {
		var p benchPerson
		if err := io.Unmarshal(oneIO, &p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareSmallUnmarshal_JSON(b *testing.B) {
	b.SetBytes(int64(len(oneJSON)))
	b.ReportAllocs()
	for b.Loop() {
		var p benchPerson
		if err := json.Unmarshal([]byte(oneJSON), &p); err != nil {
			b.Fatal(err)
		}
	}
}

// TestPayloadSizes reports the wire-size advantage; run with -v.
func TestPayloadSizes(t *testing.T) {
	t.Logf("collection of %d records: IO %d bytes, JSON %d bytes (%.1f%% of JSON)",
		benchRows, len(benchIOText), len(benchJSONText),
		100*float64(len(benchIOText))/float64(len(benchJSONText)))
	t.Logf("single record:           IO %d bytes, JSON %d bytes (%.1f%% of JSON)",
		len(oneIO), len(oneJSON), 100*float64(len(oneIO))/float64(len(oneJSON)))
	t.Logf("IO first row:  %s", strings.SplitN(benchIOText, "\n", 3)[2][:60])
	t.Logf("JSON first row: %s", benchJSONText[:60])
}
