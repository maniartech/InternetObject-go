package parsers

import "testing"

func TestLexer(t *testing.T) {
	l := NewLexer("Test")
	print(l)
}
