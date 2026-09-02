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

## 7. Open strings process escapes; the spec page says they do not — open

- **Spec** (`open-strings.md`): "No escaping — character escaping is not processed".
- **io-js2** (and the round-trip corpus): open strings process the FULL escape set — `a\:b`
  decodes to `a:b`, `Abc` to `Abc`, marker escapes claim (`\xZZq` is
  `invalid-escape-sequence`) — and the WRITER depends on it (`serializer/quoting.io` emits
  `a\:b`, `say \"hi\"`). The corpus pins the behavior; the spec page contradicts it.
- **This port**: implements the corpus behavior.

## 8. The reference writer drops time-of-day milliseconds — open, data loss

- io-js2's `dateToTimeString` splits the ISO string at `.`, so a `time` value with nonzero
  milliseconds writes without them — silent data loss on rewrite. The round-trip generator
  refused such cases, so the corpus is silent.
- **This port**: writes `.SSS` when nonzero (`t"14:30:45.123"`), zero-suppressed otherwise
  (matching the pinned `t"14:30:45"`).

## 9. Unicode keys are quoted by the writer's ASCII identifier rule — open, spec drift

- **Spec** (`value-formatting.md`): a key is quoted when numeric, keyword, structural-carrying,
  `---`-carrying, or untrimmed — nothing about non-ASCII.
- **io-js2**: the bare-safe test is `^[$A-Za-z_][A-Za-z0-9_. -]*$`, so `клавиша` is quoted
  (pinned by `serializer/quoting.io`) though it reads back fine bare.
- **This port**: matches the corpus/writer rule.

## 10. The reference writer emits bare strings containing control characters — open, data loss

- io-js2's `needsQuoting` tests whitespace with `/\s/`, which does not match `\b`, NUL, ESC or
  other C0 controls, so a string like `"\b00000"` writes bare. On re-read the control byte
  splits the word and the remainder `00000` re-reads as the NUMBER 0 — silent corruption.
- **This port**: any C0 control other than `\n\r\t` forces the regular quoted spelling, and the
  quoted spelling escapes the full C0 range (`\b`, `\f`, `\u00XX`). Found by the byte fuzzer.

## 11. Schema alias cycles crash the reference with a bare stack overflow — open, rule-10 violation

- `~ $a: $b` + `~ $b: $a` (or `~ $a: $a`) throws `RangeError: Maximum call stack size
  exceeded` — an error without a designated code (PORTING-NOTES rule 10 calls this class out).
- **This port**: chases alias chains iteratively and reports `invalid-definition`, the same
  code variable-reference cycles carry.

## 12. Datetimes whose UTC instant is unspellable round-trip into rejection — open, data loss

- The reference accepts `dt"0000-01-01T00:00:00+01:00"` (instant in year −1) and its
  serialization emits the JS extended-year form `-000001-…` — which its own reader throws on
  (`invalid-datetime`). Writer emits what reader rejects.
- **This port**: a datetime whose UTC instant falls outside years 0000–9999 is
  `invalid-datetime` at parse.

## 13. An unbounded bigint exponent is a denial of service — open

- `1e10000000n` materializes ten million digits (seconds and gigabytes; scales linearly);
  beyond V8's BigInt cap the reference throws the uncoded `Maximum BigInt size exceeded`.
- **This port**: the exponent is bounded at 1e6 (a million-digit integer decodes in
  milliseconds); beyond it the claim is broken — `invalid-bigint`. Deliberate divergence,
  needs an upstream decision on the designed bound.

## Suggested corpus cases (gaps the fuzzers exposed; all fixed in this port)

- A malformed literal in a HEADER definition is fatal (`~ A: 0B` → `invalid-number`); no case
  covers headers, so an implementation can defer-and-mask it silently.
- Surplus positional members under an open schema must be validated (`a, *` with `1, @x` →
  `undefined-variable`), not passed through raw.
- A deferred literal error nested under an `any`-typed subtree must surface
  (`x\n---\n{A: 0B}` → `invalid-number`).
- `schema: $Ref` in the long-form object typedef is a reference, same as the short form.
- `--- $$` (a `$`-named schema selector), a `*`-only header, `~ ,` records with trailing
  holes, and strings containing `\r` (which every unescaped spelling newline-normalizes) all
  round-trip through the canonical writer.
- A HEADERLESS stream (no `---` at all) with PRELOADED definitions still validates its
  records against the preloaded default schema (the reference passes definitions to `parse`
  on the legacy route too). No streaming case combines the two, so this port shipped the
  divergence until an example caught it.

## Go-specific notes (not upstream defects)

- **Lone UTF-16 surrogates.** JS strings can hold a lone surrogate from `\uD83D`; Go strings
  cannot. This port decodes a lone surrogate escape to U+FFFD. If a corpus case ever asserts a
  lone-surrogate value, it is asserting a JavaScript accident and needs an upstream decision.
