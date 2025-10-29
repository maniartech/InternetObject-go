package parsers

import (
	"fmt"
	"unsafe"
)

// FastParserBytes is a zero-allocation parser using byte slices
// This is the ultimate optimized version using []byte instead of string
type FastParserBytes struct {
	input  []byte
	pos    int
	length int

	// Arena-allocated arrays (pre-allocated, reused)
	valueArena  []FastValueBytes  // All values stored here
	memberArena []FastMemberBytes // All object members
	stringArena []byte            // String data storage

	// Indices for current parsing state
	valueCount   int
	memberCount  int
	stringOffset int
}

// FastValueBytes represents a value using indices instead of pointers
type FastValueBytes struct {
	Type ValueType

	// For primitives: direct storage
	IntValue   int64
	FloatValue float64
	BoolValue  bool

	// For strings: index into stringArena
	StringStart int
	StringLen   int

	// For objects/arrays: index into memberArena/valueArena
	FirstChild int // Index of first member/element
	ChildCount int // Number of children
}

// FastMemberBytes represents an object member
type FastMemberBytes struct {
	KeyStart int // Index into stringArena
	KeyLen   int
	ValueIdx int // Index into valueArena
}

// NewFastParserBytes creates a parser with pre-allocated arena from byte slice
func NewFastParserBytes(input []byte, estimatedValues int) *FastParserBytes {
	if estimatedValues == 0 {
		estimatedValues = 100
	}

	return &FastParserBytes{
		input:       input,
		length:      len(input),
		valueArena:  make([]FastValueBytes, 0, estimatedValues),
		memberArena: make([]FastMemberBytes, 0, estimatedValues),
		stringArena: make([]byte, 0, len(input)), // Max size is input length
	}
}

// NewFastParserBytesFromString creates a parser from a string (converts to bytes)
func NewFastParserBytesFromString(input string, estimatedValues int) *FastParserBytes {
	return NewFastParserBytes([]byte(input), estimatedValues)
}

// Parse parses the input and returns the root value index
func (p *FastParserBytes) Parse() (int, error) {
	p.skipWhitespace()
	rootIdx, err := p.parseValue()
	if err != nil {
		return -1, err
	}

	// Check for trailing content
	p.skipWhitespace()
	if p.pos < p.length {
		return -1, fmt.Errorf("unexpected content after root value at position %d", p.pos)
	}

	return rootIdx, nil
}

// parseValue parses any value and returns its index
func (p *FastParserBytes) parseValue() (int, error) {
	p.skipWhitespace()

	if p.pos >= p.length {
		return -1, fmt.Errorf("unexpected end of input")
	}

	ch := p.input[p.pos]

	switch ch {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		return p.parseQuotedString()
	case 't', 'f':
		return p.parseBoolean()
	case 'n':
		return p.parseNull()
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return p.parseNumber()
	default:
		// Try unquoted string
		return p.parseUnquotedString()
	}
}

// parseObject parses an object and returns its value index
func (p *FastParserBytes) parseObject() (int, error) {
	p.pos++ // skip '{'
	p.skipWhitespace()

	valueIdx := len(p.valueArena)
	memberStart := len(p.memberArena)
	memberCount := 0

	// Reserve space for the object value
	p.valueArena = append(p.valueArena, FastValueBytes{
		Type: TypeObject,
	})

	if p.pos < p.length && p.input[p.pos] == '}' {
		p.pos++
		p.valueArena[valueIdx].FirstChild = memberStart
		p.valueArena[valueIdx].ChildCount = 0
		return valueIdx, nil
	}

	for {
		p.skipWhitespace()

		// Parse key
		var keyStart, keyLen int
		if p.input[p.pos] == '"' {
			// Quoted key
			p.pos++
			start := p.pos
			for p.pos < p.length && p.input[p.pos] != '"' {
				p.pos++
			}
			keyStart = p.stringOffset
			keyLen = p.pos - start
			p.stringArena = append(p.stringArena, p.input[start:p.pos]...)
			p.stringOffset += keyLen
			p.pos++ // skip closing quote
		} else {
			// Unquoted key
			start := p.pos
			for p.pos < p.length && !isKeyTerminator(p.input[p.pos]) {
				p.pos++
			}
			keyStart = p.stringOffset
			keyLen = p.pos - start
			p.stringArena = append(p.stringArena, p.input[start:p.pos]...)
			p.stringOffset += keyLen
		}

		p.skipWhitespace()

		// Expect colon
		if p.pos >= p.length || p.input[p.pos] != ':' {
			return -1, fmt.Errorf("expected ':' after key")
		}
		p.pos++
		p.skipWhitespace()

		// Parse value
		valIdx, err := p.parseValue()
		if err != nil {
			return -1, err
		}

		// Check for duplicate keys
		for i := 0; i < memberCount; i++ {
			existingMember := &p.memberArena[memberStart+i]
			existingKey := p.stringArena[existingMember.KeyStart : existingMember.KeyStart+existingMember.KeyLen]
			newKey := p.stringArena[keyStart : keyStart+keyLen]

			if len(existingKey) == len(newKey) && string(existingKey) == string(newKey) {
				return -1, fmt.Errorf("duplicate key '%s' in object", string(newKey))
			}
		}

		// Add member
		p.memberArena = append(p.memberArena, FastMemberBytes{
			KeyStart: keyStart,
			KeyLen:   keyLen,
			ValueIdx: valIdx,
		})
		memberCount++

		p.skipWhitespace()

		if p.pos >= p.length {
			return -1, fmt.Errorf("unexpected end in object")
		}

		if p.input[p.pos] == '}' {
			p.pos++
			break
		}

		if p.input[p.pos] == ',' {
			p.pos++
			continue
		}

		return -1, fmt.Errorf("expected ',' or '}' in object")
	}

	p.valueArena[valueIdx].FirstChild = memberStart
	p.valueArena[valueIdx].ChildCount = memberCount
	return valueIdx, nil
}

// parseArray parses an array and returns its value index
func (p *FastParserBytes) parseArray() (int, error) {
	p.pos++ // skip '['
	p.skipWhitespace()

	valueIdx := len(p.valueArena)

	// Reserve space for the array value
	p.valueArena = append(p.valueArena, FastValueBytes{
		Type: TypeArray,
	})

	if p.pos < p.length && p.input[p.pos] == ']' {
		p.pos++
		p.valueArena[valueIdx].FirstChild = len(p.valueArena)
		p.valueArena[valueIdx].ChildCount = 0
		return valueIdx, nil
	}

	// Record where children start
	childStart := len(p.valueArena)
	childCount := 0

	for {
		p.skipWhitespace()

		// Parse element
		_, err := p.parseValue()
		if err != nil {
			return -1, err
		}
		childCount++

		p.skipWhitespace()

		if p.pos >= p.length {
			return -1, fmt.Errorf("unexpected end in array")
		}

		if p.input[p.pos] == ']' {
			p.pos++
			break
		}

		if p.input[p.pos] == ',' {
			p.pos++
			continue
		}

		return -1, fmt.Errorf("expected ',' or ']' in array")
	}

	p.valueArena[valueIdx].FirstChild = childStart
	p.valueArena[valueIdx].ChildCount = childCount
	return valueIdx, nil
}

// parseQuotedString parses a quoted string with escape sequence processing
func (p *FastParserBytes) parseQuotedString() (int, error) {
	p.pos++ // skip opening quote
	stringStart := p.stringOffset

	for p.pos < p.length && p.input[p.pos] != '"' {
		ch := p.input[p.pos]

		if ch == '\\' {
			// Handle escape sequences
			p.pos++
			if p.pos >= p.length {
				return -1, fmt.Errorf("unterminated string: unexpected end after escape")
			}

			switch p.input[p.pos] {
			case '"':
				p.stringArena = append(p.stringArena, '"')
				p.stringOffset++
			case '\\':
				p.stringArena = append(p.stringArena, '\\')
				p.stringOffset++
			case '/':
				p.stringArena = append(p.stringArena, '/')
				p.stringOffset++
			case 'b':
				p.stringArena = append(p.stringArena, '\b')
				p.stringOffset++
			case 'f':
				p.stringArena = append(p.stringArena, '\f')
				p.stringOffset++
			case 'n':
				p.stringArena = append(p.stringArena, '\n')
				p.stringOffset++
			case 'r':
				p.stringArena = append(p.stringArena, '\r')
				p.stringOffset++
			case 't':
				p.stringArena = append(p.stringArena, '\t')
				p.stringOffset++
			case 'u':
				// Unicode escape: \uXXXX
				p.pos++
				if p.pos+3 >= p.length {
					return -1, fmt.Errorf("invalid unicode escape: incomplete")
				}

				// Parse 4 hex digits
				var codepoint uint16
				for i := 0; i < 4; i++ {
					ch := p.input[p.pos+i]
					// Check for incomplete sequence (string ends early)
					if ch == '"' {
						return -1, fmt.Errorf("invalid unicode escape: incomplete")
					}
					var digit uint16
					if ch >= '0' && ch <= '9' {
						digit = uint16(ch - '0')
					} else if ch >= 'a' && ch <= 'f' {
						digit = uint16(ch - 'a' + 10)
					} else if ch >= 'A' && ch <= 'F' {
						digit = uint16(ch - 'A' + 10)
					} else {
						return -1, fmt.Errorf("invalid unicode escape: non-hex character")
					}
					codepoint = codepoint*16 + digit
				}
				p.pos += 3 // Will be incremented by 1 at end of loop

				// Convert codepoint to UTF-8 bytes
				if codepoint < 0x80 {
					p.stringArena = append(p.stringArena, byte(codepoint))
					p.stringOffset++
				} else if codepoint < 0x800 {
					p.stringArena = append(p.stringArena,
						byte(0xC0|((codepoint>>6)&0x1F)),
						byte(0x80|(codepoint&0x3F)))
					p.stringOffset += 2
				} else {
					p.stringArena = append(p.stringArena,
						byte(0xE0|((codepoint>>12)&0x0F)),
						byte(0x80|((codepoint>>6)&0x3F)),
						byte(0x80|(codepoint&0x3F)))
					p.stringOffset += 3
				}
			default:
				return -1, fmt.Errorf("invalid escape sequence: \\%c", p.input[p.pos])
			}
			p.pos++
		} else {
			// Validate UTF-8 encoding
			if ch < 0x20 {
				return -1, fmt.Errorf("invalid control character in string at position %d", p.pos)
			}

			// Check for valid UTF-8 sequence
			if ch < 0x80 {
				// Single-byte ASCII
				p.stringArena = append(p.stringArena, ch)
				p.stringOffset++
				p.pos++
			} else if ch < 0xC0 {
				// Invalid: continuation byte without lead byte
				return -1, fmt.Errorf("invalid UTF-8: unexpected continuation byte at position %d", p.pos)
			} else if ch < 0xE0 {
				// 2-byte sequence
				if p.pos+1 >= p.length || p.input[p.pos+1] == '"' {
					return -1, fmt.Errorf("invalid UTF-8: incomplete 2-byte sequence")
				}
				if (p.input[p.pos+1] & 0xC0) != 0x80 {
					return -1, fmt.Errorf("invalid UTF-8: invalid continuation byte")
				}
				p.stringArena = append(p.stringArena, ch, p.input[p.pos+1])
				p.stringOffset += 2
				p.pos += 2
			} else if ch < 0xF0 {
				// 3-byte sequence
				if p.pos+2 >= p.length || p.input[p.pos+1] == '"' || p.input[p.pos+2] == '"' {
					return -1, fmt.Errorf("invalid UTF-8: incomplete 3-byte sequence")
				}
				if (p.input[p.pos+1]&0xC0) != 0x80 || (p.input[p.pos+2]&0xC0) != 0x80 {
					return -1, fmt.Errorf("invalid UTF-8: invalid continuation byte")
				}
				p.stringArena = append(p.stringArena, ch, p.input[p.pos+1], p.input[p.pos+2])
				p.stringOffset += 3
				p.pos += 3
			} else if ch < 0xF8 {
				// 4-byte sequence
				if p.pos+3 >= p.length || p.input[p.pos+1] == '"' || p.input[p.pos+2] == '"' || p.input[p.pos+3] == '"' {
					return -1, fmt.Errorf("invalid UTF-8: incomplete 4-byte sequence")
				}
				if (p.input[p.pos+1]&0xC0) != 0x80 || (p.input[p.pos+2]&0xC0) != 0x80 || (p.input[p.pos+3]&0xC0) != 0x80 {
					return -1, fmt.Errorf("invalid UTF-8: invalid continuation byte")
				}
				p.stringArena = append(p.stringArena, ch, p.input[p.pos+1], p.input[p.pos+2], p.input[p.pos+3])
				p.stringOffset += 4
				p.pos += 4
			} else {
				return -1, fmt.Errorf("invalid UTF-8: invalid start byte at position %d", p.pos)
			}
		}
	}

	if p.pos >= p.length {
		return -1, fmt.Errorf("unterminated string")
	}

	valueIdx := len(p.valueArena)
	stringLen := p.stringOffset - stringStart
	p.pos++ // skip closing quote

	p.valueArena = append(p.valueArena, FastValueBytes{
		Type:        TypeString,
		StringStart: stringStart,
		StringLen:   stringLen,
	})

	return valueIdx, nil
}

// parseUnquotedString parses an unquoted string
func (p *FastParserBytes) parseUnquotedString() (int, error) {
	start := p.pos

	for p.pos < p.length && !isValueTerminator(p.input[p.pos]) {
		p.pos++
	}

	valueIdx := len(p.valueArena)
	stringStart := p.stringOffset
	stringLen := p.pos - start

	p.stringArena = append(p.stringArena, p.input[start:p.pos]...)
	p.stringOffset += stringLen

	p.valueArena = append(p.valueArena, FastValueBytes{
		Type:        TypeString,
		StringStart: stringStart,
		StringLen:   stringLen,
	})

	return valueIdx, nil
}

// parseNumber parses a number (int or float) directly from bytes
func (p *FastParserBytes) parseNumber() (int, error) {
	start := p.pos
	hasDecimal := false
	isNegative := false

	if p.input[p.pos] == '-' {
		isNegative = true
		p.pos++
	}

	// Fast integer parsing with overflow detection
	var intVal int64 = 0
	var floatVal float64 = 0
	const maxInt64Div10 = 922337203685477580 // math.MaxInt64 / 10
	const maxInt64Mod10 = 7                  // math.MaxInt64 % 10
	const minInt64Mod10 = 8                  // -math.MinInt64 % 10 (for negative overflow check)

	// Parse integer part
	for p.pos < p.length {
		ch := p.input[p.pos]
		if ch >= '0' && ch <= '9' {
			digit := int64(ch - '0')

			// Check for overflow before multiplication
			// For negative numbers, allow one more digit (MinInt64 = -9223372036854775808)
			maxMod := int64(maxInt64Mod10)
			if isNegative {
				maxMod = int64(minInt64Mod10)
			}

			if intVal > maxInt64Div10 || (intVal == maxInt64Div10 && digit > maxMod) {
				return -1, fmt.Errorf("number overflow: value too large at position %d", start)
			}

			intVal = intVal*10 + digit
			p.pos++
		} else if ch == '.' && !hasDecimal {
			hasDecimal = true
			floatVal = float64(intVal)
			p.pos++
			break
		} else {
			break
		}
	}

	valueIdx := len(p.valueArena)

	if hasDecimal {
		// Parse decimal part
		var divisor float64 = 10
		for p.pos < p.length {
			ch := p.input[p.pos]
			if ch >= '0' && ch <= '9' {
				floatVal += float64(ch-'0') / divisor
				divisor *= 10
				p.pos++
			} else {
				break
			}
		}

		if isNegative {
			floatVal = -floatVal
		}

		p.valueArena = append(p.valueArena, FastValueBytes{
			Type:       TypeFloat,
			FloatValue: floatVal,
		})
	} else {
		if isNegative {
			intVal = -intVal
		}

		// Check if number is actually valid (at least one digit)
		if p.pos == start || (p.pos == start+1 && isNegative) {
			return -1, fmt.Errorf("invalid number at position %d", start)
		}

		p.valueArena = append(p.valueArena, FastValueBytes{
			Type:     TypeInt,
			IntValue: intVal,
		})
	}

	return valueIdx, nil
}

// parseBoolean parses true or false using byte comparison
func (p *FastParserBytes) parseBoolean() (int, error) {
	if p.pos+4 <= p.length &&
		p.input[p.pos] == 't' &&
		p.input[p.pos+1] == 'r' &&
		p.input[p.pos+2] == 'u' &&
		p.input[p.pos+3] == 'e' {
		p.pos += 4
		valueIdx := len(p.valueArena)
		p.valueArena = append(p.valueArena, FastValueBytes{
			Type:      TypeBool,
			BoolValue: true,
		})
		return valueIdx, nil
	}

	if p.pos+5 <= p.length &&
		p.input[p.pos] == 'f' &&
		p.input[p.pos+1] == 'a' &&
		p.input[p.pos+2] == 'l' &&
		p.input[p.pos+3] == 's' &&
		p.input[p.pos+4] == 'e' {
		p.pos += 5
		valueIdx := len(p.valueArena)
		p.valueArena = append(p.valueArena, FastValueBytes{
			Type:      TypeBool,
			BoolValue: false,
		})
		return valueIdx, nil
	}

	return -1, fmt.Errorf("invalid boolean")
}

// parseNull parses null using byte comparison
func (p *FastParserBytes) parseNull() (int, error) {
	if p.pos+4 <= p.length &&
		p.input[p.pos] == 'n' &&
		p.input[p.pos+1] == 'u' &&
		p.input[p.pos+2] == 'l' &&
		p.input[p.pos+3] == 'l' {
		p.pos += 4
		valueIdx := len(p.valueArena)
		p.valueArena = append(p.valueArena, FastValueBytes{
			Type: TypeNull,
		})
		return valueIdx, nil
	}
	return -1, fmt.Errorf("invalid null")
}

// skipWhitespace skips whitespace characters
func (p *FastParserBytes) skipWhitespace() {
	for p.pos < p.length {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			p.pos++
		} else {
			break
		}
	}
}

// GetValue retrieves a value by index
func (p *FastParserBytes) GetValue(idx int) FastValueBytes {
	if idx < 0 || idx >= len(p.valueArena) {
		return FastValueBytes{Type: TypeNull}
	}
	return p.valueArena[idx]
}

// GetString retrieves a string value (zero-copy using unsafe)
func (p *FastParserBytes) GetString(val FastValueBytes) string {
	if val.Type != TypeString {
		return ""
	}
	// Use unsafe to convert []byte to string without allocation
	return unsafeBytesToString(p.stringArena[val.StringStart : val.StringStart+val.StringLen])
}

// GetStringBytes retrieves a string value as []byte (zero-copy)
func (p *FastParserBytes) GetStringBytes(val FastValueBytes) []byte {
	if val.Type != TypeString {
		return nil
	}
	return p.stringArena[val.StringStart : val.StringStart+val.StringLen]
}

// GetMember retrieves an object member by index
func (p *FastParserBytes) GetMember(idx int) FastMemberBytes {
	if idx < 0 || idx >= len(p.memberArena) {
		return FastMemberBytes{}
	}
	return p.memberArena[idx]
}

// GetMemberKey retrieves a member's key as a string (zero-copy)
func (p *FastParserBytes) GetMemberKey(member FastMemberBytes) string {
	return unsafeBytesToString(p.stringArena[member.KeyStart : member.KeyStart+member.KeyLen])
}

// GetMemberKeyBytes retrieves a member's key as []byte (zero-copy)
func (p *FastParserBytes) GetMemberKeyBytes(member FastMemberBytes) []byte {
	return p.stringArena[member.KeyStart : member.KeyStart+member.KeyLen]
}

// GetObjectValue retrieves a value from an object by key name
func (p *FastParserBytes) GetObjectValue(objIdx int, key string) *FastValueBytes {
	obj := p.GetValue(objIdx)
	if obj.Type != TypeObject {
		return nil
	}

	for i := 0; i < obj.ChildCount; i++ {
		member := p.GetMember(obj.FirstChild + i)
		memberKey := p.stringArena[member.KeyStart : member.KeyStart+member.KeyLen]
		if string(memberKey) == key {
			val := p.GetValue(member.ValueIdx)
			return &val
		}
	}

	return nil
}

// FastParseBytes is a convenience function for fast parsing from bytes
func FastParseBytes(input []byte) (*FastParserBytes, int, error) {
	// Estimate values based on input length
	estimatedValues := len(input) / 10
	if estimatedValues < 10 {
		estimatedValues = 10
	}

	parser := NewFastParserBytes(input, estimatedValues)
	rootIdx, err := parser.Parse()
	return parser, rootIdx, err
}

// FastParseBytesFromString is a convenience function for fast parsing from string
func FastParseBytesFromString(input string) (*FastParserBytes, int, error) {
	return FastParseBytes([]byte(input))
}

// ToMap converts a FastValueBytes object to a Go map (for compatibility)
func (p *FastParserBytes) ToMap(valueIdx int) map[string]interface{} {
	val := p.GetValue(valueIdx)
	if val.Type != TypeObject {
		return nil
	}

	result := make(map[string]interface{})
	for i := 0; i < val.ChildCount; i++ {
		member := p.GetMember(val.FirstChild + i)
		key := p.GetMemberKey(member)
		result[key] = p.ToInterface(member.ValueIdx)
	}
	return result
}

// ToInterface converts a FastValueBytes to interface{} (for compatibility)
func (p *FastParserBytes) ToInterface(valueIdx int) interface{} {
	val := p.GetValue(valueIdx)

	switch val.Type {
	case TypeNull:
		return nil
	case TypeBool:
		return val.BoolValue
	case TypeInt:
		return val.IntValue
	case TypeFloat:
		return val.FloatValue
	case TypeString:
		return p.GetString(val)
	case TypeObject:
		return p.ToMap(valueIdx)
	case TypeArray:
		arr := make([]interface{}, val.ChildCount)
		for i := 0; i < val.ChildCount; i++ {
			arr[i] = p.ToInterface(val.FirstChild + i)
		}
		return arr
	}
	return nil
}

// Reset resets the parser for reuse (zero allocation on subsequent parses)
func (p *FastParserBytes) Reset(input []byte) {
	p.input = input
	p.length = len(input)
	p.pos = 0
	p.valueArena = p.valueArena[:0]
	p.memberArena = p.memberArena[:0]
	p.stringArena = p.stringArena[:0]
	p.valueCount = 0
	p.memberCount = 0
	p.stringOffset = 0
}

// ResetFromString resets the parser with a string input
func (p *FastParserBytes) ResetFromString(input string) {
	p.Reset([]byte(input))
}

// unsafeBytesToString converts []byte to string without allocation
// This is safe because we're not modifying the underlying byte slice
func unsafeBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// String returns a string representation (for debugging)
func (p *FastParserBytes) String(valueIdx int) string {
	val := p.GetValue(valueIdx)

	switch val.Type {
	case TypeNull:
		return "null"
	case TypeBool:
		if val.BoolValue {
			return "true"
		}
		return "false"
	case TypeInt:
		return fastInt64ToString(val.IntValue)
	case TypeFloat:
		return fastFloat64ToString(val.FloatValue)
	case TypeString:
		return p.GetString(val)
	case TypeObject:
		// Build string representation
		result := "{"
		for i := 0; i < val.ChildCount; i++ {
			if i > 0 {
				result += ", "
			}
			member := p.GetMember(val.FirstChild + i)
			result += p.GetMemberKey(member) + ": " + p.String(member.ValueIdx)
		}
		result += "}"
		return result
	case TypeArray:
		result := "["
		for i := 0; i < val.ChildCount; i++ {
			if i > 0 {
				result += ", "
			}
			result += p.String(val.FirstChild + i)
		}
		result += "]"
		return result
	}
	return ""
}

// Fast number to string conversions (avoid strconv allocations)
func fastInt64ToString(n int64) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}

	if negative {
		buf = append(buf, '-')
	}

	// Reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return unsafeBytesToString(buf)
}

func fastFloat64ToString(f float64) string {
	// For simplicity, use a basic approach
	// In production, you might want a more sophisticated implementation
	intPart := int64(f)
	fracPart := f - float64(intPart)

	if fracPart == 0 {
		return fastInt64ToString(intPart)
	}

	// Simple fraction representation (up to 6 decimals)
	result := fastInt64ToString(intPart) + "."
	fracPart = fracPart * 1000000
	fracInt := int64(fracPart)
	fracStr := fastInt64ToString(fracInt)

	// Pad with zeros if needed
	for len(fracStr) < 6 {
		fracStr = "0" + fracStr
	}

	// Remove trailing zeros
	fracStr = trimTrailingZeros(fracStr)

	return result + fracStr
}

func trimTrailingZeros(s string) string {
	i := len(s) - 1
	for i >= 0 && s[i] == '0' {
		i--
	}
	return s[:i+1]
}
