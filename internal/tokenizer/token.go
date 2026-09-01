// Package tokenizer turns Internet Object source text into a flat token
// stream.
//
// Tokens are compact value structs holding only classification and positions;
// decoded payloads (unescaped strings, parsed numbers, byte slices) are
// produced on demand by the Stream decode methods, so steady-state
// tokenization allocates nothing per token. Errors are never returned: a
// malformed construct becomes an ERROR token carrying a stable error code,
// because Internet Object accumulates errors rather than failing fast.
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

// Sub is a token's sub-classification: the numeric base, the string form, or
// the temporal kind. SubNone renders as the empty string.
type Sub uint8

const (
	SubNone Sub = iota
	SubHex
	SubOctal
	SubBinary
	SubOpenString
	SubRegularString
	SubRawString
	SubSectionName
	SubSectionSchema
	SubBinaryString
	SubDate
	SubTime
	SubDateTime
)

var subNames = [...]string{
	SubNone:          "",
	SubHex:           "HEX",
	SubOctal:         "OCTAL",
	SubBinary:        "BINARY",
	SubOpenString:    "OPEN_STRING",
	SubRegularString: "REGULAR_STRING",
	SubRawString:     "RAW_STRING",
	SubSectionName:   "SECTION_NAME",
	SubSectionSchema: "SECTION_SCHEMA",
	SubBinaryString:  "BINARY_STRING",
	SubDate:          "DATE",
	SubTime:          "TIME",
	SubDateTime:      "DATETIME",
}

func (s Sub) String() string { return subNames[s] }

// Code is a stable error code carried by an ERROR token. Codes are the
// conformance contract (messages are not) and follow the frozen
// <predicate>-<subject> grammar; never invent one.
type Code uint8

const (
	CodeNone Code = iota
	CodeInvalidNumber
	CodeInvalidBigInt
	CodeInvalidDecimal
	CodeInvalidBinary
	CodeInvalidDate
	CodeInvalidTime
	CodeInvalidDateTime
	CodeUntermString
	CodeInvalidEscape
	CodeUnknownAnnotation
	CodeInvalidSectionName
	CodeMissingSchema
)

var codeNames = [...]string{
	CodeNone:               "",
	CodeInvalidNumber:      "invalid-number",
	CodeInvalidBigInt:      "invalid-bigint",
	CodeInvalidDecimal:     "invalid-decimal",
	CodeInvalidBinary:      "invalid-binary",
	CodeInvalidDate:        "invalid-date",
	CodeInvalidTime:        "invalid-time",
	CodeInvalidDateTime:    "invalid-datetime",
	CodeUntermString:       "unterminated-string",
	CodeInvalidEscape:      "invalid-escape-sequence",
	CodeUnknownAnnotation:  "unknown-annotation",
	CodeInvalidSectionName: "invalid-section-name",
	CodeMissingSchema:      "missing-schema",
}

func (c Code) String() string { return codeNames[c] }

// Token is one lexical token: classification plus positions into the stream's
// normalized source. It carries no decoded payload — decode through the
// Stream. Twenty bytes, held by value in the stream's slice.
type Token struct {
	Kind Kind
	Sub  Sub
	Err  Code // set only when Kind == KindError

	Start, End int32 // byte offsets into Stream.Src (half-open)
	Line, Col  int32 // 1-based position of Start
}

// Stream is the result of tokenizing one input: the normalized source (CRLF
// and CR become LF) and its tokens in order.
type Stream struct {
	Src    string
	Tokens []Token
}

// Text returns the token's raw source text.
func (s *Stream) Text(t Token) string { return s.Src[t.Start:t.End] }
