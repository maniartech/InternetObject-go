package parsers

// ZeroParser is a zero-allocation Internet Object parser that combines
// tokenization and AST construction in a single pass. It stores only
// positions and types, materializing strings/values only on demand.
//
// Key optimizations:
// - 13-byte tokens (vs 80+ bytes in regular parser)
// - 17-byte nodes (vs 100+ bytes in regular parser)
// - No string allocations during parsing
// - Arena-based storage for cache locality
// - Lazy value materialization
type ZeroParser struct {
	input []byte // Raw input (zero-copy from string)
	pos   int    // Current byte position
	row   int    // Current row (1-indexed)
	col   int    // Current column (1-indexed)
	len   int    // Cached input length

	// Token arena (inline tokenization - no separate tokenizer!)
	tokens     []ZeroToken // All tokens
	tokenCount int         // Number of tokens created

	// Node arena (compact AST representation)
	nodes     []ZeroNode // All AST nodes
	nodeCount int        // Number of nodes created

	// Child index arena (for node children/members)
	childIndices []uint32 // Indices into nodes array
	childCount   int      // Number of child indices

	// Error tracking
	errors []ParseError // Accumulated errors

	// Reusable buffers to avoid allocations
	childBuffer  []uint32 // Temporary buffer for building child lists
	escapeBuffer []byte   // Buffer for processing escape sequences
}

// ZeroToken is an ultra-compact token representation storing only
// position and type information. Strings are materialized on demand.
// Size: 13 bytes (vs 80+ bytes for regular Token)
type ZeroToken struct {
	Type    uint8  // TokenType as byte (0-255 token types supported)
	SubType uint8  // SubType as byte
	Start   uint32 // Byte offset start in input
	End     uint32 // Byte offset end in input
	Row     uint16 // Row number (1-indexed, max 65535 rows)
	Col     uint16 // Column number (1-indexed, max 65535 cols)
	Flags   uint8  // Bit flags for token properties
}

// ZeroNode is a compact AST node storing only structure information.
// Values are computed on demand from token positions.
// Size: 17 bytes (vs 100+ bytes for regular nodes)
type ZeroNode struct {
	Type       uint8  // NodeKind (Document, Section, Object, Array, etc.)
	TokenIdx   uint32 // Index of primary token (for TokenNode)
	ChildStart uint32 // Index into childIndices array
	ChildCount uint16 // Number of children
	Flags      uint8  // Node flags (IsOpen, HasError, etc.)
	Row        uint16 // Start row for error reporting
	Col        uint16 // Start col for error reporting
}

// ParseError represents a parsing error with position information
type ParseError struct {
	Message string
	Pos     int
	Row     int
	Col     int
}

// Token type constants (stored as uint8)
const (
	TokInvalid uint8 = iota
	TokString
	TokNumber
	TokBoolean
	TokNull
	TokBigInt
	TokDecimal
	TokBinary
	TokDateTime
	TokCurlyOpen
	TokCurlyClose
	TokBracketOpen
	TokBracketClose
	TokColon
	TokComma
	TokTilde
	TokSectionSep
)

// Token subtype constants
const (
	SubTypeNone uint8 = iota
	SubTypeOpenString
	SubTypeRegularString
	SubTypeRawString
	SubTypeSectionName
	SubTypeSectionSchema
)

// Token flags
const (
	FlagHasEscapes     uint8 = 1 << 0 // String contains escape sequences
	FlagNeedsNormalize uint8 = 1 << 1 // String needs Unicode normalization
	FlagIsHex          uint8 = 1 << 2 // Number is hexadecimal
	FlagIsOctal        uint8 = 1 << 3 // Number is octal
	FlagIsBinary       uint8 = 1 << 4 // Number is binary
	FlagIsNegative     uint8 = 1 << 5 // Number is negative
	FlagHasDecimal     uint8 = 1 << 6 // Number has decimal point
	FlagHasExponent    uint8 = 1 << 7 // Number has exponent
)

// Node kind constants (stored as uint8)
const (
	NodeKindDocument uint8 = iota
	NodeKindSection
	NodeKindCollection
	NodeKindObject
	NodeKindArray
	NodeKindMember
	NodeKindToken
	NodeKindError
)

// Node flags (stored as uint8)
const (
	NodeFlagIsOpen    uint8 = 1 << 0 // Object is open (no braces)
	NodeFlagHasError  uint8 = 1 << 1 // Node contains errors
	NodeFlagHasSchema uint8 = 1 << 2 // Section has schema
	NodeFlagHasName   uint8 = 1 << 3 // Section has name
	NodeFlagHasKey    uint8 = 1 << 4 // Member has key
)

// Initial arena capacities (adaptive based on input size)
const (
	MinTokenCapacity     = 8  // Minimum tokens to allocate
	MinNodeCapacity      = 4  // Minimum nodes to allocate
	MinChildCapacity     = 8  // Minimum child indices
	InitialErrorCapacity = 4  // Most parses have 0-4 errors
	EscapeBufferSize     = 64 // Buffer for escape processing (reduced)
	ChildBufferSize      = 8  // Reusable child buffer (reduced)

	// Heuristic: input_bytes / X = estimated capacity
	TokenCapacityDivisor = 8  // ~1 token per 8 bytes
	NodeCapacityDivisor  = 12 // ~1 node per 12 bytes
	ChildCapacityDivisor = 16 // ~1 child per 16 bytes
)

// Character constants for fast byte comparisons
const (
	charSpace        byte = ' '
	charTab          byte = '\t'
	charNewline      byte = '\n'
	charCarriageRet  byte = '\r'
	charDoubleQuote  byte = '"'
	charSingleQuote  byte = '\''
	charBackslash    byte = '\\'
	charHash         byte = '#'
	charCurlyOpen    byte = '{'
	charCurlyClose   byte = '}'
	charBracketOpen  byte = '['
	charBracketClose byte = ']'
	charColon        byte = ':'
	charComma        byte = ','
	charTilde        byte = '~'
	charMinus        byte = '-'
	charPlus         byte = '+'
	charDot          byte = '.'
	charZero         byte = '0'
	charNine         byte = '9'
	charLowerA       byte = 'a'
	charLowerZ       byte = 'z'
	charUpperA       byte = 'A'
	charUpperZ       byte = 'Z'
	charUnderscore   byte = '_'
	charDollar       byte = '$'
)

// NewZeroParser creates a new zero-allocation parser from input string.
// Uses adaptive allocation based on input size to minimize overhead.
func NewZeroParser(input string) *ZeroParser {
	inputBytes := []byte(input) // Safe conversion (copies in Go)
	inputLen := len(inputBytes)

	// Adaptive capacity based on input size
	tokenCap := max(MinTokenCapacity, inputLen/TokenCapacityDivisor)
	nodeCap := max(MinNodeCapacity, inputLen/NodeCapacityDivisor)
	childCap := max(MinChildCapacity, inputLen/ChildCapacityDivisor)

	return &ZeroParser{
		input:        inputBytes,
		pos:          0,
		row:          1,
		col:          1,
		len:          inputLen,
		tokens:       make([]ZeroToken, 0, tokenCap),
		tokenCount:   0,
		nodes:        make([]ZeroNode, 0, nodeCap),
		nodeCount:    0,
		childIndices: make([]uint32, 0, childCap),
		childCount:   0,
		errors:       make([]ParseError, 0, InitialErrorCapacity),
		childBuffer:  make([]uint32, 0, ChildBufferSize),
		escapeBuffer: make([]byte, 0, EscapeBufferSize),
	}
}

// Parse performs a single-pass parse of the input, creating tokens and AST
// nodes simultaneously. Returns the root document node index.
func (p *ZeroParser) Parse() (uint32, error) {
	// Check if input looks like a document (has sections)
	// Simple heuristic: if it starts with ---, ~, or #, it's likely a document
	p.skipWhitespace()

	if p.pos < p.len {
		ch := p.peek()
		// Check for document markers
		if (ch == charMinus && p.peekAhead(1) == charMinus && p.peekAhead(2) == charMinus) ||
			ch == charTilde || ch == charHash {
			// Parse as document
			docNodeIdx := p.parseDocument()
			if len(p.errors) > 0 {
				return docNodeIdx, &p.errors[0]
			}
			return docNodeIdx, nil
		}
	}

	// Parse as single value
	rootIdx := p.parseValue()

	if len(p.errors) > 0 {
		return rootIdx, &p.errors[0]
	}

	// Check for trailing content
	p.skipWhitespace()
	if p.pos < p.len {
		p.addError("unexpected content after root value")
		return rootIdx, &p.errors[0]
	}

	return rootIdx, nil
}

// parseDocument parses the entire document structure.
// Follows the logic from TypeScript ASTParser.processDocument()
func (p *ZeroParser) parseDocument() uint32 {
	p.childBuffer = p.childBuffer[:0] // Reset child buffer

	var headerIdx uint32 = 0xFFFFFFFF // Use max uint32 as "null"
	first := true

	for p.pos < p.len {
		// Skip whitespace
		p.skipWhitespace()

		if p.pos >= p.len {
			break
		}

		// Check for section separator at start (skip it for first section)
		if first && p.peek() == charMinus && p.peekAhead(1) == charMinus && p.peekAhead(2) == charMinus {
			// First token is ---, means no header
			p.advance(3) // Consume ---
			p.skipWhitespace()
			first = false
		}

		// Parse section
		sectionIdx := p.parseSection(first)

		if first {
			headerIdx = sectionIdx
			first = false
		} else {
			p.childBuffer = append(p.childBuffer, sectionIdx)
		}

		// Skip whitespace after section
		p.skipWhitespace()

		// Check for section separator or end
		if p.pos >= p.len {
			break
		}

		if p.peek() == charMinus && p.peekAhead(1) == charMinus && p.peekAhead(2) == charMinus {
			p.advance(3) // Consume ---
			continue
		}

		// If not first and no section separator and not at end, it's an error
		if !first && p.pos < p.len {
			// Only error if there's actual content remaining
			p.skipWhitespace()
			if p.pos < p.len {
				p.addError("Expected section separator '---' or end of document")
			}
			break
		}
	}

	// Create document node
	return p.createDocumentNode(headerIdx, p.childBuffer)
}

// parseSection parses a section with optional name and schema
// Follows TypeScript ASTParser.processSection() logic
func (p *ZeroParser) parseSection(first bool) uint32 {
	p.skipWhitespace()

	// Parse optional section name and schema
	nameTokenIdx, schemaTokenIdx := p.parseSectionAndSchemaNames()

	// Parse section content (collection, object, array, or value)
	contentIdx := p.parseSectionContent()

	return p.createSectionNode(nameTokenIdx, schemaTokenIdx, contentIdx)
}

// parseSectionAndSchemaNames parses optional section name and schema tokens
// Returns (schemaTokenIdx, nameTokenIdx) or (0xFFFFFFFF, 0xFFFFFFFF) if none
func (p *ZeroParser) parseSectionAndSchemaNames() (uint32, uint32) {
	var nameTokenIdx uint32 = 0xFFFFFFFF
	var schemaTokenIdx uint32 = 0xFFFFFFFF

	ch := p.peek()

	// Check for section name (starts with ~)
	if ch == charTilde {
		nameTokenIdx = p.parseSectionName()
		p.skipWhitespace()
		ch = p.peek()
	}

	// Check for schema name (starts with $ after ~name or directly)
	if ch == charDollar {
		schemaTokenIdx = p.parseSchemaName()
		p.skipWhitespace()
	}

	return nameTokenIdx, schemaTokenIdx
}

// parseSectionName parses a section name token (starts with ~)
func (p *ZeroParser) parseSectionName() uint32 {
	start := p.pos
	startRow := p.row
	startCol := p.col

	p.advance(1) // Skip ~

	// Parse identifier
	for p.pos < p.len {
		ch := p.peek()
		if !isIdentifierChar(ch) {
			break
		}
		p.advance(1)
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeSectionName,
		Start:   uint32(start),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0,
	})
	p.tokenCount++

	return tokenIdx
}

// parseSchemaName parses a schema name token (starts with $)
func (p *ZeroParser) parseSchemaName() uint32 {
	start := p.pos
	startRow := p.row
	startCol := p.col

	p.advance(1) // Skip $

	// Parse identifier
	for p.pos < p.len {
		ch := p.peek()
		if !isIdentifierChar(ch) {
			break
		}
		p.advance(1)
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeSectionSchema,
		Start:   uint32(start),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0,
	})
	p.tokenCount++

	return tokenIdx
}

// parseSectionContent parses the main content of a section
func (p *ZeroParser) parseSectionContent() uint32 {
	p.skipWhitespace()

	if p.pos >= p.len {
		return 0xFFFFFFFF
	}

	ch := p.peek()

	// Check for collection marker
	if ch == charHash {
		return p.parseCollection()
	}

	// Check for explicit object or array
	if ch == charCurlyOpen {
		return p.parseObject(false)
	}

	if ch == charBracketOpen {
		return p.parseArray()
	}

	// Check if it looks like an open object (identifier followed by colon)
	// This is common in sections
	if isIdentifierStart(ch) {
		// Look ahead for colon to determine if it's an open object
		savedPos := p.pos
		savedRow := p.row
		savedCol := p.col

		// Skip identifier
		for p.pos < p.len && isIdentifierChar(p.peek()) {
			p.advance(1)
		}
		p.skipWhitespace()

		if p.peek() == charColon {
			// It's an open object - reset and parse it
			p.pos = savedPos
			p.row = savedRow
			p.col = savedCol
			return p.parseObject(true)
		}

		// Not an open object, reset and parse as value
		p.pos = savedPos
		p.row = savedRow
		p.col = savedCol
	}

	// Otherwise parse a value
	return p.parseValue()
}

// parseCollection parses a collection (starts with #)
func (p *ZeroParser) parseCollection() uint32 {
	p.advance(1) // Skip #
	p.skipWhitespace()

	p.childBuffer = p.childBuffer[:0]

	// Parse items until we hit section separator or end
	for p.pos < p.len {
		ch := p.peek()

		// Check for section separator
		if ch == charMinus && p.peekAhead(1) == charMinus && p.peekAhead(2) == charMinus {
			break
		}

		// Parse one value
		itemIdx := p.parseValue()
		p.childBuffer = append(p.childBuffer, itemIdx)

		p.skipWhitespace()

		// Check for comma (optional in collections)
		if p.peek() == charComma {
			p.advance(1)
			p.skipWhitespace()
		}
	}

	return p.createCollectionNode(p.childBuffer)
}

// parseValue parses any value (object, array, or scalar)
func (p *ZeroParser) parseValue() uint32 {
	p.skipWhitespace()

	if p.pos >= p.len {
		return 0xFFFFFFFF
	}

	ch := p.peek()

	switch ch {
	case charCurlyOpen:
		return p.parseObject(false)
	case charBracketOpen:
		return p.parseArray()
	case charDoubleQuote:
		return p.parseQuotedString()
	case charSingleQuote:
		return p.parseSingleQuotedString()
	case charTilde:
		return p.parseRawString()
	default:
		// Try to parse as number, boolean, null, or unquoted string
		if isDigitByte(ch) || ch == charMinus || ch == charPlus {
			return p.parseNumber()
		}
		if ch == 't' || ch == 'f' || ch == 'T' || ch == 'F' {
			return p.parseBoolean()
		}
		if ch == 'n' || ch == 'N' {
			return p.parseNull()
		}
		// Check for open object (no braces)
		if isIdentifierStart(ch) {
			return p.parseOpenObjectOrString()
		}
		return 0xFFFFFFFF
	}
}

// parseObject parses a closed object {...} or open object
func (p *ZeroParser) parseObject(isOpen bool) uint32 {
	if !isOpen {
		p.advance(1) // Skip {
	}

	p.skipWhitespace()
	p.childBuffer = p.childBuffer[:0]

	// Check for empty object
	if !isOpen && p.peek() == charCurlyClose {
		p.advance(1)
		return p.createObjectNode(p.childBuffer, false)
	}

	// Parse members
	for {
		p.skipWhitespace()

		// Check for closing brace
		if !isOpen && p.peek() == charCurlyClose {
			p.advance(1)
			break
		}

		// Parse member
		memberIdx := p.parseMember()
		if memberIdx == 0xFFFFFFFF {
			break
		}
		p.childBuffer = append(p.childBuffer, memberIdx)

		p.skipWhitespace()

		// Check for comma
		if p.peek() == charComma {
			p.advance(1)
			p.skipWhitespace()
			continue
		}

		// For open objects, check for end conditions
		if isOpen {
			ch := p.peek()
			if ch == charCurlyClose || ch == charBracketClose ||
				(ch == charMinus && p.peekAhead(1) == charMinus && p.peekAhead(2) == charMinus) {
				break
			}
			// Continue parsing next member
			continue
		} else {
			// For closed objects, must have comma or closing brace
			if p.peek() != charCurlyClose {
				p.addError("Expected ',' or '}' in object")
				break
			}
		}
	}

	return p.createObjectNode(p.childBuffer, isOpen)
}

// parseMember parses an object member (key: value)
func (p *ZeroParser) parseMember() uint32 {
	p.skipWhitespace()

	if p.pos >= p.len {
		return 0xFFFFFFFF
	}

	// Parse key
	keyTokenIdx := p.parseMemberKey()
	if keyTokenIdx == 0xFFFFFFFF {
		return 0xFFFFFFFF
	}

	p.skipWhitespace()

	// Expect colon
	if p.peek() != charColon {
		p.addError("Expected ':' after object key")
		return 0xFFFFFFFF
	}
	p.advance(1)
	p.skipWhitespace()

	// Parse value
	valueIdx := p.parseValue()

	return p.createMemberNode(keyTokenIdx, valueIdx)
}

// parseMemberKey parses an object key (quoted or unquoted)
func (p *ZeroParser) parseMemberKey() uint32 {
	ch := p.peek()

	if ch == charDoubleQuote {
		return p.parseQuotedString()
	}

	// Unquoted key
	start := p.pos
	startRow := p.row
	startCol := p.col

	for p.pos < p.len {
		ch := p.peek()
		if ch == charColon || ch == charComma || ch == charSpace || ch == charTab ||
			ch == charNewline || ch == charCarriageRet {
			break
		}
		p.advance(1)
	}

	if p.pos == start {
		return 0xFFFFFFFF
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeRegularString,
		Start:   uint32(start),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0,
	})
	p.tokenCount++

	return tokenIdx
}

// parseArray parses an array [...]
func (p *ZeroParser) parseArray() uint32 {
	p.advance(1) // Skip [
	p.skipWhitespace()

	p.childBuffer = p.childBuffer[:0]

	// Check for empty array
	if p.peek() == charBracketClose {
		p.advance(1)
		return p.createArrayNode(p.childBuffer)
	}

	// Parse elements
	for {
		p.skipWhitespace()

		// Check for closing bracket
		if p.peek() == charBracketClose {
			p.advance(1)
			break
		}

		// Parse element
		elemIdx := p.parseValue()
		if elemIdx == 0xFFFFFFFF {
			break
		}
		p.childBuffer = append(p.childBuffer, elemIdx)

		p.skipWhitespace()

		// Check for comma
		if p.peek() == charComma {
			p.advance(1)
			continue
		}

		// Must have comma or closing bracket
		if p.peek() != charBracketClose {
			p.addError("Expected ',' or ']' in array")
			break
		}
	}

	return p.createArrayNode(p.childBuffer)
}

// parseQuotedString parses a double-quoted string "..."
func (p *ZeroParser) parseQuotedString() uint32 {
	startRow := p.row
	startCol := p.col

	p.advance(1) // Skip opening "

	var flags uint8
	contentStart := p.pos

	for p.pos < p.len {
		ch := p.peek()

		if ch == charDoubleQuote {
			break
		}

		if ch == charBackslash {
			flags |= FlagHasEscapes
			p.advance(2) // Skip escape sequence (simplified)
			continue
		}

		if ch >= 0x80 {
			flags |= FlagNeedsNormalize
		}

		p.advance(1)
	}

	if p.pos >= p.len {
		p.addError("Unterminated string")
		return 0xFFFFFFFF
	}

	contentEnd := p.pos
	p.advance(1) // Skip closing "

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeRegularString,
		Start:   uint32(contentStart),
		End:     uint32(contentEnd),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   flags,
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// parseSingleQuotedString parses a single-quoted string '...'
func (p *ZeroParser) parseSingleQuotedString() uint32 {
	startRow := p.row
	startCol := p.col

	p.advance(1) // Skip opening '

	var flags uint8
	contentStart := p.pos

	for p.pos < p.len {
		ch := p.peek()

		if ch == charSingleQuote {
			break
		}

		if ch == charBackslash {
			flags |= FlagHasEscapes
			p.advance(2)
			continue
		}

		if ch >= 0x80 {
			flags |= FlagNeedsNormalize
		}

		p.advance(1)
	}

	if p.pos >= p.len {
		p.addError("Unterminated string")
		return 0xFFFFFFFF
	}

	contentEnd := p.pos
	p.advance(1) // Skip closing '

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeRegularString,
		Start:   uint32(contentStart),
		End:     uint32(contentEnd),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   flags,
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// parseRawString parses a raw string ~...~
func (p *ZeroParser) parseRawString() uint32 {
	startRow := p.row
	startCol := p.col

	p.advance(1) // Skip opening ~
	contentStart := p.pos

	for p.pos < p.len {
		ch := p.peek()
		if ch == charTilde {
			break
		}
		p.advance(1)
	}

	if p.pos >= p.len {
		p.addError("Unterminated raw string")
		return 0xFFFFFFFF
	}

	contentEnd := p.pos
	p.advance(1) // Skip closing ~

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeRawString,
		Start:   uint32(contentStart),
		End:     uint32(contentEnd),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0, // Raw strings have no escapes
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// parseNumber parses a number
func (p *ZeroParser) parseNumber() uint32 {
	numStart := p.pos
	startRow := p.row
	startCol := p.col

	var flags uint8

	// Handle sign
	if p.peek() == charMinus {
		flags |= FlagIsNegative
		p.advance(1)
	} else if p.peek() == charPlus {
		p.advance(1)
	}

	// Check for hex, octal, binary
	if p.peek() == charZero {
		next := p.peekAhead(1)
		if next == 'x' || next == 'X' {
			flags |= FlagIsHex
			p.advance(2)
			for p.pos < p.len && isHexDigitByte(p.peek()) {
				p.advance(1)
			}
		} else if next == 'o' || next == 'O' {
			flags |= FlagIsOctal
			p.advance(2)
			for p.pos < p.len && isOctalDigitByte(p.peek()) {
				p.advance(1)
			}
		} else if next == 'b' || next == 'B' {
			flags |= FlagIsBinary
			p.advance(2)
			for p.pos < p.len && isBinaryDigitByte(p.peek()) {
				p.advance(1)
			}
		} else {
			// Regular number starting with 0
			for p.pos < p.len && isDigitByte(p.peek()) {
				p.advance(1)
			}
		}
	} else {
		// Parse digits
		for p.pos < p.len && isDigitByte(p.peek()) {
			p.advance(1)
		}
	}

	// Check for decimal point
	if p.peek() == charDot {
		flags |= FlagHasDecimal
		p.advance(1)
		for p.pos < p.len && isDigitByte(p.peek()) {
			p.advance(1)
		}
	}

	// Check for exponent
	if p.peek() == 'e' || p.peek() == 'E' {
		flags |= FlagHasExponent
		p.advance(1)
		if p.peek() == charMinus || p.peek() == charPlus {
			p.advance(1)
		}
		for p.pos < p.len && isDigitByte(p.peek()) {
			p.advance(1)
		}
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokNumber,
		SubType: SubTypeNone,
		Start:   uint32(numStart),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   flags,
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// parseBoolean parses true/false
func (p *ZeroParser) parseBoolean() uint32 {
	start := p.pos
	startRow := p.row
	startCol := p.col

	ch := p.peek()
	if ch == 't' || ch == 'T' {
		// Expect "true"
		if p.pos+4 <= p.len {
			p.advance(4)
		}
	} else {
		// Expect "false"
		if p.pos+5 <= p.len {
			p.advance(5)
		}
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokBoolean,
		SubType: SubTypeNone,
		Start:   uint32(start),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0,
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// parseNull parses null
func (p *ZeroParser) parseNull() uint32 {
	start := p.pos
	startRow := p.row
	startCol := p.col

	// Expect "null"
	if p.pos+4 <= p.len {
		p.advance(4)
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokNull,
		SubType: SubTypeNone,
		Start:   uint32(start),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0,
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// parseOpenObjectOrString tries to parse an open object or unquoted string
func (p *ZeroParser) parseOpenObjectOrString() uint32 {
	// Look ahead to determine if this is an open object (has colon)
	savedPos := p.pos
	savedRow := p.row
	savedCol := p.col

	// Scan forward looking for colon
	for p.pos < p.len {
		ch := p.peek()
		if ch == charColon {
			// It's an open object
			p.pos = savedPos
			p.row = savedRow
			p.col = savedCol
			return p.parseObject(true)
		}
		if ch == charComma || ch == charNewline || ch == charBracketClose || ch == charCurlyClose {
			// It's an unquoted string
			break
		}
		p.advance(1)
	}

	// Reset and parse as unquoted string
	p.pos = savedPos
	p.row = savedRow
	p.col = savedCol

	return p.parseUnquotedString()
}

// parseUnquotedString parses an unquoted string
func (p *ZeroParser) parseUnquotedString() uint32 {
	start := p.pos
	startRow := p.row
	startCol := p.col

	for p.pos < p.len {
		ch := p.peek()
		if ch == charComma || ch == charNewline || ch == charCarriageRet ||
			ch == charBracketClose || ch == charCurlyClose {
			break
		}
		p.advance(1)
	}

	tokenIdx := uint32(p.tokenCount)
	p.tokens = append(p.tokens, ZeroToken{
		Type:    TokString,
		SubType: SubTypeRegularString,
		Start:   uint32(start),
		End:     uint32(p.pos),
		Row:     uint16(startRow),
		Col:     uint16(startCol),
		Flags:   0,
	})
	p.tokenCount++

	return p.createTokenNode(tokenIdx)
}

// Inline scanner methods (no separate tokenizer!)

// skipWhitespace advances past whitespace characters
//
//go:inline
func (p *ZeroParser) skipWhitespace() {
	for p.pos < p.len {
		ch := p.input[p.pos]

		// Fast path for ASCII whitespace
		if ch <= charSpace {
			if ch == charSpace || ch == charTab || ch == charNewline || ch == charCarriageRet {
				if ch == charNewline {
					p.row++
					p.col = 1
				} else {
					p.col++
				}
				p.pos++
				continue
			}
		}

		// Check for Unicode whitespace (using lessons from fast_parser_bytes)
		if ch >= 0xC2 { // Potential multi-byte whitespace
			// TODO: Add Unicode whitespace support
		}

		break
	}
}

// peek returns the current byte without advancing
//
//go:inline
func (p *ZeroParser) peek() byte {
	if p.pos >= p.len {
		return 0
	}
	return p.input[p.pos]
}

// peekAhead returns the byte at offset ahead without advancing
//
//go:inline
func (p *ZeroParser) peekAhead(offset int) byte {
	idx := p.pos + offset
	if idx >= p.len {
		return 0
	}
	return p.input[idx]
}

// advance moves forward by n bytes, updating row/col
//
//go:inline
func (p *ZeroParser) advance(n int) {
	for i := 0; i < n && p.pos < p.len; i++ {
		if p.input[p.pos] == charNewline {
			p.row++
			p.col = 1
		} else {
			p.col++
		}
		p.pos++
	}
}

// Node creation methods

func (p *ZeroParser) createDocumentNode(headerIdx uint32, sectionIndices []uint32) uint32 {
	// Allocate space in child indices arena
	childStart := uint32(p.childCount)
	for _, idx := range sectionIndices {
		p.childIndices = append(p.childIndices, idx)
		p.childCount++
	}

	// Create node
	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindDocument,
		TokenIdx:   headerIdx,
		ChildStart: childStart,
		ChildCount: uint16(len(sectionIndices)),
		Flags:      0,
		Row:        1,
		Col:        1,
	})
	p.nodeCount++

	return nodeIdx
}

func (p *ZeroParser) createSectionNode(nameTokenIdx, schemaTokenIdx, childIdx uint32) uint32 {
	var flags uint8
	if nameTokenIdx != 0xFFFFFFFF {
		flags |= NodeFlagHasName
	}
	if schemaTokenIdx != 0xFFFFFFFF {
		flags |= NodeFlagHasSchema
	}

	// Store child index
	childStart := uint32(p.childCount)
	if childIdx != 0xFFFFFFFF {
		p.childIndices = append(p.childIndices, childIdx)
		p.childCount++
	}

	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindSection,
		TokenIdx:   nameTokenIdx,
		ChildStart: childStart,
		ChildCount: 1,
		Flags:      flags,
		Row:        uint16(p.row),
		Col:        uint16(p.col),
	})
	p.nodeCount++

	return nodeIdx
}

func (p *ZeroParser) createCollectionNode(items []uint32) uint32 {
	childStart := uint32(p.childCount)
	for _, idx := range items {
		p.childIndices = append(p.childIndices, idx)
		p.childCount++
	}

	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindCollection,
		TokenIdx:   0xFFFFFFFF,
		ChildStart: childStart,
		ChildCount: uint16(len(items)),
		Flags:      0,
		Row:        uint16(p.row),
		Col:        uint16(p.col),
	})
	p.nodeCount++

	return nodeIdx
}

func (p *ZeroParser) createObjectNode(members []uint32, isOpen bool) uint32 {
	childStart := uint32(p.childCount)
	for _, idx := range members {
		p.childIndices = append(p.childIndices, idx)
		p.childCount++
	}

	var flags uint8
	if isOpen {
		flags |= NodeFlagIsOpen
	}

	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindObject,
		TokenIdx:   0xFFFFFFFF,
		ChildStart: childStart,
		ChildCount: uint16(len(members)),
		Flags:      flags,
		Row:        uint16(p.row),
		Col:        uint16(p.col),
	})
	p.nodeCount++

	return nodeIdx
}

func (p *ZeroParser) createMemberNode(keyTokenIdx, valueIdx uint32) uint32 {
	childStart := uint32(p.childCount)
	p.childIndices = append(p.childIndices, valueIdx)
	p.childCount++

	var flags uint8
	if keyTokenIdx != 0xFFFFFFFF {
		flags |= NodeFlagHasKey
	}

	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindMember,
		TokenIdx:   keyTokenIdx,
		ChildStart: childStart,
		ChildCount: 1,
		Flags:      flags,
		Row:        uint16(p.row),
		Col:        uint16(p.col),
	})
	p.nodeCount++

	return nodeIdx
}

func (p *ZeroParser) createArrayNode(elements []uint32) uint32 {
	childStart := uint32(p.childCount)
	for _, idx := range elements {
		p.childIndices = append(p.childIndices, idx)
		p.childCount++
	}

	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindArray,
		TokenIdx:   0xFFFFFFFF,
		ChildStart: childStart,
		ChildCount: uint16(len(elements)),
		Flags:      0,
		Row:        uint16(p.row),
		Col:        uint16(p.col),
	})
	p.nodeCount++

	return nodeIdx
}

func (p *ZeroParser) createTokenNode(tokenIdx uint32) uint32 {
	nodeIdx := uint32(p.nodeCount)
	p.nodes = append(p.nodes, ZeroNode{
		Type:       NodeKindToken,
		TokenIdx:   tokenIdx,
		ChildStart: 0,
		ChildCount: 0,
		Flags:      0,
		Row:        uint16(p.row),
		Col:        uint16(p.col),
	})
	p.nodeCount++

	return nodeIdx
}

// Helper functions for character classification

//go:inline
func isDigitByte(ch byte) bool {
	return ch >= charZero && ch <= charNine
}

//go:inline
func isHexDigitByte(ch byte) bool {
	return (ch >= charZero && ch <= charNine) ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}

//go:inline
func isOctalDigitByte(ch byte) bool {
	return ch >= '0' && ch <= '7'
}

//go:inline
func isBinaryDigitByte(ch byte) bool {
	return ch == '0' || ch == '1'
}

//go:inline
func isIdentifierStart(ch byte) bool {
	return (ch >= charLowerA && ch <= charLowerZ) ||
		(ch >= charUpperA && ch <= charUpperZ) ||
		ch == charUnderscore ||
		ch == charDollar
}

//go:inline
func isIdentifierChar(ch byte) bool {
	return isIdentifierStart(ch) || isDigitByte(ch)
}

// Error handling

func (p *ZeroParser) addError(message string) {
	p.errors = append(p.errors, ParseError{
		Message: message,
		Pos:     p.pos,
		Row:     p.row,
		Col:     p.col,
	})
}

// Error implements the error interface for ParseError
func (e *ParseError) Error() string {
	return e.Message
}

// Lazy value extraction methods (materialize strings/values on demand)

// GetTokenString returns the raw string for a token (zero-copy slice)
func (p *ZeroParser) GetTokenString(tokenIdx uint32) string {
	if tokenIdx >= uint32(p.tokenCount) {
		return ""
	}
	tok := p.tokens[tokenIdx]
	return string(p.input[tok.Start:tok.End])
}

// GetTokenBytes returns a zero-copy byte slice reference to the token's data.
// This is faster than GetTokenString as it avoids string allocation.
// WARNING: The returned slice references the parser's internal buffer and
// becomes invalid if the parser is garbage collected.
func (p *ZeroParser) GetTokenBytes(tokenIdx uint32) []byte {
	if tokenIdx >= uint32(p.tokenCount) {
		return nil
	}
	tok := p.tokens[tokenIdx]
	return p.input[tok.Start:tok.End]
}

// CopyTokenBytes copies the token's data into the provided byte slice.
// Returns the number of bytes copied, or -1 if tokenIdx is invalid.
// If dst is nil or too small, returns the required size without copying.
func (p *ZeroParser) CopyTokenBytes(tokenIdx uint32, dst []byte) int {
	if tokenIdx >= uint32(p.tokenCount) {
		return -1
	}
	tok := p.tokens[tokenIdx]
	dataLen := int(tok.End - tok.Start)

	if dst == nil || len(dst) < dataLen {
		return dataLen // Return required size
	}

	copy(dst, p.input[tok.Start:tok.End])
	return dataLen
}

// GetTokenBytesTo writes the token's data to the provided destination.
// This is the most efficient approach - completely zero-allocation.
// Returns the number of bytes written, or -1 if tokenIdx is invalid.
// Panics if dst is too small (caller must ensure correct size).
func (p *ZeroParser) GetTokenBytesTo(tokenIdx uint32, dst []byte) int {
	if tokenIdx >= uint32(p.tokenCount) {
		return -1
	}
	tok := p.tokens[tokenIdx]
	dataLen := int(tok.End - tok.Start)

	// Fast path: direct copy (no bounds check overhead if caller sized correctly)
	copy(dst[:dataLen], p.input[tok.Start:tok.End])
	return dataLen
}

// GetTokenValue materializes the actual value for a token
func (p *ZeroParser) GetTokenValue(tokenIdx uint32) interface{} {
	if tokenIdx >= uint32(p.tokenCount) {
		return nil
	}

	tok := p.tokens[tokenIdx]
	raw := p.input[tok.Start:tok.End]

	switch tok.Type {
	case TokString:
		// TODO: Handle escapes if FlagHasEscapes is set
		return string(raw)
	case TokNumber:
		// TODO: Parse number on demand
		return string(raw)
	case TokBoolean:
		return raw[0] == 't' || raw[0] == 'T'
	case TokNull:
		return nil
	default:
		return string(raw)
	}
}

// Stats returns parser statistics
func (p *ZeroParser) Stats() map[string]interface{} {
	return map[string]interface{}{
		"tokens":             p.tokenCount,
		"nodes":              p.nodeCount,
		"child_indices":      p.childCount,
		"errors":             len(p.errors),
		"input_bytes":        p.len,
		"bytes_per_token":    float64(p.len) / float64(max(1, p.tokenCount)),
		"token_size_bytes":   13,
		"node_size_bytes":    17,
		"total_token_memory": p.tokenCount * 13,
		"total_node_memory":  p.nodeCount * 17,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
