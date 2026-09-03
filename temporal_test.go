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
