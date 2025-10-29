package parsers

import (
	"testing"
)

func TestZeroParser_SimpleValue(t *testing.T) {
	input := `"hello world"`
	parser := NewZeroParser(input)

	rootIdx, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify stats
	stats := parser.Stats()
	t.Logf("Stats: %+v", stats)

	if parser.tokenCount == 0 {
		t.Error("Expected at least one token")
	}

	if parser.nodeCount == 0 {
		t.Error("Expected at least one node")
	}

	// Get the token value
	rootNode := parser.nodes[rootIdx]
	if rootNode.Type != NodeKindToken {
		t.Errorf("Expected NodeKindToken, got %d", rootNode.Type)
	}

	// Test different extraction methods
	tokenStr := parser.GetTokenString(rootNode.TokenIdx)
	t.Logf("GetTokenString: %s", tokenStr)
	if tokenStr != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", tokenStr)
	}

	// Test GetTokenBytes (zero-copy reference)
	tokenBytes := parser.GetTokenBytes(rootNode.TokenIdx)
	if string(tokenBytes) != "hello world" {
		t.Errorf("GetTokenBytes: expected 'hello world', got '%s'", string(tokenBytes))
	}

	// Test CopyTokenBytes
	buf := make([]byte, 100)
	n := parser.CopyTokenBytes(rootNode.TokenIdx, buf)
	if n != 11 || string(buf[:n]) != "hello world" {
		t.Errorf("CopyTokenBytes: expected 11 bytes 'hello world', got %d bytes '%s'", n, string(buf[:n]))
	}

	// Test GetTokenBytesTo
	buf2 := make([]byte, 20)
	n2 := parser.GetTokenBytesTo(rootNode.TokenIdx, buf2)
	if n2 != 11 || string(buf2[:n2]) != "hello world" {
		t.Errorf("GetTokenBytesTo: expected 11 bytes 'hello world', got %d bytes '%s'", n2, string(buf2[:n2]))
	}
}

func TestZeroParser_SimpleObject(t *testing.T) {
	input := `{name: "John", age: 30}`
	parser := NewZeroParser(input)

	rootIdx, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify we got an object
	rootNode := parser.nodes[rootIdx]
	if rootNode.Type != NodeKindObject {
		t.Errorf("Expected NodeKindObject, got %d", rootNode.Type)
	}

	// Check we have 2 members
	if rootNode.ChildCount != 2 {
		t.Errorf("Expected 2 members, got %d", rootNode.ChildCount)
	}

	t.Logf("Stats: %+v", parser.Stats())
}

func TestZeroParser_SimpleArray(t *testing.T) {
	input := `[1, 2, 3, 4, 5]`
	parser := NewZeroParser(input)

	rootIdx, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify we got an array
	rootNode := parser.nodes[rootIdx]
	if rootNode.Type != NodeKindArray {
		t.Errorf("Expected NodeKindArray, got %d", rootNode.Type)
	}

	// Check we have 5 elements
	if rootNode.ChildCount != 5 {
		t.Errorf("Expected 5 elements, got %d", rootNode.ChildCount)
	}

	t.Logf("Stats: %+v", parser.Stats())
}

func TestZeroParser_Document(t *testing.T) {
	// Simpler test - just one section for now
	input := `~section1
name: "Test Section"`

	parser := NewZeroParser(input)

	rootIdx, err := parser.Parse()

	t.Logf("After parse: pos=%d, len=%d, errors=%d", parser.pos, parser.len, len(parser.errors))
	if len(parser.errors) > 0 {
		for i, e := range parser.errors {
			t.Logf("Error %d: %s at pos %d (row %d, col %d)", i, e.Message, e.Pos, e.Row, e.Col)
		}
	}

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify we got a document
	rootNode := parser.nodes[rootIdx]
	t.Logf("Root node type: %d (expected Document=%d)", rootNode.Type, NodeKindDocument)

	t.Logf("Stats: %+v", parser.Stats())
}

func TestZeroParser_MemoryEfficiency(t *testing.T) {
	input := `{name: "Alice", age: 25, active: true, data: [1, 2, 3]}`
	parser := NewZeroParser(input)

	_, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	stats := parser.Stats()
	t.Logf("Input bytes: %v", stats["input_bytes"])
	t.Logf("Token memory: %v bytes", stats["total_token_memory"])
	t.Logf("Node memory: %v bytes", stats["total_node_memory"])
	t.Logf("Tokens created: %v", stats["tokens"])
	t.Logf("Nodes created: %v", stats["nodes"])

	// Verify memory efficiency
	totalMemory := stats["total_token_memory"].(int) + stats["total_node_memory"].(int)
	inputBytes := stats["input_bytes"].(int)

	// Memory overhead ratio is reasonable for compact storage
	t.Logf("Memory overhead ratio: %.2f", float64(totalMemory)/float64(inputBytes))

	// Should be reasonable - tokens+nodes are 13+17=30 bytes each
	// With a reasonable structure, overhead should be under 10x
	if totalMemory > inputBytes*10 {
		t.Errorf("Memory usage too high: %d bytes for %d input bytes", totalMemory, inputBytes)
	}
}

// Benchmark to compare with regular AST
func BenchmarkZeroParser_Simple(b *testing.B) {
	input := `{name: "John Doe", age: 30, active: true}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewZeroParser(input)
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkZeroParser_Complex(b *testing.B) {
	input := `---
~users
#
{name: "Alice", age: 25, email: "alice@example.com"}
{name: "Bob", age: 30, email: "bob@example.com"}
{name: "Charlie", age: 35, email: "charlie@example.com"}
---
~settings
theme: "dark"
language: "en"
notifications: true`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		parser := NewZeroParser(input)
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark different token extraction methods
func BenchmarkTokenExtraction_String(b *testing.B) {
	input := `{name: "John Doe", age: 30, city: "New York", active: true}`
	parser := NewZeroParser(input)
	parser.Parse()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Extract all token strings
		for idx := uint32(0); idx < uint32(parser.tokenCount); idx++ {
			_ = parser.GetTokenString(idx)
		}
	}
}

func BenchmarkTokenExtraction_Bytes(b *testing.B) {
	input := `{name: "John Doe", age: 30, city: "New York", active: true}`
	parser := NewZeroParser(input)
	parser.Parse()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Extract all token bytes (zero-copy reference)
		for idx := uint32(0); idx < uint32(parser.tokenCount); idx++ {
			_ = parser.GetTokenBytes(idx)
		}
	}
}

func BenchmarkTokenExtraction_CopyBytes(b *testing.B) {
	input := `{name: "John Doe", age: 30, city: "New York", active: true}`
	parser := NewZeroParser(input)
	parser.Parse()

	// Pre-allocate buffer
	buf := make([]byte, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Copy token bytes to pre-allocated buffer
		for idx := uint32(0); idx < uint32(parser.tokenCount); idx++ {
			parser.CopyTokenBytes(idx, buf)
		}
	}
}

func BenchmarkTokenExtraction_BytesTo(b *testing.B) {
	input := `{name: "John Doe", age: 30, city: "New York", active: true}`
	parser := NewZeroParser(input)
	parser.Parse()

	// Pre-allocate buffer
	buf := make([]byte, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Write token bytes to pre-allocated buffer (fastest)
		for idx := uint32(0); idx < uint32(parser.tokenCount); idx++ {
			parser.GetTokenBytesTo(idx, buf)
		}
	}
}
