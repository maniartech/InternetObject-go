# Findings — spec gaps and reference divergences found by the Go port

Per `io-test-cases/PORT-START-HERE.md` §1, a divergence between the specification and the
reference implementation is a **defect to close upstream**, never something to encode silently.
This file is the running list. Each entry states what the spec says, what io-js2 does (probed
2026-09-02 against master), and what this port implements meanwhile.

Status legend: **open** = not yet reported/resolved upstream.

## 1. Compact time with milliseconds — open

- **Spec** (`the-structure/values/date-and-time.md`, format table): `HHmmss.SSS` is a valid
  compact time form.
- **io-js2**: `t"143045.123"` → `invalid-time`. (`t"143045"` and `t"1430"` are accepted.)
- **This port**: follows the spec — accepted.

## 2. Timezone offset forms — open

- **Spec**: prose says "both `±HH:mm` and `±HHMM` are accepted on input"; the EBNF
  (`timeZone = ("+"|"-") hourPart [minutePart]`) additionally admits bare `±HH`.
- **io-js2**: accepts only `Z` and `±HH:mm`; `dt"…+0530"` and `dt"…-08"` → `invalid-datetime`.
- **This port**: follows the spec — all three offset spellings accepted, range −12:00…+14:00.

## 3. Compact date+time datetime silently drops the time — open, data loss

- **Spec**: `dateContent ["T" timeContent] [timeZone]`, with compact forms valid on both sides.
- **io-js2**: `dt"20240320T143045.123Z"` parses **successfully** but decodes to
  `2024-03-20T00:00:00.000Z` — the time part is silently discarded. Also
  `dt"2024-03-20T1430"` (separated date, compact time) → `invalid-datetime`.
- **This port**: follows the spec — the time part is parsed and kept.
- This is the worst kind (silent data loss on a value the parser accepted); it belongs in
  `io-test-cases/PORTING-NOTES.md` once fixed upstream.

## 4. The annotation claim has an undocumented length cap of 4 — open, spec gap

- **Spec** (`open-strings.md`): "the run before a quote is read as an annotation name" — no
  length rule, which taken literally makes `costa'…` an `unknown-annotation`.
- **io-js2**: a word directly abutting a quote claims an annotation only when it is **≤ 4
  characters** (`abcd'` → `unknown-annotation`; `abcde'` → open string + fresh quote token).
  A word that classifies as a number never claims (`5'9` → NUMBER, then string). A mid-run word
  never claims (`a b"x"` → open string `a b`, then a regular string — even though `b` alone is
  a valid annotation).
- **This port**: matches io-js2 (constant `maxAnnotationLen = 4`), because the corpus derives
  from it. The rule should be written into the spec either way.

## 5. `invalid-section-name` is missing from the CONFORMANCE.md code registry — open, doc gap

- io-js2 emits `invalid-section-name` (e.g. `--- my.section`), and the code follows the frozen
  grammar, but `io-test-cases/CONFORMANCE.md` §5.1 does not list it. The registry claims to be
  exhaustive ("no suite may invent a code that is not in one of these enums").

## 6. Whitespace: prose mentions U+00A0, the EBNF does not — open, doc gap

- `whitespaces.md` prose says the format recognizes "the non-breaking space (U+00A0)" as
  whitespace; the EBNF and the table on the same page omit it. This port follows the EBNF
  (U+00A0 is not whitespace).

## Go-specific notes (not upstream defects)

- **Lone UTF-16 surrogates.** JS strings can hold a lone surrogate from `\uD83D`; Go strings
  cannot. This port decodes a lone surrogate escape to U+FFFD. If a corpus case ever asserts a
  lone-surrogate value, it is asserting a JavaScript accident and needs an upstream decision.
