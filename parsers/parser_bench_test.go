package parsers

import (
	"testing"
)

// Benchmark data - real-world like documents
var (
	simpleObject = `{name: "John Doe", age: 30, active: true}`

	complexDocument = `name: "John", age: 30, email: "john@example.com"
--- products
~ {id: 1, title: "Product 1", price: 29.99, inStock: true}
~ {id: 2, title: "Product 2", price: 49.99, inStock: false}
~ {id: 3, title: "Product 3", price: 19.99, inStock: true}
--- transactions
~ {type: "order", items: [1, 2, 3], total: 99.97}
~ {type: "shipment", status: "pending", trackingId: "ABC123"}`

	nestedStructures = `{
		user: {
			name: "Alice",
			age: 25,
			address: {
				street: "123 Main St",
				city: "New York",
				zip: "10001"
			}
		},
		orders: [
			{id: 1, items: [10, 20, 30], total: 60},
			{id: 2, items: [15, 25], total: 40}
		]
	}`

	largeArray = `[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50]`

	collection = `~ {name: "Item1", value: 100}
~ {name: "Item2", value: 200}
~ {name: "Item3", value: 300}
~ {name: "Item4", value: 400}
~ {name: "Item5", value: 500}`

	headerAndSections = `version: "1.0", type: "config"
--- settings
{debug: true, timeout: 5000, retries: 3}
--- network
{host: "localhost", port: 8080, ssl: false}`
)

// BenchmarkParseString_SimpleObject benchmarks parsing a simple object
func BenchmarkParseString_SimpleObject(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(simpleObject)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseString_ComplexDocument benchmarks parsing a complex document with sections
func BenchmarkParseString_ComplexDocument(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(complexDocument)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseString_NestedStructures benchmarks parsing deeply nested objects
func BenchmarkParseString_NestedStructures(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(nestedStructures)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseString_LargeArray benchmarks parsing a large array
func BenchmarkParseString_LargeArray(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(largeArray)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseString_Collection benchmarks parsing collections
func BenchmarkParseString_Collection(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(collection)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseString_HeaderAndSections benchmarks parsing header and sections
func BenchmarkParseString_HeaderAndSections(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(headerAndSections)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTokenizer_Only benchmarks just tokenization (no parsing)
func BenchmarkTokenizer_Only(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(complexDocument)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParser_Only benchmarks just parsing (tokenization pre-done)
func BenchmarkParser_Only(b *testing.B) {
	tokenizer := NewTokenizer(complexDocument)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcessDocument benchmarks the document processing phase
func BenchmarkProcessDocument(b *testing.B) {
	tokenizer := NewTokenizer(headerAndSections)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, err := parser.processDocument()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseObject benchmarks object parsing
func BenchmarkParseObject(b *testing.B) {
	tokenizer := NewTokenizer(simpleObject)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, err := parser.parseObject(false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseArray benchmarks array parsing
func BenchmarkParseArray(b *testing.B) {
	tokenizer := NewTokenizer(largeArray)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, err := parser.parseArray()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseMember benchmarks member parsing
func BenchmarkParseMember(b *testing.B) {
	// Parse key-value pairs
	input := `name: "test"`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, err := parser.parseMember()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseValue benchmarks value parsing
func BenchmarkParseValue(b *testing.B) {
	input := `"test string value"`
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, err := parser.parseValue()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatchHelpers benchmarks the match helper methods
func BenchmarkMatchHelpers(b *testing.B) {
	tokenizer := NewTokenizer(simpleObject)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		b.Fatal(err)
	}
	parser := NewParser(tokens)

	b.Run("match", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			parser.match([]TokenType{TokenCurlyOpen})
		}
	})

	b.Run("matchNext", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			parser.matchNext([]TokenType{TokenString})
		}
	})

	b.Run("matchPrev", func(b *testing.B) {
		parser.current = 1
		for i := 0; i < b.N; i++ {
			parser.matchPrev([]TokenType{TokenCurlyOpen})
		}
	})
}

// BenchmarkParseString_Parallel benchmarks concurrent parsing
func BenchmarkParseString_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := ParseString(complexDocument)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
