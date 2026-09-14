# SPEC 0005 — The dynamic parse builds one tree

- **Status:** DRAFT for owner review, 2026-09-14. Nothing here is implemented. SPEC 0003 §5.4 asked
  for this spec before any code, because it changes the parser's record path and the validator's
  output, which every entry point shares.
- **Decides:** how `Parse` (and everything built on the tree path: `ParseWith`, the fallback of
  `Unmarshal`, `Definitions.Parse`, the stream reader) stops allocating a second copy of every
  record, in what order, and under which gates.
- **Evidence standard:** as SPEC 0003 — every number **measured** (date, conditions) or
  **estimated**.

## 1. Where the time and memory go — measured 2026-09-14

`BenchmarkCompareParseDynamic_IO`: 1,000 records × 6 members (string, int, string, bool, number,
2-string array), Go 1.26, quiet machine. **2.67 ms · 17,950 allocs · 1.46 MB**, against
`encoding/json`'s 1.85 ms · 23,013 · 0.73 MB into `any` — io-go already allocates less, and is
still ~1.4× slower in time, with twice the bytes. Allocation profile (`-memprofilerate 1`), per
record:

| Site | Allocs / record | Bytes share | What it is |
| --- | ---: | ---: | --- |
| `parser.parseValue` | 5 | 5% | one `any` box per string and number (a bool box is free) |
| `parser.parseArray` | 3 | 5% | the `[]any` and its two string boxes |
| `schema.assemble` | 2 | **22%** | the validated record: a fresh `*Object` and its member slice |
| `parser.addMember` | 1 | **26%** | the parsed member slice, allocated at capacity 8 (48-byte members) for 6 |
| `parser.parseRecord` | 1 | 2% | the parsed `*Object` |
| `schema.validateObject` | 1 | 10% | the per-record `memberSlot` scratch slice |
| `schema.validateArray` | 1 | 2% | — |
| `parser.project` | 1 | 2% | the projection `Document.Value` returns |
| `tokenizer.Tokenize` | per document | **22%** | the token slice: 20-byte tokens, sized `len/4 + 16` |

Two trees are built: the parser's (`parseRecord` + `addMember` + the boxes) and the validator's
(`assemble`), and the first is garbage the moment the second exists. GC mark and sweep were ~30% of
CPU samples in the SPEC 0003 §4 profile.

## 2. Why the validator builds a fresh record — and why that reason is narrower than its rule

`schema.assemble` documents the choice: writing validated values back into the parsed record was
tried and **built reference cycles**. The absorption rule (a schema member whose value is the
record itself, `$P: {A: $P}`, or the mutual `B: {B}`) can make a record a member of itself or of an
ancestor; the writer then walked forever. The byte fuzzer found both shapes within seconds.

That hazard exists only where absorption can happen — a member whose definition is an object
schema or a reference to one. A record whose schema members are all scalars or arrays of scalars
(the benchmark, and the common API payload) cannot absorb anything, so its parsed record can be
validated IN PLACE: members renamed to their schema keys and reordered into schema order within the
same slice. The rule "always fresh" is broader than the reason for it.

## 3. The plan, in order — each step lands alone, reviewed per SPEC 0003 §7

### 3.1 Size the parsed member slice exactly (no semantic change)

`addMember` allocates capacity 8 for every record. The parser can count a record's top-level
members before parsing it — the framer already does exactly this scan (`parser.FrameData`'s
`frameMembers`) — or, cheaper, size from the previous record in the same collection, which in a
collection of uniform records is almost always right, and grow normally when it is not.
**Recommended: the previous record's count** (no second scan; a uniform collection is the case that
matters). Estimated −20% of bytes; allocation count unchanged.

### 3.2 Validate scalar-only records in place (the one semantic-risk step)

When a section's schema is *absorption-free* — every member's definition, recursively through
`Of`, is standalone (`MemberDef.Standalone()`, SPEC 0003 §5.2) and not an object schema or
reference — `validateObject` writes each validated value and schema key into the parsed record's
own member slice, reorders it into schema order, and returns that record instead of assembling a
new one. Anything else keeps `assemble` exactly as today.

- *Why it is safe:* no member of such a record can hold an `*Object`, so no cycle can form; the
  only values written back are the ones validation returned, which are the parsed values or a
  resolved `@`-reference or a default — scalars and scalar arrays by construction.
- *Aliasing:* the parsed record belongs to the `Doc` being built and nothing else holds it
  (`Document.Value`'s projection is taken after validation).
- *Faults:* a record that faults is replaced by its `ErrorNode` as now, so a half-rewritten member
  slice is never observed.
- *Estimated:* −2 allocs and −22% of bytes per record, and the `memberSlot` scratch slice can become
  a stack array for schemas of ≤ 16 members (−1 alloc).

### 3.3 Shrink the token (no semantic change)

A `Token` is 20 bytes (three 1-byte kinds and four `int32`s: `Start`, `End`, `Line`, `Col`). `Line`/`Col` are read
only to report a fault or a position, and can be recovered from `Start` by a line index built on
demand. A 12-byte token cuts ~40% of the tokenizer's 22% of bytes, for every path including the fast ones.
**This is a wider change** (every position consumer) and is proposed LAST, only if 3.1-3.2 leave
the parse slower than `encoding/json`.

### 3.4 Not proposed: unboxed scalars

The public value model is `Object.Members[i].Value any`; a string or number there is a box. Arena
boxes need `unsafe` to forge interface values, which this port does not use. The boxes stay; the
remaining gap, if any, is reported honestly.

## 4. Gates

Every step: the pinned corpus (the tree path IS the specification here, so this is the primary
gate), `-race`, the three forced routes, all fuzzers — `FuzzParse` and the round-trip and
idempotence fuzzers above all, since 3.2 changes what `Parse` returns — the budgets moving down and
lowered, and for 3.2 specifically:

- a differential test holding in-place validation byte-identical (`Document.String()`, `JSON()`,
  `Value()`) to the assembling path, over the corpus's documents and a fuzzer, with a switch like
  `WithTreeDecode` to force `assemble`, proven live by sabotage;
- the two cycle shapes the fuzzer found (`$P: {A: $P}`, `B: {B}`) pinned as rows that must still take
  `assemble`.

## 5. Decisions for the owner

1. Approve §2's narrowing: in-place validation for absorption-free schemas only.
2. Approve 3.1's sizing from the previous record over a counting pre-scan.
3. Approve deferring 3.3 (token shrink) until 3.1-3.2 are measured.
4. Accept §3.4: no `unsafe`, so scalar boxes remain.

## ▶ RESUME HERE

- **State:** DRAFT, awaiting owner review of §5. Nothing implemented.
- **Next, once approved:** 3.1, then 3.2 with its differential test first.
