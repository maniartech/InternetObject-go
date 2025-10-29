package parsers

import (
	"encoding/json"
	"testing"
)

// Benchmark the byte-based fast parser against string-based and JSON
func BenchmarkFastParserBytes_SimpleObject(b *testing.B) {
	input := []byte(`{"name": "John Doe", "age": 30, "active": true}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParseBytes(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

func BenchmarkFastParserBytes_ComplexDocument(b *testing.B) {
	input := []byte(`{
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
	}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParseBytes(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark with parser reuse (zero allocations)
func BenchmarkFastParserBytes_Reuse_ComplexDocument(b *testing.B) {
	input := []byte(`{
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
	}`)

	parser := NewFastParserBytes(input, 100)

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

// Compare all parser implementations
func BenchmarkAllParsers_Bytes_ComplexDocument(b *testing.B) {
	inputBytes := []byte(`{
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
	}`)
	inputString := string(inputBytes)

	b.Run("FastParserBytes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParseBytes(inputBytes)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})

	b.Run("FastParserBytes_Reuse", func(b *testing.B) {
		parser := NewFastParserBytes(inputBytes, 100)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.Reset(inputBytes)
			rootIdx, err := parser.Parse()
			if err != nil {
				b.Fatal(err)
			}
			_ = rootIdx
		}
	})

	b.Run("FastParser_String", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParse(inputString)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})

	b.Run("FastParser_String_Reuse", func(b *testing.B) {
		parser := NewFastParser(inputString, 100)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.Reset(inputString)
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
			_, err := ParseString(inputString)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var result interface{}
			err := json.Unmarshal(inputBytes, &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark IO native format
func BenchmarkFastParserBytes_IONative(b *testing.B) {
	input := []byte(`{name: John, age: 30, email: john@example.com}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParseBytes(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark large array
func BenchmarkFastParserBytes_LargeArray(b *testing.B) {
	input := []byte(`[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50]`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParseBytes(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark with ToMap conversion
func BenchmarkFastParserBytes_WithConversion(b *testing.B) {
	input := []byte(`{"name": "John", "age": 30, "active": true}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParseBytes(input)
		if err != nil {
			b.Fatal(err)
		}
		result := parser.ToMap(rootIdx)
		_ = result
	}
}

// Benchmark number parsing specifically
func BenchmarkFastParserBytes_NumberParsing(b *testing.B) {
	b.Run("Integers", func(b *testing.B) {
		input := []byte(`[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 123, 456, 789, 1000, 99999]`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParseBytes(input)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})

	b.Run("Floats", func(b *testing.B) {
		input := []byte(`[1.5, 2.7, 3.14159, 99.99, 0.001, 123.456]`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParseBytes(input)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})

	b.Run("Mixed", func(b *testing.B) {
		input := []byte(`[1, 2.5, 3, 4.75, 5, 6.0, 7, 8.125]`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParseBytes(input)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})
}

// Benchmark string parsing
func BenchmarkFastParserBytes_StringParsing(b *testing.B) {
	b.Run("QuotedStrings", func(b *testing.B) {
		input := []byte(`["hello", "world", "test", "benchmark", "performance"]`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParseBytes(input)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})

	b.Run("UnquotedStrings", func(b *testing.B) {
		input := []byte(`{name: John, city: Boston, country: USA}`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser, rootIdx, err := FastParseBytes(input)
			if err != nil {
				b.Fatal(err)
			}
			_ = parser
			_ = rootIdx
		}
	})
}

// Benchmark very large document
func BenchmarkFastParserBytes_VeryLargeDocument(b *testing.B) {
	// Build a large document programmatically
	input := []byte(`{
		"users": [`)

	for i := 0; i < 100; i++ {
		if i > 0 {
			input = append(input, ',')
		}
		userJSON := `{"id": ` + string(byte('0'+i%10)) + `, "name": "User` + string(byte('0'+i%10)) + `", "active": true}`
		input = append(input, []byte(userJSON)...)
	}

	input = append(input, []byte(`]
	}`)...)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parser, rootIdx, err := FastParseBytes(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = parser
		_ = rootIdx
	}
}

// Benchmark GetString operations (zero-copy vs copy)
func BenchmarkFastParserBytes_GetString(b *testing.B) {
	input := []byte(`{"name": "John Doe", "email": "john@example.com", "city": "Boston"}`)
	parser, rootIdx, _ := FastParseBytes(input)
	val := parser.GetValue(rootIdx)

	b.Run("GetString_ZeroCopy", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < val.ChildCount; j++ {
				member := parser.GetMember(val.FirstChild + j)
				memberVal := parser.GetValue(member.ValueIdx)
				_ = parser.GetString(memberVal)
			}
		}
	})

	b.Run("GetStringBytes", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < val.ChildCount; j++ {
				member := parser.GetMember(val.FirstChild + j)
				memberVal := parser.GetValue(member.ValueIdx)
				_ = parser.GetStringBytes(memberVal)
			}
		}
	})
}
