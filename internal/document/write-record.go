package document

import (
	"encoding/base64"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/maniartech/InternetObject-go/internal/core"
	"github.com/maniartech/InternetObject-go/internal/parser"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// Writing RECORDS: sections, the records in them, and the values in those.

type partWriter struct {
	count   int // parts committed, including flushed empties
	pending int // empty parts held back
}

// sep writes the separator for the next part, flushing any held empties.
func (w *partWriter) sep(dst []byte) []byte {
	for ; w.pending > 0; w.pending-- {
		if w.count > 0 {
			dst = append(dst, ',', ' ')
		}
		w.count++
	}
	if w.count > 0 {
		dst = append(dst, ',', ' ')
	}
	w.count++
	return dst
}

func (w *partWriter) empty() { w.pending++ }

func (d *Doc) appendSection(dst []byte, sec *parser.Section) []byte {
	sch := d.schemaFor(sec)
	if sec.Collection {
		first := true
		for _, rec := range sec.Records {
			obj, ok := rec.(*core.Object)
			if !ok {
				continue
			}
			if !first {
				dst = append(dst, '\n')
			}
			first = false
			dst = append(dst, '~', ' ')
			dst = d.appendBareRecord(dst, obj, sch)
		}
		return dst
	}
	if len(sec.Records) == 0 {
		return dst
	}
	obj, ok := sec.Records[0].(*core.Object)
	if !ok {
		return dst
	}
	mark := len(dst)
	dst = d.appendBareRecord(dst, obj, sch)
	if len(dst) == mark {
		dst = append(dst, '{', '}') // an empty bare record still needs a record
	}
	return dst
}

// appendRecord renders one record's members, schema order first.
func (d *Doc) appendRecord(dst []byte, obj *core.Object, sch *schema.Schema) []byte {
	var w partWriter

	if sch != nil {
		for _, name := range sch.Names {
			if name == "*" {
				continue
			}
			md := sch.Defs[name]
			if i := obj.Find(name); i >= 0 {
				dst = w.sep(dst)
				dst = d.appendValueWithDef(dst, obj.Members[i].Value, md)
			} else if md.Optional && !md.HasDefault {
				w.empty() // hold the position; trailing ones are dropped
			}
		}
		for _, m := range obj.Members {
			// "Already written above" is exactly "declared by the schema" —
			// every name in sch.Names is emitted in that loop, and the bare
			// wildcard is skipped there. Reading the compiled schema's own map
			// avoids building a per-record `handled` map (ADR 0006 P2).
			if !m.Positional && m.Key != "*" && sch.Defs[m.Key] != nil {
				continue
			}
			var md *schema.MemberDef
			if o, ok := sch.Open.(*schema.MemberDef); ok {
				md = o
			}
			dst = w.sep(dst)
			if !m.Positional {
				dst = appendObjectKey(dst, m.Key)
				dst = append(dst, ':', ' ')
			}
			dst = d.appendValueWithDef(dst, m.Value, md)
		}
		return dst
	}

	// No schema: a member is positional when keyless or when its key equals
	// its own index; every other name is unrecoverable and must be written.
	// An absent member is a HOLE, not a null: held as an empty slot in the
	// middle, dropped at the end.
	for i, m := range obj.Members {
		if m.Absent {
			w.empty()
			continue
		}
		dst = w.sep(dst)
		if !m.Positional && m.Key != strconv.Itoa(i) {
			dst = appendObjectKey(dst, m.Key)
			dst = append(dst, ':', ' ')
		}
		dst = d.appendValue(dst, m.Value, nil)
	}
	return dst
}

// appendBareRecord renders a record for a BARE emit site — a `~` line or a
// section's single record. A bare line that is exactly one keyless braced
// object is ambiguous unenclosed: the re-parser absorbs those braces as the
// record's own (ISSUE-15), schema or no schema, dropping a nesting level.
// Enclosing applies here only; a nested object's braces come from appendValue,
// where absorption never happens.
func (d *Doc) appendBareRecord(dst []byte, obj *core.Object, sch *schema.Schema) []byte {
	mark := len(dst)
	dst = d.appendRecord(dst, obj, sch)

	present, lastIsObject := 0, false
	for _, m := range obj.Members {
		if m.Absent {
			continue
		}
		present++
		_, lastIsObject = m.Value.(*core.Object)
	}
	if present == 1 && lastIsObject && len(dst) > mark && dst[mark] == '{' {
		// Wrap in place: one shift, and only for this rare shape.
		dst = append(dst, 0)
		copy(dst[mark+1:], dst[mark:])
		dst[mark] = '{'
		dst = append(dst, '}')
	}
	return dst
}

// appendValueWithDef renders a member value under its definition (the declared
// temporal kind wins; a nested schema renders its object positionally).
func (d *Doc) appendValueWithDef(dst []byte, v any, md *schema.MemberDef) []byte {
	if md == nil {
		return d.appendValue(dst, v, nil)
	}
	if v == nil {
		return append(dst, 'N')
	}
	if t, ok := v.(time.Time); ok {
		switch md.Type {
		case "date", "time", "datetime":
			return appendTemporal(dst, t, md.Type)
		}
	}
	if obj, ok := v.(*core.Object); ok {
		sch := md.Schema
		if sch == nil && md.SchemaRef != "" {
			sch, _ = d.Defs.SchemaOf(strings.TrimPrefix(md.SchemaRef, "$"))
		}
		dst = append(dst, '{')
		dst = d.appendRecord(dst, obj, sch)
		return append(dst, '}')
	}
	if arr, ok := v.([]any); ok {
		dst = append(dst, '[')
		for i, e := range arr {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = d.appendValueWithDef(dst, e, md.Of)
		}
		return append(dst, ']')
	}
	return d.appendValue(dst, v, md)
}

// appendValue renders one value with no (or a scalar) definition in scope.
func (d *Doc) appendValue(dst []byte, v any, md *schema.MemberDef) []byte {
	switch x := v.(type) {
	case nil:
		return append(dst, 'N')
	case bool:
		if x {
			return append(dst, 'T')
		}
		return append(dst, 'F')
	case float64:
		return appendIONumber(dst, x)
	case *big.Int:
		dst = x.Append(dst, 10)
		return append(dst, 'n')
	case core.Decimal:
		dst = append(dst, x.String()...)
		return append(dst, 'm')
	case []byte:
		dst = append(dst, 'b', '"')
		dst = base64.StdEncoding.AppendEncode(dst, x)
		return append(dst, '"')
	case time.Time:
		return appendTemporal(dst, x, "")
	case string:
		return appendAutoString(dst, x)
	case *core.Object:
		dst = append(dst, '{')
		dst = d.appendRecord(dst, x, nil)
		return append(dst, '}')
	case []any:
		dst = append(dst, '[')
		for i, e := range x {
			if i > 0 {
				dst = append(dst, ',', ' ')
			}
			dst = d.appendValue(dst, e, nil)
		}
		return append(dst, ']')
	}
	return dst
}

// writeValue keeps the string form for the header path, which runs once per
// document and reads better as a string.
func (d *Doc) writeValue(v any, md *schema.MemberDef) string {
	return string(d.appendValue(nil, v, md))
}

// ── scalars ────────────────────────────────────────────────────────────────

// ioNumber renders a float64 in IO spelling: ECMAScript shortest form with
// the IO names for the specials.
