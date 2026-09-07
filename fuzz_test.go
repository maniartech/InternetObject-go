package internetobject_test

import (
	"bytes"
	stdio "io"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// Coverage-guided fuzzing of the READER with arbitrary bytes — the complement
// of internal/document's property fuzzer, which attacks the writer with
// arbitrary values. Run with:
//
//	go test -fuzz=FuzzParse -fuzztime=60s .
//	go test -fuzz=FuzzStream -fuzztime=60s .

var fuzzSeedDocs = []string{
	"",
	"~ $schema: {name: string, age: {int, min: 0}, active: bool, tags: [string]}\n---\n~ John, 42, T, [a, b]",
	"name: string, age: int\n---\nAlice, 30",
	"~ $P: {n: string}\n~ $q: 4\n~ @v: hello\n--- sec: $P\n~ @v",
	"--- one\n~ 1\n--- two\n~ 2",
	"~ d\"2024-01-15\", t\"14:30:45.123\", dt\"2024-01-15T14:30:45.123Z\"",
	"~ 0xFF, 0b101, 0o17, 12n, 1.50m, -Inf, NaN, 1e+12n",
	`~ "quoted", 'single', r"raw ""x""", b"SGVsbG8=", open string`,
	"{a: {b: {c: [1, [2, [3]]]}}}",
	"~ x: \"\\u00e9\\n\\t\\\\\"",
	"# comment\n~ 1 # trailing\n",
	"a---b, ---",
	"~ ,,,",
	"\uFEFF~ 1",
	"~ {\"a,b\": 1, \"\": 2, \"0\": 3}",
}

// FuzzParse asserts, for every input: Parse and String never panic, and for a
// document that parses CLEANLY the canonical writer's output re-parses
// cleanly and a second write is byte-identical (writer/reader agreement).
// KNOWN FORMAT LIMIT, so it is not re-chased every time mutation finds it: a
// schema may declare a `default` its OWN member would reject —
// `{string, A, [B]}`, `{string, A, pattern:'0'}`, `{int, 5, min: 10}`. The
// default is applied unchecked (the reference does the same, probed
// 2026-09-06), so an absent member is filled with a value that same schema
// rejects and the output cannot be re-read. The corpus PINS the permissive
// behaviour — validation/defaults.io :: default_not_a_choice — so no port may
// tighten it alone; io-go tried and the corpus refused the change.
//
// The round-trip property below is therefore stricter than the format. A seed
// that encodes this shape is REMOVED rather than kept red, because it asserts
// something the corpus explicitly permits. Recorded upstream as finding #23.

func FuzzParse(f *testing.F) {
	for _, s := range fuzzSeedDocs {
		f.Add(s)
	}
	// Seed with the playground samples too. A fuzzer is only as good as where
	// it starts: mutating a hand-written toy document explores the shapes near
	// a toy, while these carry variables, schema references, nested and
	// recursive schemas, several sections and every scalar type — so a single
	// mutation lands somewhere structurally interesting instead of somewhere
	// trivially malformed.
	for _, s := range playgroundSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		doc, err := io.Parse(src)
		if doc == nil {
			t.Fatal("Parse returned a nil document")
		}
		checkErrorAttribution(t, doc)
		out := doc.String()
		if err != nil {
			return
		}
		back, err2 := io.Parse(out)
		if err2 != nil {
			// The one shape where a clean document legitimately cannot be
			// re-read: its schema declares a default the same schema rejects,
			// so an absent member is filled with an invalid value. See the
			// note above this function - the corpus permits it and the
			// reference does the same, so it is not this port's defect.
			if io.SchemaRejectsItsOwnDefault(doc) {
				return
			}
			t.Fatalf("clean document's output does not re-parse: %v\n  in=%q\n  out=%q", err2, src, out)
		}
		if second := back.String(); second != out {
			t.Fatalf("writer not idempotent:\n  in=%q\n  first=%q\n  second=%q", src, out, second)
		}
	})
}

// FuzzStream asserts the streaming reader never panics and that transport
// chunk boundaries are never semantic: the whole input in one chunk and the
// same input byte-by-byte yield identical item sequences.
func FuzzStream(f *testing.F) {
	for _, s := range fuzzSeedDocs {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		collect := func(r *chunkReader) (items []string, fatal string) {
			for item, err := range io.Stream(r, nil) {
				if err != nil {
					return items, err.Error()
				}
				if item.Err != nil {
					items = append(items, "err:"+string(item.Err.Code))
				} else {
					items = append(items, "ok:"+item.SchemaName)
				}
			}
			return items, ""
		}
		whole, wf := collect(&chunkReader{src: src, chunk: len(src) + 1})
		bytewise, bf := collect(&chunkReader{src: src, chunk: 1})
		if wf != bf || len(whole) != len(bytewise) {
			t.Fatalf("chunking changed the stream: whole=%d items fatal=%q, per-byte=%d items fatal=%q\n  in=%q",
				len(whole), wf, len(bytewise), bf, src)
		}
		for i := range whole {
			if whole[i] != bytewise[i] {
				t.Fatalf("item %d differs by chunking: %q vs %q\n  in=%q", i, whole[i], bytewise[i], src)
			}
		}
	})
}

// chunkReader serves a string in fixed-size chunks.
type chunkReader struct {
	src   string
	pos   int
	chunk int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.src) {
		return 0, stdio.EOF
	}
	n := copy(p, r.src[r.pos:min(r.pos+r.chunk, len(r.src))])
	r.pos += n
	return n, nil
}

// checkErrorAttribution asserts the section error lists reconcile with the
// document's (ADR 0005 D7): a fault is attributed to at most one section and is
// never counted twice, so the section totals can only ever fall SHORT of the
// document's — by exactly the faults that belong to no section, which are the
// header and schema-binding ones.
//
// Under-counting is the failure this cannot see directly, so it is pinned from
// the other side by TestEveryFaultRouteAttributesToItsSection, which requires
// equality for each route into a section's list.
func checkErrorAttribution(t *testing.T, doc *io.Document) {
	total := 0
	for _, sec := range doc.Sections() {
		n := len(sec.Errors())
		total += n
		if sec.HasErrors() != (n > 0) {
			t.Fatalf("section %q: HasErrors() = %v but Errors() has %d",
				sec.Name(), sec.HasErrors(), n)
		}
	}
	if got := len(doc.Errors()); total > got {
		t.Fatalf("sections report %d errors, document only %d", total, got)
	}
}

// FuzzBuilderRoundTrip asserts the builder's central promise: it cannot
// produce a document its own parser rejects. Every record it accepts must
// survive String → Parse, and the text must be idempotent.
//
// The inputs are field values, so mutation explores the spellings the writer
// has to quote correctly — the place a builder and a parser most easily
// disagree.
func FuzzBuilderRoundTrip(f *testing.F) {
	f.Add("Alice", 30, "apac")
	f.Add("", 0, "")
	f.Add("a, b", 1, "x: y")
	f.Add("~ tilde", -1, "--- sep")
	f.Add("T", 2, "N")
	f.Add("0x10", 3, "1.5m")
	f.Add("\"quoted\"", 4, "line\nbreak")
	f.Add("d\"2024-01-01\"", 5, "@ref")

	f.Fuzz(func(t *testing.T, name string, age int, note string) {
		if len(name) > 200 || len(note) > 200 {
			t.Skip()
		}
		if age < 0 || age > 130 {
			age = 30 // the schema below bounds it; other values are its own test
		}
		s, err := io.ParseSchema("{name: string, age: {int, min: 0, max: 130}, note: string}")
		if err != nil {
			t.Fatal(err)
		}
		b := io.NewBuilder().Define("R", s)
		sec := b.Section("rows", "R")
		if err := sec.Add(map[string]any{"name": name, "age": age, "note": note}); err != nil {
			// A record the schema rejects is a legitimate answer; what must
			// never happen is one being accepted and then unreadable.
			return
		}
		doc, err := b.Document()
		if err != nil {
			t.Fatalf("Document after a successful Add: %v", err)
		}
		text := doc.String()
		back, err := io.Parse(text)
		if err != nil {
			t.Fatalf("the builder produced text its own parser rejects: %v\n%q", err, text)
		}
		if back.String() != text {
			t.Fatalf("writing is not idempotent:\n%q\n%q", text, back.String())
		}
		// The values survive unchanged.
		rec, ok := back.Section("rows").Records()[0].(*io.Object)
		if !ok {
			t.Fatalf("row 0 is %T", back.Section("rows").Records()[0])
		}
		if v, _ := rec.Get("name"); v != name {
			t.Fatalf("name changed: %q -> %q (%q)", name, v, text)
		}
		if v, _ := rec.Get("note"); v != note {
			t.Fatalf("note changed: %q -> %q (%q)", note, v, text)
		}
		checkErrorAttribution(t, back)
	})
}

// FuzzStreamRoundTrip asserts the writer's promise against the reader: every
// record the StreamMarshaler accepts must come back through Stream, in order,
// with its values intact. The two are the ends of one link, and nothing else
// checks that they agree.
func FuzzStreamRoundTrip(f *testing.F) {
	f.Add("Alice", 30, "apac")
	f.Add("", 0, "")
	f.Add("a, b", 1, "~ tilde")
	f.Add("--- sep", 2, "x: y")
	f.Add("T", 3, "N")
	f.Add("\"q\"", 4, "line\nbreak")
	f.Add("0x10", 5, "1.5m")

	f.Fuzz(func(t *testing.T, a string, n int, b string) {
		if len(a) > 200 || len(b) > 200 || n < 0 || n > 130 {
			t.Skip()
		}
		s, err := io.ParseSchema("{name: string, age: {int, min: 0, max: 130}, note: string}")
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sm, err := io.NewStreamMarshaler(&buf, &io.StreamOptions{Schema: s})
		if err != nil {
			t.Fatal(err)
		}
		rec := map[string]any{"name": a, "age": n, "note": b}
		if err := sm.Marshal(rec); err != nil {
			return // a record the schema rejects is a legitimate answer
		}
		if err := sm.Marshal(rec); err != nil {
			t.Fatalf("the second identical record was rejected: %v", err)
		}
		if err := sm.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		count := 0
		for item, err := range io.Stream(bytes.NewReader(buf.Bytes()), nil) {
			if err != nil {
				t.Fatalf("the writer produced a stream its reader cannot read: %v\n%q", err, buf.String())
			}
			if item.Err != nil {
				t.Fatalf("record %d faulted on read: %s\n%q", item.Index, item.Err.Code, buf.String())
			}
			obj, ok := item.Value.(*io.Object)
			if !ok {
				t.Fatalf("record %d is %T", item.Index, item.Value)
			}
			if v, _ := obj.Get("name"); v != a {
				t.Fatalf("name changed: %q -> %q\n%q", a, v, buf.String())
			}
			if v, _ := obj.Get("note"); v != b {
				t.Fatalf("note changed: %q -> %q\n%q", b, v, buf.String())
			}
			count++
		}
		if count != 2 {
			t.Fatalf("wrote 2 records, read %d\n%q", count, buf.String())
		}
		// A stream is also an ordinary document.
		if _, err := io.Parse(buf.String()); err != nil {
			t.Fatalf("the stream is not a valid document: %v\n%q", err, buf.String())
		}
	})
}
