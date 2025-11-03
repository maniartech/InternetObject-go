package parsers

import (
	"testing"
)

func TestBaseNode_GetPositions(t *testing.T) {
	start := NewPosition(0, 1, 1)
	end := NewPosition(10, 1, 11)
	posRange := NewPositionRange(start, end)

	base := BaseNode{Position: posRange}

	if base.GetStartPos() != start {
		t.Errorf("GetStartPos() = %v, want %v", base.GetStartPos(), start)
	}

	if base.GetEndPos() != end {
		t.Errorf("GetEndPos() = %v, want %v", base.GetEndPos(), end)
	}
}

func TestDocumentNode(t *testing.T) {
	header := &SectionNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(5, 1, 6))},
	}
	section := &SectionNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(6, 2, 1), NewPosition(10, 2, 5))},
	}

	doc := NewDocumentNode(header, []*SectionNode{section}, nil)

	if doc.NodeType() != "DocumentNode" {
		t.Errorf("NodeType() = %v, want DocumentNode", doc.NodeType())
	}

	if doc.Header != header {
		t.Error("Header not set correctly")
	}

	if len(doc.Sections) != 1 || doc.Sections[0] != section {
		t.Error("Sections not set correctly")
	}

	// Check position calculation
	if doc.GetStartPos().Pos != 0 {
		t.Errorf("Start position = %d, want 0", doc.GetStartPos().Pos)
	}
}

func TestSectionNode(t *testing.T) {
	child := &CollectionNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(5, 1, 6))},
	}
	nameToken := &Token{
		Type:     TokenString,
		Value:    "users",
		Position: NewPositionRange(NewPosition(10, 2, 1), NewPosition(15, 2, 6)),
	}
	schemaToken := &Token{
		Type:     TokenString,
		Value:    "$userSchema",
		Position: NewPositionRange(NewPosition(20, 3, 1), NewPosition(25, 3, 6)),
	}

	section := NewSectionNode(child, nameToken, schemaToken)

	if section.NodeType() != "SectionNode" {
		t.Errorf("NodeType() = %v, want SectionNode", section.NodeType())
	}

	if section.GetName() != "users" {
		t.Errorf("GetName() = %v, want users", section.GetName())
	}

	if section.GetSchemaName() != "$userSchema" {
		t.Errorf("GetSchemaName() = %v, want $userSchema", section.GetSchemaName())
	}

	// Test with nil tokens
	section2 := NewSectionNode(child, nil, nil)
	if section2.GetName() != "unnamed" {
		t.Errorf("GetName() with nil token = %v, want unnamed", section2.GetName())
	}
	if section2.GetSchemaName() != "" {
		t.Errorf("GetSchemaName() with nil schema = %v, want empty string", section2.GetSchemaName())
	}
}

func TestCollectionNode(t *testing.T) {
	child1 := &TokenNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(5, 1, 6))},
		Token:    &Token{Type: TokenString, Value: "item1"},
	}
	child2 := &TokenNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(6, 1, 7), NewPosition(11, 1, 12))},
		Token:    &Token{Type: TokenString, Value: "item2"},
	}

	collection := NewCollectionNode([]Node{child1, child2})

	if collection.NodeType() != "CollectionNode" {
		t.Errorf("NodeType() = %v, want CollectionNode", collection.NodeType())
	}

	if len(collection.Children) != 2 {
		t.Errorf("Children length = %d, want 2", len(collection.Children))
	}

	// Check position calculation
	if collection.GetStartPos().Pos != 0 {
		t.Errorf("Start position = %d, want 0", collection.GetStartPos().Pos)
	}
	if collection.GetEndPos().Pos != 11 {
		t.Errorf("End position = %d, want 11", collection.GetEndPos().Pos)
	}
}

func TestObjectNode(t *testing.T) {
	openBracket := &Token{Type: TokenCurlyOpen, Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2))}
	closeBracket := &Token{Type: TokenCurlyClose, Position: NewPositionRange(NewPosition(20, 1, 21), NewPosition(21, 1, 22))}

	value := &TokenNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(2, 1, 3), NewPosition(10, 1, 11))},
		Token:    &Token{Type: TokenString, Value: "test"},
	}
	key := &Token{Type: TokenString, Value: "name", Position: NewPositionRange(NewPosition(2, 1, 3), NewPosition(6, 1, 7))}
	member := NewMemberNode(value, key)

	obj := NewObjectNode([]*MemberNode{member}, openBracket, closeBracket)

	if obj.NodeType() != "ObjectNode" {
		t.Errorf("NodeType() = %v, want ObjectNode", obj.NodeType())
	}

	if len(obj.Members) != 1 {
		t.Errorf("Members length = %d, want 1", len(obj.Members))
	}

	if obj.IsOpen {
		t.Error("IsOpen should be false")
	}

	// Test position calculation with brackets
	if obj.GetStartPos().Pos != 0 {
		t.Errorf("Start position = %d, want 0", obj.GetStartPos().Pos)
	}
	if obj.GetEndPos().Pos != 21 {
		t.Errorf("End position = %d, want 21", obj.GetEndPos().Pos)
	}

	// Test without brackets (open object)
	obj2 := NewObjectNode([]*MemberNode{member}, nil, nil)
	if !obj2.IsOpen {
		t.Error("IsOpen should be true")
	}
}

func TestArrayNode(t *testing.T) {
	openBracket := &Token{Type: TokenBracketOpen, Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2))}
	closeBracket := &Token{Type: TokenBracketClose, Position: NewPositionRange(NewPosition(20, 1, 21), NewPosition(21, 1, 22))}

	element := &TokenNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(2, 1, 3), NewPosition(5, 1, 6))},
		Token:    &Token{Type: TokenNumber, Value: 42},
	}

	array := NewArrayNode([]Node{element}, openBracket, closeBracket)

	if array.NodeType() != "ArrayNode" {
		t.Errorf("NodeType() = %v, want ArrayNode", array.NodeType())
	}

	if len(array.Elements) != 1 {
		t.Errorf("Elements length = %d, want 1", len(array.Elements))
	}

	// Check position calculation
	if array.GetStartPos().Pos != 0 {
		t.Errorf("Start position = %d, want 0", array.GetStartPos().Pos)
	}
}

func TestMemberNode(t *testing.T) {
	key := &Token{Type: TokenString, Value: "name", Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5))}
	value := &TokenNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(6, 1, 7), NewPosition(11, 1, 12))},
		Token:    &Token{Type: TokenString, Value: "John"},
	}

	member := NewMemberNode(value, key)

	if member.NodeType() != "MemberNode" {
		t.Errorf("NodeType() = %v, want MemberNode", member.NodeType())
	}

	if !member.HasKey() {
		t.Error("HasKey() should return true")
	}

	if member.Key != key {
		t.Error("Key not set correctly")
	}

	// Test without key
	member2 := NewMemberNode(value, nil)
	if member2.HasKey() {
		t.Error("HasKey() should return false when key is nil")
	}
}

func TestTokenNode(t *testing.T) {
	token := &Token{
		Type:     TokenString,
		Value:    "test",
		Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)),
	}

	node := NewTokenNode(token)

	if node.NodeType() != "TokenNode" {
		t.Errorf("NodeType() = %v, want TokenNode", node.NodeType())
	}

	if node.GetValue() != "test" {
		t.Errorf("GetValue() = %v, want test", node.GetValue())
	}

	if node.Token != token {
		t.Error("Token not set correctly")
	}

	// Check position inherited from token
	if node.GetStartPos().Pos != 0 {
		t.Errorf("Start position = %d, want 0", node.GetStartPos().Pos)
	}
}

func TestErrorNode(t *testing.T) {
	pos := NewPositionRange(NewPosition(5, 2, 3), NewPosition(10, 2, 8))
	err := NewSyntaxError(ErrorUnexpectedToken, "test error", pos)
	node := NewErrorNode(err, pos)

	if node.NodeType() != "ErrorNode" {
		t.Errorf("NodeType() = %v, want ErrorNode", node.NodeType())
	}

	if node.Error == nil {
		t.Error("Error not set")
	}

	// Check position
	if node.GetStartPos().Pos != 5 {
		t.Errorf("Start position = %d, want 5", node.GetStartPos().Pos)
	}
}
