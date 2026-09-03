package internetobject_test

import (
	"strings"
	"testing"
	"time"

	io "github.com/maniartech/InternetObject-go"
)

// A temporal value is a plain time.Time. The three literals — d"…", t"…",
// dt"…" — are SPELLINGS of one value, exactly as open, raw and quoted are
// three spellings of one string: the value model keeps the value and the
// writer re-picks a spelling on output.
//
// Which spelling comes back is decided by the SCHEMA when the member declares
// a temporal type — the normal case in a schema-first format, and it loses
// nothing. Only an undeclared temporal is spelled from its instant.

func TestTemporalDecodesToNativeTime(t *testing.T) {
	doc, err := io.Parse(`---
~ d"2024-03-20", t"14:30:45.123", dt"2024-03-20T14:30:45.123Z"`)
	if err != nil {
		t.Fatal(err)
	}
	rec := doc.Value().([]any)[0].(*io.Object)

	want := []time.Time{
		time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC),
		io.TimeAnchor.Add(14*time.Hour + 30*time.Minute + 45*time.Second + 123*time.Millisecond),
		time.Date(2024, 3, 20, 14, 30, 45, 123e6, time.UTC),
	}
	for i, m := range rec.Members {
		got, ok := m.Value.(time.Time) // a native time.Time, no wrapper
		if !ok {
			t.Fatalf("member %d is %T, want time.Time", i, m.Value)
		}
		if !got.Equal(want[i]) {
			t.Errorf("member %d = %s, want %s", i, got, want[i])
		}
	}
}

// A declared temporal type fixes the spelling, so a midnight datetime stays a
// datetime and a 1900-01-01 date stays a date — the two shapes an instant
// alone cannot tell apart.
func TestDeclaredTypeFixesTheSpelling(t *testing.T) {
	for _, tc := range []struct{ schema, value, want string }{
		{"x: datetime", `dt"2024-03-20T00:00:00.000Z"`, `dt"2024-03-20T00:00:00.000Z"`},
		{"x: date", `d"1900-01-01"`, `d"1900-01-01"`},
		{"x: date", `d"2024-03-20"`, `d"2024-03-20"`},
		{"x: time", `t"00:00:00"`, `t"00:00:00"`},
	} {
		src := tc.schema + "\n---\n" + tc.value
		doc, err := io.Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := doc.String(); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s round-tripped as %q, want it to end with %s", src, got, tc.want)
		}
	}
}

// The declared type truncates ON WRITE: a `date` member drops the clock, a
// `time` member drops the date. Probed against io-js2 2026-09-03 — every row
// below is the reference's own output, except where marked.
func TestDeclaredKindTruncatesOnWrite(t *testing.T) {
	for _, tc := range []struct{ schema, value, want string }{
		// a date member keeps the date part only
		{"date", `dt"2024-03-20T14:30:45.123Z"`, `d"2024-03-20"`},
		{"date", `t"12:00:00.000"`, `d"1900-01-01"`}, // the anchor date surfaces
		// a time member keeps the clock only
		{"time", `d"2024-03-20"`, `t"00:00:00"`},
		// DIVERGES from io-js2, which writes t"23:59:59" and loses the .999.
		// The spec's canonical Time form is HH:mm:ss.SSS (date-and-time.md
		// format table), so dropping a non-zero millisecond field is data
		// loss. See FINDINGS — no corpus case covers a non-zero ms here.
		{"time", `dt"2024-03-20T23:59:59.999Z"`, `t"23:59:59.999"`},
		// a datetime member widens rather than truncates
		{"datetime", `d"2024-03-20"`, `dt"2024-03-20T00:00:00.000Z"`},
		{"datetime", `t"12:00:00.000"`, `dt"1900-01-01T12:00:00.000Z"`},
	} {
		src := "d: " + tc.schema + "\n---\n" + tc.value
		doc, err := io.Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := doc.String(); !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s\n  got  %q\n  want it to end with %s", src, got, tc.want)
		}
	}
}

// …but the VALUE is not truncated. `validation/temporal-depth.io` pins both
// directions, so the annotation decides the spelling and never the instant.
// A truncating validator fails those two corpus cases.
func TestDeclaredKindDoesNotTruncateTheValue(t *testing.T) {
	for _, tc := range []struct {
		schema, value string
		want          time.Time
	}{
		{"time", `d"2024-03-20"`, time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)},
		{"date", `t"12:00:00.000"`, time.Date(1900, 1, 1, 12, 0, 0, 0, time.UTC)},
	} {
		doc, err := io.Parse("d: " + tc.schema + "\n---\n" + tc.value)
		if err != nil {
			t.Fatal(err)
		}
		got := doc.Records()[0].(*io.Object).Members[0].Value.(time.Time)
		if !got.Equal(tc.want) {
			t.Errorf("%s under %s = %s, want the instant kept as %s",
				tc.value, tc.schema, got.UTC(), tc.want)
		}
	}
}

// min/max compare the WHOLE instant, not the declared part — so a datetime
// under a `time` member is compared against the bound's 1900 anchor and fails
// for a reason that has nothing to do with its clock. Reference-confirmed
// 2026-09-03 and matched deliberately; the case against it is in FINDINGS.
func TestTemporalBoundsCompareWholeInstant(t *testing.T) {
	for _, tc := range []struct {
		src     string
		wantErr bool
	}{
		{`d: {date, max: d"2024-03-20"}` + "\n---\n" + `dt"2024-03-20T14:30:45.123Z"`, true},
		{`d: {date, min: d"2024-03-20"}` + "\n---\n" + `dt"2024-03-20T14:30:45.123Z"`, false},
		{`d: {time, max: t"15:00:00"}` + "\n---\n" + `dt"2024-03-20T14:30:00.000Z"`, true},
		{`d: {time, max: t"14:00:00"}` + "\n---\n" + `t"14:30:00"`, true},
		{`d: {time, min: t"14:00:00"}` + "\n---\n" + `t"14:30:00"`, false},
	} {
		_, err := io.Parse(tc.src)
		if got := err != nil; got != tc.wantErr {
			t.Errorf("%q: error=%v, want error=%v (%v)", tc.src, got, tc.wantErr, err)
		}
	}
}

// A time.Time struct field binds straight from the wire, and the `,date` /
// `,time` tag says which literal it writes back as.
func TestTimeTimeFieldBinding(t *testing.T) {
	type row struct {
		Day time.Time `io:"day,date"`
		At  time.Time `io:"at"`
		Tod time.Time `io:"tod,time"`
	}
	in := row{
		Day: time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC),
		At:  time.Date(2024, 3, 20, 14, 30, 45, 123e6, time.UTC),
		Tod: io.TimeAnchor.Add(9 * time.Hour),
	}
	text, err := io.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `d"2024-03-20"`) ||
		!strings.Contains(text, `dt"2024-03-20T14:30:45.123Z"`) ||
		!strings.Contains(text, `t"09:00:00"`) {
		t.Fatalf("tags did not choose the spellings: %q", text)
	}

	var back row
	if err := io.Unmarshal(text, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Day.Equal(in.Day) || !back.At.Equal(in.At) || !back.Tod.Equal(in.Tod) {
		t.Errorf("round trip changed the instants: %+v -> %+v", in, back)
	}
}

// An UNDECLARED temporal is spelled from what its instant evidences. This is
// the one place the spelling is normalized, and it is the same normalization
// the writer applies to a string's open/raw/quoted form.
func TestUndeclaredTemporalIsInferred(t *testing.T) {
	doc, err := io.Parse(`---` + "\n" + `~ dt"2024-03-20T00:00:00.000Z"`)
	if err != nil {
		t.Fatal(err)
	}
	got := doc.String()
	if !strings.HasSuffix(got, `d"2024-03-20"`) {
		t.Errorf("undeclared midnight datetime = %q, want it spelled as a date", got)
	}
	// The INSTANT is preserved either way — that is what the format compares.
	back, err := io.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	a := doc.Value().([]any)[0].(*io.Object).Members[0].Value.(time.Time)
	b := back.Value().([]any)[0].(*io.Object).Members[0].Value.(time.Time)
	if !a.Equal(b) {
		t.Errorf("the instant changed: %s -> %s", a, b)
	}
}
