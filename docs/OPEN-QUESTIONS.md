# Open questions

Recorded rather than blocked on, per the owner's instruction (2026-09-07): when a decision is
genuinely the owner's, write it here and move to the next task that can be finished
independently. Each entry states what was assumed so the work could continue, and what would
change if the answer differs.

## 1. A corpus gap: no streaming case uses a bare-expression header

**Found** 2026-09-07 while building the `StreamMarshaler`.

`Parse` and `Stream` disagreed about what a bare `---` binds to when the header is a bare
schema EXPRESSION (`name: string, age: int`) rather than a `$schema` definition. `Parse` bound
the members by name; the stream reader read them positionally, because its `defaultSchema()`
carried a partial copy of the resolution that had forgotten the expression form.

**Fixed here** — both now go through `document.DefaultSchemaOf`, one statement of the rule.

**The question for the corpus:** nothing pinned this. All 118 streaming cases use `$schema:` or
a named `$P`, so a port can get the expression form wrong and stay green. A case would close it:

```
~ inline_expression_header, "name: string, age: int
---
~ Alice, 30",
  { items: [{ kind: record, recordIndex: 0, value: { name: "Alice", age: 30 } }] }
```

**Assumed meanwhile:** the stream must agree with `Parse`, since the streaming spec's
equivalence rule says a stream and a document carrying the same records mean the same thing.

## 2. `Document.JSON` mapping is decided, not derived

SPEC 0001 §4.8 fixes a mapping per value type (decimal → string, bigint → number-or-string,
temporals → RFC 3339, positional members → index keys). The reference's `toJSON` was not
probed case by case before this was written down.

**Assumed:** the table in §4.8. Where a probe later shows the reference differing, that is a
finding to record, not a silent change — the mapping is a decision this port is entitled to
make, since io-specs does not cover JSON projection.

## 3. Is ADR 0004's Level-1 embedded surface still the plan? — **for the owner**

ADR 0004 describes a gradient: Level 0 (plain structs plus package functions), Level 1 (an
embedded base giving `emp.Set(…)` methods), Level 2 (`iogen`). **Level 1 has never been
built** — no object base, no `New[T]`, no `Attach`, no `io.Set`/`io.Get` package twins,
no `StreamAs[T]`.

**Why this is a question and not a task.** SPEC 0001 has now delivered every capability the
reference exports without it. Level 1 buys mutation-time validation on a struct
(`emp.SetAge(-1)` failing at the call), and ADR 0004 D2 is candid that it costs an attachment
step Go embedding cannot do for you — `io.New[T]()` or `emp.Attach(emp)` — plus a documented
hole where a direct field write bypasses it. The same guarantee already exists twice over
without any of that: `Builder.Add`, `Collection[T].Add` and `StreamMarshaler.Marshal` all
validate at the call, and `iogen` (Level 2) gives the unbypassable version.

**Assumed meanwhile:** not building it. Nothing in SPEC 0001 depends on it, and the parity
table is complete without it.

**What would change the answer:** a caller who wants a *plain struct* to reject a bad field
write in place. If that case is real, Level 1 is worth its cost; if the answer is "use the
generated type", ADR 0004's phase list should be amended to say so.

Two related loose ends ADR 0004 D4 leaves open, both blocked on this: the name for the
embedded base (the `Object`→`Record` rename was withdrawn 2026-09-07), and `StreamAs[T]`,
which is a small typed wrapper over `Stream` and could ship independently.

## 4. Encode regressed below `encoding/json` — found 2026-09-13, **FIXED 2026-09-13**

**Measured, and bisected to one commit.** `BenchmarkCompareMarshalStruct_IO` (1,000 records):

| | bytes/op | allocs | vs `encoding/json` |
| --- | ---: | ---: | --- |
| up to `eea8ca4` | 180,945 | 22 | ~1.17× **faster** |
| from `7a39123` to HEAD | 279,249 | 23 | ~1.5× **slower** |

Bytes are exact, so this is not load noise. Every commit from `40eb8b4` to `eea8ca4` measures
180,945; `7a39123` ("Two round-trip bugs…") is the first at 279,249.

**Cause.** That commit fixed a real bug — the writer emitted `0.m<U+2000>0` open, which its own
reader tokenizes as the broken decimal `0.m`. The fix splits words on `tokenizer.IsSpaceRune`
(right) but then checks **every** word for a numeric reading, not just the first. So
`Person 0` is now written `"Person 0"`. That is +2 bytes per record: the 1,000-record document
grows 64,125 → 66,125 bytes, crosses the output buffer's size estimate, and pays one growth plus
a copy — exactly the +1 alloc and +98 KB. The per-word scan also adds time.

**The quoting is unnecessary.** Probed 2026-09-13: io-js2 AND io-go both read `Person 0`,
`a 1e`, `x 0.m`, `T 1`, `hello 12.5 world` and `N 0x1F` back as the plain string. Only the
FIRST word decides whether an open value reads as a string.

**Why it was missed.** The commit's gate said "allocation counts unchanged" — true for the four
hot benchmarks it checked (2954 / 21 / 16 / 14), not for the 1,000-record marshal comparison.
There is still no benchmark gate in CI, and a bytes-per-op regression would slip past an
allocation-count gate anyway.

**Proposed** (the writer's quoting rule is a correctness decision, so spec first) — and approved by the owner the same day:
keep the reader's own space set, and check only the first word for a numeric or broken-numeric
reading. Gate it with the round-trip fuzzers that found the original bug, plus a test pinning
that `Person 0` is written open, plus a CI check on bytes/op for the comparison benchmarks.

### Resolution (2026-09-13)

**Fixed as proposed, and measured back to the numbers before the regression:** 22 allocations
and 180,920 bytes per 1,000-record marshal, and a 64,125-byte document.

- **The rule now lives in one place.** `tokenizer.BareValueReadsNonString` states the scanner's
  one-word rule, and the writer asks it. The two helpers the writer used to combine,
  `WordReadsNonString` and `WordIsBrokenClaim`, are gone.
- **The writer asks about the bytes it will write, not the raw string.** My first version
  classified the raw text and was wrong: `0X0<TAB>"` is written `0X0	\"`, and to the reader an
  escape is part of a word, so that is ONE word and a broken hex claim. The round-trip fuzzers
  caught it within seconds. My own new quoting fuzzer did not, because I had excluded structural
  and control characters from its safety check along with its minimality check. Safety now has no
  excluded domain.
- **The gate that was missing now exists.** `perf-budget_test.go` fails the ordinary `go test`
  run when bytes or allocations per operation exceed a committed budget, and pins the document's
  wire size exactly. It is load-independent, so it works in CI. It was checked by putting the
  regression back: it failed on allocations (23 > 22), on bytes (+54.3%) and on wire size
  (+2,000). An earlier draft had +1 slack on allocations, which let that 23 through; it is now
  exact for small counts.

**Two pre-existing writer bugs, found by `FuzzParse` during the soak** (both confirmed on
untouched HEAD, both reachable only through a member name that needs quoting, such as `00`,
which forces the long form). Both came from `longFormBodyOf` carrying its own drifted copy of
`typeWithConstraints`' rule. It now calls the shared `constraintParts`:

1. Any `anyOf` was written `anyOf:null`, so the document no longer re-parsed.
2. **Silent:** an array's constraints were dropped. `00?: {array, of: int, minLen: 2}` was
   written without `minLen`, re-parsed cleanly and was idempotent, so the schema quietly accepted
   arrays it used to reject. The idempotence fuzzer could never have seen it. It is pinned by
   behaviour (`[1]` must still be rejected after the round trip), not by spelling.

## 5. The lazy decoder's differential test never compared two paths — found and fixed 2026-09-14

**Found while profiling for OPEN-QUESTIONS #4's follow-up**, and proved by sabotage: with the lazy
decoder corrupting every string it bound, seven ordinary tests failed and
`TestLazyMatchesTreePath` / `FuzzLazyMatchesTreePath` both passed. The switch was read once at
init; the test flipped the environment with `t.Setenv`. Broken since the lazy path landed
(`f559ec6`). `TestFastPathMatchesTreePath` had a milder form: its `t.Setenv` lasted the whole
test, so only its first sample compared fast against tree.

**Made live, it found three shipped validation bypasses in `io.Unmarshal`** — a missing required
member, a positional member after keyed ones, and an undefined `@`-reference, each silently
accepted. Fixed by amending ADR 0007 D2: the lazy path never reports a fault, it declines, and the
general path reports. Full account in ADR 0007's amendment.

**One loose end, for the owner:** `schema/typedef.go` tests `strings.HasPrefix(s, "@")` without
the length check the other five sites used, so a lone `@` in a typedef position is treated as a
reference there and as text everywhere else. Left as found. Whether a lone `@` is ever a
reference is a format question.

## 6. A string length bound naming an undefined variable is silently ignored — found 2026-09-14

**For the owner, through the escalation process** — not changed here, because both of io-go's
paths agree and the question is what the format means.

`name: {string, minLen: @n}` with no `@n` defined validates `~ Ann` cleanly: `validateString` reads
`md.Constraints["minLen"].(float64)`, a string `"@n"` is not a float64, and the bound is skipped.
A numeric bound written the same way, `age: {int, min: @n}`, resolves the reference and reports
`undefined-variable`. So one spelling is checked and the other is not.

Found while writing SPEC 0003 §5.2's constraint-family differential rows, where a row meant to be
refused was accepted by BOTH paths. Questions for the reference and the spec: may `len`/`minLen`/
`maxLen` name a variable at all; if so, should an undefined one be `undefined-variable` as `min` is;
and does io-js2 skip it as io-go does?

## 7. Two writer gaps both paths share — found in review of SPEC 0003 §5.2, 2026-09-14

Neither was introduced by the fast paths; both are reproduced on the general path alone.

1. **An untyped string beginning with `@` is written bare and does not read back.** A string member
   holding `"@x"` marshals as `@x`, which the reader resolves as a variable reference:
   `undefined-variable`. The format reads `@`-strings as references in every string form, so
   quoting does not help. Is there a spelling for text that starts with `@`, or must a writer refuse
   it?
2. **A nullable element type loses its nullability in the header.** `[]*string` tagged
   `[{string, "null": true}]` writes the header `[string]`, so a record `[N, x]` does not read back.
   A long-form writer bug of the kind fixed on 2026-09-13; not yet fixed.

## 8. A Go struct field cannot be named `*`, though the format now allows it

`struct-plan.go` refuses an `io:"*"` field tag as "reserved for an open schema". OPEN-DECISIONS D1
frees the name (ADR 0012): a document with a `"*"` member round-trips, and schema-less
`Marshal(map[string]any{"*": 42})` already writes one. Only the struct tag cannot express it, which
makes the Go surface asymmetric with the format and with the map path.

Lifting the guard needs the fast encoder's key writer and the `fastFor` name-equality check looked
at, plus `TestWildcardFieldNameIsRefused` inverted. Deferred deliberately rather than bundled into
the D1 landing, which was already large. Raised in review, 2026-09-18.

## 9. A malformed literal used as a schema DEFAULT writes text that cannot be read back

Found by the reviewer's fuzzer, 2026-09-20. Pre-existing, and not covered by ADR 0013's refusal,
because the document is **clean** — there is no fault to key on:

```go
io.Parse("A:[{any,0B2}]---")   // err == nil
doc.Text(nil)                  // "A: [{any, default:}]\n---", err == nil
io.Parse(that)                 // expected-value at 1:18
```

`0B2` is a malformed number. As a positional `default` slot it compiles into a `core.ErrorValue`
held on the MemberDef, and the schema writer spells it as `default:` followed by nothing.

**It is not only the positional default slot** — review found the mechanism is *any* MemberDef
constraint value, so a fix scoped to "default" would miss the rest:

```
"A:[{any,0B2}]---"                      -> "A: [{any, default:}]
---"
"a: {any, default: 0B2}
---
~ 1"       -> "a: {any, default:}
---
~ 1"     (explicit keyword)
"a: [{any, 0B2}]
---
~ [1]"            -> "a: [{any, default:}]
---
~ [1]"  (array-of-schema)
"a: {any, choices: [0B2, 1]}
---
~ 1"  -> "a: {any, choices:[, 1]}
---
~ 1"  (not a default at all)
```
 That
breaks the writer's one rule — *never emit text its own reader cannot read back as the same value*
(`internal/document/write-document.go`) — and it does so silently.

Two candidate answers, and the choice is a semantics call, not an implementation detail:

1. **The compiler refuses it.** A malformed literal is not a value, so it cannot be a default;
   raise the deferred code (`invalid-number` here) at compile time. Changes what `Parse` reports for
   inputs that are currently accepted, so it needs a corpus run and probably an io-specs answer.
2. **The writer refuses it**, joining ADR 0013's rule: a schema holding an unwritable default cannot
   be written. Narrower, but leaves a document that parses clean and can never be saved.

Related: `appendValue` (`internal/document/write-record.go`) has no `default` case, so any value it
does not recognise appends **zero bytes** rather than failing. That is the mechanism by which this
surfaces as corrupt text instead of an error, and it should gain an explicit refusal either way —
see also the note in ADR 0013 D1.

## 10. A constraint set twice — positionally and by keyword — is WRITTEN twice

Found by the reviewer's fuzzer, 2026-09-20. Pre-existing, and a different mechanism from #9 (no
`ErrorValue` is involved), so a fix for #9 will not touch it. The document is **clean**:

```go
io.Parse("0A:{A:{string,choices,[A0000]},B:{string,A,[A0000],choices:[]}}---")  // err == nil
doc.Text(nil)   // …B: {string, default:"A", choices:[], choices:[]}}   err == nil
io.Parse(that)  // duplicate-member at 1:96
```

A member def may state a constraint positionally (`{string, <default>, [<choices>]}`) and again by
keyword (`choices: []`). The compiler accepts both and keeps both; the schema writer emits each one
it holds, so the member is spelled with two `choices` keys — which its own reader rejects.

Like #9, this breaks the writer's one rule: *never emit text its own reader cannot read back*. The
question is the same shape — should the COMPILER reject the second statement of a constraint
(probably `duplicate-member` at compile time, which is what re-reading the output already says), or
should the WRITER collapse them? The compiler answer looks right, since two spellings of one
constraint have no agreed meaning, but it changes what `Parse` accepts and needs a corpus run.

**Not added as a fuzz seed yet, deliberately.** `FuzzParse` is the gate that should own this, and
the input above fails it — adding the seed now would make the suite red for a bug nobody has agreed
how to fix. Add it with the fix. (`FuzzParse` did not reach this shape in 240s; a seeded run found
it in 11 seconds, so the corpus does not explore doubled constraints.)
