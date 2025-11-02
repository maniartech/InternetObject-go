package parsers

import (
	"testing"
)

// Benchmarks for tokenizer critical paths
// Target: Zero allocations on hot paths

func BenchmarkTokenizer_SimpleString(b *testing.B) {
	input := `"hello world"`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_Numbers(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"integer", "42"},
		{"float", "3.14159"},
		{"negative", "-123"},
		{"hex", "0xFF"},
		{"octal", "0o755"},
		{"binary", "0b1010"},
		{"scientific", "1.23e-4"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				tokenizer := NewTokenizer(tt.input)
				_, err := tokenizer.Tokenize()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkTokenizer_Booleans(b *testing.B) {
	input := "true"
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_Null(b *testing.B) {
	input := "null"
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_SimpleObject(b *testing.B) {
	input := `{name: "John", age: 30, active: true}`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_SimpleArray(b *testing.B) {
	input := `[1, 2, 3, 4, 5, 6, 7, 8, 9, 10]`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_NestedStructure(b *testing.B) {
	input := `{user: {name: "John", address: {city: "NYC", zip: 10001}}}`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_Collection(b *testing.B) {
	input := `~ {id: 1, name: "Item1"}
~ {id: 2, name: "Item2"}
~ {id: 3, name: "Item3"}`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_Sections(b *testing.B) {
	input := `---
{id: 1}
---
{id: 2}`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_Comments(b *testing.B) {
	input := `# This is a comment
{name: "test"} # inline comment
# Another comment`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_EscapeSequences(b *testing.B) {
	input := `"Hello\nWorld\t\"quoted\"\u0041"`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_AnnotatedString_Raw(b *testing.B) {
	input := `r"raw\nstring"`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_AnnotatedString_Byte(b *testing.B) {
	input := `b"SGVsbG8gV29ybGQ="`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_AnnotatedString_DateTime(b *testing.B) {
	input := `dt"2023-10-29T15:30:00Z"`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTokenizer_RealWorldDocument(b *testing.B) {
	input := `# User database
$userSchema: {
  name: string,
  age: number,
  email: string
}

--- users: $userSchema

~ John Doe, 30, john@example.com
~ Jane Smith, 25, jane@example.com
~ Bob Johnson, 35, bob@example.com

--- summary

{
  total: 3,
  active: 2,
  lastUpdate: dt"2023-10-29T15:30:00Z"
}`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer := NewTokenizer(input)
		_, err := tokenizer.Tokenize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmarks for specific tokenizer methods (critical paths)

func BenchmarkTokenizer_ParseNumber_Integer(b *testing.B) {
	tokenizer := NewTokenizer("12345")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer.pos = 0
		tokenizer.row = 1
		tokenizer.col = 1
		tokenizer.parseNumber()
	}
}

func BenchmarkTokenizer_ParseNumber_Float(b *testing.B) {
	tokenizer := NewTokenizer("123.456")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer.pos = 0
		tokenizer.row = 1
		tokenizer.col = 1
		tokenizer.parseNumber()
	}
}

func BenchmarkTokenizer_ParseRegularString(b *testing.B) {
	tokenizer := NewTokenizer(`"hello world"`)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer.pos = 0
		tokenizer.row = 1
		tokenizer.col = 1
		tokenizer.parseRegularString('"')
	}
}

func BenchmarkTokenizer_SkipWhitespaces(b *testing.B) {
	tokenizer := NewTokenizer("     \t\t\n    test")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer.pos = 0
		tokenizer.row = 1
		tokenizer.col = 1
		tokenizer.skipWhitespaces()
	}
}

func BenchmarkTokenizer_Advance(b *testing.B) {
	tokenizer := NewTokenizer("abcdefghijklmnopqrstuvwxyz")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tokenizer.pos = 0
		tokenizer.row = 1
		tokenizer.col = 1
		for tokenizer.pos < tokenizer.inputLength {
			tokenizer.advance(1)
		}
	}
}

// Character classification benchmarks

func BenchmarkIsDigit(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = isDigit('5')
		_ = isDigit('a')
	}
}

func BenchmarkIsHexDigit(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = isHexDigit('F')
		_ = isHexDigit('g')
	}
}

func BenchmarkIsWhitespace(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = isWhitespace(' ')
		_ = isWhitespace('a')
	}
}

func BenchmarkIsSpecialSymbol(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = isSpecialSymbol('{')
		_ = isSpecialSymbol('a')
	}
}
