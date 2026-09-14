package internetobject

import (
	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// WithTreeDecode runs fn with Unmarshal forced onto the general (tree) path and
// restores the previous setting when fn returns.
//
// The differential tests must use this, and it is SCOPED to fn on purpose. Two
// earlier mechanisms both failed silently (2026-09-14):
//
//   - t.Setenv("IO_NO_LAZY", ...) never reached the flag, which was read once at
//     init, so the lazy differential test compared the lazy path with itself
//     for its whole life (see noLazy);
//   - anything that lasts until the TEST ends leaks into the next case of a
//     table test. TestFastPathMatchesTreePath used t.Setenv that way, so only
//     its first sample ever compared fast against tree.
func WithTreeDecode(fn func()) {
	prev := noLazy.Swap(true)
	defer noLazy.Store(prev)
	fn()
}

// WithTreeEncode is WithTreeDecode's twin for Marshal's direct path.
func WithTreeEncode(fn func()) {
	prev := noFastPath.Swap(true)
	defer noFastPath.Store(prev)
	fn()
}

// Test-only accessors. See export_test.go conventions: these let the black-box
// _test package ask questions that need internal types, without widening the
// public surface.

// SchemaRejectsItsOwnDefault reports whether any member of the document's
// schema declares a `default` that the SAME member would reject.
//
// The format permits this — the corpus pins it, validation/defaults.io ::
// default_not_a_choice — and the reference applies such a default unchecked.
// The consequence is that an absent member gets filled with a value the schema
// rejects, so the document cannot be re-read. The round-trip fuzzer needs to
// recognise that shape to avoid reporting a FORMAT limit as a port defect.
func SchemaRejectsItsOwnDefault(d *Document) bool {
	for _, sec := range d.doc.Sections {
		s := d.doc.SecSchemas[sec]
		if s == nil {
			continue
		}
		for _, name := range s.Names {
			md := s.Defs[name]
			if md == nil || !md.HasDefault {
				continue
			}
			rec := &core.Object{Members: []core.Member{{Key: name, Value: md.Default}}}
			if _, errs := schema.ValidateRecord(rec, s, d.doc.Defs, true); len(errs) > 0 {
				for _, e := range errs {
					// Only the member we filled matters; a sibling reported as
					// missing is an artefact of the one-member probe.
					if e.Code != "missing-value" {
						return true
					}
				}
			}
		}
	}
	return false
}
