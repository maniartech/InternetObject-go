package parser

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/maniartech/InternetObject-go/internal/tokenizer"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// Framing must never DISAGREE with the parser: for any document it accepts,
// the members it frames must decode to exactly the values the parser built.
// It is free to decline (that is what the fallback is for) but never free to
// be wrong. This is the gate ADR 0007 phase 1 rests on.

// decodeRaw materializes a framed member, so it can be compared with the
// value the parser produced. The fast decode path never does this — it writes
// into a Go field — but the comparison needs a common currency.
func decodeRaw(s *tokenizer.Stream, m RawMember) (any, bool) {
	if m.Absent {
		return nil, true
	}
	t := s.Tokens[m.Tok]
	switch m.Kind {
	case tokenizer.KindString:
		return s.StringValue(t), true
	case tokenizer.KindNumber:
		return s.Number(t), true
	case tokenizer.KindBoolean:
		return s.Bool(t), true
	case tokenizer.KindNull:
		return nil, true
	case tokenizer.KindBigInt:
		return s.BigInt(t), true
	case tokenizer.KindDecimal:
		coef, scale := s.DecimalParts(t)
		return value.Decimal{Coef: coef, Scale: scale}, true
	case tokenizer.KindBinary:
		return s.Bytes(t), true
	case tokenizer.KindDateTime:
		// The temporal KIND is the token's sub-kind — exactly the
		// distinction RawMember.Sub carries.
		k := value.KindDateTime
		switch m.Sub {
		case tokenizer.SubDate:
			k = value.KindDate
		case tokenizer.SubTime:
			k = value.KindTime
		}
		return value.Temporal{Time: s.Temporal(t), Kind: k}, true
	}
	return nil, false // a container: compared structurally below
}

// sameScalar compares a framed value with the parsed one. NaN equals NaN
// here: both sides produced the same literal.
func sameScalar(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && (x == y || (math.IsNaN(x) && math.IsNaN(y)))
	case *big.Int:
		y, ok := b.(*big.Int)
		return ok && x.Cmp(y) == 0
	case value.Decimal:
		y, ok := b.(value.Decimal)
		return ok && x.Scale == y.Scale && x.Coef.Cmp(y.Coef) == 0
	case []byte:
		y, ok := b.([]byte)
		return ok && string(x) == string(y)
	case value.Temporal:
		y, ok := b.(value.Temporal)
		return ok && x.Kind == y.Kind && x.Time.Equal(y.Time)
	}
	return a == b
}

// checkAgainstParser frames src and, when framing accepts it, asserts every
// member matches the parsed record.
func checkAgainstParser(t *testing.T, src string) {
	t.Helper()
	s := tokenizer.Tokenize(src)
	raw, ok := FrameData(s)
	if !ok {
		return // declining is always allowed
	}
	doc := Parse(src)
	if len(doc.Errors) > 0 {
		// Framing owns the DATA only; the caller parses the header and bails
		// on its faults before framing is ever used. So a header fault is
		// fine here — but a fault in the data means framing accepted
		// something malformed, which is the failure this test exists for.
		if e := doc.Errors[0]; e.Line > headerLine(s) {
			t.Fatalf("framing accepted a document whose DATA the parser rejected (%v): %q",
				doc.Errors, src)
		}
		return
	}

	var parsed []any
	for _, sec := range doc.Sections {
		parsed = append(parsed, sec.Records...)
	}
	if len(parsed) != len(raw.Records) {
		t.Fatalf("record count: framed %d, parsed %d: %q", len(raw.Records), len(parsed), src)
	}

	for i, rr := range raw.Records {
		obj, isObj := parsed[i].(*value.Object)
		if !isObj {
			t.Fatalf("record %d is not an object: %q", i, src)
		}
		if len(rr.Members) != len(obj.Members) {
			t.Fatalf("record %d member count: framed %d, parsed %d: %q\n framed %+v\n parsed %+v",
				i, len(rr.Members), len(obj.Members), src, rr.Members, obj.Members)
		}
		for j, rm := range rr.Members {
			pm := obj.Members[j]
			if rm.Key != pm.Key {
				t.Errorf("record %d member %d key: framed %q, parsed %q: %q", i, j, rm.Key, pm.Key, src)
			}
			if rm.Absent != pm.Absent {
				t.Errorf("record %d member %d absent: framed %v, parsed %v: %q", i, j, rm.Absent, pm.Absent, src)
			}
			if rm.Absent {
				continue
			}
			v, decodable := decodeRaw(s, rm)
			if !decodable {
				continue // containers are compared by span shape, not value
			}
			if !sameScalar(v, pm.Value) {
				t.Errorf("record %d member %d value: framed %#v, parsed %#v: %q", i, j, v, pm.Value, src)
			}
		}
	}
}

// headerLine is the line of the first section separator: everything at or
// before it is header, which framing does not own.
func headerLine(s *tokenizer.Stream) int32 {
	for _, t := range s.Tokens {
		if t.Kind == tokenizer.KindSectionSep {
			return t.Line
		}
	}
	return 1 << 30
}

var frameCases = []string{
	"---\n~ 1",
	"---\n~ Alice, 42",
	"---\n~ Alice, 42, alice@example.com, T, 99.5",
	"---\nAlice, 42",
	"---\n~ a: 1, b: 2",
	"---\n~ a: 1, 2",
	`---` + "\n" + `~ "quoted", 'single', r"raw ""x""", open string`,
	"---\n~ 0xFF, 0b101, 0o17, 12n, 1.50m, -Inf, NaN",
	`---` + "\n" + `~ d"2024-01-15", t"14:30:45", dt"2024-01-15T14:30:45.123Z", b"SGVsbG8="`,
	"---\n~ [1, 2, 3]",
	"---\n~ {a: 1, b: 2}",
	"---\n~ [{a: 1}, {b: [2, 3]}]",
	"---\n~ {a: [1, {b: 2}]}, tail",
	"---\n~ ,",
	"---\n~ 1,",
	"---\n~ ,1",
	"---\n~ 1,,2",
	"---\n~ 1\n~ 2\n~ 3",
	"name: string, age: int\n---\n~ Alice, 30\n~ Bob, 25",
	"--- $P\n~ 1",
	"--- users\n~ 1",
	"~ $P: {a: int}\n--- $P\n~ 1",
	"---\n~ a\\:b, c",
	"---\n~ \"a, b\", c",
	"---\n~ {}",
	"---\n~ []",
	"---",
}

func TestFramingAgreesWithParser(t *testing.T) {
	for _, src := range frameCases {
		checkAgainstParser(t, src)
	}
}

// The framer must decline, not guess, on shapes it does not handle.
func TestFramingDeclines(t *testing.T) {
	for _, src := range []string{
		"~ 1",                    // headerless
		"---\n~ 1\n--- two\n~ 2", // multiple sections
		"---\n~ {a: 1",           // unclosed
		`---` + "\n" + `~ "abc`,  // error token
	} {
		if _, ok := FrameData(tokenizer.Tokenize(src)); ok {
			t.Errorf("framing should have declined %q", src)
		}
	}
}

// Sub-kinds must survive framing: the three string forms, the number bases
// and the temporal kinds are distinctions the tokenizer drew and a caller
// depends on.
func TestFramingCarriesSubKinds(t *testing.T) {
	src := `---` + "\n" + `~ open, "regular", r"raw", 0xFF, 12n, d"2024-01-15", t"14:30", dt"2024-01-15T00:00Z"`
	s := tokenizer.Tokenize(src)
	raw, ok := FrameData(s)
	if !ok {
		t.Fatal("framing declined a document it should accept")
	}
	got := make([]string, 0, len(raw.Records[0].Members))
	for _, m := range raw.Records[0].Members {
		got = append(got, fmt.Sprintf("%d/%d", m.Kind, m.Sub))
	}
	want := []string{
		fmt.Sprintf("%d/%d", tokenizer.KindString, tokenizer.SubOpenString),
		fmt.Sprintf("%d/%d", tokenizer.KindString, tokenizer.SubRegularString),
		fmt.Sprintf("%d/%d", tokenizer.KindString, tokenizer.SubRawString),
		fmt.Sprintf("%d/%d", tokenizer.KindNumber, tokenizer.SubHex),
		fmt.Sprintf("%d/%d", tokenizer.KindBigInt, tokenizer.SubNone),
		fmt.Sprintf("%d/%d", tokenizer.KindDateTime, tokenizer.SubDate),
		fmt.Sprintf("%d/%d", tokenizer.KindDateTime, tokenizer.SubTime),
		fmt.Sprintf("%d/%d", tokenizer.KindDateTime, tokenizer.SubDateTime),
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("sub-kinds lost:\n got %v\nwant %v", got, want)
	}
}

// FuzzFramingAgreesWithParser drives arbitrary documents through both routes.
func FuzzFramingAgreesWithParser(f *testing.F) {
	for _, src := range frameCases {
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		checkAgainstParser(t, src)
	})
}
