package parsers

// TokenType represents the type of a token (compact uint8).
type TokenType uint8

// TokenSubType represents optional sub-classification for a token (compact uint8).
type TokenSubType uint8

// Token types (stable order; do not reorder without updating tests/benchmarks)
const (
	// Structural tokens
	TokenCurlyOpen TokenType = iota
	TokenCurlyClose
	TokenBracketOpen
	TokenBracketClose
	TokenColon
	TokenComma
	TokenCollectionStart
	TokenSectionSep

	// Value tokens
	TokenString
	TokenNumber
	TokenBigInt
	TokenDecimal
	TokenBoolean
	TokenNull
	TokenUndefined
	TokenBinary

	// DateTime umbrella (value stored as time.Time)
	TokenDateTime

	// Special tokens
	TokenWhitespace
	TokenUnknown
	TokenError
)

// Token subtypes for additional classification when needed
const (
	SubNone TokenSubType = iota
	// Strings
	SubRegularString
	SubOpenString
	SubRawString
	SubBinaryString
	// Date/Time variants
	SubDTDateTime
	SubDTDate
	SubDTTime
	// Number bases
	SubHex
	SubOctal
	SubBinary
	// Section markers attached to TokenString
	SubSectionName
	SubSectionSchema
)

// String returns a human-readable name for the token type.
func (tt TokenType) String() string {
	switch tt {
	case TokenCurlyOpen:
		return "{"
	case TokenCurlyClose:
		return "}"
	case TokenBracketOpen:
		return "["
	case TokenBracketClose:
		return "]"
	case TokenColon:
		return ":"
	case TokenComma:
		return ","
	case TokenCollectionStart:
		return "~"
	case TokenSectionSep:
		return "---"
	case TokenString:
		return "STRING"
	case TokenNumber:
		return "NUMBER"
	case TokenBigInt:
		return "BIGINT"
	case TokenDecimal:
		return "DECIMAL"
	case TokenBoolean:
		return "BOOLEAN"
	case TokenNull:
		return "NULL"
	case TokenUndefined:
		return "UNDEFINED"
	case TokenBinary:
		return "BINARY"
	case TokenDateTime:
		return "DATETIME"
	case TokenWhitespace:
		return "WHITESPACE"
	case TokenUnknown:
		return "UNKNOWN"
	case TokenError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Token represents a lexical token in the Internet Object format.
// Tokens are immutable after creation for thread safety.
type Token struct {
	Type     TokenType     // Type of the token
	SubType  TokenSubType  // Optional subtype (e.g., SubRawString, SubHex, SubOpenString)
	Value    interface{}   // Parsed value of the token
	Position PositionRange // Position in source code
}

// NewToken creates a new token with the specified properties.
func NewToken(tokenType TokenType, value interface{}, pos PositionRange) *Token {
	return &Token{
		Type:     tokenType,
		Value:    value,
		Position: pos,
	}
}

// NewTokenWithSubType creates a new token with a subtype.
func NewTokenWithSubType(tokenType TokenType, subType TokenSubType, value interface{}, pos PositionRange) *Token {
	return &Token{
		Type:     tokenType,
		SubType:  subType,
		Value:    value,
		Position: pos,
	}
}

// NewErrorToken creates an error token with error information.
func NewErrorToken(err error, pos PositionRange) *Token {
	return &Token{
		Type:     TokenError,
		Value:    err,
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
		TokenNull, TokenUndefined, TokenBinary, TokenDateTime:
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
