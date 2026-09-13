package internetobject_test

import (
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// A member name that needs quoting (`00`, `a,b`) cannot carry the `?`/`*`
// markers, so the writer spells its definition in the LONG form:
// `"00": {array, of: int, minLen:2, optional: T}`. That form used to carry its
// own copy of the short form's rule, and the copy had drifted twice
// (2026-09-13). These pin both, by what the schema DOES after a round trip —
// the spelling alone cannot catch a constraint that was silently dropped.

// roundTripSchema parses a schema-only document, writes it, and returns the
// written header, failing the test if the output does not re-parse.
func roundTripSchema(t *testing.T, header string) string {
	t.Helper()
	doc, err := io.Parse(header + "\n---")
	if err != nil {
		t.Fatalf("parse %q: %v", header, err)
	}
	out := doc.String()
	if _, err := io.Parse(out); err != nil {
		t.Fatalf("the writer produced a schema its own parser rejects:\n in  %q\n out %q\n %v", header, out, err)
	}
	return strings.TrimSuffix(out, "\n---")
}

// Found by FuzzParse: any anyOf on a quoted-name member was written
// `anyOf:null`, which does not re-parse.
func TestLongFormKeepsAnyOf(t *testing.T) {
	for _, header := range []string{
		"00*: {any, anyOf: []}",
		"00*: {any, anyOf: [int, string]}",
		"00?: {any, anyOf: [int, {string, minLen: 2}]}",
	} {
		out := roundTripSchema(t, header)
		if strings.Contains(out, "anyOf:null") {
			t.Errorf("%q written as %q: the anyOf alternatives were lost", header, out)
		}
	}
}

// The silent one: an array's constraints were dropped entirely, and the result
// re-parsed cleanly — so the schema quietly stopped rejecting what it rejected.
func TestLongFormKeepsArrayConstraints(t *testing.T) {
	out := roundTripSchema(t, "00?: {array, of: int, minLen: 2}")

	// Behaviour, not spelling: after the round trip a one-element array must
	// still violate minLen, exactly as it did before.
	for _, schema := range []string{"00?: {array, of: int, minLen: 2}", out} {
		if _, err := io.Parse(schema + "\n---\n~ [1]"); err == nil {
			t.Errorf("schema %q accepted [1], but minLen is 2", schema)
		}
		if _, err := io.Parse(schema + "\n---\n~ [1, 2]"); err != nil {
			t.Errorf("schema %q rejected [1, 2]: %v", schema, err)
		}
	}
}
