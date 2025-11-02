package parsers

import (
	"encoding/json"
	"testing"
)

// Benchmark the fast parser against regular parser and JSON
func BenchmarkFastParser_SimpleObject(b *testing.B) {
	input := `{"name": "John Doe", "age": 30, "active": true}`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParse(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

func BenchmarkFastParser_ComplexDocument(b *testing.B) {
	input := `{
		"header": {"name": "John", "age": 30, "email": "john@example.com"},
		"products": [
			{"id": 1, "title": "Product 1", "price": 29.99, "inStock": true},
			{"id": 2, "title": "Product 2", "price": 49.99, "inStock": false},
			{"id": 3, "title": "Product 3", "price": 19.99, "inStock": true}
		],
		"transactions": [
			{"type": "order", "items": [1, 2, 3], "total": 99.97},
			{"type": "shipment", "status": "pending", "trackingId": "ABC123"}
		]
	}`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParse(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark with parser reuse (zero allocations)
func BenchmarkFastParser_Reuse_ComplexDocument(b *testing.B) {
	input := `{
		"header": {"name": "John", "age": 30, "email": "john@example.com"},
		"products": [
			{"id": 1, "title": "Product 1", "price": 29.99, "inStock": true},
			{"id": 2, "title": "Product 2", "price": 49.99, "inStock": false},
			{"id": 3, "title": "Product 3", "price": 19.99, "inStock": true}
		],
		"transactions": [
			{"type": "order", "items": [1, 2, 3], "total": 99.97},
			{"type": "shipment", "status": "pending", "trackingId": "ABC123"}
		]
	}`

	parser := NewFastParser(input, 100)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser.Reset(input)
		rootIdx, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
		_ = rootIdx
	}
}

// Compare all three parsers
func BenchmarkAllParsers_ComplexDocument(b *testing.B) {
	input := `{
		"header": {"name": "John", "age": 30, "email": "john@example.com"},
		"products": [
			{"id": 1, "title": "Product 1", "price": 29.99, "inStock": true},
			{"id": 2, "title": "Product 2", "price": 49.99, "inStock": false},
			{"id": 3, "title": "Product 3", "price": 19.99, "inStock": true}
		],
		"transactions": [
			{"type": "order", "items": [1, 2, 3], "total": 99.97},
			{"type": "shipment", "status": "pending", "trackingId": "ABC123"}
		]
	}`

	b.Run("FastParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParse(input)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})

	b.Run("FastParser_Reuse", func(b *testing.B) {
		parser := NewFastParser(input, 100)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.Reset(input)
			rootIdx, err := parser.Parse()
			if err != nil {
				b.Fatal(err)
			}
			_ = rootIdx
		}
	})

	b.Run("RegularParser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseString(input)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var result interface{}
			err := json.Unmarshal([]byte(input), &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark IO native format
func BenchmarkFastParser_IONative(b *testing.B) {
	input := `{name: John, age: 30, email: john@example.com}`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParse(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark large array
func BenchmarkFastParser_LargeArray(b *testing.B) {
	input := `[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50]`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParse(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark with ToMap conversion
func BenchmarkFastParser_WithConversion(b *testing.B) {
	input := `{"name": "John", "age": 30, "active": true}`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParse(input)
		if err != nil {
			b.Fatal(err)
		}
		result := parser.ToMap(rootIdx)
		_ = result
	}
}
