package internetobject_test

import (
	"strings"
	"testing"
	"time"

	io "github.com/maniartech/InternetObject-go"
)

// A temporal value KEEPS the kind it carries. The reference infers the kind
// from the instant only because a JavaScript Date has none to keep; our
// Temporal does, and inferring loses it silently — a midnight datetime came
// back as a date, a 1900-01-01 date as a time-of-day (io-test-cases
// PORTING-NOTES rules 15 and 18).
//
// The corpus cannot catch this: it has no schema-less midnight-datetime or
// 1900-01-01-date case, which is exactly why the rule was written down
// upstream. These tests are the gate.
func TestTemporalKeepsItsKind(t *testing.T) {
	type holder struct {
		X io.Temporal `io:"x"`
	}
	type anyHolder struct {
		X any `io:"x"`
	}

	cases := []struct {
		name string
		v    io.Temporal
		want string
	}{
		{"midnight datetime", io.Temporal{
			Time: time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC), Kind: io.KindDateTime,
		}, `dt"2024-03-20T00:00:00.000Z"`},
		{"1900-01-01 date", io.Temporal{
			Time: time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), Kind: io.KindDate,
		}, `d"1900-01-01"`},
		{"time of day", io.Temporal{
			Time: time.Date(1900, 1, 1, 1, 2, 3, 0, time.UTC), Kind: io.KindTime,
		}, `t"01:02:03"`},
		{"ordinary date", io.Temporal{
			Time: time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC), Kind: io.KindDate,
		}, `d"2024-03-20"`},
	}

	for _, tc := range cases {
		out, err := io.Marshal(holder{X: tc.v})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want it to contain %s", tc.name, out, tc.want)
		}
		// …and it must survive the round trip with its kind intact.
		var back holder
		if err := io.Unmarshal(out, &back); err != nil {
			t.Fatalf("%s: re-read: %v", tc.name, err)
		}
		if back.X.Kind != tc.v.Kind || !back.X.Time.Equal(tc.v.Time) {
			t.Errorf("%s: round trip changed the value: %+v -> %+v", tc.name, tc.v, back.X)
		}

		// Rule 18: the same holds under a NON-temporal declared type, where
		// the writer must read the kind off the value rather than the schema.
		outAny, err := io.Marshal(anyHolder{X: tc.v})
		if err != nil {
			t.Fatalf("%s under any: %v", tc.name, err)
		}
		if !strings.Contains(outAny, tc.want) {
			t.Errorf("%s under any: got %q, want it to contain %s", tc.name, outAny, tc.want)
		}
	}
}
