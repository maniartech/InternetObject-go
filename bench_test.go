package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

var benchDoc = "~ $schema: {name: string, age: {int, min: 0}, active: bool, tags: [string]}\n---\n" +
	strings.Repeat("~ John Doe, 42, T, [alpha, beta]\n", 200)

func BenchmarkParse(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := io.Parse(benchDoc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAndWrite(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		doc, err := io.Parse(benchDoc)
		if err != nil {
			b.Fatal(err)
		}
		_ = doc.String()
	}
}
