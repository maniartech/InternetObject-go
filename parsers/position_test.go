package parsers

import (
	"testing"
)

func TestPosition_String(t *testing.T) {
	pos := NewPosition(42, 10, 15)
	str := pos.String()

	if str != "10:15" {
		t.Errorf("String() = %s, want 10:15", str)
	}
}

func TestPosition_IsValid(t *testing.T) {
	validPos := NewPosition(0, 1, 1)
	if !validPos.IsValid() {
		t.Error("IsValid() should return true for valid position")
	}

	invalidPos := Position{Pos: -1, Row: 0, Col: 0}
	if invalidPos.IsValid() {
		t.Error("IsValid() should return false for invalid position")
	}
}

func TestPositionRange_String(t *testing.T) {
	start := NewPosition(0, 1, 1)
	end := NewPosition(10, 1, 11)
	pr := NewPositionRange(start, end)

	str := pr.String()
	if str != "1:1-11" {
		t.Errorf("String() = %s, want 1:1-11", str)
	}

	// Test multi-line range
	start2 := NewPosition(0, 1, 1)
	end2 := NewPosition(50, 3, 10)
	pr2 := NewPositionRange(start2, end2)

	str2 := pr2.String()
	if str2 != "1:1-3:10" {
		t.Errorf("String() for multi-line = %s, want 1:1-3:10", str2)
	}
}

func TestPositionRange_IsValid(t *testing.T) {
	validRange := NewPositionRange(NewPosition(0, 1, 1), NewPosition(10, 1, 11))
	if !validRange.IsValid() {
		t.Error("IsValid() should return true for valid range")
	}

	invalidRange := PositionRange{
		Start: Position{Pos: -1, Row: 0, Col: 0},
		End:   Position{Pos: 0, Row: 1, Col: 1},
	}
	if invalidRange.IsValid() {
		t.Error("IsValid() should return false for invalid range")
	}
}

func TestPositionRange_GetPositions(t *testing.T) {
	start := NewPosition(0, 1, 1)
	end := NewPosition(10, 1, 11)
	pr := NewPositionRange(start, end)

	if pr.GetStartPos() != start {
		t.Errorf("GetStartPos() = %v, want %v", pr.GetStartPos(), start)
	}

	if pr.GetEndPos() != end {
		t.Errorf("GetEndPos() = %v, want %v", pr.GetEndPos(), end)
	}
}
