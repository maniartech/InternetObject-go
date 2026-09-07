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

// Framed is a document whose HEADER has been parsed normally — it is small,
// read once, and defines the schema — while its DATA records are only framed
// (ADR 0007 phase 2). Nothing in the data has been decoded or boxed.
type Framed struct {
	Stream *tokenizer.Stream
	Raw    *parser.RawDoc
	Schema *schema.Schema // the schema the data section binds to, or nil
	Defs   schema.Defs    // for @variables and $refs inside records
}

// ParseFramed parses src's header and frames its data, or reports that it
// could not — in which case the caller uses Parse, which is the path that has
// always run.
//
// It declines the shapes framing does not own (see parser.FrameData) and, in
// addition, any document whose header does not resolve cleanly: a header
// fault must be reported by the normal path with its designated code, not
// swallowed here.
func ParseFramed(src string) (*Framed, bool) {
	s := tokenizer.Tokenize(src)

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

	he := headerFor(src[:s.Tokens[sepAt].Start])
	if !he.ok {
		return nil, false
	}

	raw, ok := parser.FrameData(s)
	if !ok {
		return nil, false
	}
	return &Framed{Stream: s, Raw: raw, Schema: he.sch, Defs: NewDefinitions(he.header)}, true
}

// A header's compiled form, memoized on the header text.
//
// Parsing and compiling the header on every call is invisible on a 1,000-record
// document and dominates a one-record one: it measured as ~45% of the
// allocations of a 133-byte payload, the shape an HTTP handler decodes all day.
// The header text is a SUFFICIENT key here because compilation is a pure
// function of it on this path — ParseFramed has already declined any `--- $Name`
// selector above, so no section binding can vary; the section it compiles
// against is built locally with no name; and Compile deliberately does not
// resolve @-references, so variable values never enter a compiled schema.
//
// Sharing the compiled *Schema across goroutines is only sound because a
// compiled schema is now read-only — the lazily-compiled `pattern` regexp that
// used to be written during validation was a data race, fixed in
// schema.compilePattern. Do not reintroduce a write-at-validation field.
//
// Deliberately NOT cached: the *Definitions, which is mutable (it memoizes
// per-name compilation), so each document gets a fresh one.
type headerEntry struct {
	header *parser.Header
	sch    *schema.Schema
	ok     bool // false: this header is not framable, decline as before
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
	sch, cerr := sectionSchema(&parser.Section{Name: "data"}, NewDefinitions(hdoc.Header))
	if cerr != nil {
		return headerEntry{}
	}
	if hdoc.Header != nil && len(hdoc.Header.Vars) > 0 {
		return headerEntry{} // @variables resolve during the tree walk
	}
	return headerEntry{header: hdoc.Header, sch: sch, ok: true}
}

// SchemaMemberDefs exposes the compiled member definitions in schema order,
// which is what a binder walks alongside the framed members.
func SchemaMemberDefs(s *schema.Schema) ([]string, map[string]*schema.MemberDef) {
	if s == nil {
		return nil, nil
	}
	return s.Names, s.Defs
}

// IsSimpleSchema reports whether every declared member is a plain typed
// member with no constraints, defaults, choices, nested schema or reference —
// the shape a binder can check by looking at a token's kind alone. Anything
// else keeps the general validation path.
func IsSimpleSchema(s *schema.Schema) bool {
	if s == nil || s.Open != nil {
		return false
	}
	for _, name := range s.Names {
		md := s.Defs[name]
		if md == nil || len(md.Keys) > 0 || md.HasDefault || md.Choices != nil ||
			md.Schema != nil || md.SchemaRef != "" || md.AnyOf != nil ||
			len(md.Constraints) > 0 {
			return false
		}
		switch md.Type {
		case "string", "number", "int", "bool", "any":
		case "array":
			if md.Of == nil || len(md.Of.Keys) > 0 || md.Of.Schema != nil ||
				md.Of.SchemaRef != "" || len(md.Of.Constraints) > 0 {
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
