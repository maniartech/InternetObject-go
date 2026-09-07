package document

import (
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Frozen is a header that has been compiled ONCE, completely, and will never
// be written to again — so it can be shared across goroutines and reused for
// any number of documents.
//
// Definitions, the ordinary form, memoizes as it resolves: it compiles a named
// schema the first time something asks for it and remembers the answer. That
// makes it fast and single-owner, and it is why every parsed document gets a
// fresh one. A caller who wants to compile a header once and reuse it needs the
// opposite property, so Freeze does all the work up front and hands back
// something with no mutable state left.
//
// It is consulted as a READ-ONLY PARENT by a document's own Definitions
// (see Definitions.parent), which is what implements the precedence io-specs
// requires: the document's own header is checked first, so in-stream
// definitions override preloaded ones.
type Frozen struct {
	Header  *parser.Header
	Schemas map[string]*schema.Schema // every named schema, already compiled
	Vars    map[string]any            // every @variable, already resolved
	Default *schema.Schema            // the header's own $schema, or nil
	Names   []string                  // definition names in document order
}

// Freeze compiles every definition a header carries and returns a value that is
// safe to share. A definition that does not compile is an error here rather
// than at the point of use — the caller asked for the whole header, so the
// whole header is checked.
func Freeze(h *parser.Header) (*Frozen, *errs.Error) {
	f := &Frozen{
		Header:  h,
		Schemas: map[string]*schema.Schema{},
		Vars:    map[string]any{},
	}
	if h == nil {
		return f, nil
	}

	// Resolution runs through an ordinary Definitions, so there is ONE
	// statement of what a name means; Freeze only decides that everything is
	// resolved now rather than later.
	d := NewDefinitions(h)

	for _, def := range h.Defs {
		f.Names = append(f.Names, def.Key)
		switch def.Kind {
		case parser.DefSchema:
			s, cerr := d.SchemaOf(def.Key)
			if cerr != nil {
				return nil, cerr
			}
			f.Schemas[def.Key] = s
		case parser.DefVar:
			v, cerr := d.Var(def.Key)
			if cerr != nil {
				return nil, cerr
			}
			f.Vars[def.Key] = v
		}
	}

	// The header's own default schema — an inline expression, or a `$schema`
	// definition — is what a document with a bare `---` binds to.
	if s, cerr := sectionSchema(&parser.Section{Name: "data"}, d); cerr != nil {
		return nil, cerr
	} else {
		f.Default = s
	}
	return f, nil
}

// ParseWithDefs parses src with a frozen header in scope. The document's own
// header still applies and WINS on a name they both define, which is the
// precedence io-specs/streaming/schema-and-state.md requires; a document that
// declares no schema of its own binds to the frozen default.
func ParseWithDefs(src string, parent *Frozen) *Doc {
	return parseWith(src, nil, parent)
}
