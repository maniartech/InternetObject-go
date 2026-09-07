// Package streaming implements the record protocol over an incremental byte
// stream: `~` introduces each logical record, the FIRST `---` terminates the
// header, and later `---` frames switch the schema context.
//
// Chunk boundaries are never semantic - the same input split any way yields an
// identical item sequence, because the framer keeps its scan state across feeds
// and every frame goes through the same path a one-record document uses.
package streaming

import (
	"strings"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// outcome, nil when iteration completed normally.
type Reader struct {
	opts StreamOptions

	buf        []byte
	scan       int  // bytes of buf already framed
	frameStart int  // start of the frame being accumulated
	pendingCR  bool // a chunk ended in \r; a following \n is the same newline

	// framer state
	inString  bool
	strQuote  byte
	strRaw    bool // annotated literal: no backslash escapes
	inComment bool

	headerDone bool
	defs       *document.Definitions
	current    *schema.Schema // active schema context
	currentSel string         // explicit selector name (with $), "" when default

	index int
	fatal *ItemError
	done  bool
}

// NewReader creates a reader.
func NewReader(opts StreamOptions) *Reader {
	return &Reader{opts: opts}
}

// Feed appends a chunk and returns the items it completed.
func (r *Reader) Feed(chunk []byte) []Item {
	if r.done {
		return nil
	}
	for _, b := range chunk {
		if r.pendingCR && b == '\n' {
			r.pendingCR = false
			continue // CRLF collapsed to the LF already written
		}
		r.pendingCR = b == '\r'
		if b == '\r' {
			b = '\n'
		}
		r.buf = append(r.buf, b)
	}
	return r.advance(false)
}

// Close signals end of stream, flushing the final frame. The returned error
// is the terminal fatal outcome; nil means normal completion.
func (r *Reader) Close() ([]Item, *ItemError) {
	if r.done {
		return nil, r.fatal
	}
	items := r.advance(true)
	if !r.done {
		if !r.headerDone {
			// Legacy headerless stream: everything buffered is data.
			items = append(items, r.legacyFlush()...)
		} else if r.frameStart < len(r.buf) {
			items = append(items, r.endFrame(len(r.buf))...)
		}
	}
	r.done = true
	return items, r.fatal
}

// advance scans newly buffered bytes for frame boundaries and processes every
// completed frame. atEOF only stops the scan-state bookkeeping from waiting
// for a possible `--` continuation.
func (r *Reader) advance(atEOF bool) []Item {
	var items []Item
	for r.scan < len(r.buf) && !r.done {
		b := r.buf[r.scan]
		switch {
		case r.inComment:
			if b == '\n' {
				r.inComment = false
			}
		case r.inString:
			if b == '\\' && !r.strRaw {
				r.scan++ // skip the escaped byte too
			} else if b == r.strQuote {
				r.inString = false
			}
		case b == '"' || b == '\'':
			r.inString, r.strQuote = true, b
			r.strRaw = r.scan > 0 && isAnnotationByte(r.buf[r.scan-1])
		case b == '#':
			r.inComment = true
		case b == '\\':
			r.scan++ // an escaped character outside a string is content
		case b == '~':
			items = append(items, r.endFrame(r.scan)...)
			r.frameStart = r.scan
		case b == '-':
			if r.scan+3 > len(r.buf) {
				if !atEOF {
					return items // wait: this may become `---`
				}
			} else if r.buf[r.scan+1] == '-' && r.buf[r.scan+2] == '-' {
				items = append(items, r.endFrame(r.scan)...)
				if !r.headerDone {
					r.resolveHeader(string(r.buf[:r.scan]))
					if r.done {
						return items
					}
					// the header frame is consumed; a re-visit (a control
					// line still arriving) must not replay it as data
					r.frameStart = r.scan
				}
				// the control frame runs to the end of its line
				end := r.scan + 3
				for end < len(r.buf) && r.buf[end] != '\n' && r.buf[end] != '~' {
					end++
				}
				if end >= len(r.buf) && !atEOF {
					return items // the selector line may still be arriving
				}
				r.control(string(r.buf[r.scan+3 : end]))
				r.scan = end
				r.frameStart = end
				continue
			}
		}
		r.scan++
	}
	return items
}

func isAnnotationByte(b byte) bool {
	return b == 'r' || b == 'b' || b == 'd' || b == 't'
}

// control processes one `---` control frame: the first terminates the header;
// later ones switch the schema context.
func (r *Reader) control(selector string) {
	selector = strings.TrimSpace(selector)
	if r.opts.Schema != nil {
		// An attached schema wins outright: in-stream selectors change the
		// reported name, never the schema records are validated against.
		r.current, r.currentSel = r.opts.Schema, strings.TrimSpace(strings.TrimPrefix(selector, "$"))
		if r.currentSel != "" {
			r.currentSel = "$" + r.currentSel
		}
		return
	}
	if selector == "" {
		r.current, r.currentSel = r.defaultSchema(), ""
		return
	}
	if strings.HasPrefix(selector, "$") {
		s, cerr := r.defs.SchemaOf(selector[1:])
		if cerr != nil {
			r.fatal = &ItemError{Category: categoryOf(cerr.Code), Code: cerr.Code}
			r.done = true
			return
		}
		r.current, r.currentSel = s, selector
		return
	}
	// `--- name: $ref` — a named section binding
	if name, ref, found := strings.Cut(selector, ":"); found {
		_ = name
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, "$") {
			s, cerr := r.defs.SchemaOf(ref[1:])
			if cerr != nil {
				r.fatal = &ItemError{Category: categoryOf(cerr.Code), Code: cerr.Code}
				r.done = true
				return
			}
			r.current, r.currentSel = s, ref
			return
		}
	}
	r.current, r.currentSel = r.defaultSchema(), ""
}

// resolveHeader parses the buffered header text ATOMICALLY (references inside
// it are position-independent) and merges it over the preloaded definitions.
func (r *Reader) resolveHeader(text string) {
	merged := &parser.Header{Schemas: map[string]any{}, Vars: map[string]any{}}
	apply := func(h *parser.Header) {
		if h == nil {
			return
		}
		for _, def := range h.Defs {
			switch def.Kind {
			case parser.DefSchema:
				merged.Schemas[def.Key] = def.Value
			case parser.DefVar:
				merged.Vars[def.Key] = def.Value
			}
			found := false
			for i := range merged.Defs {
				if merged.Defs[i].Kind == def.Kind && merged.Defs[i].Key == def.Key {
					merged.Defs[i].Value = def.Value
					found = true
				}
			}
			if !found {
				merged.Defs = append(merged.Defs, def)
			}
		}
		if h.Inline != nil {
			merged.Inline = h.Inline
		}
	}
	if r.opts.Definitions != "" {
		if h, ok := parseHeaderText(r.opts.Definitions); ok {
			apply(h)
		}
	}
	if strings.TrimSpace(text) != "" {
		h, ok := parseHeaderText(text)
		if !ok {
			r.fatal = &ItemError{Category: "syntax", Code: errs.InvalidDefinition}
			r.done = true
			return
		}
		apply(h)
	}
	r.defs = document.NewDefinitions(merged)
	r.headerDone = true
	r.current, r.currentSel = r.defaultSchema(), ""
}

// parseHeaderText parses header text (definitions before a `---`). ok is
// false only when the text does not parse; an empty header is fine.
func parseHeaderText(text string) (*parser.Header, bool) {
	doc := parser.Parse(strings.TrimRight(text, " \t\n") + "\n---\n")
	if len(doc.Errors) > 0 {
		return nil, false
	}
	return doc.Header, true
}

// defaultSchema resolves the bare-`---` context: the in-stream `$schema`,
// else the reader option's fallback, else none.
func (r *Reader) defaultSchema() *schema.Schema {
	if r.opts.Schema != nil {
		return r.opts.Schema // an attached schema outranks everything
	}
	if r.defs == nil {
		return nil
	}
	if _, ok := r.defs.Header.Schemas["schema"]; ok {
		s, _ := r.defs.SchemaOf("schema")
		return s
	}
	if r.opts.DefaultSchema != "" {
		s, _ := r.defs.SchemaOf(strings.TrimPrefix(r.opts.DefaultSchema, "$"))
		return s
	}
	return nil
}

// endFrame processes the frame ending at pos, emitting at most one item.
func (r *Reader) endFrame(pos int) []Item {
	if !r.headerDone {
		return nil // still buffering the header
	}
	text := strings.TrimSpace(string(r.buf[r.frameStart:pos]))
	r.frameStart = pos
	if text == "" || !strings.HasPrefix(text, "~") {
		return nil // nothing, or stray content between frames
	}
	item := r.processRecord(text)
	item.RecordIndex = r.index
	r.index++
	if r.currentSel != "" {
		item.SchemaName = r.currentSel
	}
	return []Item{item}
}

// processRecord runs one record frame through the same core path a
// one-record document uses.
func (r *Reader) processRecord(text string) Item {
	doc := parser.Parse(text)
	if len(doc.Errors) > 0 {
		e := doc.Errors[0]
		return Item{Kind: "record-error", Err: &ItemError{Category: categoryOf(e.Code), Code: e.Code}}
	}
	if len(doc.Sections) == 0 || len(doc.Sections[0].Records) == 0 {
		return Item{Kind: "record", Value: &core.Object{}}
	}
	return r.recordItem(doc.Sections[0].Records[0])
}

// recordItem converts one parsed record to an item under the ACTIVE schema
// context — the shared tail of the framed and legacy paths, so a headerless
// stream validates against preloaded definitions exactly as a framed one does
// (the reference passes its definitions to parse() on both routes).
func (r *Reader) recordItem(rec any) Item {
	if e, ok := rec.(core.ErrorNode); ok {
		return Item{Kind: "record-error", Err: &ItemError{Category: categoryOf(e.Code), Code: e.Code}}
	}
	obj, ok := rec.(*core.Object)
	if !ok {
		return Item{Kind: "record", Value: parser.ProjectValue(rec)}
	}

	if r.current != nil {
		validated, verrs := schema.ValidateRecord(obj, r.current, r.defs, false)
		if len(verrs) > 0 {
			e := verrs[0]
			return Item{Kind: "record-error", Err: &ItemError{Category: categoryOf(e.Code), Code: e.Code}}
		}
		return Item{Kind: "record", Value: parser.ProjectValue(validated)}
	}

	if verr := document.ResolveVars(obj, r.defs); verr != nil {
		return Item{Kind: "record-error", Err: &ItemError{Category: categoryOf(verr.Code), Code: verr.Code}}
	}
	var deferred []errs.Error
	document.SurfaceDeferred(obj, &deferred)
	if len(deferred) > 0 {
		e := deferred[0]
		return Item{Kind: "record-error", Err: &ItemError{Category: categoryOf(e.Code), Code: e.Code}}
	}
	return Item{Kind: "record", Value: parser.ProjectValue(obj)}
}

// legacyFlush handles a stream that never carried a `---`: the whole buffer
// is data, as a non-streaming document would read it.
func (r *Reader) legacyFlush() []Item {
	r.resolveHeader("")
	if r.done {
		return nil
	}
	text := string(r.buf)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	doc := parser.Parse(text)
	var items []Item
	for _, sec := range doc.Sections {
		for _, rec := range sec.Records {
			item := r.recordItem(rec)
			item.RecordIndex = r.index
			r.index++
			items = append(items, item)
		}
	}
	if len(doc.Errors) > 0 && len(items) == 0 {
		e := doc.Errors[0]
		items = append(items, Item{Kind: "record-error", RecordIndex: r.index,
			Err: &ItemError{Category: categoryOf(e.Code), Code: e.Code}})
		r.index++
	}
	return items
}

// categoryOf maps a designated code to its wire category, mirroring the core
// error class that raises it in the reference implementation.
func categoryOf(code errs.Code) string {
	switch code {
	case errs.UnexpectedToken, errs.ExpectedClosingBracket, errs.ExpectedValue,
		errs.InvalidKey, errs.InvalidDefinition, errs.DuplicateSectionName,
		errs.UnexpectedPositionalMember, errs.InvalidSchema, errs.EmptyMemberdef,
		"unterminated-string", "invalid-escape-sequence", "unknown-annotation",
		"invalid-binary", "invalid-number", "invalid-bigint", "invalid-decimal",
		"invalid-date", "invalid-time", "invalid-datetime",
		"invalid-section-name", "missing-schema":
		return "syntax"
	case "stream-source-error", "stream-aborted", "stream-buffer-exceeded":
		return "stream"
	}
	return "validation"
}
