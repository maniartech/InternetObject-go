package parsers

import (
	"testing"
)

// AST Node construction benchmarks - should have minimal allocations

func BenchmarkNewPosition(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewPosition(100, 10, 15)
	}
}

func BenchmarkNewPositionRange(b *testing.B) {
	start := NewPosition(0, 1, 1)
	end := NewPosition(10, 1, 11)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewPositionRange(start, end)
	}
}

func BenchmarkNewToken(b *testing.B) {
	pos := NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewToken(TokenString, "test", pos)
	}
}

func BenchmarkToken_Clone(b *testing.B) {
	token := NewToken(TokenString, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = token.Clone()
	}
}

func BenchmarkToken_IsStructural(b *testing.B) {
	token := NewToken(TokenCurlyOpen, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = token.IsStructural()
	}
}

func BenchmarkToken_IsValue(b *testing.B) {
	token := NewToken(TokenString, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = token.IsValue()
	}
}

func BenchmarkNewTokenNode(b *testing.B) {
	token := NewToken(TokenString, "test", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewTokenNode(token)
	}
}

func BenchmarkNewObjectNode(b *testing.B) {
	openBracket := NewToken(TokenCurlyOpen, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2)))
	closeBracket := NewToken(TokenCurlyClose, nil, NewPositionRange(NewPosition(10, 1, 11), NewPosition(11, 1, 12)))

	value := NewTokenNode(NewToken(TokenString, "test", NewPositionRange(NewPosition(2, 1, 3), NewPosition(6, 1, 7))))
	key := NewToken(TokenString, "name", NewPositionRange(NewPosition(2, 1, 3), NewPosition(6, 1, 7)))
	member := NewMemberNode(value, key)
	members := []*MemberNode{member}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewObjectNode(members, openBracket, closeBracket)
	}
}

func BenchmarkNewArrayNode(b *testing.B) {
	openBracket := NewToken(TokenBracketOpen, nil, NewPositionRange(NewPosition(0, 1, 1), NewPosition(1, 1, 2)))
	closeBracket := NewToken(TokenBracketClose, nil, NewPositionRange(NewPosition(10, 1, 11), NewPosition(11, 1, 12)))

	element := NewTokenNode(NewToken(TokenNumber, 42, NewPositionRange(NewPosition(2, 1, 3), NewPosition(4, 1, 5))))
	elements := []Node{element}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewArrayNode(elements, openBracket, closeBracket)
	}
}

func BenchmarkNewMemberNode(b *testing.B) {
	value := NewTokenNode(NewToken(TokenString, "test", NewPositionRange(NewPosition(5, 1, 6), NewPosition(9, 1, 10))))
	key := NewToken(TokenString, "name", NewPositionRange(NewPosition(0, 1, 1), NewPosition(4, 1, 5)))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewMemberNode(value, key)
	}
}

func BenchmarkNewCollectionNode(b *testing.B) {
	child1 := NewTokenNode(NewToken(TokenString, "item1", NewPositionRange(NewPosition(0, 1, 1), NewPosition(5, 1, 6))))
	child2 := NewTokenNode(NewToken(TokenString, "item2", NewPositionRange(NewPosition(6, 1, 7), NewPosition(11, 1, 12))))
	children := []Node{child1, child2}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewCollectionNode(children)
	}
}

func BenchmarkNewSectionNode(b *testing.B) {
	child := NewCollectionNode([]Node{})
	nameToken := NewToken(TokenString, "users", NewPositionRange(NewPosition(0, 1, 1), NewPosition(5, 1, 6)))
	schemaToken := NewToken(TokenString, "$schema", NewPositionRange(NewPosition(7, 1, 8), NewPosition(14, 1, 15)))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewSectionNode(child, nameToken, schemaToken)
	}
}

func BenchmarkNewDocumentNode(b *testing.B) {
	section := &SectionNode{
		BaseNode: BaseNode{Position: NewPositionRange(NewPosition(0, 1, 1), NewPosition(10, 1, 11))},
	}
	sections := []*SectionNode{section}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewDocumentNode(nil, sections, nil)
	}
}

// Error construction benchmarks

func BenchmarkNewSyntaxError(b *testing.B) {
	pos := NewPositionRange(NewPosition(5, 1, 6), NewPosition(10, 1, 11))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewSyntaxError(ErrorUnexpectedToken, "unexpected token", pos)
	}
}

func BenchmarkNewIOError(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewIOError(ErrorStringNotClosed, "string not closed")
	}
}

// Position utilities benchmarks

func BenchmarkPosition_String(b *testing.B) {
	pos := NewPosition(42, 10, 15)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = pos.String()
	}
}

func BenchmarkPositionRange_String(b *testing.B) {
	pr := NewPositionRange(NewPosition(0, 1, 1), NewPosition(10, 1, 11))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = pr.String()
	}
}

func BenchmarkPosition_IsValid(b *testing.B) {
	pos := NewPosition(42, 10, 15)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = pos.IsValid()
	}
}
