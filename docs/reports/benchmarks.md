# Performance report — where io-go stands

**Date:** 2026-09-02 · **Machine:** AMD Ryzen 7 5700G, Go 1.26.0, windows/amd64 ·
**Reproduce:** `go test -bench Compare -benchmem -run '^$' .`

## Headline

**We were 2.7×–12.8× slower than `encoding/json`. One session of targeted fixes closed a
third of that; we are now 1.8×–8.7× slower, and the remaining gap is understood, localized,
and fixable.** The scanner is not the problem — it runs at **153 MB/s with 3 allocations per
document**, competitive with any JSON parser. Everything above it is where the time goes.

## The numbers

1,000 records × 6 members (string, int, string, bool, float, string array); 64 KB of IO text
against 114 KB of equivalent JSON, decoded into the same Go structs.

| Operation | Before | **Now** | encoding/json | Gap now |
| --------- | -----: | ------: | ------------: | ------: |
| Unmarshal → struct | 5.92 ms · 40,830 allocs | **4.65 ms · 34,804** | 2.32 ms · 6,019 | 2.0× |
| Marshal ← struct | 5.50 ms · 38,701 allocs | **3.43 ms · 23,698** | 0.39 ms · **2** | 8.7× |
| Parse → dynamic | 5.19 ms · 31,073 allocs | **3.37 ms · 25,050** | 1.92 ms · 23,013 | 1.8× |
| Validate (no JSON equivalent) | 3.33 ms | **2.10 ms** | — | — |
| Small record (133 B) decode | 9.6 µs · 84 allocs | **9.1 µs · 70** | 2.3 µs · 11 | 4.0× |

Throughput now: decode **13.8 MB/s**, encode **18.7 MB/s**, dynamic parse **19.1 MB/s**,
against JSON's 49 / 290 / 60 MB/s. Run-to-run variance on this machine is roughly ±10%, so
treat one-digit differences as noise; the ratios are not noise.

### Where we already win: the wire

| Payload | IO | JSON | |
| ------- | -: | ---: | - |
| 1,000 records | **64,125 B** | 114,374 B | **56% of JSON** |
| single record | 133 B | 97 B | 137% — the schema header costs more than it saves |

The format's promise holds at data scale (keys are written once, in the header, instead of
once per record) and inverts for a single small record, which is expected and worth stating
plainly rather than hiding.

## Diagnosis — where the time actually goes

Allocation profiles (`-memprofile`), after this session's fixes:

**Encode (Marshal).** `encodeStruct` 25% — building `*value.Object` trees; `writeRecord` +
`writeSection` **40%** — the writer builds a `[]string` of formatted parts at every nesting
level and `strings.Join`s them, so a 1,000-record document allocates thousands of short-lived
strings and slices. `encoding/json` reaches **2 allocations** by streaming bytes into one
growing buffer. This single structural difference is most of the 8.7×.

**Decode (Unmarshal).** `Tokenize` 20% (now one sized allocation — the cost of the token
buffer itself), `parser.addMember` 20%, `schema.assemble` 16%, reflection 7%. Decoding runs
four passes over the data — tokenize, build tree, validate into a *new* tree, bind into Go
values — where `encoding/json` does one.

**The scanner is exonerated.** `BenchmarkTokenize`: **41.8 µs, 152.9 MB/s, 3 allocs** for the
same 64 KB. Tokenization is ~1% of decode time. Nothing about the *format* is slow.

## What changed this session

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
| 1 | **Writer streams into one `strings.Builder`** instead of `[]string`+`Join` at every level | Encode −50…70%; would land near 60–90 MB/s | Medium — touches the fuzz-critical writer, but corpus + 3 fuzzers gate it |
| 2 | **Validation without per-record maps** — `slots`/`processed` become slices indexed by schema position | Decode −15…25% | Low |
| 3 | **Bind straight from the parsed tree** — `Unmarshal` currently materializes a validated *second* tree; bind during validation instead | Decode −20…30% | Medium |
| 4 | **Don't deep-clone in `Project()`** — the dynamic path clones the whole tree just to rename positional keys | Dynamic parse −30% | Low |
| 5 | **Tune the token-slice heuristic** (`len/4`) against real documents; over-allocation is now visible in the profile | Decode bytes −10% | Low |
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
