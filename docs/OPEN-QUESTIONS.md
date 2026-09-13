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

## 4. Encode regressed below `encoding/json` — found 2026-09-13, **not fixed**

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

**Proposed, not applied** (the writer's quoting rule is a correctness decision, so spec first):
keep the reader's own space set, and check only the first word for a numeric or broken-numeric
reading. Gate it with the round-trip fuzzers that found the original bug, plus a test pinning
that `Person 0` is written open, plus a CI check on bytes/op for the comparison benchmarks.
