package main

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// PROTOTYPE — hand-written to stand in for what an INLINED iogen would emit.
// Not generated, not shipped, deliberately confined to a _test.go file. It
// exists to measure the ceiling and to make the cost of reaching it concrete.
//
// Two things it does that the delegating version cannot:
//  1. the schema header is a CONSTANT, not re-rendered on every call;
//  2. the record is written straight into one buffer, with the constraints
//     compiled into `if`s — no value tree, no reflection.
const personHeaderConst = "name: {string, minLen:2, maxLen:50}, age: {int, min:0, max:130}, " +
	"email: string, active: bool, score: number, tags: [string]\n---\n"

var (
	errName = errors.New("mismatched-minLen at $.name")
	errAge  = errors.New("mismatched-min at $.age")
)

func (p *Person) marshalInlined() (string, error) {
	// Constraints are compile-time constants, so they compile to comparisons.
	if n := utf8.RuneCountInString(p.name); n < 2 || n > 50 {
		return "", errName
	}
	if p.age < 0 || p.age > 130 {
		return "", errAge
	}

	buf := make([]byte, 0, len(personHeaderConst)+64)
	buf = append(buf, personHeaderConst...)
	buf = appendOpen(buf, p.name)
	buf = append(buf, ", "...)
	buf = strconv.AppendInt(buf, int64(p.age), 10)
	buf = append(buf, ", "...)
	buf = appendOpen(buf, p.email)
	buf = append(buf, ", "...)
	if p.active {
		buf = append(buf, 'T')
	} else {
		buf = append(buf, 'F')
	}
	buf = append(buf, ", "...)
	buf = strconv.AppendFloat(buf, p.score, 'g', -1, 64)
	buf = append(buf, ", ["...)
	for i, tag := range p.tags {
		if i > 0 {
			buf = append(buf, ", "...)
		}
		buf = appendOpen(buf, tag)
	}
	buf = append(buf, ']')
	return string(buf), nil
}

// The duplicated decision — and the reason it must not be duplicated.
//
// The FIRST version of this quoted "alice@example.com", because it treated @
// as always-significant. It is not: @ introduces a variable reference only at
// the START of a value, and # only opens a comment there too. ONE realistic
// email address was enough to prove the copy wrong. This version is closer,
// and still not proven — "closer" is not a standard. The engine's own speller
// is the standard, and generated code has to call it rather than re-derive it.
func appendOpen(dst []byte, s string) []byte {
	mustQuote := s == "" || s != strings.TrimSpace(s)
	if !mustQuote {
		for _, r := range s {
			switch r {
			case ',', ':', '{', '}', '[', ']', '"', '\'', '\n', '\r', '\t':
				mustQuote = true
			}
		}
	}
	if !mustQuote {
		switch s[0] {
		case '@', '#', '~':
			mustQuote = true
		}
	}
	if mustQuote {
		return strconv.AppendQuote(dst, s)
	}
	return append(dst, s...)
}

// The prototype must agree with the engine, or its speed means nothing.
func TestInlinedPrototypeAgrees(t *testing.T) {
	want, err := benchGen.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := benchGen.marshalInlined()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("prototype differs from the engine:\n inlined %q\n engine  %q", got, want)
	}
	var back Person
	if err := back.Unmarshal(got); err != nil {
		t.Fatalf("inlined output does not parse: %v", err)
	}
}

func BenchmarkInlinedMarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := benchGen.marshalInlined(); err != nil {
			b.Fatal(err)
		}
	}
}
