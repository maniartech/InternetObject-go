package parser

import (
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// RawMember is one member of a framed record: its key (empty when
// positional) and the half-open token range holding its value.
//
// Kind and Sub are the TOKENIZER's own vocabulary, not a parallel one: a
// member says it is a KindString/SubOpenString or a KindString/SubRawString,
// a KindNumber/SubHex, a KindDateTime/SubDate, and a container says
// KindCurlyOpen or KindBracketOpen. Carrying the sub-kind matters — the three
// string forms decode differently, and a caller inspecting a framed document
// needs the same distinctions the tokenizer drew.
type RawMember struct {
	// Key is the member's name, or "" when positional. It is a substring of
	// the source in the common case, so carrying it is free.
	Key       string
	KeyQuoted bool // the key was written quoted

	Tok int32 // index of the value's first token
	End int32 // one past the value's last token

	Kind tokenizer.Kind // the value's token kind
	Sub  tokenizer.Sub  // the value's sub-kind (string form, number base, temporal kind)

	// Absent marks an empty comma slot: a positional hole with no value, and
	// therefore no tokens (Tok == End).
	Absent bool
}

// Positional reports whether the member was written without a key.
func (m RawMember) Positional() bool { return m.Key == "" }

// IsContainer reports whether the member's value is a braced object or a
// bracketed array rather than a single scalar token.
func (m RawMember) IsContainer() bool {
	return m.Kind == tokenizer.KindCurlyOpen || m.Kind == tokenizer.KindBracketOpen
}
