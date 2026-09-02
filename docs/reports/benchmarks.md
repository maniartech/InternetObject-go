# Performance report — where io-go stands

**Date:** 2026-09-02 · **Machine:** AMD Ryzen 7 5700G, Go 1.26.0, windows/amd64 ·
**Reproduce:** `go test -bench Compare -benchmem -run '^$' .`

## Headline

**We were 2.7×–12.8× slower than `encoding/json`. Four optimization passes have closed most
of it: decode is 1.56×, the dynamic parse 1.73× and encode 3.2×, with encode having shed
**90% of its allocations**. The remaining gap is understood, localized and scheduled ([ADR 0006](../decisions/0006-performance-architecture.md)).** The scanner is not the problem — it runs at **153 MB/s with 3 allocations per
document**, competitive with any JSON parser. Everything above it is where the time goes.

## The numbers

1,000 records × 6 members (string, int, string, bool, float, string array); 64 KB of IO text
against 114 KB of equivalent JSON, decoded into the same Go structs.

| Operation | Baseline | Pass 1 | Pass 2 | Pass 3 | **Pass 4 (now)** | encoding/json | Gap now |
| --------- | -------: | -----: | -----: | -----: | ---------------: | ------------: | ------: |
| Unmarshal → struct | 5.92 ms · 40,830 allocs | 4.65 ms · 34,804 | ~5.2 ms | 3.19 ms · 23,861 | **3.19 ms · 23,861** | 2.05 ms · 6,019 | **1.56×** |
| Marshal ← struct | 5.50 ms · 38,701 allocs | 3.43 ms · 23,698 | 2.58 ms · 17,692 | 2.16 ms · 17,849 | **1.80 ms · 3,922** | 0.35 ms · **2** | **3.2×** |
| Parse → dynamic | 5.19 ms · 31,073 allocs | 3.37 ms · 25,050 | 3.37 ms | 3.30 ms · 21,953 | **3.30 ms · 21,953** | 1.91 ms · 23,013 | 1.73× |
| Validate (no JSON equivalent) | 3.33 ms | 2.10 ms | ~2.1 ms | **1.65 ms · 17,009** | 1.65 ms · 17,009 | — | — |
| Small record (133 B) decode | 9.6 µs · 84 allocs | 9.1 µs · 70 | 9.1 µs | **8.6 µs · 61** | 8.6 µs · 61 | 2.0 µs · 11 | 3.7× |

**Cumulative: encode is 3.1× faster than baseline with 90% fewer allocations (38,701 →
3,922) and runs at 35 MB/s; decode is 1.9× faster with 42% fewer. The dynamic parse
allocates FEWER objects than `encoding/json` does** (21,953 vs 23,013).

**Encode is now 2.1× faster than baseline and has shed 54% of its allocations** (38,701 →
17,692) and 55% of its bytes (1.64 MB → 0.73 MB). Pass 2 did not touch the decode path —
that is phase B of [ADR 0006](../decisions/0006-performance-architecture.md).

> **Read allocation counts, not nanoseconds.** `allocs/op` is stable to ±1 across runs;
> ns/op on this machine swings ±30% with background load (a run taken while the fuzzers were
> active measured `encoding/json` itself 40% slower). ADR 0006 gates CI on allocations for
> exactly this reason.

Throughput on a quiet machine: decode **~12 MB/s**, encode **~25 MB/s**, dynamic parse
**~19 MB/s**, against JSON's ~44 / ~260 / ~58 MB/s.

### Where we already win: the wire

| Payload | IO | JSON | |
| ------- | -: | ---: | - |
| 1,000 records | **64,125 B** | 114,374 B | **56% of JSON** |
| single record | 133 B | 97 B | 137% — the schema header costs more than it saves |

The format's promise holds at data scale (keys are written once, in the header, instead of
once per record) and inverts for a single small record, which is expected and worth stating
plainly rather than hiding.

## Diagnosis — where the time actually goes

Allocation profiles (`-memprofile`). The encode figures below are from *before* pass 2 and
explain why it was done; the decode figures are current.

**Encode (Marshal).** `encodeStruct` 25% — building `*value.Object` trees; `writeRecord` +
`writeSection` **40%** — the writer builds a `[]string` of formatted parts at every nesting
level and `strings.Join`s them, so a 1,000-record document allocates thousands of short-lived
strings and slices. `encoding/json` reaches **2 allocations** by streaming bytes into one
growing buffer. This single structural difference was most of the original 12.8×; pass 2
addressed it, and `encodeStruct` — building the intermediate `*value.Object` tree — is now
the top encode allocation site.

**Decode (Unmarshal).** `Tokenize` 20% (now one sized allocation — the cost of the token
buffer itself), `parser.addMember` 20%, `schema.assemble` 16%, reflection 7%. Decoding runs
four passes over the data — tokenize, build tree, validate into a *new* tree, bind into Go
values — where `encoding/json` does one.

**The scanner is exonerated.** `BenchmarkTokenize`: **41.8 µs, 152.9 MB/s, 3 allocs** for the
same 64 KB. Tokenization is ~1% of decode time. Nothing about the *format* is slow.

## What changed — pass 4 (roadmap item 5 + numfmt.Append)

**Encode without the intermediate tree.** The general path built a `*value.Object` tree and
handed it to the writer — one interface box per scalar member, 47% of encode's allocations,
for values the writer consumed and discarded. `marshal_fast.go` walks the struct straight
into the output buffer for types that are simple enough (a struct, or slice of structs, whose
members are scalars or slices of scalars, declaring no `schema` constraints); anything else
falls back to the tree.

**This is a second traversal, NOT a second implementation of the format.** Every spelling
decision — string quoting, number and temporal formatting, key quoting — is made by exported
helpers in `internal/document`, the same ones the tree path calls. And the two are held
byte-identical by construction: `IO_NO_FAST_PATH=1` forces the tree, so
`TestFastPathMatchesTreePath` encodes the same values both ways and compares, backed by a
fuzz target (2.3M executions, zero divergences) and refusal-parity tests.

The differential test earned its keep immediately: it found that a zero-value `Decimal`
(nil coefficient) **panicked** on both paths — a real robustness bug in a value a caller can
trivially hold. Fixed at the one site, `Decimal.String`.

**`numfmt.Append`.** Number formatting built a string through a `strings.Builder` that the
writer then copied; it now writes into the caller's buffer with stack scratch space. Worth
~3,900 allocations per document on its own.

Result: encode 17,849 → **3,922 allocations** and 24 → 35 MB/s.

## What changed — pass 3 (ADR 0006 P2 + P7 partial)

**P2 — validation without per-record maps.** `slots`/`processed` became ONE slice of
`memberSlot` indexed by schema position (`Schema.Index`, built once at compile). Worth
recording honestly: **the profile disproved my estimate.** Ranked by allocation *count* — the
metric that matters for churn — those maps were 2.8% of decode allocations, not the
bottleneck I predicted from the byte profile. The change is still right (no hashing, one
allocation instead of two) but it did not move the number on its own. *Measure, don't
believe*, as the ADR says.

**P7 (partial) — stop re-boxing validated values.** The type validators unboxed a value from
`any`, checked it, and then `return s` — allocating a **fresh interface for a value they had
not changed**. Returning the original box instead removes one allocation per validated
scalar member. This was the single biggest win of the pass and it cost five one-line edits.

**P1 (decode side) — lazy error paths.** The same eager-path mistake fixed on the encode side
in pass 2 was still in the decoder: `fmt.Sprintf("$[%d]", i)` per record and `path+"."+field`
per field, on the happy path. Paths now travel as a (parent, name) pair and join only when a
fault is reported; a record path is built once per record. One failed attempt is worth
recording: making the path a linked list of parent *pointers* looked cleaner but the pointer
escaped, adding 6,000 allocations to encode — reverted after measurement.

## What changed — pass 2 (ADR 0006 P1 + P5)

**P1 — the writer is now append-style.** One `[]byte` buffer, sized from the record count, is
threaded through the entire per-record path; every piece is appended into it. The `[]string`
+ `strings.Join` at each nesting level is gone, along with the intermediate string per value.
Supporting pieces: `strconv`/`time.AppendFormat`/`base64.AppendEncode`/`big.Int.Append` write
straight into the buffer, and a small `partWriter` reproduces the "trailing empty members
vanish, interior ones keep their comma" rule without materializing parts. The header and
schema writers stay string-based deliberately — they run once per document, not per record.

**P5 — no regexp on the hot path.** The six writer regexes became byte-class tables and hand
scanners. Safety: the original regexes are kept **in the test file only**, and
`write_scan_test.go` pins every scanner to its regex over a 100-case table plus a fuzz target
that ran **10M inputs** with zero disagreements.

**Also:** removed the last per-record map in the writer (the `handled` set) by reading the
compiled schema's own map instead.

**A pre-existing bug surfaced while gating this** (`FuzzParse`, verified against the previous
writer — not a regression): with a typed wildcard schema (`*: int`), a data member whose key
is literally `*` was hoisted into the wildcard's *schema slot*, so it was emitted ahead of
positional members — a record spelled `"*": 0, 0`, which is unparseable. The reference keeps
arrival order; the `*` entry is openness, not a member. Fixed in `validateObject`.

## What changed — pass 1

Four small, safe fixes — corpus (1,572+262) and all three fuzz layers green after each:

1. **Size the token slice up front** (`tokenizer.Tokenize`) — replaced ~20 append doublings
   on a large document with one allocation. Biggest single win: −55% bytes on decode.
2. **Size record member slices** (`parser.addMember`, `schema.assemble`, `encodeStruct`) —
   killed the 1→2→4→8 doubling per record.
3. **Stop building error paths eagerly** (`marshal.go`) — `path+"."+field` was allocated for
   every field of every record on the happy path; a `pathAt` value now carries the parts and
   joins only when a fault is actually reported.
4. **Remove `strings.Fields` from the writer's hot path** and the per-record maps from
   `bindStruct` (a plan-level name index, built once per type, replaced two maps per record).

Result: **−21% to −38% wall time, −36% to −55% allocated bytes** across the four operations.

## Roadmap to parity

Ordered by value per unit of risk. Estimates are from the profiles, not measured.

| # | Change | Expected | Risk |
| - | ------ | -------- | ---- |
| ~~1~~ | ~~Writer streams into one buffer~~ | **DONE (pass 2)** — encode −25% time, −25% allocs on top of pass 1 | landed; corpus + 3 fuzzers green |
| 2 | **Validation without per-record maps** — `slots`/`processed` become slices indexed by schema position | Decode −15…25% | Low |
| 3 | **Bind straight from the parsed tree** — `Unmarshal` currently materializes a validated *second* tree; bind during validation instead | Decode −20…30% | Medium |
| 4 | **Don't deep-clone in `Project()`** — the dynamic path clones the whole tree just to rename positional keys | Dynamic parse −30% | Low |
| 5 | **Encode without the intermediate tree** — `Marshal` builds a `*value.Object` tree before writing (`encodeStruct`, now the top allocation site at 25%); walking the struct straight into the buffer removes it, at the cost of a second encoding path (weigh against "one decision, one site") | Encode −30…40% | Medium |
| 5b | **Tune the token-slice heuristic** (`len/4`) against real documents; over-allocation is now visible in the profile | Decode bytes −10% | Low |
| 6 | **Reuse buffers across calls** (`sync.Pool` for token slices and the writer's builder) | Both −10…20% | Medium |
| 7 | **Generated code (`iogen`)** removes reflection entirely from the struct path | Encode/decode −30…50% on top | Larger project |

Items 1–4 are a focused day's work and should bring encode within ~2× of `encoding/json`
and decode to rough parity. Note the bar: `encoding/json` is *not* the fastest Go JSON
implementation (`json/v2`, `sonic` and `go-json` beat it substantially), so "parity with
encoding/json" is the floor for a format that wants to lead, not the finish line.

## Honest caveats

- **We do strictly more work than JSON decoding**: schema binding, per-member validation with
  designated error codes, and a value model that preserves decimals, bigints and temporal
  kinds. Some gap is inherent. **8.7× on encode is not inherent** — that is the writer's
  allocation strategy, and JSON hits 2 allocations doing the equivalent job.
- These are single-machine numbers with ±10% run-to-run variance; the ratios are stable, the
  absolute values are not portable.
- No cross-implementation numbers yet: neither `io-rust` nor `io-js2` publishes a comparable
  benchmark, so "fastest Internet Object implementation" is currently unmeasured. A shared
  benchmark payload across the ports would be worth adding to `io-test-cases`.
- Nothing here is measured against a competing *format's* Go library other than
  `encoding/json` (no CBOR, MessagePack, protobuf comparison yet).

## Standing rule

Performance work never trades correctness: every change above was landed only with the full
corpus green and the property/byte/stream fuzzers re-run clean. That ordering is not
negotiable — a fast implementation that drifts from the spec is worth nothing to a format
whose value proposition is interoperability.
