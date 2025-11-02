package parsers

import (
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Tokenizer performs lexical analysis on Internet Object format input.
// It converts the input string into a sequence of tokens.
// The tokenizer is not thread-safe for concurrent use on the same instance,
// but multiple tokenizers can operate concurrently on different inputs.
type Tokenizer struct {
	input       string          // Input string to tokenize
	pos         int             // Current byte position
	row         int             // Current row (1-indexed)
	col         int             // Current column (1-indexed)
	reachedEnd  bool            // True if we've reached end of input
	inputLength int             // Cached input length
	tokens      []Token         // Collected tokens (value-based for zero-alloc)
	builder     strings.Builder // Reusable string builder to reduce allocations
}

// NewTokenizer creates a new tokenizer for the given input string.
func NewTokenizer(input string) *Tokenizer {
	// Better capacity estimation: avg 5-6 chars per token
	capacity := len(input) / 5
	if capacity < 16 {
		capacity = 16
	}
	return &Tokenizer{
		input:       input,
		pos:         0,
		row:         1,
		col:         1,
		reachedEnd:  false,
		inputLength: len(input),
		tokens:      make([]Token, 0, capacity),
	}
}

// Tokenize processes the entire input and returns all tokens.
// Returns an error if tokenization fails unrecoverably.
func (t *Tokenizer) Tokenize() ([]Token, error) {
	for !t.reachedEnd {
		if !t.tokenizeNext() {
			break
		}
	}
	// Trim capacity to avoid holding excess memory after tokenization
	if cap(t.tokens) > len(t.tokens) {
		trimmed := make([]Token, len(t.tokens))
		copy(trimmed, t.tokens)
		t.tokens = trimmed
	}
	return t.tokens, nil
}

// tokenizeNext processes the next token from the input.
// Returns false if end of input is reached, true otherwise.
func (t *Tokenizer) tokenizeNext() bool {
	if t.pos >= t.inputLength {
		t.reachedEnd = true
		return false
	}

	ch := rune(t.input[t.pos])

	// Whitespace - skip
	if isWhitespace(ch) {
		t.advance(1)
		return true
	}

	// Single-line comments
	if ch == SymbolHash {
		t.parseSingleLineComment()
		return true
	}

	// Regular strings
	if ch == SymbolDoubleQuote || ch == SymbolSingleQuote {
		token := t.parseRegularString(ch)
		t.tokens = append(t.tokens, *token)
		return true
	}

	// Special symbols
	if isSpecialSymbol(ch) {
		token := t.parseSpecialSymbol(ch)
		t.tokens = append(t.tokens, *token)
		return true
	}

	// Numbers
	if ch == SymbolPlus || ch == SymbolMinus || ch == SymbolDot || isDigit(ch) {
		// Check for section separator ---
		if ch == SymbolMinus && t.pos+2 < t.inputLength &&
			t.input[t.pos:t.pos+3] == SectionSeparator {
			t.parseSectionSeparator()
			return true
		}

		token := t.parseNumber()
		if token != nil {
			// Check if number is followed by non-whitespace/non-symbol (making it an open string)
			spaces := t.skipWhitespaces()
			if !t.reachedEnd && !isSpecialSymbol(rune(t.input[t.pos])) && !isWhitespace(rune(t.input[t.pos])) {
				nextToken := t.parseLiteralOrOpenString()
				if nextToken != nil {
					// Merge tokens
					merged := t.mergeTokens(token, nextToken, spaces)
					t.tokens = append(t.tokens, *merged)
					return true
				}
			}
			t.tokens = append(t.tokens, *token)
			return true
		}

		// Not a number, try literal or open string
		token = t.parseLiteralOrOpenString()
		if token != nil {
			t.tokens = append(t.tokens, *token)
		}
		return true
	}

	// Literals or open strings (including annotated strings)
	annotation := t.checkIfAnnotatedString()
	if annotation != nil {
		var token *Token
		switch annotation.name {
		case AnnotationRaw:
			token = t.parseRawString(annotation)
		case AnnotationByte:
			token = t.parseByteString(annotation)
		case AnnotationDate, AnnotationDateTime, AnnotationTime:
			token = t.parseDateTime(annotation)
		default:
			// Unsupported annotation - create error token
			start := t.currentPosition()
			msg := fmt.Sprintf("Unsupported annotation '%s'. Supported annotations are: 'r' (raw string), 'b' (binary), 'dt' (datetime), 'd' (date), 't' (time).", annotation.name)
			err := NewSyntaxError(ErrorUnsupportedAnnotation, msg, start)
			token = NewErrorToken(err, start)
			t.skipToNextTokenBoundary()
		}
		t.tokens = append(t.tokens, *token)
		return true
	}

	// Regular literal or open string
	token := t.parseLiteralOrOpenString()
	if token != nil {
		t.tokens = append(t.tokens, *token)
	}
	return true
}

// currentPosition returns the current position as a PositionRange.
func (t *Tokenizer) currentPosition() PositionRange {
	pos := NewPosition(t.pos, t.row, t.col)
	return NewPositionRange(pos, pos)
}

// advance moves the current position forward by the specified number of bytes.
// It properly tracks newlines for row/column counting.
func (t *Tokenizer) advance(count int) {
	for i := 0; i < count && t.pos < t.inputLength; i++ {
		if t.input[t.pos] == SymbolNewline {
			t.row++
			t.col = 1
		} else {
			t.col++
		}
		t.pos++
	}

	if t.pos >= t.inputLength {
		t.reachedEnd = true
	}
}

// peek returns the current rune without advancing.
// Returns 0 if at end of input.
func (t *Tokenizer) peek() rune {
	if t.reachedEnd || t.pos >= t.inputLength {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(t.input[t.pos:])
	return r
}

// skipWhitespaces skips over whitespace characters and returns them as a string.
// This is used to handle whitespace normalization in strings.
func (t *Tokenizer) skipWhitespaces() string {
	start := t.pos
	for !t.reachedEnd && isWhitespace(rune(t.input[t.pos])) {
		ch := t.input[t.pos]
		// Normalize \r\n or \r to \n
		if ch == SymbolCarriageReturn {
			if t.pos+1 < t.inputLength && t.input[t.pos+1] == SymbolNewline {
				t.advance(1)
			}
			t.advance(1)
		} else {
			t.advance(1)
		}
	}

	if start == t.pos {
		return ""
	}

	spaces := t.input[start:t.pos]
	// Normalize \r sequences
	if strings.Contains(spaces, string(SymbolCarriageReturn)) {
		spaces = strings.ReplaceAll(spaces, "\r\n", "\n")
		spaces = strings.ReplaceAll(spaces, "\r", "\n")
	}
	return spaces
}

// skipToNextTokenBoundary skips characters until a token boundary is found.
// This is used for error recovery.
func (t *Tokenizer) skipToNextTokenBoundary() {
	for !t.reachedEnd && !isWhitespace(rune(t.input[t.pos])) &&
		!isSpecialSymbol(rune(t.input[t.pos])) &&
		t.input[t.pos] != SymbolComma &&
		t.input[t.pos] != SymbolNewline {
		t.advance(1)
	}
}

// parseSingleLineComment skips over a single-line comment (from # to end of line).
func (t *Tokenizer) parseSingleLineComment() {
	for !t.reachedEnd && t.input[t.pos] != SymbolNewline {
		t.advance(1)
	}
}

// parseSpecialSymbol parses a structural symbol token.
func (t *Tokenizer) parseSpecialSymbol(ch rune) *Token {
	start := t.currentPosition()
	t.advance(1)

	var tokenType TokenType
	switch ch {
	case SymbolCurlyOpen:
		tokenType = TokenCurlyOpen
	case SymbolCurlyClose:
		tokenType = TokenCurlyClose
	case SymbolBracketOpen:
		tokenType = TokenBracketOpen
	case SymbolBracketClose:
		tokenType = TokenBracketClose
	case SymbolColon:
		tokenType = TokenColon
	case SymbolComma:
		tokenType = TokenComma
	case SymbolTilde:
		tokenType = TokenCollectionStart
	default:
		tokenType = TokenUnknown
	}

	end := t.currentPosition()
	return NewToken(tokenType, nil, NewPositionRange(start.Start, end.Start))
}

// parseRegularString parses a quoted string (single or double quoted).
func (t *Tokenizer) parseRegularString(encloser rune) *Token {
	start := t.currentPosition()
	// startPos not needed since we don't store raw per token

	t.advance(1) // Skip opening quote
	var value strings.Builder
	needToNormalize := false

	for !t.reachedEnd && rune(t.input[t.pos]) != encloser {
		if isWhitespace(rune(t.input[t.pos])) {
			spaces := t.skipWhitespaces()
			value.WriteString(spaces)
			continue
		}

		// Handle escape sequences
		if t.input[t.pos] == SymbolBackslash {
			escaped, normalize, err := t.escapeString()
			if err != nil {
				// For invalid escape sequences, treat as literal
				if !t.reachedEnd {
					escapeChar := t.input[t.pos]
					value.WriteByte(escapeChar)
					t.advance(1)

					// Handle \u and \x sequences
					if escapeChar == CharU {
						for i := 0; i < 4 && !t.reachedEnd; i++ {
							value.WriteByte(t.input[t.pos])
							t.advance(1)
						}
					} else if escapeChar == 'x' {
						for i := 0; i < 2 && !t.reachedEnd; i++ {
							value.WriteByte(t.input[t.pos])
							t.advance(1)
						}
					}
				}
				continue
			}
			value.WriteString(escaped)
			if normalize {
				needToNormalize = true
			}
		} else {
			value.WriteByte(t.input[t.pos])
			t.advance(1)
		}
	}

	// Check for unclosed string
	if t.reachedEnd {
		// raw := t.input[startPos:t.pos]
		end := t.currentPosition()
		err := NewSyntaxErrorEOF(ErrorStringNotClosed, "Unterminated string literal. Expected closing quote before end of input.")
		return NewErrorToken(err, NewPositionRange(start.Start, end.Start))
	}

	t.advance(1) // Skip closing quote

	finalValue := value.String()

	// Normalize if needed (NFC normalization for Unicode)
	if needToNormalize {
		// Go strings are already UTF-8, NFC normalization would require unicode/norm package
		// For now, skip normalization to keep dependencies minimal
	}

	end := t.currentPosition()
	return NewTokenWithSubType(TokenString, SubRegularString, finalValue, NewPositionRange(start.Start, end.Start))
}

// escapeString processes an escape sequence starting at current position (after backslash).
// Returns the escaped string, whether normalization is needed, and any error.
func (t *Tokenizer) escapeString() (string, bool, error) {
	t.advance(1) // Skip backslash
	if t.reachedEnd {
		return "", false, NewSyntaxErrorEOF(ErrorInvalidEscapeSeq, "Invalid escape sequence at end of input. Expected escape character after backslash.")
	}

	ch := t.input[t.pos]
	t.advance(1)

	switch ch {
	case 'b':
		return "\b", false, nil
	case 'f':
		return "\f", false, nil
	case 'n':
		return "\n", false, nil
	case 'r':
		return "\r", false, nil
	case 't':
		return "\t", false, nil
	case CharU:
		// Unicode escape \uXXXX
		if t.pos+4 > t.inputLength {
			return "", false, NewSyntaxError(ErrorInvalidEscapeSeq, "Invalid Unicode escape sequence. Expected 4 hexadecimal digits.", t.currentPosition())
		}
		hex := t.input[t.pos : t.pos+4]
		if !isHex4(hex) {
			return "", false, NewSyntaxError(ErrorInvalidEscapeSeq, fmt.Sprintf("Invalid Unicode escape sequence '\\u%s'. Expected 4 hexadecimal digits (0-9, A-F).", hex), t.currentPosition())
		}
		val, _ := strconv.ParseInt(hex, 16, 32)
		t.advance(4)
		return string(rune(val)), true, nil
	case 'x':
		// Hex escape \xXX
		if t.pos+2 > t.inputLength {
			return "", false, NewSyntaxError(ErrorInvalidEscapeSeq, "Invalid hexadecimal escape sequence. Expected 2 hexadecimal digits.", t.currentPosition())
		}
		hex := t.input[t.pos : t.pos+2]
		if !isHex2(hex) {
			return "", false, NewSyntaxError(ErrorInvalidEscapeSeq, fmt.Sprintf("Invalid hexadecimal escape sequence '\\x%s'. Expected 2 hexadecimal digits (0-9, A-F).", hex), t.currentPosition())
		}
		val, _ := strconv.ParseInt(hex, 16, 32)
		t.advance(2)
		return string(rune(val)), true, nil
	default:
		// Treat unrecognized escape as literal character
		return string(ch), false, nil
	}
}

// isHex4 checks if a string is exactly 4 hexadecimal digits (zero allocation)
func isHex4(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if !isHexDigit(rune(s[i])) {
			return false
		}
	}
	return true
}

// isHex2 checks if a string is exactly 2 hexadecimal digits (zero allocation)
func isHex2(s string) bool {
	if len(s) != 2 {
		return false
	}
	return isHexDigit(rune(s[0])) && isHexDigit(rune(s[1]))
}

// isValidBase64 checks if a string contains only valid base64 characters (zero allocation)
func isValidBase64(s string) bool {
	equalCount := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '=' {
			equalCount++
			if equalCount > 2 || i < len(s)-2 {
				return false
			}
		} else if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') || ch == '+' || ch == '/') {
			return false
		}
	}
	return true
}

// Annotation holds information about an annotated string prefix.
type Annotation struct {
	name  string
	quote rune
}

// checkIfAnnotatedString checks if the current position starts an annotated string.
// Returns annotation information if found, nil otherwise (zero allocation path).
func (t *Tokenizer) checkIfAnnotatedString() *Annotation {
	if t.pos+2 > t.inputLength {
		return nil
	}

	// Parse annotation name (1-4 alpha characters)
	nameStart := t.pos
	nameEnd := t.pos
	for nameEnd < t.inputLength && nameEnd-nameStart < 4 {
		ch := t.input[nameEnd]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
			break
		}
		nameEnd++
	}

	if nameEnd == nameStart {
		return nil // No alphabetic characters
	}

	// Next character must be a quote
	if nameEnd >= t.inputLength {
		return nil
	}

	quote := rune(t.input[nameEnd])
	if quote != '"' && quote != '\'' {
		return nil
	}

	name := t.input[nameStart:nameEnd]
	return &Annotation{
		name:  name,
		quote: quote,
	}
}

// parseAnnotatedString parses an annotated string (e.g., r"...", b"...", dt"...").
func (t *Tokenizer) parseAnnotatedString(annotation *Annotation) *Token {
	start := t.currentPosition()
	startPos := t.pos

	// Skip annotation prefix
	t.advance(len(annotation.name))
	t.advance(1) // Skip opening quote

	// Read until closing quote
	for !t.reachedEnd && rune(t.input[t.pos]) != annotation.quote {
		t.advance(1)
	}

	var value string

	if t.reachedEnd {
		// Unclosed annotated string - extract what we have
		value = t.input[startPos+len(annotation.name)+1 : t.pos]
	} else {
		t.advance(1) // Skip closing quote
		fullRaw := t.input[startPos:t.pos]
		value = fullRaw[len(annotation.name)+1 : len(fullRaw)-1]
	}

	end := t.currentPosition()
	token := NewToken(TokenString, value, NewPositionRange(start.Start, end.Start))
	return token
}

// parseRawString parses a raw string (r"...").
func (t *Tokenizer) parseRawString(annotation *Annotation) *Token {
	token := t.parseAnnotatedString(annotation)
	if token.Type == TokenError {
		return token
	}
	token.SubType = SubRawString
	return token
}

// parseByteString parses a byte/binary string (b"...").
func (t *Tokenizer) parseByteString(annotation *Annotation) *Token {
	token := t.parseAnnotatedString(annotation)
	if token.Type == TokenError {
		return token
	}

	// Validate and decode base64
	valueStr, ok := token.Value.(string)
	if !ok {
		return token
	}

	if !isValidBase64(valueStr) {
		err := NewSyntaxError(ErrorInvalidEscapeSeq, "Invalid base64 format in byte string", token.Position)
		return NewErrorToken(err, token.Position)
	}

	decoded, err := base64.StdEncoding.DecodeString(valueStr)
	if err != nil {
		syntaxErr := NewSyntaxError(ErrorInvalidEscapeSeq, fmt.Sprintf("Invalid base64 encoding: %v", err), token.Position)
		return NewErrorToken(syntaxErr, token.Position)
	}

	token.Type = TokenBinary
	token.SubType = SubBinaryString
	token.Value = decoded
	return token
}

// parseDateTime parses a datetime, date, or time string (dt"...", d"...", t"...").
func (t *Tokenizer) parseDateTime(annotation *Annotation) *Token {
	token := t.parseAnnotatedString(annotation)
	if token.Type == TokenError {
		return token
	}

	valueStr, ok := token.Value.(string)
	if !ok {
		return token
	}

	var parsedTime time.Time
	var parseErr error

	switch annotation.name {
	case AnnotationDateTime:
		parsedTime, parseErr = time.Parse(time.RFC3339, valueStr)
		token.SubType = SubDTDateTime
	case AnnotationDate:
		parsedTime, parseErr = time.Parse("2006-01-02", valueStr)
		token.SubType = SubDTDate
	case AnnotationTime:
		parsedTime, parseErr = time.Parse("15:04:05", valueStr)
		token.SubType = SubDTTime
	}

	if parseErr != nil {
		typeName := "datetime"
		if annotation.name == AnnotationDate {
			typeName = "date"
		} else if annotation.name == AnnotationTime {
			typeName = "time"
		}
		msg := fmt.Sprintf("Invalid %s format '%s'. Expected valid ISO 8601 format.", typeName, valueStr)
		err := NewSyntaxError(ErrorInvalidDateTime, msg, token.Position)
		return NewErrorToken(err, token.Position)
	}

	token.Type = TokenDateTime
	token.Value = parsedTime
	return token
}

// parseNumber parses a numeric literal (integer, float, hex, octal, binary, BigInt, Decimal, Inf, NaN).
func (t *Tokenizer) parseNumber() *Token {
	start := t.currentPosition()
	// startPos no longer needed since we don't store raw per token

	var rawValue strings.Builder
	base := 10
	hasDecimal := false
	hasExponent := false
	prefix := ""
	var subType TokenSubType

	// Handle sign
	if t.input[t.pos] == SymbolPlus || t.input[t.pos] == SymbolMinus {
		sign := string(t.input[t.pos])

		// Check for Infinity
		if t.pos+4 <= t.inputLength && t.input[t.pos+1:t.pos+4] == "Inf" {
			t.advance(4)
			end := t.currentPosition()
			val := math.Inf(1) // +Inf
			if sign == "-" {
				val = math.Inf(-1) // -Inf
			}
			// raw := t.input[startPos:t.pos]
			return NewToken(TokenNumber, val, NewPositionRange(start.Start, end.Start))
		}

		// Allow sign only if followed by digit or dot
		if t.pos+1 < t.inputLength && (isDigit(rune(t.input[t.pos+1])) || t.input[t.pos+1] == SymbolDot) {
			rawValue.WriteString(sign)
			t.advance(1)
		} else {
			return nil
		}
	} else if t.pos+3 <= t.inputLength && t.input[t.pos:t.pos+3] == "Inf" {
		// Infinity without sign
		t.advance(3)
		end := t.currentPosition()
		return NewToken(TokenNumber, math.Inf(1), NewPositionRange(start.Start, end.Start))
	}

	// Check for leading dot (decimal number starting with .)
	if t.input[t.pos] == SymbolDot {
		if t.pos+1 >= t.inputLength || !isDigit(rune(t.input[t.pos+1])) {
			return nil
		}
	}

	// Determine number format (hex, octal, binary, or decimal)
	if t.input[t.pos] == CharZero && t.pos+1 < t.inputLength {
		next := t.input[t.pos+1]
		switch next {
		case CharX, CharXUpper:
			base = 16
			subType = SubHex
			prefix = t.input[t.pos : t.pos+2]
			t.advance(2)
			for !t.reachedEnd && isHexDigit(rune(t.input[t.pos])) {
				rawValue.WriteByte(t.input[t.pos])
				t.advance(1)
			}
		case CharO, CharOUpper:
			base = 8
			subType = SubOctal
			prefix = t.input[t.pos : t.pos+2]
			t.advance(2)
			for !t.reachedEnd && isOctalDigit(rune(t.input[t.pos])) {
				rawValue.WriteByte(t.input[t.pos])
				t.advance(1)
			}
		case CharB, CharBUpper:
			base = 2
			subType = SubBinary
			prefix = t.input[t.pos : t.pos+2]
			t.advance(2)
			for !t.reachedEnd && isBinaryDigit(rune(t.input[t.pos])) {
				rawValue.WriteByte(t.input[t.pos])
				t.advance(1)
			}
		default:
			// Regular decimal number
			for !t.reachedEnd && isDigit(rune(t.input[t.pos])) {
				rawValue.WriteByte(t.input[t.pos])
				t.advance(1)
			}
		}
	} else {
		// Parse whole part
		for !t.reachedEnd && isDigit(rune(t.input[t.pos])) {
			rawValue.WriteByte(t.input[t.pos])
			t.advance(1)
		}
	}

	// Parse decimal point and fractional part (only for base 10)
	if base == 10 && !t.reachedEnd && t.input[t.pos] == SymbolDot {
		hasDecimal = true
		rawValue.WriteByte(SymbolDot)
		t.advance(1)
		for !t.reachedEnd && isDigit(rune(t.input[t.pos])) {
			rawValue.WriteByte(t.input[t.pos])
			t.advance(1)
		}
	}

	// Parse exponent (e.g., e10, E-5)
	if base == 10 && !t.reachedEnd && (t.input[t.pos] == CharE || t.input[t.pos] == CharEUpper) {
		hasExponent = true
		rawValue.WriteByte(t.input[t.pos])
		t.advance(1)
		if !t.reachedEnd && (t.input[t.pos] == SymbolPlus || t.input[t.pos] == SymbolMinus) {
			rawValue.WriteByte(t.input[t.pos])
			t.advance(1)
		}
		for !t.reachedEnd && isDigit(rune(t.input[t.pos])) {
			rawValue.WriteByte(t.input[t.pos])
			t.advance(1)
		}
	}

	rawStr := rawValue.String()
	if rawStr == "" {
		return nil
	}

	var tokenType TokenType = TokenNumber
	var value interface{}
	end := t.currentPosition()

	// Check for BigInt suffix 'n'
	if !t.reachedEnd && t.input[t.pos] == CharN {
		tokenType = TokenBigInt
		t.advance(1)
		// Parse as big.Int
		bigInt := new(big.Int)
		_, success := bigInt.SetString(prefix+rawStr, base)
		if !success {
			err := NewSyntaxError(ErrorUnexpectedToken, fmt.Sprintf("Invalid BigInt literal: %s", prefix+rawStr), NewPositionRange(start.Start, end.Start))
			return NewErrorToken(err, NewPositionRange(start.Start, end.Start))
		}
		value = bigInt
	} else if !t.reachedEnd && t.input[t.pos] == CharM {
		// Decimal suffix 'm' - for now treat as float64
		// A full implementation would use a decimal library
		tokenType = TokenDecimal
		t.advance(1)
		val, err := strconv.ParseFloat(rawStr, 64)
		if err != nil {
			syntaxErr := NewSyntaxError(ErrorUnexpectedToken, fmt.Sprintf("Invalid decimal literal: %s", rawStr), NewPositionRange(start.Start, end.Start))
			return NewErrorToken(syntaxErr, NewPositionRange(start.Start, end.Start))
		}
		value = val
	} else {
		// Regular number
		if base == 10 && (hasDecimal || hasExponent) {
			val, err := strconv.ParseFloat(rawStr, 64)
			if err != nil {
				syntaxErr := NewSyntaxError(ErrorUnexpectedToken, fmt.Sprintf("Invalid number: %s", rawStr), NewPositionRange(start.Start, end.Start))
				return NewErrorToken(syntaxErr, NewPositionRange(start.Start, end.Start))
			}
			value = val
		} else {
			val, err := strconv.ParseInt(rawStr, base, 64)
			if err != nil {
				syntaxErr := NewSyntaxError(ErrorUnexpectedToken, fmt.Sprintf("Invalid integer: %s", rawStr), NewPositionRange(start.Start, end.Start))
				return NewErrorToken(syntaxErr, NewPositionRange(start.Start, end.Start))
			}
			value = val
		}
	}

	end = t.currentPosition()
	if subType != SubNone {
		return NewTokenWithSubType(tokenType, subType, value, NewPositionRange(start.Start, end.Start))
	}
	return NewToken(tokenType, value, NewPositionRange(start.Start, end.Start))
}

// parseLiteralOrOpenString parses a literal value (true, false, null, NaN) or an open string.
func (t *Tokenizer) parseLiteralOrOpenString() *Token {
	start := t.currentPosition()

	var value strings.Builder

	for !t.reachedEnd && isValidOpenStringChar(rune(t.input[t.pos])) {
		ch := t.input[t.pos]

		if isWhitespace(rune(ch)) {
			spaces := t.skipWhitespaces()
			value.WriteString(spaces)
			continue
		}

		// Check for section separator
		if ch == SymbolMinus && t.pos+2 < t.inputLength && t.input[t.pos:t.pos+3] == SectionSeparator {
			break
		}

		// Handle escape sequences
		// Handle escape sequences
		if ch == SymbolBackslash {
			escaped, _, err := t.escapeString()
			if err != nil {
				// For open strings, preserve backslash and character
				value.WriteByte(SymbolBackslash)
				if !t.reachedEnd {
					escapeChar := t.input[t.pos]
					value.WriteByte(escapeChar)
					t.advance(1)

					if escapeChar == CharU {
						for i := 0; i < 4 && !t.reachedEnd; i++ {
							value.WriteByte(t.input[t.pos])
							t.advance(1)
						}
					} else if escapeChar == 'x' {
						for i := 0; i < 2 && !t.reachedEnd; i++ {
							value.WriteByte(t.input[t.pos])
							t.advance(1)
						}
					}
				}
				continue
			}
			value.WriteString(escaped)
		} else {
			value.WriteByte(ch)
			t.advance(1)
		}
	}

	str := strings.TrimRight(value.String(), " \t")
	if str == "" {
		return nil
	}

	// if needToNormalize {
	// 	// Unicode normalization would go here
	// }

	end := t.currentPosition()
	pos := NewPositionRange(start.Start, end.Start)

	// Check for literals
	switch str {
	case LiteralTrue, LiteralT:
		return NewToken(TokenBoolean, true, pos)
	case LiteralFalse, LiteralF:
		return NewToken(TokenBoolean, false, pos)
	case LiteralNull, LiteralN:
		return NewToken(TokenNull, nil, pos)
	case LiteralInf:
		return NewToken(TokenNumber, math.Inf(1), pos)
	case LiteralNaN:
		return NewToken(TokenNumber, math.NaN(), pos)
	default:
		return NewTokenWithSubType(TokenString, SubOpenString, str, pos)
	}
}

// parseSectionSeparator parses a section separator (---) and optional section name/schema.
func (t *Tokenizer) parseSectionSeparator() {
	start := t.currentPosition()

	// Parse ---
	t.advance(3)
	end := t.currentPosition()
	sepToken := NewToken(TokenSectionSep, nil, NewPositionRange(start.Start, end.Start))
	t.tokens = append(t.tokens, *sepToken)

	// Skip horizontal whitespace
	for !t.reachedEnd && isHorizontalWhitespace(rune(t.input[t.pos])) {
		t.advance(1)
	}

	// Try to match section name and/or schema (manual parsing for zero allocation)
	if t.reachedEnd {
		return
	}

	// Check for schema first ($identifier)
	if t.input[t.pos] == '$' {
		start := t.currentPosition()
		schemaStart := t.pos
		t.advance(1) // Skip $

		// Parse schema identifier
		for !t.reachedEnd && (isAlphaNumeric(rune(t.input[t.pos])) || t.input[t.pos] == '-' || t.input[t.pos] == '_') {
			t.advance(1)
		}

		schema := t.input[schemaStart:t.pos]
		end := t.currentPosition()
		token := NewTokenWithSubType(TokenString, SubSectionSchema, schema, NewPositionRange(start.Start, end.Start))
		t.tokens = append(t.tokens, *token)
		t.skipWhitespaces()
		return
	}

	// Try to parse name (letter/digit/dash/underscore identifier)
	if isAlphaNumeric(rune(t.input[t.pos])) || t.input[t.pos] == '-' || t.input[t.pos] == '_' {
		start := t.currentPosition()
		nameStart := t.pos

		for !t.reachedEnd && (isAlphaNumeric(rune(t.input[t.pos])) || t.input[t.pos] == '-' || t.input[t.pos] == '_') {
			t.advance(1)
		}

		name := t.input[nameStart:t.pos]
		end := t.currentPosition()
		token := NewTokenWithSubType(TokenString, SubSectionName, name, NewPositionRange(start.Start, end.Start))
		t.tokens = append(t.tokens, *token)
		t.skipWhitespaces()

		// Check for separator ':'
		if !t.reachedEnd && t.input[t.pos] == ':' {
			t.advance(1) // Skip colon
			t.skipWhitespaces()

			// Schema must follow separator
			if t.reachedEnd || t.input[t.pos] != '$' {
				err := NewSyntaxError(ErrorSchemaMissing, "Missing schema definition after section separator. Expected schema name starting with '$'.", t.currentPosition())
				errToken := NewErrorToken(err, t.currentPosition())
				t.tokens = append(t.tokens, *errToken)
				return
			}

			// Parse schema after colon
			start := t.currentPosition()
			schemaStart := t.pos
			t.advance(1) // Skip $

			for !t.reachedEnd && (isAlphaNumeric(rune(t.input[t.pos])) || t.input[t.pos] == '-' || t.input[t.pos] == '_') {
				t.advance(1)
			}

			schema := t.input[schemaStart:t.pos]
			end := t.currentPosition()
			token := NewTokenWithSubType(TokenString, SubSectionSchema, schema, NewPositionRange(start.Start, end.Start))
			t.tokens = append(t.tokens, *token)
			t.skipWhitespaces()
		}
	}
}

// mergeTokens merges two tokens into one (used when number is followed by open string).
func (t *Tokenizer) mergeTokens(first, second *Token, spaces string) *Token {
	// Reconstruct text from positions for first and use second.Value for the second part
	fStart := first.Position.Start.Pos
	fEnd := first.Position.End.Pos
	sStart := second.Position.Start.Pos
	sEnd := second.Position.End.Pos
	firstText := t.input[fStart:fEnd]
	_ = t.input[sStart:sEnd] // second raw not needed explicitly
	value := firstText + spaces + fmt.Sprint(second.Value)
	pos := NewPositionRange(first.Position.Start, second.Position.End)
	return NewTokenWithSubType(TokenString, SubOpenString, value, pos)
}
