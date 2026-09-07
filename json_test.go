package internetobject_test

import (
	"encoding/json"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The mapping is a DECISION (io-specs does not cover JSON projection), so it is
// pinned rather than left to discovery.
func TestJSONTypeMapping(t *testing.T) {
	src := `~ $E: {name: string, n: number, i: int, big: bigint, price: decimal, ` +
		`when: datetime, on: date, at: time, tags: [string], ok: bool}
--- $E
~ Alice, 3.5, 42, 123456789012345678901234567890n, 19.99m, ` +
		`dt"2024-03-20T14:30:00Z", d"2024-03-20", t"14:30:00", [a, b], T
`
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := doc.JSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"name":"Alice"`,
		`"n":3.5`,
		`"i":42`,
		`"big":"123456789012345678901234567890"`, // exact: a double would drop digits
		`"price":"19.99"`,                        // the scale is part of the value
		`"when":"2024-03-20T14:30:00Z"`,
		`"on":"2024-03-20"`, // a date has no clock
		`"at":"14:30:00"`,   // a time has no date
		`"tags":["a","b"]`,
		`"ok":true`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	// It is valid JSON, checked by the standard library rather than by eye.
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("the output is not valid JSON: %v\n%s", err, out)
	}
}

// A bigint that fits stays a number; one that does not becomes an exact string.
func TestJSONBigIntAndBinary(t *testing.T) {
	doc, err := io.Parse(`---` + "\n" + `~ small: 42n, big: 123456789012345678901234567890n, bin: b"SGVsbG8="`)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := doc.JSON(nil)
	for _, want := range []string{`"small":42`, `"big":"123456789012345678901234567890"`, `"bin":"SGVsbG8="`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

// Member ORDER survives, which a Go map would destroy.
func TestJSONPreservesMemberOrder(t *testing.T) {
	doc, err := io.Parse("---\n~ zebra: 1, apple: 2, mango: 3")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := doc.JSON(nil)
	z, a, m := strings.Index(string(out), "zebra"), strings.Index(string(out), "apple"), strings.Index(string(out), "mango")
	if !(z < a && a < m) {
		t.Errorf("member order was not preserved: %s", out)
	}
}

// A failed row keeps its place, so the rows around it stay at the indices the
// document gave them; SkipErrors drops it WITHOUT renumbering the rest.
func TestJSONErrorRows(t *testing.T) {
	doc, _ := io.Parse("~ $S: {n: int}\n--- $S\n~ 1\n~ bad\n~ 3")
	out, err := doc.JSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `[{"n":1},null,{"n":3}]` {
		t.Errorf("with errors = %s", out)
	}
	skipped, err := doc.JSON(&io.JSONOptions{SkipErrors: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(skipped) != `[{"n":1},{"n":3}]` {
		t.Errorf("skipErrors = %s", skipped)
	}
	// The document's own error still names row 1, which is why the survivors
	// are not renumbered.
	if e := doc.Errors()[0]; e.RecordIndex != 1 {
		t.Errorf("the error names row %d", e.RecordIndex)
	}
}

func TestJSONMultiSection(t *testing.T) {
	doc, err := io.Parse("--- a\n~ x: 1\n~ x: 2\n--- b\n~ y: 3\n")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := doc.JSON(nil)
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(v) != 2 || v["a"] == nil || v["b"] == nil {
		t.Errorf("sections = %v", v)
	}
}

// Strings are escaped so the output is valid JSON and safe to embed in HTML —
// the same choice encoding/json makes.
func TestJSONEscaping(t *testing.T) {
	for _, in := range []string{
		"a\"b", "back\\slash", "tab\there", "new\nline", "<script>", "a&b",
		"\x00\x01\x1f", "unicode é 日本",
	} {
		doc, err := io.Parse("---\n~ s: " + quoteIO(in))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		out, err := doc.JSON(nil)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		var v []map[string]string
		if err := json.Unmarshal(out, &v); err != nil {
			t.Fatalf("%q produced invalid JSON: %v\n%s", in, err, out)
		}
		if len(v) != 1 || v[0]["s"] != in {
			t.Errorf("%q round-tripped as %q", in, v[0]["s"])
		}
		for _, unsafe := range []string{"<", ">", "&"} {
			if strings.Contains(string(out), unsafe) {
				t.Errorf("%q left %s unescaped: %s", in, unsafe, out)
			}
		}
	}
}

// A Document works anywhere encoding/json does.
func TestDocumentMarshalJSON(t *testing.T) {
	doc, err := io.Parse("---\n~ a: 1")
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := json.Marshal(map[string]any{"payload": doc})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapped), `"payload":[{"a":1}]`) {
		t.Errorf("embedded = %s", wrapped)
	}
	// A nil document is null, not a panic.
	var nilDoc *io.Document
	if out, err := nilDoc.JSON(nil); err != nil || string(out) != "null" {
		t.Errorf("nil document = %q, %v", out, err)
	}
}

func TestJSONIndent(t *testing.T) {
	doc, _ := io.Parse("---\n~ a: 1, b: [2, 3]")
	out, err := doc.JSON(&io.JSONOptions{Indent: "  "})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "\n  ") {
		t.Errorf("not indented:\n%s", out)
	}
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("indented output is not valid JSON: %v\n%s", err, out)
	}
	// Empty containers stay compact rather than gaining a blank line.
	e, _ := io.Parse("---\n~ a: [], b: {}")
	eo, _ := e.JSON(&io.JSONOptions{Indent: "  "})
	if strings.Contains(string(eo), "[\n\n") || strings.Contains(string(eo), "{\n\n") {
		t.Errorf("empty containers gained blank lines:\n%s", eo)
	}
}

// quoteIO writes a Go string as an IO string literal.
func quoteIO(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u`)
				const hex = "0123456789abcdef"
				b.WriteByte('0')
				b.WriteByte('0')
				b.WriteByte(hex[byte(r)>>4])
				b.WriteByte(hex[byte(r)&0xF])
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
