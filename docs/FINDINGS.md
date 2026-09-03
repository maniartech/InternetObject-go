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

## 14. A self-absorbing schema stack-overflows the reference — open, rule-10 violation

- `~ $P: {A: $P}` fed `{$P: 0}` (or `{x: 1}`) throws V8's uncoded
  `RangeError: Maximum call stack size exceeded`; so does the mutual pair `$P: {A: $Q}`,
  `$Q: {A: $P}`. The lone-object absorption rule (ISSUE-15) hands the WHOLE record to the
  first declared member without consuming anything, so a cycle in the "first member's
  schema" chain absorbs forever. Legitimate recursion (`{A: {A: N}}`) works in both
  implementations — real nesting consumes a level of data per step.
- **This port**: absorption is skipped when that chain cycles before some schema on it
  declares the record's own first key; the record then reports the fault it actually has
  (`unknown-member`). No invented code, and recursive schemas keep working. Found by the
  stream byte fuzzer.

## 15. Stream-absolute error positions: required by the spec, done by nobody — open

- `io-specs/streaming/error-model.md:104-109` — positions "MUST be **stream-absolute**… It
  MUST NOT report record-relative positions."
- **io-js2**: reports frame-relative positions; no rebasing logic exists in
  `src/streaming/reader.ts`. **This port**: reports `1:1` (see #16). Neither conforms, so the
  requirement is currently unimplementable-as-written from the reference's behavior.
- Needs an upstream decision: enforce it (both implementations change) or amend the spec.

## 16. Error positions are asserted by ZERO corpus cases — open, gating gap

- A regex for position keys across every live `.io` case returns one hit, and it is a false
  positive (a schema member named `at` in `schema/primitives.io:30`). `CONFORMANCE.md:261`:
  "asserting **codes only** for errors". The only position affordance (`at: {line, col}`)
  sits in a stale YAML section, is marked optional, and no case uses it.
- Consequence, measured: this port drifted to a hardcoded `Line: 1, Col: 1` at **13 sites**,
  including the two helpers that govern the entire validation and schema-compile surface,
  and the corpus stayed green throughout. Any port can do the same.
- **Suggested**: one optional position column on the error case tables plus a §8 rule, so
  every port is held to it. See `io-go/docs/reports/error-model.md`.

## 17. CONFORMANCE.md §2 and §5 describe an abandoned layout — open, doc defect

- §2/§5 document a YAML case format using camelCase codes and `message:` assertions, both
  forbidden by the live contract; §8/§9 document the real `.io` format the corpus actually
  uses. The stale sections are the ones a new port reads first.

## 18. CONFORMANCE.md contradicts itself on error ordering — open, doc defect

- §5: multi-code expectations are "order-independent unless `ordered: true` is set".
  §8: `error_codes` means "these errors, **in this order**". §7.1 also requires "the same
  error codes, **in the same order**". `ordered: true` is implemented nowhere and used by no
  case; the corpus data follows §8, and two cases (`validation/accumulation.io:19-22,31-34`)
  genuinely depend on order.

## 19. The reference has no structured member path on errors — open

- io-js2 interpolates the failing member's path into the message *string*
  (`schema/types/common-number.ts:92-108`), and messages are explicitly non-normative
  (`CONFORMANCE.md:176-177`), so no conformance rule can assert which member failed. A
  structured field would serve every port; today each one invents its own or omits it.

## 20. A data member keyed `*` is hoisted into the wildcard's slot — open (this port; reference unaffected)

- Under a typed wildcard schema (`*: int`), a record carrying a member whose key is literally
  `*` bound to the wildcard DEFINITION rather than being treated as an ordinary extra, so it
  was emitted in schema order — ahead of positional members — producing `"*": 0, 0`, which
  the reader rejects (positional-after-keyed). The reference keeps arrival order
  (`{"1":0,"*":0}`), confirming the `*` entry is openness, not a member.
- **This port**: fixed in `validateObject`. Found by the byte fuzzer, not by the corpus — no
  case combines a typed wildcard with a literal `*` data key. Worth a corpus case.

## 21. A `time` serializes without its milliseconds — open, reference defect (data loss)

- **Spec** (`the-structure/values/date-and-time.md`, format table): the canonical Time form is
  `HH:mm:ss.SSS`.
- **io-js2**: writes `t"14:30:45"` for `t"14:30:45.999"` — the millisecond field is dropped
  whether the member is declared `time` or undeclared. The value survives in memory
  (`toObject()` returns `1900-01-01T14:30:45.999Z`); only the serialization loses it, so
  `parse → toString → parse` silently changes the instant. `datetime` is unaffected: it always
  writes `.SSS`.
- **This port**: follows the spec — `t"14:30:45.999"`, with `.000` still elided (`t"14:30:45"`)
  so the one corpus case that covers this, `serializer/scalars.io :: time_value`, still passes.
- **Gating gap:** that case uses `t"14:30:45.000"` — a ZERO millisecond field — so it cannot
  tell "elides a zero" from "drops the field". No corpus case anywhere uses a non-zero
  millisecond in a serialized time, which is why five ports could disagree here undetected.
  **Suggested case:** `~ time_millis, 't"14:30:45.999"', '---\nt"14:30:45.999"'`.
- Related to #1: the reference also rejects `t"143045.123"` on input. Its time handling
  disregards fractional seconds in both directions.

## 22. Temporal `min`/`max` compare the whole instant, ignoring the declared precision — open

- **Spec**: says nothing about how a bound is compared against a value of a different temporal
  annotation.
- **io-js2**: compares the raw instants. Two consequences, both probed 2026-09-03:
  - `{date, max: d"2024-03-20"}` rejects `dt"2024-03-20T14:30:45.123Z"` with `mismatched-max`,
    although the value's DATE is exactly the bound.
  - `{time, max: t"15:00:00"}` rejects `dt"2024-03-20T14:30:00Z"`, although 14:30 precedes
    15:00. It fails because the bound is anchored at 1900-01-01 and the value is not — a
    comparison between a real date and the time-of-day anchor, which is not meaningful.
- **This port**: matches the reference (`temporal_test.go :: TestTemporalBoundsCompareWholeInstant`
  pins it). No corpus case covers temporal bounds across annotations.
- **The case for changing it upstream:** the format already decided that the annotation governs
  precision — `validation/temporal-depth.io` permits any temporal under any annotation, and the
  serializer truncates to the declared kind on write (a `date` member writes `d"…"` and drops
  the clock). Comparison is the one place that precision is *not* applied, so a value can be
  rejected on a component that the same schema will discard on output. Scoping the comparison
  to the declared precision — date vs date, clock vs clock — would make the three operations
  agree. This needs a spec decision before any port implements it, since it changes accept/reject
  outcomes; raised here rather than diverging unilaterally.

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

- **A temporal is a plain `time.Time`; the kind is not kept on the value.** PORTING-NOTES
  rule 15 asks a host to keep `date` / `time` / `datetime` distinct end to end. Rule 15
  scopes itself to a **kinded host** — Rust's `Temporal`, Python's `date`/`time`/`datetime`,
  JS's tagged wrapper — and names the three types it expects to find there. Go's standard
  library has exactly one temporal type, so this port decodes all three literals to
  `time.Time` and treats the kind as **presentational**, the same class of fact as a string's
  open / raw / quoted spelling, which no host keeps either.

  The evidence that the kind is presentational rather than semantic is in the reference, not
  in convenience: validation states outright that "the three temporal kinds are
  interchangeable at the type check" — a `date` satisfies `datetime` and vice versa — and the
  corpus comparator compares temporals **by instant**, with the kind ignored. Nothing in the
  format's semantics can observe the difference.

  So the kind is decided on WRITE, where a spelling decision belongs. When the member
  declares a temporal type the schema supplies it, which in a schema-first format is the
  normal case and is lossless — a midnight `datetime` stays a `datetime`, a 1900-01-01 `date`
  stays a `date`. Only an **undeclared** temporal is spelled from its instant, exactly as the
  reference infers one, and the writer's `InferTemporalKind` is the single site that does it.
  `temporal_test.go` pins all four behaviors. Reported here because it is a deliberate
  divergence from the letter of rule 15, not because the rule is wrong for kinded hosts.

- **Lone UTF-16 surrogates.** JS strings can hold a lone surrogate from `\uD83D`; Go strings
  cannot. This port decodes a lone surrogate escape to U+FFFD. If a corpus case ever asserts a
  lone-surrogate value, it is asserting a JavaScript accident and needs an upstream decision.
