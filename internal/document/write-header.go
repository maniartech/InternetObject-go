package document

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Writing the HEADER: the definitions block above the first `---`.

func SchemaText(s *schema.Schema) string {
	d := &Doc{Defs: NewDefinitions(nil)}
	return d.writeSchemaBody(s)
}

// ── header ─────────────────────────────────────────────────────────────────

func (d *Doc) writeHeader() string {
	if d.cachedHeader != "" {
		return d.cachedHeader
	}
	h := d.Header
	// Schema-only mode: a header holding nothing but the default schema is
	// written as the bare schema line.
	schemaOnly := (len(h.Defs) == 1 && h.Defs[0].Kind == parser.DefSchema && h.Defs[0].Key == "schema") ||
		(len(h.Defs) == 0 && h.Inline != nil)
	if schemaOnly {
		s, cerr := d.Defs.SchemaOf("schema")
		if cerr != nil || s == nil {
			if h.Inline != nil {
				s, _ = schema.Compile(h.Inline, "")
			}
		}
		if s != nil {
			return d.writeSchemaBody(s)
		}
		return ""
	}

	var lines []string
	for _, def := range h.Defs {
		switch def.Kind {
		case parser.DefSchema:
			if ref, ok := def.Value.(string); ok && strings.HasPrefix(ref, "$") {
				lines = append(lines, "~ $"+headerName(def.Key)+": "+refSpelling(ref))
				continue
			}
			s, cerr := d.Defs.SchemaOf(def.Key)
			if cerr != nil || s == nil {
				continue
			}
			body := d.writeSchemaBody(s)
			lines = append(lines, "~ $"+headerName(def.Key)+": {"+body+"}")
		case parser.DefVar:
			lines = append(lines, "~ @"+headerName(def.Key)+": "+d.writeValue(def.Value, nil))
		default:
			lines = append(lines, "~ "+formatObjectKey(def.Key)+": "+d.writeValue(def.Value, nil))
		}
	}
	return strings.Join(lines, "\n")
}

// ── schemas ────────────────────────────────────────────────────────────────

// writeSchemaBody renders a compiled schema's member declarations (no braces).

// NewUnvalidatedHeader wraps a parsed header for WRITING it alone — the
// stringify-a-header operation, used when a header compiled once is sent to a
// peer that will read it back with the same definitions.
func NewUnvalidatedHeader(h *parser.Header) *Doc {
	pdoc := &parser.Document{Header: h}
	return &Doc{Document: pdoc, Defs: NewDefinitions(h), SecSchemas: map[*parser.Section]*schema.Schema{}}
}
