package parsers

import (
	"testing"
)

// Benchmark object pooling vs regular allocation
func BenchmarkTokenAllocation_WithPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := GetToken()
		t.Type = TokenString
		t.Value = "test"
		t.Raw = "test"
		PutToken(t)
	}
}

func BenchmarkTokenAllocation_WithoutPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t := &Token{
			Type:  TokenString,
			Value: "test",
			Raw:   "test",
		}
		_ = t
	}
}

func BenchmarkParallelTokenizer_SmallInput(b *testing.B) {
	input := `{name: "John", age: 30}`
	pt := NewParallelTokenizer()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokens, err := pt.Tokenize(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = tokens
	}
}

func BenchmarkParallelTokenizer_MediumInput(b *testing.B) {
	input := `
---
users
name, age, email
---
John Doe, 30, john@example.com
Jane Smith, 25, jane@example.com
Bob Johnson, 35, bob@example.com
Alice Williams, 28, alice@example.com
Charlie Brown, 32, charlie@example.com
Diana Prince, 29, diana@example.com
Ethan Hunt, 34, ethan@example.com
Fiona Green, 26, fiona@example.com
George Miller, 31, george@example.com
Hannah White, 27, hannah@example.com
`
	pt := NewParallelTokenizer()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokens, err := pt.Tokenize(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = tokens
	}
}

func BenchmarkParallelTokenizer_LargeInput(b *testing.B) {
	// Create large input with multiple sections
	input := `
---
users
name, age, email, city, country
---
`
	for i := 0; i < 100; i++ {
		input += "John Doe, 30, john@example.com, New York, USA\n"
	}

	input += `
---
orders
id, userId, product, quantity, price
---
`
	for i := 0; i < 100; i++ {
		input += "1001, 42, Premium Widget, 5, 99.99\n"
	}

	input += `
---
products
sku, name, price, stock, category
---
`
	for i := 0; i < 100; i++ {
		input += "SKU-001, Widget, 19.99, 100, Electronics\n"
	}

	pt := NewParallelTokenizer()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokens, err := pt.Tokenize(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = tokens
	}
}

// Compare parallel vs sequential tokenizer
func BenchmarkTokenizer_Sequential_Large(b *testing.B) {
	// Same large input as parallel benchmark
	input := `
---
users
name, age, email, city, country
---
`
	for i := 0; i < 100; i++ {
		input += "John Doe, 30, john@example.com, New York, USA\n"
	}

	input += `
---
orders
id, userId, product, quantity, price
---
`
	for i := 0; i < 100; i++ {
		input += "1001, 42, Premium Widget, 5, 99.99\n"
	}

	input += `
---
products
sku, name, price, stock, category
---
`
	for i := 0; i < 100; i++ {
		input += "SKU-001, Widget, 19.99, 100, Electronics\n"
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		t := NewTokenizer(input)
		tokens, err := t.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
		_ = tokens
	}
}

// Benchmark pooled slices
func BenchmarkSliceAllocation_WithPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		slice := GetTokenSlice()
		for j := 0; j < 10; j++ {
			*slice = append(*slice, &Token{})
		}
		PutTokenSlice(slice)
	}
}

func BenchmarkSliceAllocation_WithoutPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		slice := make([]*Token, 0, 64)
		for j := 0; j < 10; j++ {
			slice = append(slice, &Token{})
		}
		_ = slice
	}
}
