package document

import (
	"strings"
	"unicode/utf8"

	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// Emitting a STRING in the spelling write-string-scan.go chose: bare, quoted
// or escaped.

func autoString(s string) string {
	return string(appendAutoString(nil, s))
}

// appendAutoString is the append-style form every record path uses: quoted
// when the text is ambiguous or carries a character a bare run cannot hold,
// open with escaped structural characters, raw when only a newline or tab
// needs covering, bare otherwise.
func appendAutoString(dst []byte, s string) []byte {
	if s == "" {
		return appendRegularString(dst, s)
	}

	// One pass collects every character fact, including whether any WORD
	// starts with a character that could begin a number. Only such text can
	// read back as a number, a broken claim or a temporal, so everything else
	// skips those whole-string checks entirely — which is most real text.
	var flags byte
	numStart := false
	atWordStart := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		cl := strClass[c]
		flags |= cl
		if atWordStart && (cl&clDigit != 0 || c == '+' || c == '-' || c == '.') {
			numStart = true
		}
		atWordStart = cl&clSpace != 0
	}

	// Untrimmed text must be quoted — which is a fact about the EDGES, not
	// about containing a space: testing `flags&clSpace` ran a whole TrimSpace
	// over every string with a space in the middle. Only a multi-byte edge
	// needs the Unicode-aware check.
	if strClass[s[0]]&clSpace != 0 || strClass[s[len(s)-1]]&clSpace != 0 {
		flags |= clQuote
	} else if s[0] >= utf8.RuneSelf || s[len(s)-1] >= utf8.RuneSelf {
		// Multi-byte edges ask the READER's whitespace rule, not
		// unicode.IsSpace: the two disagree (U+FEFF), and a value the reader
		// would skip must never be written bare.
		first, _ := utf8.DecodeRuneInString(s)
		last, _ := utf8.DecodeLastRuneInString(s)
		if tokenizer.IsSpaceRune(first) || tokenizer.IsSpaceRune(last) {
			flags |= clQuote
		}
	}
	if flags&clDash != 0 && strings.Contains(s, "---") {
		flags |= clQuote
	}
	if flags&clQuote == 0 && wouldNotReadBack(s, flags, numStart) {
		flags |= clQuote
	}

	switch {
	case flags&clQuote != 0:
		return appendRegularString(dst, s)
	case flags&clStruct != 0:
		return appendOpenEscaped(dst, s)
	case flags&clRaw != 0:
		dst = append(dst, 'r', '"')
		for i := 0; i < len(s); i++ {
			if s[i] == '"' {
				dst = append(dst, '"')
			}
			dst = append(dst, s[i])
		}
		return append(dst, '"')
	}
	return append(dst, s...)
}

// regularString spells s as a regular quoted string. Every C0 control
// character is escaped — named where the reader names one, \u00XX otherwise —
// so the quoted form never carries a raw control byte.
func regularString(s string) string {
	return string(appendRegularString(nil, s))
}

func appendRegularString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			dst = append(dst, '\\', '\\')
		case '"':
			dst = append(dst, '\\', '"')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return append(dst, '"')
}

func appendOpenEscaped(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '{', '}', '[', ']', ':', '#', '"', '\'', '\\', '~':
			dst = append(dst, '\\', c)
			continue
		}
		// A control character written raw ENDS the run on re-read, silently
		// truncating the value (the same defect as FINDINGS #10, and open
		// strings process the full escape set, so an escape round-trips).
		if c < 0x20 {
			dst = appendControlEscape(dst, c)
			continue
		}
		dst = append(dst, c)
	}
	return dst
}

// appendControlEscape spells one C0 control character — named where the
// reader names one, `\u00XX` otherwise. Shared by the quoted and open
// spellings so the two never disagree about an escape.
func appendControlEscape(dst []byte, c byte) []byte {
	switch c {
	case '\n':
		return append(dst, '\\', 'n')
	case '\r':
		return append(dst, '\\', 'r')
	case '\t':
		return append(dst, '\\', 't')
	case '\b':
		return append(dst, '\\', 'b')
	case '\f':
		return append(dst, '\\', 'f')
	}
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
}

// headerName spells a `@variable` or `$schema` name. Such a name is read back
// as a bare run and CANNOT be quoted, so every character that would end that
// run — the reader's own terminator set, whitespace, a `---`, a backslash or
// a control — is escaped instead. Values do not share this path: they have
// quoting available and use it (a comma, for instance, forces a quoted
// string rather than an escape).
