package parsers

// TokenType represents the type of a token in the Internet Object format.
// These types match the TypeScript implementation for consistency.
type TokenType string

// Token types - matching TypeScript implementation
const (
	// Structural tokens
	TokenCurlyOpen       TokenType = "CURLY_OPEN"
	TokenCurlyClose      TokenType = "CURLY_CLOSE"
	TokenBracketOpen     TokenType = "BRACKET_OPEN"
	TokenBracketClose    TokenType = "BRACKET_CLOSE"
	TokenColon           TokenType = "COLON"
	TokenComma           TokenType = "COMMA"
	TokenCollectionStart TokenType = "COLLECTION_START"
	TokenSectionSep      TokenType = "SECTION_SEP"

	// Value tokens
	TokenString    TokenType = "STRING"
	TokenNumber    TokenType = "NUMBER"
	TokenBigInt    TokenType = "BIGINT"
	TokenDecimal   TokenType = "DECIMAL"
	TokenBoolean   TokenType = "BOOLEAN"
	TokenNull      TokenType = "NULL"
	TokenUndefined TokenType = "UNDEFINED"
	TokenBinary    TokenType = "BINARY"

	// DateTime tokens
	TokenDateTime TokenType = "DATETIME"
	TokenDate     TokenType = "DATE"
	TokenTime     TokenType = "TIME"

	// Section tokens
	TokenSectionName   TokenType = "SECTION_NAME"
	TokenSectionSchema TokenType = "SECTION_SCHEMA"

	// Special tokens
	TokenWhitespace TokenType = "WHITESPACE"
	TokenUnknown    TokenType = "UNKNOWN"
	TokenError      TokenType = "ERROR"
)

// Token represents a lexical token in the Internet Object format.
// Tokens are immutable after creation for thread safety.
type Token struct {
	Type     TokenType     // Type of the token
	SubType  string        // Optional subtype (e.g., "RAW_STRING", "HEX", "OPEN_STRING")
	Value    interface{}   // Parsed value of the token
	Raw      string        // Raw text from source
	Position PositionRange // Position in source code
}

// NewToken creates a new token with the specified properties.
func NewToken(tokenType TokenType, value interface{}, raw string, pos PositionRange) *Token {
	return &Token{
		Type:     tokenType,
		Value:    value,
		Raw:      raw,
		Position: pos,
	}
}

// NewTokenWithSubType creates a new token with a subtype.
func NewTokenWithSubType(tokenType TokenType, subType string, value interface{}, raw string, pos PositionRange) *Token {
	return &Token{
		Type:     tokenType,
		SubType:  subType,
		Value:    value,
		Raw:      raw,
		Position: pos,
	}
}

// NewErrorToken creates an error token with error information.
func NewErrorToken(err error, raw string, pos PositionRange) *Token {
	return &Token{
		Type:     TokenError,
		Value:    err,
		Raw:      raw,
		Position: pos,
	}
}

// Clone creates a deep copy of the token.
// This is useful when a token needs to be modified (e.g., type conversion).
func (t *Token) Clone() *Token {
	return &Token{
		Type:     t.Type,
		SubType:  t.SubType,
		Value:    t.Value,
		Raw:      t.Raw,
		Position: t.Position,
	}
}

// IsError returns true if this is an error token.
func (t *Token) IsError() bool {
	return t.Type == TokenError
}

// IsStructural returns true if this is a structural token (brackets, braces, etc.).
func (t *Token) IsStructural() bool {
	switch t.Type {
	case TokenCurlyOpen, TokenCurlyClose, TokenBracketOpen, TokenBracketClose,
		TokenColon, TokenComma, TokenCollectionStart, TokenSectionSep:
		return true
	default:
		return false
	}
}

// IsValue returns true if this is a value token.
func (t *Token) IsValue() bool {
	switch t.Type {
	case TokenString, TokenNumber, TokenBigInt, TokenDecimal, TokenBoolean,
		TokenNull, TokenUndefined, TokenBinary, TokenDateTime, TokenDate, TokenTime:
		return true
	default:
		return false
	}
}

// GetStartPos returns the starting position (implements PositionRange interface).
func (t *Token) GetStartPos() Position {
	return t.Position.Start
}

// GetEndPos returns the ending position (implements PositionRange interface).
func (t *Token) GetEndPos() Position {
	return t.Position.End
}
