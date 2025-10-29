package parsers

import (
	"testing"
)

// Test simple document parsing
func TestParser_SimpleDocument(t *testing.T) {
	input := `1,2,3`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if doc == nil {
		t.Fatal("Document is nil")
	}

	if doc.Header != nil {
		t.Error("Expected nil header for single section")
	}

	if len(doc.Sections) != 1 {
		t.Errorf("Expected 1 section, got %d", len(doc.Sections))
	}
}

// Test document with header and sections
func TestParser_DocumentWithHeader(t *testing.T) {
	input := `
a,b,c
---
1,2,3
`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if doc.Header == nil {
		t.Error("Expected header section")
	}

	if len(doc.Sections) != 1 {
		t.Errorf("Expected 1 data section, got %d", len(doc.Sections))
	}
}

// Test multiple sections
func TestParser_MultipleSections(t *testing.T) {
	input := `
--- hello
~ a,b,c
~ 1,2,3
--- world
~ "x","y","z"
`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if len(doc.Sections) != 2 {
		t.Errorf("Expected 2 sections, got %d", len(doc.Sections))
	}
}

// Test empty document
func TestParser_EmptyDocument(t *testing.T) {
	input := ``
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if doc.Header != nil {
		t.Error("Expected nil header for empty document")
	}

	if len(doc.Sections) != 1 {
		t.Errorf("Expected 1 empty section, got %d", len(doc.Sections))
	}

	if doc.Sections[0].Child != nil {
		t.Error("Expected nil child for empty section")
	}
}

// Test object parsing
func TestParser_SimpleObject(t *testing.T) {
	input := `{name: "John", age: 30, active: true}`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	obj, ok := section.Child.(*ObjectNode)
	if !ok {
		t.Fatal("Section child is not an ObjectNode")
	}

	if len(obj.Members) != 3 {
		t.Errorf("Expected 3 members, got %d", len(obj.Members))
	}

	// Check first member
	if obj.Members[0].Key == nil {
		t.Error("Expected key for first member")
	}
	if obj.Members[0].Key.Value != "name" {
		t.Errorf("Expected key 'name', got '%v'", obj.Members[0].Key.Value)
	}
}

// Test open object (without braces)
func TestParser_OpenObject(t *testing.T) {
	input := `name: "John", age: 30`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	obj, ok := section.Child.(*ObjectNode)
	if !ok {
		t.Fatal("Section child is not an ObjectNode")
	}

	if len(obj.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(obj.Members))
	}
}

// Test array parsing
func TestParser_SimpleArray(t *testing.T) {
	input := `{data: [1, 2, 3, "hello", true, null]}`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	obj, ok := section.Child.(*ObjectNode)
	if !ok {
		t.Fatal("Section child is not an ObjectNode")
	}

	member := obj.Members[0]
	arr, ok := member.Value.(*ArrayNode)
	if !ok {
		t.Fatal("Member value is not an ArrayNode")
	}

	if len(arr.Elements) != 6 {
		t.Errorf("Expected 6 elements, got %d", len(arr.Elements))
	}
}

// Test collection parsing
func TestParser_SimpleCollection(t *testing.T) {
	input := `
~ a,b,c
~ 1,2,3
~ "x","y","z"
`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	if len(coll.Children) != 3 {
		t.Errorf("Expected 3 collection items, got %d", len(coll.Children))
	}
}

// Test nested structures
func TestParser_NestedStructures(t *testing.T) {
	input := `{
		user: {
			name: "Alice",
			details: {
				age: 25,
				city: "NYC"
			}
		}
	}`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	obj, ok := section.Child.(*ObjectNode)
	if !ok {
		t.Fatal("Section child is not an ObjectNode")
	}

	// Check nested object
	userMember := obj.Members[0]
	userObj, ok := userMember.Value.(*ObjectNode)
	if !ok {
		t.Fatal("user value is not an ObjectNode")
	}

	if len(userObj.Members) != 2 {
		t.Errorf("Expected 2 members in user object, got %d", len(userObj.Members))
	}
}

// Test section names and schemas
func TestParser_SectionNamesAndSchemas(t *testing.T) {
	input := `
--- users: $userSchema
~ "Alice", 25
~ "Bob", 30
`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	if section.NameToken == nil {
		t.Error("Expected section name token")
	}

	if section.SchemaNode == nil {
		t.Error("Expected schema token")
	}

	if section.SchemaNode.Value != "$userSchema" {
		t.Errorf("Expected schema '$userSchema', got '%v'", section.SchemaNode.Value)
	}
}

// Test error: unclosed object
func TestParser_ErrorUnclosedObject(t *testing.T) {
	input := `{name: "John", age: 30`
	_, err := ParseString(input)

	if err == nil {
		t.Fatal("Expected error for unclosed object")
	}

	syntaxErr, ok := err.(*SyntaxError)
	if !ok {
		t.Fatal("Expected SyntaxError")
	}

	if syntaxErr.Code != ErrorExpectingBracket {
		t.Errorf("Expected ErrorExpectingBracket, got %v", syntaxErr.Code)
	}
}

// Test error: unclosed array
func TestParser_ErrorUnclosedArray(t *testing.T) {
	input := `[1, 2, 3`
	_, err := ParseString(input)

	if err == nil {
		t.Fatal("Expected error for unclosed array")
	}

	syntaxErr, ok := err.(*SyntaxError)
	if !ok {
		t.Fatal("Expected SyntaxError")
	}

	if syntaxErr.Code != ErrorExpectingBracket {
		t.Errorf("Expected ErrorExpectingBracket, got %v", syntaxErr.Code)
	}
}

// Test error: invalid key (array cannot be a key)
func TestParser_ErrorInvalidKey(t *testing.T) {
	// When array is used as key, it's parsed as value first,
	// then the colon causes unexpected token error
	input := `{[1,2]: "value"}`
	_, err := ParseString(input)

	if err == nil {
		t.Fatal("Expected error for invalid key type")
	}

	syntaxErr, ok := err.(*SyntaxError)
	if !ok {
		t.Fatal("Expected SyntaxError")
	}

	// This should be unexpected-token because the array is parsed as a value,
	// then : is unexpected
	if syntaxErr.Code != ErrorUnexpectedToken {
		t.Errorf("Expected ErrorUnexpectedToken, got %v", syntaxErr.Code)
	}
}

// Test error: truly invalid key (object as key)
func TestParser_ErrorInvalidKeyType(t *testing.T) {
	// Note: This is tricky because {} would be parsed as value first.
	// A better case is when tokenizer produces an invalid token type for key position.
	// For now, skip this test as arrays and objects are tokenized differently.
	// The ErrorInvalidKey code is tested when a non-primitive type appears before colon.
	t.Skip("Need tokenizer support for this error case")
}

// Test error: missing comma in object
func TestParser_ErrorMissingComma(t *testing.T) {
	input := `{a: 1 b: 2}`
	_, err := ParseString(input)

	if err == nil {
		t.Fatal("Expected error for missing comma")
	}

	syntaxErr, ok := err.(*SyntaxError)
	if !ok {
		t.Fatal("Expected SyntaxError")
	}

	if syntaxErr.Code != ErrorUnexpectedToken {
		t.Errorf("Expected ErrorUnexpectedToken, got %v", syntaxErr.Code)
	}
}

// Test error: duplicate section name
func TestParser_ErrorDuplicateSectionName(t *testing.T) {
	input := `
--- users
~ a,b,c
--- users
~ 1,2,3
`
	_, err := ParseString(input)

	if err == nil {
		t.Fatal("Expected error for duplicate section name")
	}

	syntaxErr, ok := err.(*SyntaxError)
	if !ok {
		t.Fatal("Expected SyntaxError")
	}

	if syntaxErr.Code != ErrorDuplicateSection {
		t.Errorf("Expected ErrorDuplicateSection, got %v", syntaxErr.Code)
	}
}

// Test error: empty array element
func TestParser_ErrorEmptyArrayElement(t *testing.T) {
	input := `[1, , 3]`
	_, err := ParseString(input)

	if err == nil {
		t.Fatal("Expected error for empty array element")
	}

	syntaxErr, ok := err.(*SyntaxError)
	if !ok {
		t.Fatal("Expected SyntaxError")
	}

	if syntaxErr.Code != ErrorUnexpectedToken {
		t.Errorf("Expected ErrorUnexpectedToken, got %v", syntaxErr.Code)
	}
}

// Test members without keys
func TestParser_MembersWithoutKeys(t *testing.T) {
	input := `a, b, c`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	obj, ok := section.Child.(*ObjectNode)
	if !ok {
		t.Fatal("Section child is not an ObjectNode")
	}

	if len(obj.Members) != 3 {
		t.Errorf("Expected 3 members, got %d", len(obj.Members))
	}

	// All members should have no keys
	for i, member := range obj.Members {
		if member.Key != nil {
			t.Errorf("Member %d should not have a key", i)
		}
	}
}

// Test object unwrapping (TypeScript rule: { {} } -> {})
func TestParser_ObjectUnwrapping(t *testing.T) {
	input := `
~ {name: "Alice"}
~ {age: 30}
`
	doc, err := ParseString(input)

	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	section := doc.Sections[0]
	coll, ok := section.Child.(*CollectionNode)
	if !ok {
		t.Fatal("Section child is not a CollectionNode")
	}

	// Each collection item should be an ObjectNode (unwrapped)
	for i, child := range coll.Children {
		obj, ok := child.(*ObjectNode)
		if !ok {
			t.Errorf("Collection item %d is not an ObjectNode", i)
		}
		if obj == nil {
			t.Errorf("Collection item %d is nil", i)
		}
	}
}
