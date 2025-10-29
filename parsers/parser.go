package parsers

import (
	"fmt"
)

// Parser converts a stream of tokens into an Abstract Syntax Tree (AST).
// It implements a recursive descent parser that handles Internet Object documents.
type Parser struct {
	tokens       []*Token        // Tokens produced by the tokenizer
	current      int             // Current token index
	sectionNames map[string]bool // Track section names to detect duplicates
}

// NewParser creates a new parser from a token stream.
func NewParser(tokens []*Token) *Parser {
	return &Parser{
		tokens:       tokens,
		current:      0,
		sectionNames: make(map[string]bool),
	}
}

// Parse converts the token stream into a DocumentNode.
// This is the main entry point for parsing.
func (p *Parser) Parse() (*DocumentNode, error) {
	return p.processDocument()
}

// ParseString is a convenience function that tokenizes and parses an input string.
// It combines the tokenizer and parser into a single operation.
func ParseString(input string) (*DocumentNode, error) {
	tokenizer := NewTokenizer(input)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		return nil, err
	}

	parser := NewParser(tokens)
	return parser.Parse()
}

// processDocument parses the entire document structure.
// Returns a DocumentNode with optional header and sections.
func (p *Parser) processDocument() (*DocumentNode, error) {
	var sections []*SectionNode
	var header *SectionNode
	first := true

	for {
		token := p.peek()

		// Skip initial section separator if document starts with ---
		if first && token != nil && token.Type == TokenSectionSep {
			first = false
		}

		// Parse the section
		section, err := p.processSection(first)
		if err != nil {
			return nil, err
		}

		token = p.peek()

		// End of document
		if token == nil {
			if section != nil {
				sections = append(sections, section)
			}
			break
		}

		// Store the section
		if first {
			header = section
		} else {
			sections = append(sections, section)
		}

		if first {
			first = false
		}

		// Validate section separator
		if token.Type != TokenSectionSep {
			err := NewSyntaxError(
				ErrorUnexpectedToken,
				fmt.Sprintf("Expected section separator '---' but found '%s'. Each section must be properly closed before starting a new one.", token.Raw),
				p.currentPosition(),
			)
			return nil, err
		}

		// Move to next token after separator
		p.advance()
	}

	return NewDocumentNode(header, sections), nil
}

// processSection parses a single section.
// first indicates if this is the first section (potential header).
func (p *Parser) processSection(first bool) (*SectionNode, error) {
	token := p.peek()

	// Skip section separator if present
	if token != nil && token.Type == TokenSectionSep {
		p.advance()
	}

	// Parse section and schema names (e.g., "--- users: $userSchema")
	schemaToken, nameToken := p.parseSectionAndSchemaNames()

	// Determine section name for duplicate check
	name := "unnamed"
	if nameToken != nil && nameToken.Value != nil {
		name = fmt.Sprintf("%v", nameToken.Value)
	} else if schemaToken != nil && schemaToken.Value != nil {
		schemaStr := fmt.Sprintf("%v", schemaToken.Value)
		if len(schemaStr) > 1 {
			name = schemaStr[1:] // Remove leading $
		}
	}

	// Check for duplicate section names
	if name != "" && p.sectionNames[name] {
		return nil, NewSyntaxError(
			ErrorDuplicateSection,
			fmt.Sprintf("Duplicate section name '%s'. Each section must have a unique name within the document.", name),
			p.currentPosition(),
		)
	}

	// Register section name (skip for first/header section with default name)
	if !first || (first && name != "unnamed" && p.peek() != nil && p.peek().Type != TokenSectionSep) {
		p.sectionNames[name] = true
	}

	// Parse section content - if error, still create section node but return error
	content, err := p.parseSectionContent()

	// Always create section node even if there's an error
	section := NewSectionNode(content, nameToken, schemaToken)
	return section, err
}

// parseSectionAndSchemaNames extracts section name and schema reference.
// Returns (schemaToken, nameToken) - both can be nil. Order matches TypeScript implementation.
func (p *Parser) parseSectionAndSchemaNames() (*Token, *Token) {
	var nameToken, schemaToken *Token

	token := p.peek()
	if token == nil {
		return nil, nil
	}

	// Check for section name first
	if token.Type == TokenString && token.SubType == string(TokenSectionName) {
		nameToken = token
		p.advance()
		token = p.peek()

		// Check for schema reference after name
		if token != nil && token.Type == TokenString && token.SubType == string(TokenSectionSchema) {
			schemaToken = token
			p.advance()
		}
	} else if token.Type == TokenString && token.SubType == string(TokenSectionSchema) {
		// Schema without name
		schemaToken = token
		p.advance()
	}

	return schemaToken, nameToken
}

// parseSectionContent parses the content of a section.
// Returns ObjectNode, CollectionNode, or nil for empty sections.
func (p *Parser) parseSectionContent() (Node, error) {
	token := p.peek()
	if token == nil || token.Type == TokenSectionSep {
		return nil, nil
	}

	// Collection starts with ~
	if token.Type == TokenCollectionStart {
		return p.processCollection()
	}

	// Single object
	return p.processObject(false)
}

// processCollection parses a collection (multiple objects separated by ~).
func (p *Parser) processCollection() (*CollectionNode, error) {
	var children []Node
	var lastErr error

	for {
		token := p.peek()
		if token == nil || token.Type == TokenSectionSep {
			break
		}

		if token.Type != TokenCollectionStart {
			break
		}

		// Skip the ~ token
		p.advance()

		// Parse the object
		obj, err := p.processObject(true)
		if err != nil {
			lastErr = err
			// Create error node and continue
			errorNode := NewErrorNode(err, p.currentPosition())
			children = append(children, errorNode)
			// Skip to next collection item
			p.skipToNextCollectionItem()
			continue
		}

		children = append(children, obj)
	}

	return NewCollectionNode(children), lastErr
}

// skipToNextCollectionItem skips tokens until next ~ or section separator.
// Used for error recovery in collections.
func (p *Parser) skipToNextCollectionItem() {
	for {
		token := p.peek()
		if token == nil || token.Type == TokenCollectionStart || token.Type == TokenSectionSep {
			break
		}
		p.advance()
	}
}

// processObject parses an object following TypeScript implementation rules.
// isCollectionContext indicates if we're inside a collection.
func (p *Parser) processObject(isCollectionContext bool) (*ObjectNode, error) {
	obj, err := p.parseObject(true) // Always parse as open object initially
	if err != nil {
		return obj, err
	}

	// Check for pending tokens after parsing object
	token := p.peek()
	if err := p.checkForPendingTokens(token, isCollectionContext); err != nil {
		return obj, err
	}

	// Unwrap single-member objects without keys (TypeScript logic)
	// For example: { {} } should be unwrapped to {}
	if len(obj.Members) == 1 {
		firstMember := obj.Members[0]
		if firstMember != nil && firstMember.Key == nil && firstMember.Value != nil {
			if nestedObj, ok := firstMember.Value.(*ObjectNode); ok {
				return nestedObj, nil
			}
		}
	}

	return obj, nil
}

// checkForPendingTokens validates that no unexpected tokens remain after object.
func (p *Parser) checkForPendingTokens(token *Token, isCollectionContext bool) error {
	if token == nil {
		return nil
	}

	if token.Type == TokenSectionSep {
		return nil
	}

	if isCollectionContext && token.Type == TokenCollectionStart {
		return nil
	}

	return NewSyntaxError(
		ErrorUnexpectedToken,
		fmt.Sprintf("Unexpected token '%v'. Expected end of section or start of new collection item '~'.", token.Value),
		token.Position,
	)
}

// parseObject parses an object with explicit or implicit braces.
// isOpenObject indicates if the object can be without curly braces.
func (p *Parser) parseObject(isOpenObject bool) (*ObjectNode, error) {
	var members []*MemberNode
	var openBracket, closeBracket *Token

	token := p.peek()
	if isOpenObject {
		openBracket = nil
	} else {
		openBracket = token
	}

	// Consume opening bracket for explicit objects
	if !isOpenObject {
		if token == nil || token.Type != TokenCurlyOpen {
			return nil, NewSyntaxError(
				ErrorExpectedToken,
				"Expected '{' to start object",
				p.currentPosition(),
			)
		}
		p.advance()
	}

	index := 0
	for {
		nextToken := p.peek()

		// Check for end conditions
		if nextToken == nil || p.match([]TokenType{TokenCurlyClose, TokenCollectionStart, TokenSectionSep}) {
			break
		}

		// Handle comma (potential undefined member)
		if nextToken.Type == TokenComma {
			// Check if this comma represents an undefined value
			if p.matchNext([]TokenType{TokenComma, TokenCurlyClose, TokenCollectionStart, TokenSectionSep}) ||
				p.current+1 == len(p.tokens) {
				p.pushUndefinedMember(&members, nextToken)
			}
			p.advance()
			continue
		}

		// Validate comma separator between members
		if index > 0 {
			if !p.matchPrev([]TokenType{TokenComma, TokenCurlyOpen}) {
				return nil, NewSyntaxError(
					ErrorUnexpectedToken,
					fmt.Sprintf("Missing comma before '%v'. Object members must be separated by commas.", nextToken.Value),
					nextToken.Position,
				)
			}
		}

		// Parse the member
		member, err := p.parseMember()
		if err != nil {
			return nil, err
		}
		members = append(members, member)
		index++
	}

	// Handle closing bracket for explicit objects
	if !isOpenObject {
		if !p.match([]TokenType{TokenCurlyClose}) {
			return nil, NewSyntaxError(
				ErrorExpectingBracket,
				"Missing closing brace '}'. Object must be properly closed.",
				p.currentPosition(),
			)
		}
		closeBracket = p.peek()
		p.advance()
		return NewObjectNode(members, openBracket, closeBracket), nil
	}

	return NewObjectNode(members, nil, nil), nil
}

// skipToNextMember skips tokens until next comma or object end.
// Used for error recovery in objects.
func (p *Parser) skipToNextMember() {
	for {
		token := p.peek()
		if token == nil || token.Type == TokenComma || token.Type == TokenCurlyClose ||
			token.Type == TokenSectionSep || token.Type == TokenCollectionStart {
			break
		}
		p.advance()
	}
}

// parseMember parses a key-value pair or single value following TypeScript rules.
func (p *Parser) parseMember() (*MemberNode, error) {
	leftToken := p.peek()
	if leftToken == nil {
		return nil, NewSyntaxError(
			ErrorUnexpectedEOF,
			"Unexpected end of input while parsing member",
			p.currentPosition(),
		)
	}

	// Check if next token is a colon (key: value pair)
	if p.matchNext([]TokenType{TokenColon}) {
		// Validate key type
		validKeyTypes := []TokenType{TokenString, TokenNumber, TokenBoolean, TokenNull}
		isValidKey := false
		for _, t := range validKeyTypes {
			if leftToken.Type == t {
				isValidKey = true
				break
			}
		}

		if !isValidKey {
			return nil, NewSyntaxError(
				ErrorInvalidKey,
				fmt.Sprintf("Invalid key '%s'. Object keys must be strings, numbers, booleans, or null.", leftToken.Raw),
				leftToken.Position,
			)
		}

		// Consume key and colon
		p.advance() // consume key
		p.advance() // consume colon

		// Parse the value
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		return NewMemberNode(value, leftToken), nil
	}

	// No colon - it's a value without a key
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return NewMemberNode(value, nil), nil
}

// parseValue parses a single value following TypeScript implementation.
func (p *Parser) parseValue() (Node, error) {
	token := p.peek()
	if token == nil {
		return nil, NewSyntaxError(
			ErrorValueRequired,
			"Unexpected end of input. Expected a value (string, number, boolean, null, array, or object).",
			p.currentPosition(),
		)
	}

	switch token.Type {
	case TokenString, TokenNumber, TokenBigInt, TokenDecimal, TokenBoolean, TokenNull, TokenDateTime, TokenDate, TokenTime:
		// Primitive value - create TokenNode and advance
		node := NewTokenNode(token)
		p.advance()
		return node, nil

	case TokenBracketOpen:
		// Array
		return p.parseArray()

	case TokenCurlyOpen:
		// Nested object
		return p.parseObject(false)

	default:
		return nil, NewSyntaxError(
			ErrorUnexpectedToken,
			fmt.Sprintf("Unexpected token '%v'. Expected a valid value (string, number, boolean, null, array, or object).", token.Value),
			token.Position,
		)
	}
}

// parseArray parses an array following TypeScript implementation rules.
func (p *Parser) parseArray() (*ArrayNode, error) {
	var elements []Node

	openBracket := p.peek()
	if openBracket == nil || openBracket.Type != TokenBracketOpen {
		return nil, NewSyntaxError(
			ErrorExpectingBracket,
			fmt.Sprintf("Expected opening bracket '[' to start array but found '%s'.", p.tokenString(openBracket)),
			p.currentPosition(),
		)
	}

	// Consume opening bracket
	p.advance()

	for {
		currentToken := p.peek()
		if currentToken == nil {
			// Unexpected end of input
			return nil, NewSyntaxErrorEOF(
				ErrorExpectingBracket,
				"Unexpected end of input while parsing array. Expected closing bracket ']'.",
			)
		}

		if currentToken.Type == TokenBracketClose {
			break
		}

		if currentToken.Type == TokenComma {
			// Check for empty array elements (not allowed)
			if p.matchNext([]TokenType{TokenComma, TokenBracketClose}) {
				nextToken := p.tokens[p.current+1]
				return nil, NewSyntaxError(
					ErrorUnexpectedToken,
					"Unexpected comma. Array elements cannot be empty - remove the extra comma or add a value.",
					nextToken.Position,
				)
			}
			// Consume comma
			p.advance()
			continue
		}

		// Parse member (could be key:value or just value)
		member, err := p.parseMember()
		if err != nil {
			return nil, err
		}

		// If member has a key, wrap in ObjectNode
		if member.Key != nil {
			elements = append(elements, NewObjectNode([]*MemberNode{member}, nil, nil))
		} else {
			elements = append(elements, member.Value)
		}
	}

	// Expect closing bracket
	if !p.match([]TokenType{TokenBracketClose}) {
		return nil, NewSyntaxError(
			ErrorExpectingBracket,
			"Missing closing bracket ']'. Array must be properly closed.",
			p.currentPosition(),
		)
	}

	closeBracket := p.peek()
	p.advance()

	return NewArrayNode(elements, openBracket, closeBracket), nil
}

// skipToNextArrayElement skips tokens until next comma or array end.
// Used for error recovery in arrays.
func (p *Parser) skipToNextArrayElement() {
	for {
		token := p.peek()
		if token == nil || token.Type == TokenComma || token.Type == TokenBracketClose {
			break
		}
		p.advance()
	}
}

// pushUndefinedMember adds an undefined member to the members list.
// This handles cases like trailing commas or empty values.
func (p *Parser) pushUndefinedMember(members *[]*MemberNode, currentCommaToken *Token) {
	// Clone the token and change it to undefined
	valueToken := &Token{
		Type:     TokenUndefined,
		SubType:  "",
		Value:    nil,
		Raw:      currentCommaToken.Raw,
		Position: currentCommaToken.Position,
	}
	valueNode := NewTokenNode(valueToken)
	member := NewMemberNode(valueNode, nil)
	*members = append(*members, member)
}

// match checks if the current token matches any of the given types.
func (p *Parser) match(types []TokenType) bool {
	token := p.peek()
	if token == nil {
		return false
	}
	for _, t := range types {
		if token.Type == t {
			return true
		}
	}
	return false
}

// matchNext checks if the next token (without advancing) matches any of the given types.
func (p *Parser) matchNext(types []TokenType) bool {
	if p.current+1 >= len(p.tokens) {
		return false
	}
	nextToken := p.tokens[p.current+1]
	if nextToken == nil {
		return false
	}
	for _, t := range types {
		if nextToken.Type == t {
			return true
		}
	}
	return false
}

// matchPrev checks if the previous token matches any of the given types.
func (p *Parser) matchPrev(types []TokenType) bool {
	if p.current == 0 {
		return false
	}
	prevToken := p.tokens[p.current-1]
	if prevToken == nil {
		return false
	}
	for _, t := range types {
		if prevToken.Type == t {
			return true
		}
	}
	return false
}

// tokenString returns a string representation of a token for error messages.
func (p *Parser) tokenString(token *Token) string {
	if token == nil {
		return "end of input"
	}
	if token.Raw != "" {
		return token.Raw
	}
	return fmt.Sprintf("%v", token.Value)
}

// peek returns the current token without advancing.
func (p *Parser) peek() *Token {
	if p.current >= len(p.tokens) {
		return nil
	}
	return p.tokens[p.current]
}

// advance moves to the next token.
func (p *Parser) advance() {
	if p.current < len(p.tokens) {
		p.current++
	}
}

// currentPosition returns the current position in the input.
func (p *Parser) currentPosition() PositionRange {
	if p.current >= len(p.tokens) {
		// Return last position if at end
		if len(p.tokens) > 0 {
			last := p.tokens[len(p.tokens)-1]
			return last.Position
		}
		return NewPositionRange(NewPosition(1, 1, 0), NewPosition(1, 1, 0))
	}
	return p.tokens[p.current].Position
}
