package parsers

import (
	"fmt"
	"strconv"
	"strings"
)

// FastParser is a zero-allocation parser using arena allocation
// and index-based data structures instead of pointers
type FastParser struct {
	input  string
	pos    int
	length int

	// Arena-allocated arrays (pre-allocated, reused)
	valueArena  []FastValue  // All values stored here
	memberArena []FastMember // All object members
	stringArena []byte       // String data storage

	// Indices for current parsing state
	valueCount   int
	memberCount  int
	stringOffset int
}

// FastValue represents a value using indices instead of pointers
type FastValue struct {
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

// FastMember represents an object member
type FastMember struct {
	KeyStart int // Index into stringArena
	KeyLen   int
	ValueIdx int // Index into valueArena
}

// ValueType for fast parser
type ValueType byte

const (
	TypeNull ValueType = iota
	TypeBool
	TypeInt
	TypeFloat
	TypeString
	TypeObject
	TypeArray
)

// NewFastParser creates a parser with pre-allocated arena
func NewFastParser(input string, estimatedValues int) *FastParser {
	if estimatedValues == 0 {
		estimatedValues = 100
	}

	return &FastParser{
		input:       input,
		length:      len(input),
		valueArena:  make([]FastValue, 0, estimatedValues),
		memberArena: make([]FastMember, 0, estimatedValues),
		stringArena: make([]byte, 0, len(input)), // Max size is input length
	}
}

// Parse parses the input and returns the root value index
func (p *FastParser) Parse() (int, error) {
	p.skipWhitespace()
	return p.parseValue()
}

// parseValue parses any value and returns its index
func (p *FastParser) parseValue() (int, error) {
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
func (p *FastParser) parseObject() (int, error) {
	p.pos++ // skip '{'
	p.skipWhitespace()

	valueIdx := len(p.valueArena)
	memberStart := len(p.memberArena)
	memberCount := 0

	// Reserve space for the object value
	p.valueArena = append(p.valueArena, FastValue{
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

		// Add member
		p.memberArena = append(p.memberArena, FastMember{
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
func (p *FastParser) parseArray() (int, error) {
	p.pos++ // skip '['
	p.skipWhitespace()

	valueIdx := len(p.valueArena)

	// Reserve space for the array value
	p.valueArena = append(p.valueArena, FastValue{
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

// parseQuotedString parses a quoted string
func (p *FastParser) parseQuotedString() (int, error) {
	p.pos++ // skip opening quote
	start := p.pos

	for p.pos < p.length && p.input[p.pos] != '"' {
		if p.input[p.pos] == '\\' {
			p.pos++ // skip escape char
		}
		p.pos++
	}

	if p.pos >= p.length {
		return -1, fmt.Errorf("unterminated string")
	}

	valueIdx := len(p.valueArena)
	stringStart := p.stringOffset
	stringLen := p.pos - start

	p.stringArena = append(p.stringArena, p.input[start:p.pos]...)
	p.stringOffset += stringLen
	p.pos++ // skip closing quote

	p.valueArena = append(p.valueArena, FastValue{
		Type:        TypeString,
		StringStart: stringStart,
		StringLen:   stringLen,
	})

	return valueIdx, nil
}

// parseUnquotedString parses an unquoted string
func (p *FastParser) parseUnquotedString() (int, error) {
	start := p.pos

	for p.pos < p.length && !isValueTerminator(p.input[p.pos]) {
		p.pos++
	}

	valueIdx := len(p.valueArena)
	stringStart := p.stringOffset
	stringLen := p.pos - start

	p.stringArena = append(p.stringArena, p.input[start:p.pos]...)
	p.stringOffset += stringLen

	p.valueArena = append(p.valueArena, FastValue{
		Type:        TypeString,
		StringStart: stringStart,
		StringLen:   stringLen,
	})

	return valueIdx, nil
}

// parseNumber parses a number (int or float)
func (p *FastParser) parseNumber() (int, error) {
	start := p.pos
	hasDecimal := false

	if p.input[p.pos] == '-' {
		p.pos++
	}

	for p.pos < p.length {
		ch := p.input[p.pos]
		if ch >= '0' && ch <= '9' {
			p.pos++
		} else if ch == '.' && !hasDecimal {
			hasDecimal = true
			p.pos++
		} else {
			break
		}
	}

	numStr := p.input[start:p.pos]
	valueIdx := len(p.valueArena)

	if hasDecimal {
		val, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return -1, err
		}
		p.valueArena = append(p.valueArena, FastValue{
			Type:       TypeFloat,
			FloatValue: val,
		})
	} else {
		val, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return -1, err
		}
		p.valueArena = append(p.valueArena, FastValue{
			Type:     TypeInt,
			IntValue: val,
		})
	}

	return valueIdx, nil
}

// parseBoolean parses true or false
func (p *FastParser) parseBoolean() (int, error) {
	if p.pos+4 <= p.length && p.input[p.pos:p.pos+4] == "true" {
		p.pos += 4
		valueIdx := len(p.valueArena)
		p.valueArena = append(p.valueArena, FastValue{
			Type:      TypeBool,
			BoolValue: true,
		})
		return valueIdx, nil
	}

	if p.pos+5 <= p.length && p.input[p.pos:p.pos+5] == "false" {
		p.pos += 5
		valueIdx := len(p.valueArena)
		p.valueArena = append(p.valueArena, FastValue{
			Type:      TypeBool,
			BoolValue: false,
		})
		return valueIdx, nil
	}

	return -1, fmt.Errorf("invalid boolean")
}

// parseNull parses null
func (p *FastParser) parseNull() (int, error) {
	if p.pos+4 <= p.length && p.input[p.pos:p.pos+4] == "null" {
		p.pos += 4
		valueIdx := len(p.valueArena)
		p.valueArena = append(p.valueArena, FastValue{
			Type: TypeNull,
		})
		return valueIdx, nil
	}
	return -1, fmt.Errorf("invalid null")
}

// skipWhitespace skips whitespace characters
func (p *FastParser) skipWhitespace() {
	for p.pos < p.length {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			p.pos++
		} else {
			break
		}
	}
}

// Helper functions
func isKeyTerminator(ch byte) bool {
	return ch == ':' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isValueTerminator(ch byte) bool {
	return ch == ',' || ch == '}' || ch == ']' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// GetValue retrieves a value by index
func (p *FastParser) GetValue(idx int) FastValue {
	if idx < 0 || idx >= len(p.valueArena) {
		return FastValue{Type: TypeNull}
	}
	return p.valueArena[idx]
}

// GetString retrieves a string value
func (p *FastParser) GetString(val FastValue) string {
	if val.Type != TypeString {
		return ""
	}
	return string(p.stringArena[val.StringStart : val.StringStart+val.StringLen])
}

// GetMember retrieves an object member by index
func (p *FastParser) GetMember(idx int) FastMember {
	if idx < 0 || idx >= len(p.memberArena) {
		return FastMember{}
	}
	return p.memberArena[idx]
}

// GetMemberKey retrieves a member's key as a string
func (p *FastParser) GetMemberKey(member FastMember) string {
	return string(p.stringArena[member.KeyStart : member.KeyStart+member.KeyLen])
}

// FastParse is a convenience function for fast parsing
func FastParse(input string) (*FastParser, int, error) {
	// Estimate values based on input length
	estimatedValues := len(input) / 10
	if estimatedValues < 10 {
		estimatedValues = 10
	}

	parser := NewFastParser(input, estimatedValues)
	rootIdx, err := parser.Parse()
	return parser, rootIdx, err
}

// ToMap converts a FastValue object to a Go map (for compatibility)
func (p *FastParser) ToMap(valueIdx int) map[string]interface{} {
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

// ToInterface converts a FastValue to interface{} (for compatibility)
func (p *FastParser) ToInterface(valueIdx int) interface{} {
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
func (p *FastParser) Reset(input string) {
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

// String returns a string representation (for debugging)
func (p *FastParser) String(valueIdx int) string {
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
		return strconv.FormatInt(val.IntValue, 10)
	case TypeFloat:
		return strconv.FormatFloat(val.FloatValue, 'f', -1, 64)
	case TypeString:
		return p.GetString(val)
	case TypeObject:
		var b strings.Builder
		b.WriteString("{")
		for i := 0; i < val.ChildCount; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			member := p.GetMember(val.FirstChild + i)
			b.WriteString(p.GetMemberKey(member))
			b.WriteString(": ")
			b.WriteString(p.String(member.ValueIdx))
		}
		b.WriteString("}")
		return b.String()
	case TypeArray:
		var b strings.Builder
		b.WriteString("[")
		for i := 0; i < val.ChildCount; i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.String(val.FirstChild + i))
		}
		b.WriteString("]")
		return b.String()
	}
	return ""
}
