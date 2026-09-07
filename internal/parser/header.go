package parser

import (
	"github.com/maniartech/InternetObject-go/internal/core"
)

// Header holds the definitions written before the first `---`.
type Header struct {
	Plain   *core.Object   // plain key: value definitions, in order, last wins
	Schemas map[string]any // $name → schema shape (sigil stripped), last wins
	Vars    map[string]any // @name → value (sigil stripped), last wins
	Inline  any            // the schema shape when the header is a bare schema expression

	// Defs lists every definition in document order (one entry per key, a
	// duplicate updates in place), so a writer can reproduce the header.
	Defs []HeaderDef
}

func (h *Header) upsertDef(kind DefKind, key string, val any) {
	for i := range h.Defs {
		if h.Defs[i].Kind == kind && h.Defs[i].Key == key {
			h.Defs[i].Value = val
			return
		}
	}
	h.Defs = append(h.Defs, HeaderDef{Kind: kind, Key: key, Value: val})
}
