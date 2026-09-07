package tokenizer

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
