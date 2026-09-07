package internetobject

import (
	"encoding/base64"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/maniartech/InternetObject-go/internal/core"
)

// JSONOptions control how a document projects into JSON.
//
// The zero value is the default: failed rows are written as null, and the
// output is compact.
type JSONOptions struct {
	// SkipErrors omits rows that failed instead of writing null for them.
	//
	// It does NOT renumber the survivors: an index carried by an Error refers
	// to the document, so a filtered projection that renumbered would make
	// those indices point at the wrong rows.
	SkipErrors bool
	// Indent, when non-empty, pretty-prints with this string per level.
	Indent string
}

// JSON renders the document as JSON.
//
// The format carries values JSON has no spelling for, so the mapping is a
// DECISION and is stated here rather than discovered:
//
//	object            object, member order PRESERVED (never through a Go map)
//	positional member its index as the key, exactly as Value does
//	number            number
//	bigint            a number when it fits int64, else a string — exact either way
//	decimal           a string, so the scale and every digit survive
//	datetime/date/time RFC 3339 / YYYY-MM-DD / HH:MM:SS[.fff]
//	binary            a base64 string
//	failed row        null, or omitted with SkipErrors
//
// bigint and decimal become strings rather than JSON numbers wherever a number
// would lose digits: JSON numbers are doubles by convention, and a format that
// exists to carry exact values must not hand them to one silently.
//
// A multi-section document projects as an object keyed by section name, as
// [Document.Value] does; a single unnamed section projects as its records
// alone.
func (d *Document) JSON(opts *JSONOptions) ([]byte, error) {
	o := JSONOptions{}
	if opts != nil {
		o = *opts
	}
	if d == nil || d.doc == nil {
		return []byte("null"), nil
	}
	e := &jsonEncoder{opts: o}
	e.value(d.Value(), 0)
	if e.err != nil {
		return nil, e.err
	}
	return e.buf, nil
}

// MarshalJSON makes a Document usable anywhere encoding/json is, so it can be
// embedded in a larger response without a manual conversion.
func (d *Document) MarshalJSON() ([]byte, error) { return d.JSON(nil) }

type jsonEncoder struct {
	buf  []byte
	opts JSONOptions
	err  error
}

func (e *jsonEncoder) newline(depth int) {
	if e.opts.Indent == "" {
		return
	}
	e.buf = append(e.buf, '\n')
	for i := 0; i < depth; i++ {
		e.buf = append(e.buf, e.opts.Indent...)
	}
}

func (e *jsonEncoder) value(v any, depth int) {
	switch x := v.(type) {
	case nil:
		e.buf = append(e.buf, "null"...)
	case bool:
		e.buf = strconv.AppendBool(e.buf, x)
	case string:
		e.str(x)
	case float64:
		e.number(x)
	case *core.Object:
		e.object(x, depth)
	case core.ErrorNode:
		// A failed row keeps its place, so the rows around it stay at the
		// indices the document gave them.
		e.buf = append(e.buf, "null"...)
	case []any:
		e.array(x, depth)
	case *big.Int:
		// Exact when a JSON number can hold it; a string when it cannot,
		// rather than a double that has quietly dropped digits.
		if x.IsInt64() {
			e.buf = strconv.AppendInt(e.buf, x.Int64(), 10)
		} else {
			e.str(x.String())
		}
	case core.Decimal:
		e.str(x.String()) // the scale is part of the value
	case time.Time:
		e.str(temporalJSON(x))
	case []byte:
		e.str(base64.StdEncoding.EncodeToString(x))
	default:
		e.err = &MarshalError{Path: "$", Msg: "cannot render " +
			typeNameOf(v) + " as JSON"}
	}
}

func (e *jsonEncoder) object(o *core.Object, depth int) {
	e.buf = append(e.buf, '{')
	n := 0
	for i := range o.Members {
		m := o.Members[i]
		if m.Absent {
			continue
		}
		if n > 0 {
			e.buf = append(e.buf, ',')
		}
		e.newline(depth + 1)
		key := m.Key
		if m.Positional {
			key = strconv.Itoa(i) // the index IS the identity, as Value projects it
		}
		e.str(key)
		e.buf = append(e.buf, ':')
		if e.opts.Indent != "" {
			e.buf = append(e.buf, ' ')
		}
		e.value(m.Value, depth+1)
		n++
	}
	if n > 0 {
		e.newline(depth)
	}
	e.buf = append(e.buf, '}')
}

func (e *jsonEncoder) array(a []any, depth int) {
	e.buf = append(e.buf, '[')
	n := 0
	for _, item := range a {
		if _, failed := item.(core.ErrorNode); failed && e.opts.SkipErrors {
			continue
		}
		if n > 0 {
			e.buf = append(e.buf, ',')
		}
		e.newline(depth + 1)
		e.value(item, depth+1)
		n++
	}
	if n > 0 {
		e.newline(depth)
	}
	e.buf = append(e.buf, ']')
}

// number writes a float the way JSON spells one. A non-finite value has no
// JSON spelling at all, so it becomes null rather than invalid output.
func (e *jsonEncoder) number(f float64) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		e.buf = append(e.buf, "null"...)
		return
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		e.buf = strconv.AppendInt(e.buf, int64(f), 10)
		return
	}
	e.buf = strconv.AppendFloat(e.buf, f, 'g', -1, 64)
}

// str writes a JSON string, escaping what JSON requires and what a consumer
// embedding the output in HTML or a JS source would otherwise be hurt by.
func (e *jsonEncoder) str(s string) {
	e.buf = append(e.buf, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			e.buf = append(e.buf, s[start:i]...)
			switch c {
			case '"':
				e.buf = append(e.buf, '\\', '"')
			case '\\':
				e.buf = append(e.buf, '\\', '\\')
			case '\n':
				e.buf = append(e.buf, '\\', 'n')
			case '\r':
				e.buf = append(e.buf, '\\', 'r')
			case '\t':
				e.buf = append(e.buf, '\\', 't')
			default:
				// Control characters, and <>& so the output is safe to embed
				// in HTML — the same choice encoding/json makes by default.
				const hex = "0123456789abcdef"
				e.buf = append(e.buf, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 becomes the replacement character; JSON has no way
			// to carry an unpaired byte, and emitting it raw makes the output
			// unparseable.
			e.buf = append(e.buf, s[start:i]...)
			e.buf = append(e.buf, '\\', 'u', 'f', 'f', 'f', 'd')
			i += size
			start = i
			continue
		}
		i += size
	}
	e.buf = append(e.buf, s[start:]...)
	e.buf = append(e.buf, '"')
}

// temporalJSON spells a temporal the way its own kind reads back: a date has
// no clock, a time has no date, and a datetime is the whole instant.
func temporalJSON(t time.Time) string {
	u := t.UTC()
	switch {
	case u.Year() == core.TimeAnchor.Year() &&
		u.Month() == core.TimeAnchor.Month() &&
		u.Day() == core.TimeAnchor.Day():
		// A time-of-day carries the anchor date, which means nothing to a
		// consumer, so only the clock is written.
		if u.Nanosecond() != 0 {
			return u.Format("15:04:05.000")
		}
		return u.Format("15:04:05")
	case u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0:
		return u.Format("2006-01-02")
	default:
		return u.Format(time.RFC3339Nano)
	}
}

func typeNameOf(v any) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}
