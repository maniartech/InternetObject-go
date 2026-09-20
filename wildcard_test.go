package internetobject_test

import (
	"slices"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// A bare `*` (or `*: T`) is the open-schema WILDCARD: it says "and any other
// members". A QUOTED `"*"` is an ordinary member that happens to be named `*`.
//
// The wildcard is openness, not a member, so it lives on the compiled schema's
// open marker ALONE and never enters Names/Defs — the format's decision D1, option A.
// That is what leaves the name `*` free for a real member. When the wildcard was
// also stored under Defs["*"], the two were indistinguishable by name, and four
// sites that meant "skip the wildcard" compared the name and dropped the member:
// the header writer, the nested-schema writer, the record writer, and the public
// MemberNames. The member parsed and validated, then vanished on the way out —
// doc.String() wrote a header the reader rejected with unknown-member.
//
// The reference implementation pins the same rule, as its decision D1.

func TestQuotedStarIsAnOrdinaryMember(t *testing.T) {
	const src = "~ $schema: {name: string, \"*\": int}\n---\n~ John, \"*\": 7"

	doc, err := io.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	s, err := doc.SchemaOf("schema")
	if err != nil {
		t.Fatalf("SchemaOf: %v", err)
	}
	if got := s.MemberNames(); !slices.Equal(got, []string{"name", "*"}) {
		t.Errorf("MemberNames() = %q, want [name *]", got)
	}
	// A NAME does not open the schema.
	if s.Open() {
		t.Errorf("a schema whose only `*` is quoted reports Open() = true")
	}

	// The member must be written QUOTED. Unquoted it would read back as the
	// wildcard — a different schema that happens to project the same data, which
	// is why the round-trip check below compares schema shape and not just values.
	assertRoundTrips(t, doc, src, "name: string, \"*\": int\n---\n~ John, 7")
}

func TestQuotedStarSurvivesInANestedSchema(t *testing.T) {
	const src = "~ $schema: {o: {a: int, \"*\": int}}\n---\n~ o: {a: 1, \"*\": 2}"
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	assertRoundTrips(t, doc, src, "o: {a: int, \"*\": int}\n---\n~ {{1, 2}}")
}

// The one input where a quoted `"*"` member and the wildcard coexist: the only
// shape in which Defs holds a `*` entry WHILE the schema is open.
func TestQuotedStarBesideTheWildcard(t *testing.T) {
	const src = "~ $schema: {a: int, \"*\": int, *}\n---\n~ 1, 2, other: 3"
	doc, err := io.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := doc.SchemaOf("schema")
	if err != nil {
		t.Fatalf("SchemaOf: %v", err)
	}
	if got := s.MemberNames(); !slices.Equal(got, []string{"a", "*"}) {
		t.Errorf("MemberNames() = %q, want [a *]", got)
	}
	if !s.Open() {
		t.Errorf("Open() = false, want true — the bare `*` opens it")
	}
	assertRoundTrips(t, doc, src, "")
}

// The ambiguous state is unreachable because declaring both spellings collides.
// This is what makes "is the `*` entry the wildcard?" a question nobody has to ask.
func TestWildcardAndQuotedStarCannotBothBeDeclared(t *testing.T) {
	for _, def := range []string{
		"~ $schema: {\"*\": int, *: int}\n---\n~ 1",
		"~ $schema: {*: int, \"*\": int}\n---\n~ 1",
	} {
		if _, err := io.Parse(def); err == nil {
			t.Errorf("%q compiled; want duplicate-member", def)
		}
	}
}

// The wildcard is openness: it is never a member, and is written from the open
// marker rather than as a declaration of its own.
func TestWildcardFormsStillRoundTrip(t *testing.T) {
	for _, tc := range []struct{ src, wantText string }{
		{"~ $schema: {name: string, *}\n---\n~ John, extra: 7", "name: string, *\n---\n~ John, extra: 7"},
		{"~ $schema: {name: string, *: int}\n---\n~ John, extra: 7", "name: string, *:int\n---\n~ John, extra: 7"},
	} {
		doc, err := io.Parse(tc.src)
		if err != nil {
			t.Errorf("parse %q: %v", tc.src, err)
			continue
		}
		s, err := doc.SchemaOf("schema")
		if err != nil {
			t.Errorf("SchemaOf %q: %v", tc.src, err)
			continue
		}
		if got := s.MemberNames(); !slices.Equal(got, []string{"name"}) {
			t.Errorf("%q: MemberNames() = %q, want [name] — the wildcard is not a member", tc.src, got)
		}
		if !s.Open() {
			t.Errorf("%q: Open() = false", tc.src)
		}
		assertRoundTrips(t, doc, tc.src, tc.wantText)
	}
}

// A data key spelled `*` is an ordinary name, whatever the schema says. These
// four rows differ only in how the first member's key is spelled, or in whether
// the schema is open; all four bind the record to the declared member `a`.
// Three of them reported a fault while a `*` key was treated as special.
func TestStarIsAnOrdinaryDataKey(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"~ $schema: {a: {*}}\n---\n~ q: 5", `[{"a":{"q":5}}]`},
		{"~ $schema: {a: {*}}\n---\n~ *: 5", `[{"a":{"*":5}}]`},
		{"~ $schema: {a: {*}, *}\n---\n~ *: 5", `[{"a":{"*":5}}]`},
		{"~ $schema: {a: {*}, *: int}\n---\n~ *: 5", `[{"a":{"*":5}}]`},
		// Under an open schema a `*` key is an ordinary EXTRA and must keep its
		// arrival position, after the declared members. Hoisting it into schema
		// order produced a record the writer could only spell as unparseable
		// `"*": 0, 0` — found by the byte fuzzer, and the reason for a branch
		// ADR 0012 deleted.
		{"~ $schema: {name: string, *: int}\n---\n~ John, \"*\": 7", `[{"name":"John","*":7}]`},
	} {
		doc, err := io.Parse(tc.src)
		if err != nil {
			t.Errorf("parse %q: %v", tc.src, err)
			continue
		}
		got, err := doc.JSON(nil)
		if err != nil {
			t.Errorf("JSON %q: %v", tc.src, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%q\n  got  %s\n  want %s", tc.src, got, tc.want)
		}
	}
}

// A declared `"*"` member can be filled positionally like any other.
func TestQuotedStarTakesAPositionalValue(t *testing.T) {
	doc, err := io.Parse("~ $schema: {\"*\": int}\n---\n~ 9")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := doc.JSON(nil)
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if string(got) != `[{"*":9}]` {
		t.Errorf("got %s, want [{\"*\":9}]", got)
	}
}

// assertRoundTrips holds the written document to both properties that matter:
// it means the same thing, AND it describes the same schema. Comparing values
// alone cannot tell `"*": int` (a member) from `*: int` (the wildcard) — they
// project identical data — so the schema shape is compared too. Pass wantText to
// pin the exact bytes where the spelling is the point; "" skips that.
func assertRoundTrips(t *testing.T, doc *io.Document, src, wantText string) {
	t.Helper()

	want, err := doc.JSON(nil)
	if err != nil {
		t.Fatalf("JSON of %q: %v", src, err)
	}

	out := doc.String()
	if wantText != "" && out != wantText {
		t.Errorf("%q\n  wrote %q\n  want  %q", src, out, wantText)
	}

	back, err := io.Parse(out)
	if err != nil {
		t.Fatalf("%q wrote %q, which does not parse: %v", src, out, err)
	}
	got, err := back.JSON(nil)
	if err != nil {
		t.Fatalf("JSON of re-parsed %q: %v", out, err)
	}
	if string(got) != string(want) {
		t.Errorf("%q\n  wrote    %q\n  means    %s\n  original %s", src, out, got, want)
	}

	// The schema must survive too, or a member could come back as the wildcard.
	before, err1 := doc.SchemaOf("schema")
	after, err2 := back.SchemaOf("schema")
	if err1 != nil || err2 != nil {
		return // not every case names its schema
	}
	if !slices.Equal(after.MemberNames(), before.MemberNames()) || after.Open() != before.Open() {
		t.Errorf("%q wrote %q: schema changed\n  before %q open=%v\n  after  %q open=%v",
			src, out, before.MemberNames(), before.Open(), after.MemberNames(), after.Open())
	}
}
