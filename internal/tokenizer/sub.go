package tokenizer

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
