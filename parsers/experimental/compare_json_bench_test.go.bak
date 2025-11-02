package parsers

import (
	"encoding/json"
	"testing"
)

// Benchmark data for comparison with JSON parser
// Since InternetObject can parse JSON syntax, we use the same JSON data for both!
var (
	// Simple object - standard JSON format (works in both parsers)
	jsonSimpleObject = `{"name": "John Doe", "age": 30, "active": true}`

	// Complex nested structure - standard JSON format
	jsonComplexDocument = `{
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

	// Nested structures - standard JSON format
	jsonNestedStructures = `{
		"user": {
			"name": "Alice",
			"age": 25,
			"address": {
				"street": "123 Main St",
				"city": "New York",
				"zip": "10001"
			}
		},
		"orders": [
			{"id": 1, "items": [10, 20, 30], "total": 60},
			{"id": 2, "items": [15, 25], "total": 40}
		]
	}`

	// Large array - standard JSON format
	jsonLargeArray = `[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50]`

	// InternetObject native format (more concise than JSON)
	ioNativeFormat = `name: "John", age: 30, email: "john@example.com"
--- products
~ {id: 1, title: "Product 1", price: 29.99, inStock: true}
~ {id: 2, title: "Product 2", price: 49.99, inStock: false}
~ {id: 3, title: "Product 3", price: 19.99, inStock: true}`
) // BenchmarkIO_vs_JSON_SimpleObject compares simple object parsing
// Both parsers process the same JSON data
func BenchmarkIO_vs_JSON_SimpleObject(b *testing.B) {
	b.Run("InternetObject", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseString(jsonSimpleObject)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var result interface{}
			err := json.Unmarshal([]byte(jsonSimpleObject), &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkIO_vs_JSON_ComplexDocument compares complex document parsing
// Both parsers process the same JSON data
func BenchmarkIO_vs_JSON_ComplexDocument(b *testing.B) {
	b.Run("InternetObject", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseString(jsonComplexDocument)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var result interface{}
			err := json.Unmarshal([]byte(jsonComplexDocument), &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkIO_vs_JSON_NestedStructures compares nested structure parsing
// Both parsers process the same JSON data
func BenchmarkIO_vs_JSON_NestedStructures(b *testing.B) {
	b.Run("InternetObject", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseString(jsonNestedStructures)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var result interface{}
			err := json.Unmarshal([]byte(jsonNestedStructures), &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkIO_vs_JSON_LargeArray compares large array parsing
// Both parsers process the same JSON data
func BenchmarkIO_vs_JSON_LargeArray(b *testing.B) {
	b.Run("InternetObject", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseString(jsonLargeArray)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var result interface{}
			err := json.Unmarshal([]byte(jsonLargeArray), &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkIO_vs_JSON_Tokenizer compares just tokenization/lexing phase
func BenchmarkIO_vs_JSON_Tokenizer(b *testing.B) {
	b.Run("InternetObject_Tokenizer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tokenizer := NewTokenizer(jsonComplexDocument)
			_, err := tokenizer.Tokenize()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Note: JSON decoder doesn't expose tokenization separately in a comparable way
}

// BenchmarkIO_vs_JSON_SizeComparison shows data size differences
func BenchmarkIO_vs_JSON_SizeComparison(b *testing.B) {
	b.Run("JSON_SimpleObject_Size", func(b *testing.B) {
		b.ReportMetric(float64(len(jsonSimpleObject)), "bytes")
	})

	b.Run("IO_Native_SimpleObject_Size", func(b *testing.B) {
		ioNative := `{name: "John Doe", age: 30, active: true}`
		b.ReportMetric(float64(len(ioNative)), "bytes")
	})

	b.Run("JSON_ComplexDocument_Size", func(b *testing.B) {
		b.ReportMetric(float64(len(jsonComplexDocument)), "bytes")
	})

	b.Run("IO_Native_ComplexDocument_Size", func(b *testing.B) {
		b.ReportMetric(float64(len(ioNativeFormat)), "bytes")
	})

	b.Run("JSON_NestedStructures_Size", func(b *testing.B) {
		b.ReportMetric(float64(len(jsonNestedStructures)), "bytes")
	})

	b.Run("IO_Native_NestedStructures_Size", func(b *testing.B) {
		ioNative := `{user: {name: "Alice", age: 25, address: {street: "123 Main St", city: "New York", zip: "10001"}}, orders: [{id: 1, items: [10, 20, 30], total: 60}, {id: 2, items: [15, 25], total: 40}]}`
		b.ReportMetric(float64(len(ioNative)), "bytes")
	})
}

// BenchmarkIO_ParsingStages breaks down parsing into stages
func BenchmarkIO_ParsingStages(b *testing.B) {
	b.Run("Stage1_Tokenization", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tokenizer := NewTokenizer(jsonComplexDocument)
			_, err := tokenizer.Tokenize()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	tokenizer := NewTokenizer(jsonComplexDocument)
	tokens, _ := tokenizer.Tokenize()

	b.Run("Stage2_ParsingOnly", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parser := NewParser(tokens)
			_, err := parser.Parse()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Stage3_Combined", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := ParseString(jsonComplexDocument)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkIO_NativeFormat tests InternetObject's native (more concise) format
func BenchmarkIO_NativeFormat(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseString(ioNativeFormat)
		if err != nil {
			b.Fatal(err)
		}
	}
}
