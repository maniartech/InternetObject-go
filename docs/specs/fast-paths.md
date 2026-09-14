# SPEC 0003 — Fast paths for the schemas people actually write

- **Status:** APPROVED by the owner, 2026-09-14 — all four §6 decisions, as recommended. §3's
  correctness work landed first, because it could not wait: it is the foundation the rest stands on.
- **Decides:** how io-go closes its remaining performance gaps against `encoding/json`, in what
  order, and under which gates. Sits under [ADR 0007](../decisions/0007-lazy-decode.md) (lazy
  decoding, amended 2026-09-14), ADR 0006 (performance architecture) and
  [ADR 0010](../decisions/0010-code-generation.md) D4 (generated code).
- **Owner's requirement:** performance must beat native `encoding/json`.
- **Evidence standard:** every number here is marked **measured** (with date and conditions) or
  **estimated**. An estimate is a reason to measure, never a result.

---

## 1. Where io-go stands — measured

All on 2026-09-13/14, AMD Ryzen 7 5700G, Go 1.26.0, machine load 13-21%, `encoding/json` measured
in the same run as the control. Times are the minimum of 6-10 runs.

| Operation | io-go | `encoding/json` | |
| --- | ---: | ---: | --- |
| Decode 1,000 records → struct, **plain** schema | 1.07-1.52 ms · 4,024 allocs | 1.65-2.10 ms · 6,019 | faster |
| Encode 1,000 records ← struct | 290 µs · 20 allocs | 361 µs · 2 | ~1.24× faster |
| Parse 1,000 records → dynamic | 2.61 ms · 17,952 allocs | 1.76 ms · 23,013 | ~1.48× slower |
| Decode one small record | 2.87 µs · 14 allocs | 1.93 µs · 11 | ~1.48× slower |
| **Decode 1,000 records → struct, CONSTRAINED schema** | **4.13 ms · 18,997 allocs** | 1.65 ms · 6,019 | **~2.5× slower** |
| Generated `Marshal`, one record (constrained) | 2,053 ns · 17 allocs | 476 ns · 2 | ~4.3× slower |
| Generated `Unmarshal`, one record (constrained) | 8,521 ns · 49 allocs | 1,798 ns · 11 | ~4.7× slower |
| Tagged `Unmarshal`, same record (constrained) | 12,101 ns · 82 allocs | 1,798 ns · 11 | ~6.7× slower |
| *Inlined generated `Marshal` prototype* | *186 ns · 1 alloc* | *476 ns · 2* | *~2.6× faster — the ceiling* |

The constrained schema is the one in `examples/06-codegen/person.io`:
`name: {string, minLen: 2, maxLen: 50}, age: {int, min: 0, max: 130}`, plus four plain members.

**The benchmarks the port publishes flatter it.** `bench_compare_test.go` uses a schema with no
constraints. Internet Object is schema-first; constraints are the normal case, and on them io-go
is 2.5-6.7× slower than `encoding/json`, not faster.

## 2. One root cause behind every remaining gap — verified in code

1. **Both fast paths decline any constraint.**
   - Decode: `document.IsSimpleSchema` rejects a member with any key or constraint
     (`len(md.Keys) > 0 || len(md.Constraints) > 0`).
   - Encode: `fastEligible` rejects a plan whose type carries `schema` tags (`plan.validate`).
2. **The `…With` functions never use a fast path.** `unmarshalLazy` has exactly one caller,
   `Unmarshal`; `MarshalWith` always goes through `encodeStruct` + `checkRecords`. Generated code
   calls the `…With` functions, so it is on the slow path by construction — its schema, and its
   constraints, notwithstanding.
3. **This was the design, never delivered.** ADR 0007 D2 said constraints "run in this loop
   against the typed local", boxing a value only when a constraint needs the generic checker.
   The implementation excluded them instead, and nothing recorded the difference.

## 3. Correctness first — done 2026-09-14, and why it had to be

Extending a fast path is only safe if its equivalence to the general path is verified.
**It was not.** `TestLazyMatchesTreePath` and `FuzzLazyMatchesTreePath` compared the lazy decoder
with itself from the commit that introduced them (the switch was read once at init; `t.Setenv`
never reached it). Proved by sabotage; made live; it found three shipped validation bypasses in
`io.Unmarshal` — a missing required member, a positional member after keyed ones, and an
undefined `@`-reference, each silently accepted. All fixed, and ADR 0007 D2 amended: **the lazy
path never reports a fault; it declines, and the general path reports.**

That amendment is what makes §5.2 tractable. A constraint the fast path finds violated simply
declines — the fast path never needs to reproduce the validator's fault list, order or positions.
It only has to be right about the HAPPY path, which a differential test can hold it to.

## 4. Where the time goes — profiled 2026-09-14

**Constrained decode (1,000 records, 4.13 ms).** `Unmarshal` first tries the fast path, which
tokenizes, compiles the header and frames **every record** — then `IsSimpleSchema` says no, all of
it is discarded, and `document.Parse` tokenizes the document again. By allocated bytes: `Tokenize`
24.8% (done twice), `frameMembers` + `FrameData` 21.7% (thrown away), parser tree `addMember`
16.4%, `assemble` 12.5%. **About a third of all bytes are the abandoned attempt.** Roughly half the
CPU samples are the runtime — span sweeping, locks, preemption — driven by 2.5 MB per call.

**Dynamic parse (2.61 ms).** Two trees are built and one is discarded: the parser's (`addMember`,
24.6% of bytes) and validation's (`assemble`, 20.9%). GC sweep and mark take ~30% of CPU.

**Small record (2.87 µs, 14 allocs, fast path).** `document.NewDefinitions` is **21% of
allocations** — ~3 per call — building a `Framed.Defs` that nothing reads (verified: the field is
set and never consumed). Framing is ~24%; `reflect.New` for the publish-on-success temporary is 1.

**Generated `Unmarshal` (49 allocs).** `parseHeader` is **42%**: `UnmarshalWith` re-parses the
document's header on every call, though the caller supplied a compiled schema. The header cache
serves only the fast path, which `UnmarshalWith` never takes.

**Generated `Marshal` (17 allocs).** The tree encoder (`encodeValue` + `encodeStruct`) is 43%, and
tree validation (`checkRecords`) most of the rest — because the schema has constraints.

## 5. The plan, in order

Every step lands only with: the pinned corpus, `-race`, the three forced routes, all fuzz targets,
the performance budgets, and — new, per §3's lesson — **a sabotage check proving each new or
extended differential test fails when the fast path is wrong.**

### 5.1 Stop doing work that is thrown away — no semantic change

| # | Change | Evidence | Expected |
| --- | --- | --- | --- |
| a | Decide eligibility from the (cached) header schema **before** framing records | framing is discarded on every constrained decode | constrained decode −~20% bytes (**estimated** from the profile share) |
| b | On fallback, parse from the token stream already produced instead of re-tokenizing | `Tokenize` runs twice | constrained decode −~12% bytes (**estimated**) |
| c | Stop building `Framed.Defs` | 21% of small-record allocations, never read | small record **14 → 11 allocs, `encoding/json`'s count** (**estimated**: 3 allocs attributed) |

Risk: low. (c) removes a field with no reader; (a) and (b) reorder work without changing what is
decided. Gate: budgets must move DOWN; the differential tests must stay green.

**Landed 2026-09-14 — measured** (allocs/op and B/op, the floor of repeated runs; bytes vary by a
few hundred run to run):

| Budget | Before | After |
| --- | ---: | ---: |
| **Marshal 1,000 structs** | 20 · 180,680 B | **4 · 180,248 B** — `encoding/json` spends 2 |
| Unmarshal one small record | 14 · 2,160 B | **10 · 1,968 B** — `encoding/json` spends 11 |
| Unmarshal 1,000 constrained | 18,997 · 2,501,261 B | 18,974 · **1,589,533 B (−36%)** |
| Unmarshal one constrained record | 82 · 9,984 B | 71 · 7,528 B |
| Unmarshal 1,000 structs (plain) | 4,024 · 1,145,608 B | 4,020 · 1,145,416 B |

How: `Unmarshal` tokenizes once and hands the same `tokenizer.Stream` to both paths
(`parser.ParseTokens`, `document.ParseTokens`); `document.HeaderSchema` returns the cached header
schema WITHOUT framing, so every schema-only decline happens before `parser.FrameData`; the
`Framed` type and its unread `Defs` are gone. `unmarshalLazy` now returns a plain `bool` — since
the D2 amendment it had no error to return. The small record beat the estimate (11) because the
second scan's stream also went.

**(d), found while landing it:** `marshalFast` re-rendered the schema header — a constant per Go
type — on every call: 16 of Marshal's 20 allocations. The plan now renders it once
(`structPlan.header`).

**Gates for 5.2, from this landing's review:** the constrained benchmark schema must gain a
`choices` member and a `pattern` member before 5.2's code lands (`choices` compares with
`core.Equal`, the representation-sensitive case), and 5.2 must say whether its encode-side check
reaches `StreamMarshaler`, which validates each record on the tree path today.

### 5.2 Constraints on the fast paths — ADR 0007 D2 as written

**Decode.** After a member binds, if its definition carries keys or constraints, box the decoded
Go value into the value model's type and ask the schema package's OWN per-member check whether it
passes. Pass → continue; fail → decline (§3). No constraint rule is re-implemented: the check is
the validator's, exported as a narrow pass/fail question (for example
`schema.ValueSatisfies(val any, md *MemberDef, defs Defs) bool`).

**Encode.** The same question, asked of each field value before it is written, lets
`fastEligible` stop rejecting `plan.validate`.

**Recommended over the alternative.** Typed per-constraint checks on the Go value (an `int` against
`min` without boxing) would be faster still, but they are a second copy of every constraint rule —
the bug class behind the long-form schema writer's two drifts (2026-09-13), the Parse/Stream
divergences of 2026-09-07, and the five hand-written copies of the `@`-reference rule found
2026-09-14. Boxing only CONSTRAINED members bounds the cost.

**Expected (estimated):** constrained 1,000-record decode from 18,997 allocations to roughly the
plain path's 4,024 plus one box per constrained member (~2,000 here), and from 4.13 ms to under
2 ms. To be measured, not assumed.

**What must be pinned, because it is where this can go wrong:**
- the value-model type each Go kind boxes into must be exactly what the tree gives the validator
  (a Go `int` field must reach an `int` member's check as the same representation the tree uses);
- `choices` uses `core.Equal`, so representation matters there too;
- constraint values that are `@`-references resolve through `defs` — the fast path already
  declines headers that define variables, so this stays declined until proven.

**Gates:** the lazy and encoder differential fuzzers extended to constrained schemas; the shape
tests extended with one constrained case per constraint family, each passing AND each shown to fail
when that family's check is disabled.

**Detailed design — written 2026-09-14, before the code.**

*The one question.* `schema.Accepts(val any, md *MemberDef) bool` runs `validateMember(val, true,
md, NoDefs{})` — the validator's entire per-member sequence (choices, type, pattern, lengths, bounds,
`multipleOf`, the sized-integer ranges) — and reports whether it raised a fault. It reuses the
validator's own panic/recover; nothing is copied. `NoDefs` is sound because a header that defines
variables already never reaches a fast path; a constraint naming one (`min: @x`) therefore fails
the check, and the path declines.

*What is boxed, and why it is the tree's representation.* The value passed is the one the TOKEN
decodes to, not the Go field: a string member's `Stream.StringValue`, a number's `Stream.Number`
(`float64`, which is what the parser boxes for every number token), a bool's `Stream.Bool`. The
field's Go type is irrelevant to the check — exactly as on the tree path, which validates before it
binds. The differential tests are what hold this claim.

*Which definitions it admits.* `IsSimpleSchema` keeps declining everything whose value the fast
path cannot produce or that needs more than one member to judge: `default` (the fast path has no
value to bind), `anyOf`, nested and referenced schemas, open schemas. It stops declining
`Constraints` and `choices`. Array-level constraints (`len`, `minLen`, `maxLen` on the array
itself) stay declined in this step — checking them needs the boxed `[]any` the path exists to avoid;
element constraints (`[{int, min: 0}]`) are checked per element through `md.Of`.

*Cost.* A check runs only for a member whose definition carries a key; an unconstrained member pays
one nil test. Boxing a string or float64 whose box does not escape `Accepts` should stay on the
stack — measured, not assumed, by the constrained budgets.

*Encode.* The fast encoder asks the same question of each constrained field's value (boxed as the
tree encoder would box it — `encodeValue`'s representation) against the plan's compiled member
definition, before writing it; a refusal makes `marshalFast` decline, and the tree path reports.
`fastEligible` stops rejecting `plan.validate` for flat types. `StreamMarshaler` is NOT changed in
this step: it validates each record on the tree path by design, and moving it waits for 5.3.

*Benchmark schema.* Before this code lands, the constrained benchmark gains a `choices` member and
a `pattern` member (review of 5.6), so their cost is inside a budget.

### 5.3 The `…With` functions take the fast paths

`UnmarshalWith` framing with a supplied schema (including headerless input, which `FrameData`
currently declines), and `MarshalWith` on the direct encoder. Lifts generated code directly: its
`Unmarshal` stops re-parsing the header on every call. Depends on 5.2, since generated schemas carry
constraints.

### 5.4 The dynamic parse builds one tree, not two

Frame the data, then let validation materialize the validated tree once, from spans; allocate
member slices from a per-document arena. The largest change here, and the only structural one.
**It gets its own spec before any code**, because it touches the parser's record path.

### 5.5 Generated code writes directly

The inlined prototype's 186 ns needs exported spelling primitives (ADR 0010 D4) so generated code
never copies a format decision, plus 5.2's constraint check. Last, because it depends on both.

### 5.6 The benchmarks tell the truth

Add constrained-schema variants to `bench_compare_test.go` and to `perf-budget_test.go`, **first**,
before 5.1. The published comparison will look worse before it looks better; that is the honest
order, and it is the only way the budgets can hold 5.2's gains once they exist.

## 6. Decisions — approved by the owner, 2026-09-14

1. **The ADR 0007 D2 amendment stands:** the fast path declines instead of reporting.
2. **5.2's approach:** the validator's own check on boxed values, not typed per-constraint fast
   checks (faster, but a second copy of every rule).
3. **The order:** 5.6 → 5.1 → 5.2 → 5.3 → 5.4 (own spec) → 5.5.
4. **The published numbers get worse first** (5.6).
5. **Added by the owner: an independent review gate on every landing** (§7), for performance,
   code quality, and a public API that reads as idiomatic Go — "must never look like Java".

## 7. Obligations — every landing

- **An independent reviewer signs off before the commit**, on three axes, and its findings are
  fixed or answered in the same change:
  - *performance* — the budgets moved as claimed, no hidden allocation or copy on a hot path;
  - *quality* — tests prove the behavior, differential tests proven live, one statement per rule;
  - *Go-nativity* — the API a Go programmer meets reads like the standard library: small
    interfaces, no getters/builders/factories/"Manager" types for their own sake, errors as
    values, useful zero values, `io.Writer`/`[]byte`/`context` where Go expects them, names
    without stutter, godoc-form docs. Internal packages follow the same idiom.

- Budgets move in the expected direction, and are lowered to lock each win in.
- Every differential test touched is proved live by sabotage in the same change.
- No step adds a second statement of any rule the format already states once; where one is found
  (as the `@`-reference rule was), it is consolidated, not copied again.

## ▶ RESUME HERE

- **State:** APPROVED 2026-09-14 (§6). Implementation under way in the §5 order, each step
  reviewed per §7 before its commit.
- **Done before this spec, 2026-09-14:** §3 — the lazy decoder's differential test made live, three
  validation bypasses fixed (commit `5507afc`); the encoder's per-call `os.Getenv` removed
  (Marshal 22 → 20 allocs).
- **Done:** 5.6 (constrained benchmarks and budgets), 5.1 (discarded work) — see their sections.
- **Next:** the correctness batch from the Go-nativity audit (SPEC 0004 §A — bugs, not design), then
  5.2.
