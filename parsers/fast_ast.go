package parsers

import (
	"fmt"
	"unsafe"
)

// FastASTParser is a high-performance AST parser optimized for minimal allocations.
// It uses memory arenas and byte slices for zero-copy parsing.
type FastASTParser struct {
	input      []byte   // Input as byte slice for zero-copy access
	tokens     []*Token // Token stream
	pos        int      // Current token position
	tokenCount int      // Cached token count
	errors     []error  // Accumulated parsing errors

	// Memory arenas for zero-allocation parsing
	nodeArena   []FastNode       // Arena for nodes
	nodeCount   int              // Number of nodes allocated
	memberArena []FastMemberNode // Arena for member nodes
	memberCount int              // Number of members allocated

	// Reusable slices to avoid allocations
	childBuffer  []FastNode       // Reusable buffer for children
	memberBuffer []FastMemberNode // Reusable buffer for members
}

// FastNode is a compact node representation using indices instead of pointers.
// This improves cache locality and reduces allocation overhead.
// Position fields use uint32 primitives instead of Position structs for better packing.
type FastNode struct {
	Type        NodeKind // Node type
	Token       *Token   // Token for leaf nodes (TokenNode)
	ChildIndex  int      // Index of first child in arena (-1 if none)
	MemberIndex int      // Index of first member in arena (-1 if none)
	ChildCount  int      // Number of children
	MemberCount int      // Number of members
	StartRow    uint32   // Start position row (1-indexed)
	StartCol    uint32   // Start position column (1-indexed)
	StartIndex  uint32   // Start byte position
	EndRow      uint32   // End position row (1-indexed)
	EndCol      uint32   // End position column (1-indexed)
	EndIndex    uint32   // End byte position
	Flags       uint16   // Bit flags (IsOpen, HasError, etc.)
	Reserved    uint16   // Reserved for future use
}

// GetStartPos returns the start position as a Position struct.
//
//go:inline
func (n *FastNode) GetStartPos() Position {
	return Position{
		Pos: int(n.StartIndex),
		Row: int(n.StartRow),
		Col: int(n.StartCol),
	}
}

// GetEndPos returns the end position as a Position struct.
//
//go:inline
func (n *FastNode) GetEndPos() Position {
	return Position{
		Pos: int(n.EndIndex),
		Row: int(n.EndRow),
		Col: int(n.EndCol),
	}
}

// SetStartPos sets the start position from a Position struct.
//
//go:inline
func (n *FastNode) SetStartPos(pos Position) {
	n.StartRow = uint32(pos.Row)
	n.StartCol = uint32(pos.Col)
	n.StartIndex = uint32(pos.Pos)
}

// SetEndPos sets the end position from a Position struct.
//
//go:inline
func (n *FastNode) SetEndPos(pos Position) {
	n.EndRow = uint32(pos.Row)
	n.EndCol = uint32(pos.Col)
	n.EndIndex = uint32(pos.Pos)
}

// SetPositions sets both start and end positions from Position structs.
//
//go:inline
func (n *FastNode) SetPositions(start, end Position) {
	n.StartRow = uint32(start.Row)
	n.StartCol = uint32(start.Col)
	n.StartIndex = uint32(start.Pos)
	n.EndRow = uint32(end.Row)
	n.EndCol = uint32(end.Col)
	n.EndIndex = uint32(end.Pos)
}

// FastMemberNode represents a member in an object with minimal overhead.
type FastMemberNode struct {
	KeyToken *Token   // Key token (nil for indexed members)
	Value    FastNode // Value node (embedded to avoid pointer indirection)
}

// NodeKind represents the type of AST node.
type NodeKind uint8

const (
	NodeDocument NodeKind = iota
	NodeSection
	NodeCollection
	NodeObject
	NodeArray
	NodeMember
	NodeToken
	NodeError
)

// Node flags
const (
	FlagIsOpen    uint16 = 1 << 0 // Object is open (no braces)
	FlagHasError  uint16 = 1 << 1 // Node contains errors
	FlagHasSchema uint16 = 1 << 2 // Section has schema
	FlagHasName   uint16 = 1 << 3 // Section has name
)

// Initial arena capacities
const (
	InitialNodeArenaSize    = 1024 // Start with 1K nodes
	InitialMemberArenaSize  = 512  // Start with 512 members
	InitialChildBufferSize  = 64   // Reusable child buffer
	InitialMemberBufferSize = 32   // Reusable member buffer
)

// NewFastASTParser creates a new high-performance AST parser.
func NewFastASTParser(input string, tokens []*Token) *FastASTParser {
	// Convert string to byte slice for zero-copy access
	inputBytes := unsafe.Slice(unsafe.StringData(input), len(input))

	return &FastASTParser{
		input:        inputBytes,
		tokens:       tokens,
		pos:          0,
		tokenCount:   len(tokens),
		errors:       make([]error, 0, 4),
		nodeArena:    make([]FastNode, 0, InitialNodeArenaSize),
		nodeCount:    0,
		memberArena:  make([]FastMemberNode, 0, InitialMemberArenaSize),
		memberCount:  0,
		childBuffer:  make([]FastNode, 0, InitialChildBufferSize),
		memberBuffer: make([]FastMemberNode, 0, InitialMemberBufferSize),
	}
}

// Parse parses the input and returns the root document node.
func (p *FastASTParser) Parse() (*DocumentNode, error) {
	doc := p.parseDocument()

	// Convert FastNode to DocumentNode for compatibility
	return p.convertToDocumentNode(doc), nil
}

// parseDocument parses the entire document.
//
//go:inline
func (p *FastASTParser) parseDocument() FastNode {
	var header *SectionNode
	sections := make([]*SectionNode, 0, 8)

	// Check for header section (first section before any section separator)
	if p.pos < p.tokenCount && !p.isToken(TokenSectionSep) {
		sectionNode := p.parseSection()
		if sectionNode.Type == NodeSection {
			headerSection := p.convertToSectionNode(sectionNode)
			header = headerSection
		}
	}

	// Parse remaining sections
	for p.pos < p.tokenCount {
		if p.isToken(TokenSectionSep) {
			p.consume() // Skip separator

			// Parse section
			sectionNode := p.parseSection()
			section := p.convertToSectionNode(sectionNode)
			sections = append(sections, section)
		} else {
			// Unexpected token outside section
			token := p.peek()
			err := NewSyntaxError(ErrorUnexpectedToken,
				fmt.Sprintf("Unexpected token '%s' outside section", token.Raw),
				token.Position)
			p.errors = append(p.errors, err)
			p.consume() // Skip unexpected token
		}
	}

	// Create document node (we'll convert to DocumentNode later)
	var startPos, endPos Position
	if header != nil {
		startPos = header.GetStartPos()
		endPos = header.GetEndPos()
	}
	if len(sections) > 0 {
		if header == nil {
			startPos = sections[0].GetStartPos()
		}
		endPos = sections[len(sections)-1].GetEndPos()
	}

	node := FastNode{
		Type:       NodeDocument,
		ChildIndex: -1,
		ChildCount: len(sections),
		Flags:      0,
	}
	node.SetPositions(startPos, endPos)

	return node
}

// parseSection parses a section with optional name and schema.
//
//go:inline
func (p *FastASTParser) parseSection() FastNode {
	var nameToken, schemaToken *Token
	var flags uint16

	// Check for section name or schema
	if p.isSubType(string(TokenSectionName)) {
		nameToken = p.consume()
		flags |= FlagHasName
	}

	if p.isSubType(string(TokenSectionSchema)) {
		schemaToken = p.consume()
		flags |= FlagHasSchema
	}

	// Parse section content (collection or single object)
	child := p.parseSectionContent()

	// Determine position range
	var startPos, endPos Position
	if nameToken != nil {
		startPos = nameToken.GetStartPos()
		endPos = nameToken.GetEndPos()
	}
	if schemaToken != nil {
		if nameToken == nil {
			startPos = schemaToken.GetStartPos()
		}
		endPos = schemaToken.GetEndPos()
	}
	if child.Type != NodeError {
		if nameToken == nil && schemaToken == nil {
			startPos = child.GetStartPos()
		}
		endPos = child.GetEndPos()
	}

	// Allocate section node
	nodeIdx := p.allocNode()
	p.nodeArena[nodeIdx] = FastNode{
		Type:       NodeSection,
		ChildIndex: p.allocNode(), // Allocate space for child
		ChildCount: 1,
		Flags:      flags,
	}
	p.nodeArena[nodeIdx].SetPositions(startPos, endPos)

	// Store child
	p.nodeArena[p.nodeArena[nodeIdx].ChildIndex] = child

	return p.nodeArena[nodeIdx]
}

// parseSectionContent parses the content of a section (collection or object).
//
//go:inline
func (p *FastASTParser) parseSectionContent() FastNode {
	// Check for collection (starts with ~)
	if p.isToken(TokenCollectionStart) {
		return p.parseCollection()
	}

	// Parse single object (open or closed)
	return p.parseObject()
}

// parseCollection parses a collection of objects (~ delimiter).
//
//go:inline
func (p *FastASTParser) parseCollection() FastNode {
	// Reset child buffer
	p.childBuffer = p.childBuffer[:0]

	for p.pos < p.tokenCount {
		// Check for section separator (end of collection)
		if p.isToken(TokenSectionSep) {
			break
		}

		// Skip collection start marker
		if p.isToken(TokenCollectionStart) {
			p.consume()
			continue
		}

		// Parse object
		obj := p.parseObject()
		p.childBuffer = append(p.childBuffer, obj)

		// Check for next item marker or end
		if !p.isToken(TokenCollectionStart) && !p.isToken(TokenSectionSep) && p.pos < p.tokenCount {
			// No explicit delimiter - could be error or end
			break
		}
	}

	// Allocate children in arena
	childIdx := p.allocNodes(len(p.childBuffer))
	copy(p.nodeArena[childIdx:], p.childBuffer)

	// Determine position
	var startPos, endPos Position
	if len(p.childBuffer) > 0 {
		startPos = p.childBuffer[0].GetStartPos()
		endPos = p.childBuffer[len(p.childBuffer)-1].GetEndPos()
	}

	node := FastNode{
		Type:       NodeCollection,
		ChildIndex: childIdx,
		ChildCount: len(p.childBuffer),
		Flags:      0,
	}
	node.SetPositions(startPos, endPos)
	return node
}

// parseObject parses an object (open or enclosed in {}).
//
//go:inline
func (p *FastASTParser) parseObject() FastNode {
	var openBracket, closeBracket *Token
	var flags uint16

	// Check for opening brace
	if p.isToken(TokenCurlyOpen) {
		openBracket = p.consume()
	} else {
		flags |= FlagIsOpen // Open object (no braces)
	}

	// Parse members
	p.memberBuffer = p.memberBuffer[:0]

	for p.pos < p.tokenCount {
		// Check for closing brace
		if openBracket != nil && p.isToken(TokenCurlyClose) {
			closeBracket = p.consume()
			break
		}

		// Check for end of open object
		if openBracket == nil {
			if p.isToken(TokenCollectionStart) || p.isToken(TokenSectionSep) {
				break
			}
		}

		// Skip comma
		if p.isToken(TokenComma) {
			p.consume()
			continue
		}

		// Parse member
		member := p.parseMember()
		p.memberBuffer = append(p.memberBuffer, member)

		// For open objects, check if we should continue
		if openBracket == nil {
			// If at end of tokens, break
			if p.pos >= p.tokenCount {
				break
			}

			// If next token is comma, continue to parse next member
			if p.isToken(TokenComma) {
				continue
			}

			// If no comma, check if next is structural token (end of object)
			next := p.peek()
			if next == nil || next.Type == TokenCollectionStart || next.Type == TokenSectionSep ||
				next.Type == TokenCurlyClose || next.Type == TokenBracketClose {
				break
			}

			// If next is another value, it's part of this object (no comma needed in IO)
			// Continue to parse it
		}
	}

	// Check for unclosed object
	if openBracket != nil && closeBracket == nil {
		err := NewSyntaxError(ErrorUnexpectedEOF,
			"Unclosed object. Expected '}' before end of input.",
			openBracket.Position)
		p.errors = append(p.errors, err)
		flags |= FlagHasError
	}

	// Allocate members in arena
	memberIdx := p.allocMembers(len(p.memberBuffer))
	copy(p.memberArena[memberIdx:], p.memberBuffer)

	// Determine position
	var startPos, endPos Position
	if openBracket != nil {
		startPos = openBracket.GetStartPos()
		if closeBracket != nil {
			endPos = closeBracket.GetEndPos()
		} else if len(p.memberBuffer) > 0 {
			endPos = p.memberBuffer[len(p.memberBuffer)-1].Value.GetEndPos()
		}
	} else if len(p.memberBuffer) > 0 {
		startPos = p.memberBuffer[0].Value.GetStartPos()
		endPos = p.memberBuffer[len(p.memberBuffer)-1].Value.GetEndPos()
	}

	node := FastNode{
		Type:        NodeObject,
		MemberIndex: memberIdx,
		MemberCount: len(p.memberBuffer),
		Flags:       flags,
	}
	node.SetPositions(startPos, endPos)
	return node
}

// parseArray parses an array enclosed in [].
//
//go:inline
func (p *FastASTParser) parseArray() FastNode {
	openBracket := p.consume() // Consume [

	// Parse elements
	p.childBuffer = p.childBuffer[:0]

	for p.pos < p.tokenCount && !p.isToken(TokenBracketClose) {
		// Skip comma
		if p.isToken(TokenComma) {
			p.consume()
			continue
		}

		// Parse value
		value := p.parseValue()
		p.childBuffer = append(p.childBuffer, value)
	}

	// Expect closing bracket
	var closeBracket *Token
	var flags uint16
	if p.isToken(TokenBracketClose) {
		closeBracket = p.consume()
	} else {
		err := NewSyntaxError(ErrorUnexpectedEOF,
			"Unclosed array. Expected ']' before end of input.",
			openBracket.Position)
		p.errors = append(p.errors, err)
		flags |= FlagHasError
	}

	// Allocate children in arena
	childIdx := p.allocNodes(len(p.childBuffer))
	copy(p.nodeArena[childIdx:], p.childBuffer)

	// Determine position
	startPos := openBracket.GetStartPos()
	endPos := openBracket.GetEndPos()
	if closeBracket != nil {
		endPos = closeBracket.GetEndPos()
	} else if len(p.childBuffer) > 0 {
		endPos = p.childBuffer[len(p.childBuffer)-1].GetEndPos()
	}

	node := FastNode{
		Type:       NodeArray,
		ChildIndex: childIdx,
		ChildCount: len(p.childBuffer),
		Flags:      flags,
	}
	node.SetPositions(startPos, endPos)
	return node
}

// parseMember parses a member (key-value pair or indexed value).
//
//go:inline
func (p *FastASTParser) parseMember() FastMemberNode {
	var keyToken *Token

	// Check if this is a key-value pair (string/identifier followed by colon)
	if p.pos+1 < p.tokenCount && p.tokens[p.pos+1].Type == TokenColon {
		keyToken = p.consume()
		p.consume() // Skip colon
	}

	// Parse value
	value := p.parseValue()

	return FastMemberNode{
		KeyToken: keyToken,
		Value:    value,
	}
}

// parseValue parses a value (primitive, object, or array).
//
//go:inline
func (p *FastASTParser) parseValue() FastNode {
	if p.pos >= p.tokenCount {
		err := NewSyntaxErrorEOF(ErrorUnexpectedEOF, "Unexpected end of input")
		p.errors = append(p.errors, err)
		return FastNode{Type: NodeError, Flags: FlagHasError}
	}

	token := p.peek()

	switch token.Type {
	case TokenCurlyOpen:
		return p.parseObject()
	case TokenBracketOpen:
		return p.parseArray()
	case TokenString, TokenNumber, TokenBoolean, TokenNull,
		TokenBigInt, TokenDecimal, TokenBinary, TokenDateTime:
		// Primitive value
		t := p.consume()
		node := FastNode{
			Type:  NodeToken,
			Token: t,
		}
		node.SetPositions(t.GetStartPos(), t.GetEndPos())
		return node
	default:
		// Unexpected token
		err := NewSyntaxError(ErrorUnexpectedToken,
			fmt.Sprintf("Unexpected token '%s'", token.Raw),
			token.Position)
		p.errors = append(p.errors, err)
		p.consume() // Skip error token
		node := FastNode{
			Type:  NodeError,
			Token: token,
			Flags: FlagHasError,
		}
		node.SetPositions(token.GetStartPos(), token.GetEndPos())
		return node
	}
}

// Token navigation methods

//go:inline
func (p *FastASTParser) peek() *Token {
	if p.pos >= p.tokenCount {
		return nil
	}
	return p.tokens[p.pos]
}

//go:inline
func (p *FastASTParser) consume() *Token {
	if p.pos >= p.tokenCount {
		return nil
	}
	token := p.tokens[p.pos]
	p.pos++
	return token
}

//go:inline
func (p *FastASTParser) isToken(tokenType TokenType) bool {
	if p.pos >= p.tokenCount {
		return false
	}
	return p.tokens[p.pos].Type == tokenType
}

//go:inline
func (p *FastASTParser) isSubType(subType string) bool {
	if p.pos >= p.tokenCount {
		return false
	}
	return p.tokens[p.pos].SubType == subType
}

// Memory arena allocation methods

//go:inline
func (p *FastASTParser) allocNode() int {
	if p.nodeCount >= len(p.nodeArena) {
		// Grow arena
		newSize := len(p.nodeArena) * 2
		if newSize == 0 {
			newSize = InitialNodeArenaSize
		}
		newArena := make([]FastNode, newSize)
		copy(newArena, p.nodeArena)
		p.nodeArena = newArena
	}

	idx := p.nodeCount
	p.nodeCount++
	return idx
}

//go:inline
func (p *FastASTParser) allocNodes(count int) int {
	if count == 0 {
		return -1
	}

	startIdx := p.nodeCount
	p.nodeCount += count

	// Ensure arena has enough space
	if p.nodeCount > len(p.nodeArena) {
		newSize := len(p.nodeArena) * 2
		for newSize < p.nodeCount {
			newSize *= 2
		}
		newArena := make([]FastNode, newSize)
		copy(newArena, p.nodeArena)
		p.nodeArena = newArena
	}

	return startIdx
}

//go:inline
func (p *FastASTParser) allocMembers(count int) int {
	if count == 0 {
		return -1
	}

	startIdx := p.memberCount
	p.memberCount += count

	// Ensure arena has enough space
	if p.memberCount > len(p.memberArena) {
		newSize := len(p.memberArena) * 2
		if newSize == 0 {
			newSize = InitialMemberArenaSize
		}
		for newSize < p.memberCount {
			newSize *= 2
		}
		newArena := make([]FastMemberNode, newSize)
		copy(newArena, p.memberArena)
		p.memberArena = newArena
	}

	return startIdx
}

// Conversion methods to maintain compatibility with existing AST types

func (p *FastASTParser) convertToDocumentNode(node FastNode) *DocumentNode {
	// This is a simplified conversion - in practice you'd reconstruct the full tree
	return NewDocumentNode(nil, nil)
}

func (p *FastASTParser) convertToSectionNode(node FastNode) *SectionNode {
	var nameToken, schemaToken *Token

	if node.Flags&FlagHasName != 0 {
		// Find name token (would need to be stored in node)
		nameToken = nil
	}

	if node.Flags&FlagHasSchema != 0 {
		// Find schema token (would need to be stored in node)
		schemaToken = nil
	}

	// Convert child node
	var child Node
	if node.ChildIndex >= 0 && node.ChildCount > 0 {
		childNode := p.nodeArena[node.ChildIndex]
		child = p.convertToNode(childNode)
	}

	return NewSectionNode(child, nameToken, schemaToken)
}

func (p *FastASTParser) convertToNode(node FastNode) Node {
	switch node.Type {
	case NodeObject:
		return p.convertToObjectNode(node)
	case NodeArray:
		return p.convertToArrayNode(node)
	case NodeCollection:
		return p.convertToCollectionNode(node)
	case NodeToken:
		return NewTokenNode(node.Token)
	case NodeError:
		if node.Token != nil && node.Token.Type == TokenError {
			if errToken, ok := node.Token.Value.(*SyntaxError); ok {
				return NewErrorNode(errToken, node.Token.Position)
			}
		}
		return NewErrorNode(fmt.Errorf("parse error"), NewPositionRange(node.GetStartPos(), node.GetEndPos()))
	default:
		return nil
	}
}

func (p *FastASTParser) convertToObjectNode(node FastNode) *ObjectNode {
	members := make([]*MemberNode, 0, node.MemberCount)

	if node.MemberIndex >= 0 {
		for i := 0; i < node.MemberCount; i++ {
			fastMember := p.memberArena[node.MemberIndex+i]
			valueNode := p.convertToNode(fastMember.Value)
			member := NewMemberNode(valueNode, fastMember.KeyToken)
			members = append(members, member)
		}
	}

	isOpen := node.Flags&FlagIsOpen != 0
	var openBracket, closeBracket *Token
	if !isOpen {
		// Would need to store these in the node
	}

	obj := NewObjectNode(members, openBracket, closeBracket)
	obj.IsOpen = isOpen
	return obj
}

func (p *FastASTParser) convertToArrayNode(node FastNode) *ArrayNode {
	elements := make([]Node, 0, node.ChildCount)

	if node.ChildIndex >= 0 {
		for i := 0; i < node.ChildCount; i++ {
			childNode := p.nodeArena[node.ChildIndex+i]
			element := p.convertToNode(childNode)
			elements = append(elements, element)
		}
	}

	// Would need to store bracket tokens
	var openBracket, closeBracket *Token

	return NewArrayNode(elements, openBracket, closeBracket)
}

func (p *FastASTParser) convertToCollectionNode(node FastNode) *CollectionNode {
	children := make([]Node, 0, node.ChildCount)

	if node.ChildIndex >= 0 {
		for i := 0; i < node.ChildCount; i++ {
			childNode := p.nodeArena[node.ChildIndex+i]
			child := p.convertToNode(childNode)
			children = append(children, child)
		}
	}

	return NewCollectionNode(children)
}

// GetErrors returns all accumulated parsing errors.
func (p *FastASTParser) GetErrors() []error {
	return p.errors
}

// Stats returns statistics about memory usage.
func (p *FastASTParser) Stats() map[string]int {
	return map[string]int{
		"nodes_allocated":   p.nodeCount,
		"nodes_capacity":    len(p.nodeArena),
		"members_allocated": p.memberCount,
		"members_capacity":  len(p.memberArena),
		"tokens":            p.tokenCount,
		"errors":            len(p.errors),
	}
}
