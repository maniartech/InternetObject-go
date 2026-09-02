package parser

import "github.com/maniartech/InternetObject-go/internal/tokenizer"

// Lazy, token-backed records (ADR 0007 phase 1).
//
// Framing records WHERE each member's value is in the token stream instead of
// decoding it into a boxed value. A decoded token costs nothing — a string is
// a substring of the source and a number is a strconv call over one — so a
// document that is only going to be bound into Go structs never needs the
// value tree, and never pays the interface box per member that building it
// costs.
//
// This is NOT a second grammar. Every lexical decision — where a string ends,
// which escapes it carries, whether it is open, raw or regular, what an
// annotation claims, where a section separator is — was already made by the
// tokenizer, and this walks its output. What is added is member splitting:
// track bracket depth, cut at depth-zero commas, note whether a member
// carried a key. Anything it does not recognize makes it decline, and the
// caller falls back to the value tree.

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

// RawRecord is one record as a window into the document's member arena.
type RawRecord struct {
	Members []RawMember
}

// RawDoc is a framed document: the token stream, plus one span per record.
// Members for every record live in a single arena, so a record costs no
// allocation of its own.
type RawDoc struct {
	Stream  *tokenizer.Stream
	Records []RawRecord

	arena []RawMember
}

// FrameData frames the DATA records of a token stream — everything after the
// first section separator — and reports whether it could.
//
// It declines (ok == false) for anything outside the shape it is sure of: an
// error token, a second section, a document with no separator, or a record it
// cannot split. Declining is always safe: the caller falls back to the value
// tree, which is the code that has always run.
func FrameData(s *tokenizer.Stream) (*RawDoc, bool) {
	toks := s.Tokens

	sep := -1
	for i := range toks {
		if toks[i].Kind == tokenizer.KindSectionSep {
			sep = i
			break
		}
	}
	if sep < 0 {
		return nil, false // headerless: the tree path owns that shape
	}
	i := sep + 1

	// A section name or `$schema` selector may follow the separator.
	for i < len(toks) && toks[i].Kind == tokenizer.KindString &&
		(toks[i].Sub == tokenizer.SubSectionName || toks[i].Sub == tokenizer.SubSectionSchema) {
		i++
	}

	d := &RawDoc{Stream: s, arena: make([]RawMember, 0, 8+len(toks)/4)}
	collection := i < len(toks) && toks[i].Kind == tokenizer.KindCollectionStart

	for i < len(toks) {
		switch toks[i].Kind {
		case tokenizer.KindSectionSep:
			return nil, false // a second section
		case tokenizer.KindError:
			return nil, false // let the tree path report it
		}
		if collection {
			if toks[i].Kind != tokenizer.KindCollectionStart {
				return nil, false
			}
			i++
		}
		end := i
		for end < len(toks) &&
			toks[end].Kind != tokenizer.KindCollectionStart &&
			toks[end].Kind != tokenizer.KindSectionSep {
			end++
		}
		// A record that is exactly one braced object IS that object: the
		// parser absorbs those braces rather than treating the record as
		// holding a single anonymous member (ISSUE-15). Framing must do the
		// same or the two disagree about member count.
		from, until := i, end
		if inner, unwrapped := unwrapLoneObject(toks, from, until); unwrapped {
			from, until = inner[0], inner[1]
		}
		start := len(d.arena)
		var ok bool
		if d.arena, ok = frameMembers(s, d.arena, from, until); !ok {
			return nil, false
		}
		d.Records = append(d.Records, RawRecord{Members: d.arena[start:len(d.arena):len(d.arena)]})
		i = end
		if !collection {
			break
		}
	}
	if i < len(toks) {
		return nil, false
	}
	return d, true
}

// FrameSpan frames the members inside an arbitrary token range — the inside
// of a container, re-framed on demand by a binder that has decided to descend
// into it. It allocates one slice for the result; the document-level arena is
// not involved.
func FrameSpan(s *tokenizer.Stream, from, end int32) ([]RawMember, bool) {
	if from > end || int(end) > len(s.Tokens) {
		return nil, false
	}
	out, ok := frameMembers(s, nil, int(from), int(end))
	if !ok {
		return nil, false
	}
	return out, true
}

// unwrapLoneObject reports the inside of a record that consists of exactly
// one braced object, which the parser treats as the record itself.
func unwrapLoneObject(toks []tokenizer.Token, from, to int) ([2]int, bool) {
	if from >= to || toks[from].Kind != tokenizer.KindCurlyOpen {
		return [2]int{}, false
	}
	end, ok := spanOfValue(toks, from, to)
	if !ok || end != to {
		return [2]int{}, false // something follows the object: not a lone one
	}
	return [2]int{from + 1, to - 1}, true
}

// frameMembers splits the tokens in [from, to) into members at depth-zero
// commas, appending them to arena. It returns false for any shape it is
// unsure of.
func frameMembers(s *tokenizer.Stream, arena []RawMember, from, to int) ([]RawMember, bool) {
	toks := s.Tokens
	start := len(arena) // this record's first member, for the duplicate scan
	i := from
	for i <= to {
		if i == to || toks[i].Kind == tokenizer.KindComma {
			if i == to && i == from {
				break // an empty record has no members
			}
			arena = append(arena, RawMember{Absent: true, Tok: int32(i), End: int32(i)})
			if i == to {
				break
			}
			i++
			continue
		}

		var m RawMember
		if toks[i].Kind == tokenizer.KindString && i+1 < to && toks[i+1].Kind == tokenizer.KindColon {
			t := toks[i]
			if t.Sub == tokenizer.SubSectionName || t.Sub == tokenizer.SubSectionSchema {
				return arena, false
			}
			m.Key = s.StringValue(t)
			m.KeyQuoted = t.Sub != tokenizer.SubOpenString
			// A repeated key is `duplicate-member`. Framing does not report
			// faults, so it declines and the tree path reports it properly —
			// the same scan the parser's addMember does, over a record's few
			// members.
			for k := start; k < len(arena); k++ {
				if !arena[k].Absent && arena[k].Key == m.Key {
					return arena, false
				}
			}
			i += 2
			if i >= to || toks[i].Kind == tokenizer.KindComma {
				return arena, false // `key:` with no value — the tree reports it
			}
		}

		m.Tok = int32(i)
		valueEnd, ok := spanOfValue(toks, i, to)
		if !ok {
			return arena, false
		}
		m.End = int32(valueEnd)
		m.Kind, m.Sub = toks[i].Kind, toks[i].Sub

		// A container's SPAN is enough to bind it later, but its interior must
		// still be a shape framing understands — otherwise a record holding a
		// malformed object would be accepted and its fault never reported.
		// Checking reuses this very function, so there is one splitter, and
		// the interior members are discarded: they are re-framed on demand.
		if m.IsContainer() {
			mark := len(arena)
			var innerOK bool
			if arena, innerOK = frameMembers(s, arena, i+1, valueEnd-1); !innerOK {
				return arena, false
			}
			for k := mark; k < len(arena); k++ {
				// An empty comma slot is a RECORD-level feature (`~ a, , c`);
				// inside a container it is a syntax error. And an array
				// element never carries a key.
				if arena[k].Absent ||
					(m.Kind == tokenizer.KindBracketOpen && arena[k].Key != "") {
					return arena, false
				}
			}
			arena = arena[:mark]
		}
		arena = append(arena, m)

		i = valueEnd
		if i >= to {
			break
		}
		if toks[i].Kind != tokenizer.KindComma {
			return arena, false // two values with no separator between them
		}
		i++
		if i == to {
			arena = append(arena, RawMember{Absent: true, Tok: int32(i), End: int32(i)})
			break
		}
	}
	return arena, true
}

// spanOfValue returns the token index one past the value starting at i,
// tracking both bracket shapes so mixed nesting is followed correctly.
func spanOfValue(toks []tokenizer.Token, i, to int) (int, bool) {
	switch toks[i].Kind {
	case tokenizer.KindCurlyOpen, tokenizer.KindBracketOpen:
		// Brackets must NEST, not merely balance: `{[}` has one of each open
		// and one of each closed, and is a syntax error. The stack lives in a
		// fixed array, so framing allocates nothing and simply declines a
		// document nested deeper than that (the tree path handles those).
		var stackArr [32]tokenizer.Kind
		stack := stackArr[:0]
		for j := i; j < to; j++ {
			switch k := toks[j].Kind; k {
			case tokenizer.KindCurlyOpen, tokenizer.KindBracketOpen:
				if len(stack) == len(stackArr) {
					return 0, false // deeper than this frames
				}
				stack = append(stack, k)
			case tokenizer.KindCurlyClose, tokenizer.KindBracketClose:
				want := tokenizer.KindCurlyOpen
				if k == tokenizer.KindBracketClose {
					want = tokenizer.KindBracketOpen
				}
				if len(stack) == 0 || stack[len(stack)-1] != want {
					return 0, false // mismatched: a syntax error the tree reports
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return j + 1, true
				}
			case tokenizer.KindError:
				return 0, false
			}
		}
		return 0, false // unclosed
	case tokenizer.KindColon, tokenizer.KindComma,
		tokenizer.KindCurlyClose, tokenizer.KindBracketClose,
		tokenizer.KindCollectionStart, tokenizer.KindSectionSep, tokenizer.KindError:
		return 0, false
	}
	return i + 1, true
}
