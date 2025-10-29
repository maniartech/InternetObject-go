package parsers

import "fmt"

// Position represents a specific location in the source code.
// It tracks both the byte offset and the human-readable row/column position.
type Position struct {
	Pos int // Byte offset in the source (0-indexed)
	Row int // Line number (1-indexed)
	Col int // Column number (1-indexed)
}

// PositionRange represents a range in the source code from start to end position.
// This is used to track the location of tokens and AST nodes.
type PositionRange struct {
	Start Position // Starting position
	End   Position // Ending position
}

// NewPosition creates a new Position at the specified location.
func NewPosition(pos, row, col int) Position {
	return Position{
		Pos: pos,
		Row: row,
		Col: col,
	}
}

// NewPositionRange creates a new PositionRange with the given start and end positions.
func NewPositionRange(start, end Position) PositionRange {
	return PositionRange{
		Start: start,
		End:   end,
	}
}

// String returns a human-readable representation of the position.
func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Row, p.Col)
}

// String returns a human-readable representation of the position range.
func (pr PositionRange) String() string {
	if pr.Start.Row == pr.End.Row {
		return fmt.Sprintf("%d:%d-%d", pr.Start.Row, pr.Start.Col, pr.End.Col)
	}
	return fmt.Sprintf("%d:%d-%d:%d", pr.Start.Row, pr.Start.Col, pr.End.Row, pr.End.Col)
}

// IsValid returns true if the position has valid values.
func (p Position) IsValid() bool {
	return p.Pos >= 0 && p.Row > 0 && p.Col > 0
}

// IsValid returns true if the position range has valid positions.
func (pr PositionRange) IsValid() bool {
	return pr.Start.IsValid() && pr.End.IsValid()
}

// GetStartPos returns the starting position of the range.
// This provides compatibility with the Node interface.
func (pr PositionRange) GetStartPos() Position {
	return pr.Start
}

// GetEndPos returns the ending position of the range.
// This provides compatibility with the Node interface.
func (pr PositionRange) GetEndPos() Position {
	return pr.End
}
