package parsers

import (
	"testing"
)

// TestFastAST_SimpleCreation tests basic parser creation
func TestFastAST_SimpleCreation(t *testing.T) {
	input := "test, 123"
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		t.Fatalf("Tokenization failed: %v", err)
	}

	t.Logf("Tokens: %d", len(tokens))
	for i, tok := range tokens {
		t.Logf("  [%d] Type=%v Value=%v Raw=%s", i, tok.Type, tok.Value, tok.Raw)
	}

	// Create parser without calling Parse
	parser := NewFastASTParser(input, tokens)
	stats := parser.Stats()

	t.Logf("Parser stats: %+v", stats)
}

// TestFastAST_ParseValue tests parsing a single value
func TestFastAST_ParseValue(t *testing.T) {
	input := "test"
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		t.Fatalf("Tokenization failed: %v", err)
	}

	parser := NewFastASTParser(input, tokens)

	// Manually call parseValue to test it in isolation
	value := parser.parseValue()

	t.Logf("Parsed value type: %v", value.Type)
	t.Logf("Parser errors: %v", parser.GetErrors())
}

// TestFastAST_ParseObject tests parsing a simple object
func TestFastAST_ParseObject(t *testing.T) {
	input := "name, age"
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		t.Fatalf("Tokenization failed: %v", err)
	}

	t.Logf("Tokens for '%s': %d", input, len(tokens))
	for i, tok := range tokens {
		t.Logf("  [%d] Type=%v Value=%v Raw=%s", i, tok.Type, tok.Value, tok.Raw)
	}

	parser := NewFastASTParser(input, tokens)

	// Add position tracking
	t.Logf("Initial position: %d/%d", parser.pos, parser.tokenCount)

	// Manually call parseObject to test it in isolation
	obj := parser.parseObject()

	t.Logf("Final position: %d/%d", parser.pos, parser.tokenCount)
	t.Logf("Parsed object type: %v", obj.Type)
	t.Logf("Member count: %d", obj.MemberCount)
	t.Logf("Parser errors: %v", parser.GetErrors())

	stats := parser.Stats()
	t.Logf("Parser stats: %+v", stats)
}

// TestFastAST_FullParse tests the full parsing flow
func TestFastAST_FullParse(t *testing.T) {
	input := `name, age
---
John, 30`

	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		t.Fatalf("Tokenization failed: %v", err)
	}

	t.Logf("Tokens for document: %d", len(tokens))
	for i, tok := range tokens {
		t.Logf("  [%d] Type=%v SubType=%s Value=%v Raw=%s", i, tok.Type, tok.SubType, tok.Value, tok.Raw)
	}

	parser := NewFastASTParser(input, tokens)

	// This might hang - let's see where
	t.Log("Starting Parse()...")
	doc, err := parser.Parse()

	if err != nil {
		t.Logf("Parse error: %v", err)
	}

	if doc != nil {
		t.Logf("Document parsed successfully")
	}

	t.Logf("Parser errors: %v", parser.GetErrors())

	stats := parser.Stats()
	t.Logf("Parser stats: %+v", stats)
}
