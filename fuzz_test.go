package internetobject_test

import (
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
					items = append(items, "err:"+item.Err.Code)
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
