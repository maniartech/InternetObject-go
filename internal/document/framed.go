package document

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// HeaderSchema returns the schema a framed decode of s binds its data to
// (ADR 0007 phase 2): the compiled schema of s's header, from the header cache.
// It reports false for a document framing does not own — no separator, a
// `--- $Name` selector, or a header that does not resolve cleanly, whose fault
// the tree path must report with its designated code. A nil schema with true
// is a header that declares none.
//
// It frames NOTHING. A caller decides from the schema whether it can use a
// framed decode at all, and only then calls parser.FrameData. Framing first
// and asking afterwards used to frame every record of every document the lazy
// decoder then declined — measured 2026-09-14 as ~22% of the bytes of a
// constrained 1,000-record Unmarshal, all of it thrown away.
func HeaderSchema(s *tokenizer.Stream) (*schema.Schema, bool) {
	// The header is everything before the first separator. Re-parsing just
	// that text is O(header), not O(document).
	sepAt := -1
	for i := range s.Tokens {
		if s.Tokens[i].Kind == tokenizer.KindSectionSep {
			sepAt = i
			break
		}
	}
	if sepAt < 0 {
		return nil, false
	}

	// An explicit `--- $Name` selector is not framed yet: the default-schema
	// resolution below covers the common case, and a selector needs the
	// section binding rules the tree path owns.
	if sepAt+1 < len(s.Tokens) {
		if t := s.Tokens[sepAt+1]; t.Kind == tokenizer.KindString &&
			(t.Sub == tokenizer.SubSectionName || t.Sub == tokenizer.SubSectionSchema) {
			return nil, false
		}
	}

	he := headerFor(s.Src[:s.Tokens[sepAt].Start])
	return he.sch, he.ok
}

// A header's compiled form, memoized on the header text.
//
// Parsing and compiling the header on every call is invisible on a 1,000-record
// document and dominates a one-record one: it measured as ~45% of the
// allocations of a 133-byte payload, the shape an HTTP handler decodes all day.
// The header text is a SUFFICIENT key here because compilation is a pure
// function of it on this path — HeaderSchema has already declined any `--- $Name`
// selector above, so no section binding can vary; the section it compiles
// against is built locally with no name; and Compile deliberately does not
// resolve @-references, so variable values never enter a compiled schema.
//
// Sharing the compiled *Schema across goroutines is only sound because a
// compiled schema is now read-only — the lazily-compiled `pattern` regexp that
// used to be written during validation was a data race, fixed in
// schema.compilePattern. Do not reintroduce a write-at-validation field.
type headerEntry struct {
	sch *schema.Schema
	ok  bool // false: this header is not framable, decline as before
}

// The cache never evicts, so it is bounded twice: a header bigger than this
// amortizes its own compilation anyway, and past the entry cap the behavior
// degrades to exactly what it was before. Both matter when the header text is
// attacker-controlled — an unbounded memo would be a memory-exhaustion vector.
// Wrong keys cannot return right entries (an entry is stored only after a pure
// compile of that exact text), so there is no poisoning to defend against.
const (
	maxCachedHeaderLen = 4 << 10
	maxCachedHeaders   = 1024
)

var (
	headerCache   sync.Map // string → headerEntry
	headerCached  atomic.Int64
	noHeaderCache = os.Getenv("IO_NO_HEADER_CACHE") != ""
)

func headerFor(headerSrc string) headerEntry {
	if !noHeaderCache {
		if e, ok := headerCache.Load(headerSrc); ok {
			return e.(headerEntry)
		}
	}
	e := compileHeader(headerSrc)
	if !noHeaderCache && len(headerSrc) <= maxCachedHeaderLen &&
		headerCached.Load() < maxCachedHeaders {
		// Clone the key: headerSrc is a substring of the whole document, and
		// storing it as-is would pin every byte of that document forever.
		if _, loaded := headerCache.LoadOrStore(strings.Clone(headerSrc), e); !loaded {
			headerCached.Add(1)
		}
	}
	return e
}

// compileHeader is the uncached original, and stays the only statement of what
// a framable header is.
func compileHeader(headerSrc string) headerEntry {
	hdoc := parser.Parse(headerSrc + "\n---\n")
	if len(hdoc.Errors) > 0 {
		return headerEntry{} // the normal path reports the header's fault
	}
	defs := NewDefinitions(hdoc.Header)
	if defs.Fault() != nil {
		// A named schema that does not resolve fails the whole document on the
		// tree path, referenced or not — so the lazy path must decline it, not
		// bind the records against the default schema. It used to accept
		// `$draft: {title: nosuchtype}` beside a good `$schema` (review of
		// SPEC 0003 §5.1, 2026-09-14): a fast path may only decline.
		return headerEntry{}
	}
	sch, cerr := sectionSchema(&parser.Section{Name: "data"}, defs)
	if cerr != nil {
		return headerEntry{}
	}
	if hdoc.Header != nil && len(hdoc.Header.Vars) > 0 {
		return headerEntry{} // @variables resolve during the tree walk
	}
	return headerEntry{sch: sch, ok: true}
}

// IsSimpleSchema reports whether every declared member is one a binder can
// judge from its own token: a plain type, optionally with constraints or
// choices (which MemberDef.Accepts checks on the decoded value), but no default,
// union, nested schema or reference, and no constraint on an array as a whole.
// Anything else keeps the general validation path.
func IsSimpleSchema(s *schema.Schema) bool {
	if s == nil || s.Open != nil {
		return false
	}
	for _, name := range s.Names {
		md := s.Defs[name]
		if md == nil || !md.Standalone() {
			return false
		}
		switch md.Type {
		case "string", "number", "int", "bool", "any":
		case "array":
			// Array-level constraints (len, minLen, maxLen) need the whole boxed
			// array to judge; the elements are judged one by one through Of.
			if len(md.Constraints) > 0 || md.Of == nil || !md.Of.Standalone() {
				return false
			}
			switch md.Of.Type {
			case "string", "number", "int", "bool":
			default:
				return false
			}
		default:
			return false
		}
		if strings.ContainsAny(name, " \t") {
			return false
		}
	}
	return true
}
