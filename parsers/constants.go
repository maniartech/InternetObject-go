package parsers

// Literal constants for special values in Internet Object format
const (
	LiteralTrue  = "true"
	LiteralT     = "T"
	LiteralFalse = "false"
	LiteralF     = "F"
	LiteralNull  = "null"
	LiteralN     = "N"
	LiteralInf   = "Inf"
	LiteralNaN   = "NaN"
)

// Symbol constants for structural characters
const (
	SymbolCurlyOpen      = '{'
	SymbolCurlyClose     = '}'
	SymbolBracketOpen    = '['
	SymbolBracketClose   = ']'
	SymbolColon          = ':'
	SymbolComma          = ','
	SymbolTilde          = '~'
	SymbolMinus          = '-'
	SymbolPlus           = '+'
	SymbolDot            = '.'
	SymbolBackslash      = '\\'
	SymbolHash           = '#'
	SymbolDoubleQuote    = '"'
	SymbolSingleQuote    = '\''
	SymbolSpace          = ' '
	SymbolTab            = '\t'
	SymbolNewline        = '\n'
	SymbolCarriageReturn = '\r'
)

// Character classification constants
const (
	CharZero   = '0'
	CharNine   = '9'
	CharA      = 'a'
	CharF      = 'f'
	CharAUpper = 'A'
	CharFUpper = 'F'
	CharX      = 'x'
	CharXUpper = 'X'
	CharO      = 'o'
	CharOUpper = 'O'
	CharB      = 'b'
	CharBUpper = 'B'
	CharE      = 'e'
	CharEUpper = 'E'
	CharN      = 'n'
	CharM      = 'm'
	CharR      = 'r'
	CharD      = 'd'
	CharT      = 't'
	CharU      = 'u'
)

// Number format prefixes
const (
	PrefixHex       = "0x"
	PrefixHexAlt    = "0X"
	PrefixOctal     = "0o"
	PrefixOctalAlt  = "0O"
	PrefixBinary    = "0b"
	PrefixBinaryAlt = "0B"
)

// String annotation prefixes
const (
	AnnotationRaw      = "r"  // Raw string r"..."
	AnnotationByte     = "b"  // Byte/binary string b"..."
	AnnotationDateTime = "dt" // DateTime dt"..."
	AnnotationDate     = "d"  // Date d"..."
	AnnotationTime     = "t"  // Time t"..."
)

// Section separator
const SectionSeparator = "---"

// isDigit returns true if the character is a decimal digit (0-9).
func isDigit(ch rune) bool {
	return ch >= CharZero && ch <= CharNine
}

// isHexDigit returns true if the character is a hexadecimal digit (0-9, a-f, A-F).
func isHexDigit(ch rune) bool {
	return isDigit(ch) || (ch >= CharA && ch <= CharF) || (ch >= CharAUpper && ch <= CharFUpper)
}

// isOctalDigit returns true if the character is an octal digit (0-7).
func isOctalDigit(ch rune) bool {
	return ch >= CharZero && ch <= '7'
}

// isBinaryDigit returns true if the character is a binary digit (0-1).
func isBinaryDigit(ch rune) bool {
	return ch == CharZero || ch == '1'
}

// isWhitespace returns true if the character is whitespace.
// Matches the TypeScript implementation which includes:
// - ASCII whitespace and control characters (U+0000 to U+0020)
// - Non-breaking space (U+00A0)
// - Various Unicode spaces (U+2000-U+200A, U+2028, U+2029, etc.)
func isWhitespace(ch rune) bool {
	// Fast path: ASCII whitespace and control characters (U+0000 to U+0020)
	if ch <= 0x20 {
		return true
	}

	// Fast path: Extended ASCII range (U+0021 to U+00FF) - only U+00A0 is whitespace
	if ch <= 0xFF {
		return ch == 0x00A0
	}

	// Fast path: Anything above U+FEFF is never whitespace
	if ch > 0xFEFF {
		return false
	}

	// Fast path: Unicode range U+2000-U+200A (various em/en spaces)
	if ch >= 0x2000 && ch <= 0x200A {
		return true
	}

	// Lookup for remaining Unicode whitespace characters
	switch ch {
	case 0x1680, // Ogham space mark
		0x2028, // Line separator
		0x2029, // Paragraph separator
		0x202F, // Narrow no-break space
		0x205F, // Medium mathematical space
		0x3000, // Ideographic space
		0xFEFF: // BOM/Zero width no-break space
		return true
	default:
		return false
	}
}

// isHorizontalWhitespace returns true if the character is horizontal whitespace (space or tab).
func isHorizontalWhitespace(ch rune) bool {
	return ch == SymbolSpace || ch == SymbolTab
}

// isSpecialSymbol returns true if the character is a special structural symbol.
func isSpecialSymbol(ch rune) bool {
	switch ch {
	case SymbolCurlyOpen, SymbolCurlyClose, SymbolBracketOpen, SymbolBracketClose,
		SymbolColon, SymbolComma, SymbolTilde:
		return true
	default:
		return false
	}
}

// isValidOpenStringChar returns true if the character is valid in an open string.
// Open strings cannot contain structural symbols or newlines.
func isValidOpenStringChar(ch rune) bool {
	return !isSpecialSymbol(ch) && ch != SymbolNewline && ch != SymbolCarriageReturn
}

// isAlpha returns true if the character is an alphabetic character.
func isAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// isAlphaNumeric returns true if the character is alphanumeric.
func isAlphaNumeric(ch rune) bool {
	return isAlpha(ch) || isDigit(ch)
}
