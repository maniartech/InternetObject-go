package document

import (
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// The canonical document writer: the top-level rule, and the shape of a
// whole document.
//
// ONE RULE governs every file in this group: a writer must never emit text
// its own reader cannot read back as the same value. Everything else here -
// when a string is quoted, when a key is written, how sections are laid out -
// is that rule applied to one kind of text, and the round-trip corpus pins it.

// The canonical writer. One rule governs everything here: a writer must never
// emit text its own reader cannot read back as the same value. The concrete
// choices (when a string is quoted, escaped or raw; when a key is written;
// how the header and sections are laid out) mirror the reference writer,
// which the round-trip corpus pins.

// reservedSectionNames are the parser defaults a writer treats as no name.
var reservedSectionNames = map[string]bool{"data": true, "schema": true, "$schema": true}

// IsDefaultSectionName reports a name the parser supplied rather than the
// document: `---` becomes the section "data". Callers that ask "did the author
// NAME this section?" must go through here, not compare against "data"
// themselves, or the answer drifts between the writer and everyone else.
func IsDefaultSectionName(name string) bool {
	return name == "" || reservedSectionNames[name]
}

// String renders the loaded document in canonical form: header included,
// schemas spelled with types, keys emitted only where a name is not
// recoverable ("extras" mode).
func (d *Doc) String() string {
	// One buffer for the whole document, sized from the record count so it
	// doubles at most once or twice (ADR 0006 P1).
	n := 0
	for _, sec := range d.Sections {
		n += len(sec.Records)
	}
	dst := make([]byte, 0, 64+64*n)

	wrote := false
	if d.Header != nil || d.cachedHeader != "" {
		if h := d.writeHeader(); h != "" {
			dst = append(dst, h...)
			wrote = true
		}
	}

	for _, sec := range d.Sections {
		hasNamedSchema := sec.SchemaName != "" && sec.SchemaName != "schema"
		// A name the section-name grammar cannot spell is unwritable; it can
		// only have been borrowed from a schema selector (`--- $$` names the
		// section "$"), and the selector-only spelling reproduces that.
		hasRealName := sec.Name != "" && !reservedSectionNames[sec.Name] &&
			tokenizer.ValidSectionName(sec.Name)

		if wrote {
			dst = append(dst, '\n')
			if hasRealName || hasNamedSchema {
				dst = append(dst, '\n') // a blank line before a named/bound section
			}
		}
		wrote = true
		switch {
		case hasRealName && hasNamedSchema:
			dst = append(dst, "--- "...)
			dst = append(dst, sec.Name...)
			dst = append(dst, ": $"...)
			dst = append(dst, sec.SchemaName...)
		case hasRealName:
			dst = append(dst, "--- "...)
			dst = append(dst, sec.Name...)
		case hasNamedSchema:
			dst = append(dst, "--- $"...)
			dst = append(dst, sec.SchemaName...)
		default:
			dst = append(dst, "---"...)
		}

		mark := len(dst)
		dst = append(dst, '\n')
		body := len(dst)
		dst = d.appendSection(dst, sec)
		if len(dst) == body {
			dst = dst[:mark] // the section wrote nothing; drop the newline
		}
	}
	return string(dst)
}

// SchemaText renders a compiled schema's member declarations in canonical
// syntax — the text between the braces of `{…}`, also valid as a schema-only
// document header.
