package tokenizer

// Kind classifies a token.
type Kind uint8

const (
	KindError Kind = iota
	KindNumber
	KindBigInt
	KindDecimal
	KindBoolean
	KindNull
	KindString
	KindBinary
	KindDateTime
	KindCurlyOpen
	KindCurlyClose
	KindBracketOpen
	KindBracketClose
	KindColon
	KindComma
	KindCollectionStart
	KindSectionSep
)

var kindNames = [...]string{
	KindError:           "ERROR",
	KindNumber:          "NUMBER",
	KindBigInt:          "BIGINT",
	KindDecimal:         "DECIMAL",
	KindBoolean:         "BOOLEAN",
	KindNull:            "NULL",
	KindString:          "STRING",
	KindBinary:          "BINARY",
	KindDateTime:        "DATETIME",
	KindCurlyOpen:       "CURLY_OPEN",
	KindCurlyClose:      "CURLY_CLOSE",
	KindBracketOpen:     "BRACKET_OPEN",
	KindBracketClose:    "BRACKET_CLOSE",
	KindColon:           "COLON",
	KindComma:           "COMMA",
	KindCollectionStart: "COLLECTION_START",
	KindSectionSep:      "SECTION_SEP",
}

func (k Kind) String() string { return kindNames[k] }
