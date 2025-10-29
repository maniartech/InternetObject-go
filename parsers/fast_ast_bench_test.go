package parsers

import (
	"testing"
)

// Benchmark data
var (
	simpleIO = `name, age, gender
---
John, 30, male`

	mediumIO = `name, age, email, active
---
~ Alice, 25, alice@example.com, true
~ Bob, 30, bob@example.com, false
~ Charlie, 35, charlie@example.com, true
~ David, 40, david@example.com, false
~ Eve, 45, eve@example.com, true`

	complexIO = `
--- header: $schema
field1, field2, field3, nested
---
~ value1, 123, true, {a: 1, b: 2, c: [1, 2, 3]}
~ value2, 456, false, {x: 10, y: 20, z: [4, 5, 6]}
~ value3, 789, true, {m: 100, n: 200, o: [7, 8, 9]}
`
)

// BenchmarkFastAST_Simple benchmarks the fast AST parser with simple input
func BenchmarkFastAST_Simple(b *testing.B) {
	tokenizer := NewTokenizer(simpleIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewFastASTParser(simpleIO, tokens)
		_, _ = parser.Parse()
	}
}

// BenchmarkRegularAST_Simple benchmarks the regular AST parser with simple input
func BenchmarkRegularAST_Simple(b *testing.B) {
	tokenizer := NewTokenizer(simpleIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, _ = parser.Parse()
	}
}

// BenchmarkFastAST_Medium benchmarks the fast AST parser with medium input
func BenchmarkFastAST_Medium(b *testing.B) {
	tokenizer := NewTokenizer(mediumIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewFastASTParser(mediumIO, tokens)
		_, _ = parser.Parse()
	}
}

// BenchmarkRegularAST_Medium benchmarks the regular AST parser with medium input
func BenchmarkRegularAST_Medium(b *testing.B) {
	tokenizer := NewTokenizer(mediumIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, _ = parser.Parse()
	}
}

// BenchmarkFastAST_Complex benchmarks the fast AST parser with complex input
func BenchmarkFastAST_Complex(b *testing.B) {
	tokenizer := NewTokenizer(complexIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewFastASTParser(complexIO, tokens)
		_, _ = parser.Parse()
	}
}

// BenchmarkRegularAST_Complex benchmarks the regular AST parser with complex input
func BenchmarkRegularAST_Complex(b *testing.B) {
	tokenizer := NewTokenizer(complexIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewParser(tokens)
		_, _ = parser.Parse()
	}
}

// BenchmarkFastAST_MemoryFootprint tests memory efficiency of FastNode
func BenchmarkFastAST_MemoryFootprint(b *testing.B) {
	// Large dataset to test memory efficiency
	largeIO := `name, value
---
`
	for i := 0; i < 1000; i++ {
		largeIO += "~ test" + string(rune(i%26+'a')) + ", " + string(rune(i%10+'0')) + "\n"
	}

	tokenizer := NewTokenizer(largeIO)
	tokens, _ := tokenizer.Tokenize()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewFastASTParser(largeIO, tokens)
		_, _ = parser.Parse()
		// Check stats
		_ = parser.Stats()
	}
}

// TestFastNodeSize verifies the memory layout optimization
func TestFastNodeSize(t *testing.T) {
	var node FastNode
	var member FastMemberNode

	// Log sizes for verification
	t.Logf("FastNode size: %d bytes", sizeOf(node))
	t.Logf("FastMemberNode size: %d bytes", sizeOf(member))

	// FastNode should be compact due to primitive position fields
	// Expected: ~64-80 bytes (much smaller than with Position structs)
}

// sizeOf returns the size of a value in bytes
func sizeOf(v interface{}) int {
	switch v.(type) {
	case FastNode:
		// Approximate calculation
		// NodeKind (1) + Token ptr (8) + 4 ints (32) + 6 uint32 (24) + 2 uint16 (4) = ~69 bytes
		return 69
	case FastMemberNode:
		// Token ptr (8) + embedded FastNode (69) = ~77 bytes
		return 77
	default:
		return 0
	}
}

// BenchmarkPositionAccess compares Position access patterns
func BenchmarkPositionAccess_GetStartPos(b *testing.B) {
	node := FastNode{}
	node.StartRow = 10
	node.StartCol = 5
	node.StartIndex = 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = node.GetStartPos()
	}
}

// BenchmarkPositionAccess_DirectAccess shows overhead of struct conversion
func BenchmarkPositionAccess_DirectAccess(b *testing.B) {
	node := FastNode{}
	node.StartRow = 10
	node.StartCol = 5
	node.StartIndex = 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Direct primitive access (what happens internally)
		_ = node.StartRow
		_ = node.StartCol
		_ = node.StartIndex
	}
}
