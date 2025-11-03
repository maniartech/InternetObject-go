package parsers

import (
	"testing"
)

// TestParser_CollectionErrorRecovery tests that parser continues parsing
// after encountering an error in a collection item
func TestParser_CollectionErrorRecovery(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ {unclosed: "object"
~ name: "Bob", age: 30
`
	doc, err := ParseString(input)

	// Parser should return a result even with errors in collection items
	if doc == nil {
		t.Fatal("Expected document to be non-nil even with collection errors")
	}

	// Should still have an error (last error from collection)
	if err == nil {
		t.Error("Expected error to be returned from collection parsing")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	// Should have 3 children: valid, error, valid
	if len(coll.Children) != 3 {
		t.Fatalf("Expected 3 items (including error node), got %d", len(coll.Children))
	}

	// Check that first item is valid ObjectNode
	if _, ok := coll.Children[0].(*ObjectNode); !ok {
		t.Error("Expected ObjectNode at index 0")
	}

	// Check that middle one is ErrorNode
	errorNode, ok := coll.Children[1].(*ErrorNode)
	if !ok {
		t.Error("Expected ErrorNode at index 1")
	} else {
		if errorNode.Error == nil {
			t.Error("ErrorNode should contain an error")
		}
	}

	// Check that last item is valid ObjectNode
	if _, ok := coll.Children[2].(*ObjectNode); !ok {
		t.Error("Expected ObjectNode at index 2")
	}
}

// TestParser_MultipleCollectionErrors tests recovery from multiple errors
func TestParser_MultipleCollectionErrors(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ {unclosed: "obj1"
~ [1,2,3
~ name: "Bob", age: 30
~ missing:
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil even with multiple errors")
	}

	if err == nil {
		t.Error("Expected error to be returned from collection parsing")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	// Should have 5 items: valid, error, error, valid, error
	if len(coll.Children) != 5 {
		t.Fatalf("Expected 5 items, got %d", len(coll.Children))
	}

	// Count ErrorNodes
	errorCount := 0
	validCount := 0
	for i, child := range coll.Children {
		if _, ok := child.(*ErrorNode); ok {
			errorCount++
			t.Logf("ErrorNode found at index %d", i)
		} else if _, ok := child.(*ObjectNode); ok {
			validCount++
			t.Logf("Valid ObjectNode found at index %d", i)
		}
	}

	if errorCount != 3 {
		t.Errorf("Expected 3 error nodes, got %d", errorCount)
	}

	if validCount != 2 {
		t.Errorf("Expected 2 valid nodes, got %d", validCount)
	}
}

// TestParser_CollectionErrorAtStart tests error in first collection item
func TestParser_CollectionErrorAtStart(t *testing.T) {
	input := `
~ {unclosed: "object"
~ name: "Bob", age: 30
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	if len(coll.Children) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(coll.Children))
	}

	// First should be error
	if _, ok := coll.Children[0].(*ErrorNode); !ok {
		t.Error("Expected ErrorNode at index 0")
	}

	// Second should be valid
	if _, ok := coll.Children[1].(*ObjectNode); !ok {
		t.Error("Expected ObjectNode at index 1")
	}

	// Should still return an error
	if err == nil {
		t.Error("Expected error to be returned")
	}
}

// TestParser_CollectionErrorAtEnd tests error in last collection item
func TestParser_CollectionErrorAtEnd(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ name: "Bob", age: 30
~ {unclosed
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	if len(coll.Children) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(coll.Children))
	}

	// First two should be valid
	if _, ok := coll.Children[0].(*ObjectNode); !ok {
		t.Error("Expected ObjectNode at index 0")
	}
	if _, ok := coll.Children[1].(*ObjectNode); !ok {
		t.Error("Expected ObjectNode at index 1")
	}

	// Last should be error
	if _, ok := coll.Children[2].(*ErrorNode); !ok {
		t.Error("Expected ErrorNode at index 2")
	}

	if err == nil {
		t.Error("Expected error to be returned")
	}
}

// TestParser_CollectionAllErrors tests collection with all items failing
func TestParser_CollectionAllErrors(t *testing.T) {
	input := `
~ {unclosed1
~ [unclosed2
~ unclosed3:
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	if len(coll.Children) != 3 {
		t.Fatalf("Expected 3 items (all errors), got %d", len(coll.Children))
	}

	// All should be ErrorNodes
	for i, child := range coll.Children {
		if _, ok := child.(*ErrorNode); !ok {
			t.Errorf("Expected ErrorNode at index %d, got %T", i, child)
		}
	}

	if err == nil {
		t.Error("Expected error to be returned")
	}
}

// TestParser_CollectionValidAfterError tests that valid items parse correctly after error
func TestParser_CollectionValidAfterError(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ {unclosed: "object"
~ name: "Charlie", city: "NYC"
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	// Check that valid items have correct data
	if obj, ok := coll.Children[0].(*ObjectNode); ok {
		if len(obj.Members) != 2 {
			t.Errorf("First item should have 2 members, got %d", len(obj.Members))
		}
		// Verify first member key is "name"
		if obj.Members[0].Key != nil && obj.Members[0].Key.Value != "name" {
			t.Errorf("Expected first member key 'name', got '%v'", obj.Members[0].Key.Value)
		}
	} else {
		t.Error("First item should be ObjectNode")
	}

	// Middle item is error - already tested

	// Check third item
	if obj, ok := coll.Children[2].(*ObjectNode); ok {
		if len(obj.Members) != 2 {
			t.Errorf("Third item should have 2 members, got %d", len(obj.Members))
		}
		// Verify first member key is "name"
		if obj.Members[0].Key != nil && obj.Members[0].Key.Value != "name" {
			t.Errorf("Expected first member key 'name', got '%v'", obj.Members[0].Key.Value)
		}
	} else {
		t.Error("Third item should be ObjectNode")
	}

	if err == nil {
		t.Error("Expected error to be returned")
	}
}

// TestParser_CollectionErrorThenSection tests that section separator stops error recovery
func TestParser_CollectionErrorThenSection(t *testing.T) {
	input := `
--- section1
~ name: "Alice"
~ {unclosed: "object"
--- section2
~ name: "Bob"
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	// Should have 2 sections
	if len(doc.Sections) != 2 {
		t.Fatalf("Expected 2 sections, got %d", len(doc.Sections))
	}

	// First section should have collection with 2 items
	section1 := doc.Sections[0]
	coll1, ok := section1.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section 1 child is not a CollectionNode")
	}

	if len(coll1.Children) != 2 {
		t.Fatalf("Section 1 should have 2 items, got %d", len(coll1.Children))
	}

	// Second item should be ErrorNode
	if _, ok := coll1.Children[1].(*ErrorNode); !ok {
		t.Error("Expected ErrorNode at section 1, index 1")
	}

	// Second section should parse normally
	section2 := doc.Sections[1]
	coll2, ok := section2.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section 2 child is not a CollectionNode")
	}

	if len(coll2.Children) != 1 {
		t.Fatalf("Section 2 should have 1 item, got %d", len(coll2.Children))
	}

	// Second section item should be valid
	if _, ok := coll2.Children[0].(*ObjectNode); !ok {
		t.Error("Expected ObjectNode in section 2")
	}

	if err == nil {
		t.Error("Expected error to be returned from section 1")
	}
}

// TestParser_CollectionErrorPosition tests that ErrorNode has correct position
func TestParser_CollectionErrorPosition(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ {unclosed: "object"
~ name: "Bob", age: 30
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	errorNode, ok := coll.Children[1].(*ErrorNode)
	if !ok {
		t.Fatal("Expected ErrorNode at index 1")
	}

	// ErrorNode should have position information
	pos := errorNode.GetStartPos()
	if pos.Row <= 0 {
		t.Error("ErrorNode should have valid row position")
	}
	if pos.Col <= 0 {
		t.Error("ErrorNode should have valid column position")
	}

	// The error should be a SyntaxError
	if errorNode.Error != nil {
		if syntaxErr, ok := errorNode.Error.(*SyntaxError); ok {
			t.Logf("Captured error: %s at row %d, col %d", syntaxErr.Message, pos.Row, pos.Col)
		} else {
			t.Logf("Error type: %T, message: %v", errorNode.Error, errorNode.Error)
		}
	}

	if err == nil {
		t.Error("Expected error to be returned")
	}
}

// TestParser_SkipToNextCollectionItem tests the skip mechanism
func TestParser_SkipToNextCollectionItem(t *testing.T) {
	// This input has tokens after the error that should be skipped
	input := `
~ name: "Alice"
~ {unclosed: "object", more: "garbage"
~ name: "Bob"
`
	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document to be non-nil")
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	// Should have exactly 3 items (not more due to garbage)
	if len(coll.Children) != 3 {
		t.Fatalf("Expected 3 items, got %d (skip should prevent garbage from creating extra items)", len(coll.Children))
	}

	// Middle should be error
	if _, ok := coll.Children[1].(*ErrorNode); !ok {
		t.Error("Expected ErrorNode at index 1")
	}

	// Last should be valid (proves skip worked)
	if obj, ok := coll.Children[2].(*ObjectNode); ok {
		if obj.Members[0].Key != nil && obj.Members[0].Key.Value != "name" {
			t.Error("Last item should have 'name' key, proving parser recovered correctly")
		}
	} else {
		t.Error("Expected ObjectNode at index 2")
	}

	if err == nil {
		t.Error("Expected error to be returned")
	}
}
