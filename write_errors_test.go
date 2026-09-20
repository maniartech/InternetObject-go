package internetobject_test

import (
	"errors"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// Parsing accumulates faults and returns the document anyway — that is the
// format's promise. The other half of that promise is that the damaged document
// cannot quietly become a damaged FILE: a projection may DESCRIBE errors, but a
// file must not CONTAIN them, so writing one is refused.
//
// Before this rule, doc.String() dropped every failed record and said nothing.
// A caller who parsed tolerantly, then saved, wrote a silently truncated file.
//
// The reference implementation settled the rule first, having shipped the same
// bug: a collected error could become a corrupt file with nothing to signal it.

const faultedDoc = "name: string, age: int\n---\n~ Alice, 30\n~ Bob, oops\n~ Carol, 40"

func TestWritingRefusesADocumentHoldingAFailedRecord(t *testing.T) {
	doc, perr := io.Parse(faultedDoc)
	if perr == nil {
		t.Fatal("the fixture must fault at parse, or this test proves nothing")
	}
	if doc == nil {
		t.Fatal("Parse returned no document; the surviving records are the point")
	}

	out, err := doc.Text(nil)
	if err == nil {
		t.Fatalf("Text wrote %q; want a refusal", out)
	}
	if out != "" {
		t.Errorf("Text returned %q alongside its error; want the empty string", out)
	}

	var e io.Error
	if !errors.As(err, &e) {
		t.Fatalf("error is %T, want io.Error", err)
	}
	if e.Code != io.ForbiddenErrorNode {
		t.Errorf("Code = %q, want %q", e.Code, io.ForbiddenErrorNode)
	}
	// The fault arose in the library, not in the source text or a schema check.
	if e.Category != io.CategoryGeneral {
		t.Errorf("Category = %q, want %q", e.Category, io.CategoryGeneral)
	}
	// It must point at the record that caused it, not at the document.
	if e.RecordIndex != 1 {
		t.Errorf("RecordIndex = %d, want 1 (Bob)", e.RecordIndex)
	}
}

// SkipErrors is the documented escape, and it must produce a real file: valid,
// re-parseable, and missing exactly the records that failed.
func TestSkipErrorsWritesTheSurvivors(t *testing.T) {
	doc, _ := io.Parse(faultedDoc)

	out, err := doc.Text(&io.TextOptions{SkipErrors: true})
	if err != nil {
		t.Fatalf("Text(SkipErrors): %v", err)
	}
	const want = "name: string, age: int\n---\n~ Alice, 30\n~ Carol, 40"
	if out != want {
		t.Errorf("wrote %q,\n want %q", out, want)
	}

	// What it writes must read back clean — a truncated file is still a file.
	back, err := io.Parse(out)
	if err != nil {
		t.Fatalf("SkipErrors output does not re-parse: %v", err)
	}
	got, err := back.JSON(nil)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if string(got) != `[{"name":"Alice","age":30},{"name":"Carol","age":40}]` {
		t.Errorf("re-parsed as %s", got)
	}
}

// A fmt.Stringer cannot report an error, so String must not hand back something
// that looks like a document. It renders a note instead — loud, and a syntax
// error, which is what stops it being written to a file and mistaken for one.
//
// Every fault shape must hold that property, not just the convenient one. A
// validation fault carries a position, so `…forbidden-error-node at $[1].age
// (4:8)>` ends in something no key can precede; a PARSE-RECOVERY fault carries
// none, so the note collapses to `<internetobject: forbidden-error-node>` —
// and `key: value` is valid Internet Object, so that spelling READ BACK as a
// one-member record. The unclosed brace is what makes the property hold for
// both.
func TestStringMarksARefusedDocumentInsteadOfTruncatingIt(t *testing.T) {
	faults := []string{
		faultedDoc,                          // validation: carries a position
		"~ 1\n~ }",                          // parse recovery: carries none
		"~ 1\n~ ]",                          //
		"~ 1\n~ {a",                         //
		"~ 1\n~ [1",                         //
		"~ 1\n~ \"unterminated",             //
		"~ 1\n~ a: b: c",                    //
		"n: int\n---\n~ 1\n~ }",             //
		"~ a: d\"2024-99-99\"\n~ b: 1",      // malformed literal, no error node
		"~ meta: d\"2024-99-99\"\n---\n~ 1", // a fault in the HEADER
	}
	for _, src := range faults {
		doc, perr := io.Parse(src)
		if perr == nil {
			t.Errorf("%q parsed clean; it is not a fault fixture", src)
			continue
		}
		if doc == nil {
			continue
		}
		s := doc.String()
		if !strings.HasPrefix(s, "{<internetobject:") {
			t.Errorf("%q: String() = %q; want the refusal note", src, s)
		}
		if !strings.Contains(s, string(io.ForbiddenErrorNode)) {
			t.Errorf("%q: String() = %q; want it to name the code", src, s)
		}
		// The note's unparseability must not depend on the message: a `}` in it
		// would close the brace that makes it a syntax error.
		if strings.Contains(strings.TrimPrefix(s, "{"), "}") {
			t.Errorf("%q: String() = %q; the message closes the brace", src, s)
		}
		if _, err := io.Parse(s); err == nil {
			t.Errorf("%q: String() = %q, which PARSES — a refusal must not look like a document", src, s)
		}
	}

	// The old bug: String returned the surviving records as if nothing was lost.
	if s := mustParseFaulted(t, faultedDoc).String(); strings.Contains(s, "Alice") {
		t.Errorf("String() = %q; it wrote record data for a document it cannot write", s)
	}
}

// The refusal must key on the document's FAULT LIST, not on the shape of its
// records. A failed record becomes an error node; a malformed literal does not,
// and a fault in the header leaves no marked record at all. Both used to write
// corrupt text and report success.
func TestWritingRefusesEveryFaultShapeNotJustFailedRecords(t *testing.T) {
	for _, src := range []string{
		"~ a: d\"2024-99-99\"\n~ b: 1",            // deferred literal, record survives
		"a: 0B",                                   // deferred literal, single record
		"~ a: [d\"2024-99-99\", 1]",               // inside an array
		"~ a: {b: d\"2024-99-99\"}",               // inside a child object
		"~ meta: d\"2024-99-99\"\n---\n~ a: 1",    // in the header
		"~ $draft: {title: nosuchtype}\n---\n~ 1", // a broken definition
		"$s: {n: int}\n--- one: $s\n~ 1",          // fine, but see the binding case below
	} {
		doc, perr := io.Parse(src)
		if perr == nil {
			continue // not every row here faults; the ones that do are the point
		}
		if doc == nil {
			continue
		}
		out, err := doc.Text(nil)
		if err == nil {
			t.Errorf("%q: Text wrote %q and reported success; want a refusal", src, out)
			continue
		}
		var e io.Error
		if errors.As(err, &e) && e.Code != io.ForbiddenErrorNode {
			t.Errorf("%q: Code = %q, want %q", src, e.Code, io.ForbiddenErrorNode)
		}
	}
}

// SkipErrors skips RECORDS. A fault that abandoned the load left everything
// after it unvalidated, so there is nothing to skip — writing the document
// verbatim would emit the very text that failed to read.
func TestSkipErrorsStillRefusesAFatalFault(t *testing.T) {
	for _, src := range []string{
		"$s: {n: int}\n--- one: $s\n~ 1\n--- two: $s\n~ oops", // a broken schema binding
		"~ meta: d\"2024-99-99\"\n---\n~ a: 1",                // a fault in the header
		"~ $draft: {title: nosuchtype}\n---\n~ a: 1",          // a broken definition
	} {
		doc, perr := io.Parse(src)
		if perr == nil {
			t.Errorf("%q parsed clean; it is not a fault fixture", src)
			continue
		}
		if doc == nil {
			continue
		}
		out, err := doc.Text(&io.TextOptions{SkipErrors: true})
		if err == nil {
			t.Errorf("%q: Text(SkipErrors) wrote %q; a fatal fault leaves nothing to skip", src, out)
		}
	}
	// And whatever SkipErrors DOES write elsewhere must read back clean.
	ok, _ := io.Parse(faultedDoc)
	survivors, err := ok.Text(&io.TextOptions{SkipErrors: true})
	if err != nil {
		t.Fatalf("Text(SkipErrors) on a record fault: %v", err)
	}
	if _, err := io.Parse(survivors); err != nil {
		t.Errorf("SkipErrors wrote %q, which does not re-parse: %v", survivors, err)
	}
}

// SkipErrors drops failed RECORDS, so it can only rescue a fault whose record
// actually left the document. A deferred malformed literal is not fatal and
// leaves its record in place, holding a value nothing can spell — there is
// nothing to skip, and writing anyway emitted text the reader could not read
// back, which is the corruption this whole rule exists to prevent.
func TestSkipErrorsRefusesWhenThereIsNoRecordToSkip(t *testing.T) {
	for _, src := range []string{
		"~ a: d\"2024-99-99\"\n~ b: 1",           // record survives, value cannot be written
		"a: 0B",                                  // single record
		"~ a: [d\"2024-99-99\", 1]",              // inside an array
		"~ a: {b: d\"2024-99-99\"}",              // inside a child object
		"n: int\n--- one\noops\n--- two\n~ a: 1", // a bare record fails the load
		"n: int\n--- one\noops\n--- two\n~ @nosuchvar",
	} {
		doc, perr := io.Parse(src)
		if perr == nil || doc == nil {
			t.Errorf("%q did not fault as expected", src)
			continue
		}
		out, err := doc.Text(&io.TextOptions{SkipErrors: true})
		if err == nil {
			t.Errorf("%q: Text(SkipErrors) wrote %q; nothing here is skippable", src, out)
			continue
		}
		// Whatever SkipErrors DOES write anywhere must re-parse.
		if out != "" {
			t.Errorf("%q: refusal returned %q alongside its error", src, out)
		}
	}
}

// The other side of the same rule: every fault whose record DID leave the
// document is skippable, whichever route removed it. Three routes build an
// error node — parse recovery, schema validation, and variable resolution in a
// schema-less section — and a fault from any of them must let SkipErrors write
// the rest.
func TestSkipErrorsWritesSurvivorsForEveryRecoveredRoute(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// parse recovery: the second record is unreadable
		{"~ a: 1\n~ }\n~ a: 3", "---\n~ a: 1\n~ a: 3"},
		// schema validation: the second record is the wrong type
		{"a: int\n---\n~ 1\n~ oops\n~ 3", "a: int\n---\n~ 1\n~ 3"},
		// variable resolution in a schema-less collection
		{"~ @v: 1\n---\n~ a: @v\n~ b: @nosuch\n~ c: 3", "~ @v: 1\n---\n~ a: 1\n~ c: 3"},
	} {
		doc, perr := io.Parse(tc.src)
		if perr == nil || doc == nil {
			t.Errorf("%q did not fault as expected", tc.src)
			continue
		}
		out, err := doc.Text(&io.TextOptions{SkipErrors: true})
		if err != nil {
			t.Errorf("%q: Text(SkipErrors) refused (%v); the failed record did leave the document", tc.src, err)
			continue
		}
		if out != tc.want {
			t.Errorf("%q\n  wrote %q\n  want  %q", tc.src, out, tc.want)
		}
		if _, err := io.Parse(out); err != nil {
			t.Errorf("%q: SkipErrors wrote %q, which does not re-parse: %v", tc.src, out, err)
		}
	}
}

// A BARE record's fault abandons the load, so it is not skippable even though
// the record was replaced by an error node — nothing after it was validated.
func TestABareRecordsFaultIsNotSkippable(t *testing.T) {
	for _, src := range []string{
		"~ @v: 1\n--- one\n@nosuch\n--- two\n~ a: 1", // variable resolution, bare
		"a: int\n--- one\noops\n--- two\n~ 1",        // schema validation, bare
	} {
		doc, perr := io.Parse(src)
		if perr == nil || doc == nil {
			t.Errorf("%q did not fault as expected", src)
			continue
		}
		if out, err := doc.Text(&io.TextOptions{SkipErrors: true}); err == nil {
			t.Errorf("%q: Text(SkipErrors) wrote %q; a bare record's fault abandons the load", src, out)
		}
	}
}

// The note's unclosed brace is what makes it a syntax error, so the message
// must not be able to close it. A member NAME reaches the message through the
// fault's path, and a quoted name may carry a `}` — or a `#`, which comments
// away everything after it, including the position that would otherwise break
// the parse.
func TestTheRefusalNoteCannotBeClosedByAMemberName(t *testing.T) {
	for _, src := range []string{
		"~ $schema: {\"a} #\": int}\n---\noops",
		"~ $schema: {\"a}#\": int}\n---\noops",
		"~ $schema: {\"} #\": int}\n---\noops",
		"~ $schema: {\"a} , \": int}\n---\noops",
		"~ $schema: {\"b} #\": int}\n---\noops",
	} {
		doc, perr := io.Parse(src)
		if perr == nil || doc == nil {
			t.Errorf("%q did not fault as expected", src)
			continue
		}
		note := doc.String()
		if strings.Contains(strings.TrimPrefix(note, "{"), "}") {
			t.Errorf("%q: note %q carries a `}` that closes its brace", src, note)
		}
		if _, err := io.Parse(note); err == nil {
			t.Errorf("%q: note %q PARSES — the member name closed the refusal", src, note)
		}
	}
}

// MarshalText is Text with the defaults, so it refuses on the same terms.
func TestMarshalTextRefusesAFaultedDocument(t *testing.T) {
	if b, err := mustParseFaulted(t, faultedDoc).MarshalText(); err == nil {
		t.Errorf("MarshalText returned %q; want a refusal", b)
	}
	clean, _ := io.Parse("name: string\n---\n~ Alice")
	b, err := clean.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText on a clean document: %v", err)
	}
	if got, want := string(b), "name: string\n---\n~ Alice"; got != want {
		t.Errorf("MarshalText = %q, want %q", got, want)
	}
}

// A fault inside a collection must name the record that caused it. Validation
// always did; parse recovery built its error node with a bare code, so a syntax
// fault could only be reported against the document — and the refusal, which
// reports whatever the fault carried, inherited that.
func TestAFaultInsideACollectionNamesItsRecord(t *testing.T) {
	for _, tc := range []struct {
		src  string
		code io.Code
	}{
		{"~ 1\n~ }", io.UnexpectedToken},
		{"~ 1\n~ [1", io.ExpectedClosingBracket},
		{"n: int\n---\n~ 1\n~ }", io.UnexpectedToken},
	} {
		doc, err := io.Parse(tc.src)
		if err == nil || doc == nil {
			t.Errorf("%q did not fault as expected", tc.src)
			continue
		}
		list := doc.Errors()
		if len(list) == 0 {
			t.Errorf("%q: no errors recorded", tc.src)
			continue
		}
		e := list[0]
		if e.Code != tc.code {
			t.Errorf("%q: Code = %q, want %q", tc.src, e.Code, tc.code)
		}
		if e.RecordIndex != 1 {
			t.Errorf("%q: RecordIndex = %d, want 1", tc.src, e.RecordIndex)
		}
		if e.Path != "$[1]" {
			t.Errorf("%q: Path = %q, want $[1]", tc.src, e.Path)
		}
		if e.Line == 0 || e.Col == 0 {
			t.Errorf("%q: position is %d:%d, want a real one", tc.src, e.Line, e.Col)
		}
		// The refusal reports what the fault carried, so it inherits the fix.
		if _, terr := doc.Text(nil); terr == nil {
			t.Errorf("%q: Text did not refuse", tc.src)
		} else if !strings.Contains(terr.Error(), "$[1]") {
			t.Errorf("%q: refusal %q does not name the record", tc.src, terr)
		}
	}
}

func mustParseFaulted(t *testing.T, src string) *io.Document {
	t.Helper()
	doc, err := io.Parse(src)
	if err == nil {
		t.Fatalf("%q parsed clean; it is not a fault fixture", src)
	}
	if doc == nil {
		t.Fatalf("%q returned no document", src)
	}
	return doc
}

// The other half of the rule, and the reason the refusal is safe: the
// projections still carry the failure, so a playground or a report can show it.
func TestProjectionsStillDescribeTheFailure(t *testing.T) {
	doc, _ := io.Parse(faultedDoc)

	if n := len(doc.Errors()); n != 1 {
		t.Errorf("Errors() has %d entries, want 1", n)
	}
	v, ok := doc.Value().([]any)
	if !ok {
		t.Fatalf("Value() is %T, want []any", doc.Value())
	}
	if len(v) != 3 {
		t.Errorf("Value() has %d records, want 3 — the failed one is still there", len(v))
	}
	j, err := doc.JSON(nil)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.Contains(string(j), "Alice") || !strings.Contains(string(j), "Carol") {
		t.Errorf("JSON = %s; want the surviving records", j)
	}
}

// A clean document is untouched by any of this: Text agrees with String, and
// both still round-trip.
func TestACleanDocumentIsUnaffected(t *testing.T) {
	const src = "name: string, age: int\n---\n~ Alice, 30\n~ Carol, 40"
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	out, err := doc.Text(nil)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if out != src {
		t.Errorf("Text() = %q, want %q", out, src)
	}
	if s := doc.String(); s != out {
		t.Errorf("String() = %q, Text() = %q; they must agree when there is no fault", s, out)
	}
	// SkipErrors changes nothing when there is nothing to skip.
	if skipped, err := doc.Text(&io.TextOptions{SkipErrors: true}); err != nil || skipped != out {
		t.Errorf("Text(SkipErrors) = %q, %v; want %q, nil", skipped, err, out)
	}
}
