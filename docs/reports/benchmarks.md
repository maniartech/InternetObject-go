# Performance report — where io-go stands

**Date:** 2026-09-02, re-measured 2026-09-03 · **Machine:** AMD Ryzen 7 5700G, Go 1.26.0,
windows/amd64 · **Reproduce:** `go test -bench Compare -benchmem -run '^$' -count=6 .`

## Headline

**We were 2.7×–12.8× slower than `encoding/json`. Six optimization passes closed it: on the
typed path — the one real applications use — io-go now BEATS `encoding/json` in both
directions, decoding 1.45× faster with a third fewer allocations and encoding 1.17× faster
with 22 allocations against JSON's 2. Both directions shed ~90% of their allocations.**

**Pass 7 then closed the small-payload gap** — a 133-byte record decodes in 2,144 B and 14
allocations, down 72% and 73%, against `encoding/json`'s 480 B and 11 — and took 20% off the
dynamic path's bytes. It also turned up **a shipped data race** in `pattern` validation, which
is the more important finding of the two ([ADR 0009](../decisions/0009-shared-compiled-state.md)).
What remains slower than JSON is the **dynamic** parse alone, and the two structural items that
would close it are named in the roadmap.

The scanner is not the problem — it runs at **144 MB/s with 3 allocations per document**,
competitive with any JSON parser. Everything above it is where the time goes.

## The numbers

1,000 records × 6 members (string, int, string, bool, float, string array); 64 KB of IO text
against 114 KB of equivalent JSON, decoded into the same Go structs.

| Operation | Baseline | Pass 1 | Pass 2 | Pass 3 | Pass 4 | **Now (pass 6)** | encoding/json | Gap now |
| --------- | -------: | -----: | -----: | -----: | -----: | ---------------: | ------------: | ------: |
| Unmarshal → struct | 5.92 ms · 40,830 allocs | 4.65 ms · 34,804 | ~5.2 ms | 3.19 ms · 23,861 | 1.65 ms · 4,060 | **1.33 ms · 4,060** | 1.92 ms · 6,019 | **0.69× — faster** |
| Marshal ← struct | 5.50 ms · 38,701 allocs | 3.43 ms · 23,698 | 2.58 ms · 17,692 | 2.16 ms · 17,849 | 1.80 ms · 3,922 | **0.33 ms · 22** | 0.39 ms · **2** | **0.85× — faster** |
| Parse → dynamic | 5.19 ms · 31,073 allocs | 3.37 ms · 25,050 | 3.37 ms | 3.30 ms · 21,953 | 3.30 ms · 21,953 | **3.01 ms · 20,953** | 1.83 ms · 23,013 | 1.65× |
| Validate (no JSON equivalent) | 3.33 ms | 2.10 ms | ~2.1 ms | 1.65 ms · 17,009 | 1.65 ms · 17,009 | **1.37 ms · 14,009** | — | — |
| Small record (133 B) decode | 9.6 µs · 84 allocs | 9.1 µs · 70 | 9.1 µs | 8.6 µs · 61 | 8.6 µs · 61 | **8.2 µs · 51** | 1.69 µs · 11 | 4.9× |
| …with the schema hoisted | — | — | — | — | — | **3.5 µs · 30** | 1.69 µs · 11 | 2.1× |

**Pass 7 (2026-09-03).** Allocation counts are exact and load-independent, and this project
gates on them for that reason:

| Operation | pass 6 | **pass 7** | `encoding/json` | allocs vs JSON |
| --------- | -----: | ---------: | --------------: | -------------: |
| Small record (133 B) decode | 7,728 B · 51 allocs | **2,144 B · 14** | 480 B · 11 | **1.3×** |
| Parse → dynamic | 1,824,815 B · 20,953 | **1,456,610 B · 17,952** | 728,785 B · 23,013 | **0.78×** |
| Unmarshal → struct | 1,150,107 B · 4,060 | **1,145,606 B · 4,024** | 390,810 B · 6,019 | **0.67×** |
| Marshal ← struct | 180,920 B · 22 | 180,925 B · 22 | 114,995 B · 2 | 11× |

The small payload shed **72% of its bytes and 73% of its allocations**, taking it from 4.6× to
1.3× `encoding/json`'s allocation count while still doing schema binding and per-member
validation JSON does not do. The dynamic parse shed 20% of its bytes.
Detail in [ADR 0009](../decisions/0009-shared-compiled-state.md).

**Timings, taken later the same day on a half-loaded machine** — read the RATIOS, not the
absolutes. `encoding/json`'s own numbers are the control here, and they came back 13-17% above
their quiet-day values, so the io-go column is inflated by roughly the same and the gap ratios
are the only figures worth quoting:

| Operation | gap before | **gap now** | note |
| --------- | ---------: | ----------: | ---- |
| Small record decode | 4.9× slower | **~1.5× slower** | 8.2 µs → 2.9 µs measured, ~2.4-2.8× faster after discounting the inflation |
| Parse → dynamic | 1.65× slower | **~1.47× slower** | a modest ~10% off the wall clock — less than the 15-25% the byte reduction suggested |
| Unmarshal → struct | 1.45× faster | 1.4-1.8× faster | unchanged by this pass, as expected |

The dynamic path is the honest disappointment: −20% bytes bought ~10% time, not the ~20% a
GC-bound path implied. That points at the remaining per-record work (roadmap 10) rather than at
allocation volume, and it is the first thing to profile next.

The "Now" column is a fresh 6-run measurement taken 2026-09-03 after the temporal refactor
([ADR 0008](../decisions/0008-temporal-is-time-time.md)); allocation counts are unchanged from
pass 6, confirming the refactor cost nothing. Times are the **minimum** of six runs, the
estimator this machine's ±30% noise forces — the ratios reproduce (decode 0.69× on both days),
the absolute values do not.

**Both directions now beat `encoding/json` on the typed path.** Decode: 4.5× faster than
baseline, 40,830 → 4,060 allocations, and 1.33 ms against JSON's 1.92 ms with a third fewer
allocations. Encode: **17× faster than baseline, 38,701 → 22 allocations**, and 0.33 ms
against JSON's 0.39 ms. The dynamic parse also allocates fewer objects than `encoding/json`
(20,953 vs 23,013) while taking longer — its cost is time, not churn.

Encode moves fewer bytes per second than JSON (194 vs 295 MB/s) while finishing sooner,
because the IO document is 56% of the JSON one — the format's own advantage showing up in the
wall clock. Read the ns/op for "who finishes first" and the MB/s only for "how hard is the
machine working", never the two interchangeably: the two formats are not moving the same
number of bytes.

> **Read allocation counts, not nanoseconds.** `allocs/op` is stable to ±1 across runs;
> ns/op on this machine swings ±30% with background load (a run taken while the fuzzers were
> active measured `encoding/json` itself 40% slower). ADR 0006 gates CI on allocations for
> exactly this reason.

Throughput over each format's own bytes: decode **48 MB/s**, encode **194 MB/s**, dynamic
parse **21 MB/s**, tokenizer **144 MB/s**, against JSON's 59 / 295 / 62 MB/s.

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

**The scanner is exonerated.** `BenchmarkTokenize`: **44.5 µs, 143.8 MB/s, 3 allocs** for the
same 64 KB. Tokenization is ~3% of decode time. Nothing about the *format* is slow.

### Gap 2 — small payloads paid for the schema every call (FIXED in pass 7)

> The diagnosis below is what led to the fix. `headerFor` now memoizes the compiled header, so
> `Unmarshal` no longer pays this: the row is **2,144 B · 14 allocs**, better than the
> hand-hoisted number this section proposed as the prize. Kept for the reasoning.

The 133-byte single-record case is our worst ratio (4.9× JSON), and an allocation profile
taken 2026-09-03 names the reason: **the header schema is parsed and compiled on every
`Unmarshal` call.** `schema.addMember` (15%), `compileMemberDef` (13%), `compileSchema`,
`parseHeader` and `newDefs` together are **~45% of the allocations** for a document whose
data is one line. At 1,000 records that cost amortizes to nothing, which is exactly why it
stayed invisible until the small case was profiled.

Hoisting the schema out of the loop — `ParseSchema` once, then `UnmarshalWith` — measures the
size of the prize:

| 133-byte record, decoded into a struct | time | bytes | allocs |
| -------------------------------------- | ---: | ----: | -----: |
| `Unmarshal` (header compiled per call) | 8.2 µs | 7,728 B | 51 |
| `UnmarshalWith` (schema hoisted) | **3.5 µs** | **2,400 B** | **30** |
| `encoding/json` | 1.69 µs | 480 B | 11 |

**2.4× faster and 69% fewer bytes, with no change to the library** — the API already supports
it, and this is the shape an HTTP handler wants anyway (one schema, many payloads). What the
library still owes is the case where the caller *cannot* hoist: a compiled-schema cache keyed
on the header text would collect most of the same win automatically. Filed as roadmap item 8.

The residual 2.1× after hoisting is the honest floor of doing more work — schema binding and
per-member validation with designated codes — on a payload too small to amortize anything.

## What changed — pass 7 (the two gaps, and a race found on the way)

Attacking the two remaining gaps meant profiling the two benchmarks that never had been. Full
rationale in [ADR 0009](../decisions/0009-shared-compiled-state.md); in short:

1. **A shipped data race, fixed first.** A member's `pattern` regexp was compiled lazily and
   cached on the `*MemberDef` — which is reachable from the global plan cache, so two
   goroutines calling `io.Validate` on the same struct type wrote it concurrently. `go test
   -race` confirms it. It survived because **no test and no corpus case used a `pattern`
   through a `schema` tag**; the existing concurrency test exercised everything except the one
   field that was written. The regexp is now built once, at compile time. This was a
   prerequisite for anything that shares a compiled schema, not merely a tidy-up.
2. **The compiled header is memoized** (`headerFor`), because it was being re-parsed and
   re-compiled on every call: ~45% of a 133-byte payload's allocations, invisible at 1,000
   records. Bounded by header size and entry count, keyed on `strings.Clone`d header text so a
   small header cannot pin a large document, and forced off by `IO_NO_HEADER_CACHE=1`.
3. **The projection is copy-on-write.** Projecting drops absent slots and numbers unkeyed
   members; a validated record has neither, so it now projects to itself instead of being
   deep-cloned. That deleted the dynamic path's *third* full materialization of every record.
   The cost is a public contract change — `Value()` and `Records()` return views, documented on
   both.
4. **The token-slice estimate gained a flat `+16`.** Density is not constant: measured over our
   own writer's output it climbs from 3.44 bytes/token at one record to 4.00 at a thousand, so
   `len/4` under-shot *every* document below ~100 records and each paid a doubling plus a copy.
   Widening the divisor to `len/3` was tried and **measured worse** (+6-9% bytes on large
   documents); a constant margin fixes the small band without touching the rest.

**A fuzzer false positive was fixed too.** `FuzzLazyMatchesTreePath` compared decodes with
`reflect.DeepEqual`, which calls `NaN != NaN` — so it reported two *identical* decodes as
divergent the moment a document contained `NaN` (`N,N,NaN`). The comparator is now NaN-aware
and otherwise exactly as strict, keeping nil-vs-empty slices distinct.

## What changed — pass 6 (encode: the last of the per-value work)

Encode went from 3,922 allocations to **22**, and from 1.80 ms to 0.37 ms, in four steps —
each one found by a profile, not by guesswork:

1. **Error paths were 99.4% of the remaining allocations.** `pathAt.String` (86%) and
   `recordPath` (13%) built a location string for every field of every record, on the happy
   path, and threw it away. `pathAt` now carries four parts flat — root, record index, member
   name, element index — and joins them only when a fault is actually reported.
2. **`strings.ContainsAny` rebuilds a 256-bit ASCII set on every call**, and the writer called
   it several times per string, each rescanning. One table-driven pass now collects every
   character fact at once.
3. **Type dispatch per value.** `runtime.ifaceeq` and `reflect.Elem` were re-deriving each
   field's type on every record; the plan already knew it. Field kinds are now compiled into
   the plan once (ADR 0006 F1) and the encoder switches on a `uint8`.
4. **Whole-string numeric checks are gated on word starts.** Only text where some word begins
   with a digit, sign or point can read back as a number, a broken claim or a temporal — most
   real text skips those checks entirely. And integer-valued floats now spell straight through
   `strconv.AppendInt`, with `FuzzFastPathEqualsGeneral` (13M executions) holding that
   identical to the full ECMAScript algorithm.

Three writer bugs surfaced while gating this, all PRE-EXISTING and each verified against the
previous writer before fixing: a header name containing a control character was written raw;
a value that is exactly a BOM was written bare and then skipped on re-read (the writer used
`unicode.IsSpace`, the reader treats U+FEFF as whitespace — one decision, two sites, now
one); and a `@variable` name containing a comma was written unescaped, because values handle
commas by quoting and a bare name cannot. Header names now escape against the reader's own
terminator set.

## What changed — pass 5 (ADR 0007: lazy, token-backed decoding)

The decode path no longer builds a value tree. The header is parsed once for
its schema; the data records are **framed** — key, token span, and the
tokenizer's own Kind and Sub — and each member is decoded straight into its Go
field. A decoded string is a substring of the source, so binding one allocates
nothing, and the type check and the decode are the same step: a non-numeric
token for an `int` member IS `expected-integer`, reported at that token's line
and column.

**Decode: 23,861 → 4,060 allocations, 3.19 ms → 1.65 ms — past `encoding/json`
on both.**

Safety, in the same shape as the encode fast path: the lazy route takes only
what it is certain of (a struct or slice of structs, a schema of plain typed
members, no constraints, defaults, choices, references or variables) and
otherwise falls back to the path that has always run — so the fallback is the
specification. `IO_NO_LAZY=1` forces it, and a differential fuzzer (4.6M
executions) holds the two to identical values and identical designated codes.
Framing itself is held to the parser by a second differential fuzzer (36M
executions), which found five real divergences during development.

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
| ~~8~~ | ~~Cache the compiled schema across `Unmarshal` calls~~ | **DONE (pass 7)** — small payload −72% bytes, −73% allocs | landed; ADR 0009 D2 |
| ~~9~~ | ~~`Project()` deep-clone~~ | **DONE (pass 7)** — dynamic −20% bytes, −14% allocs | landed; ADR 0009 D3 |
| 10 | **Arena-allocate `[]value.Member`** (ADR 0006 P4, still unclaimed). `parser.go`'s `make([]value.Member, 0, 8)` per record and `assemble`'s per-record slice are now the two largest remaining allocation sites on the dynamic path. Note the objection recorded at `assemble` does NOT apply: an arena still yields a *fresh* Object, so no reference cycle is possible — only the backing array is shared | Dynamic −8…15% | Medium |
| 11 | **Frame the data instead of building the parser's tree**, materializing the tree once (validated) with `Project` as the view it now already is. The only version where the dynamic path costs one tree instead of two | Dynamic, structural | Larger |

Items 10 and 11 are what is left on the dynamic path; the typed path and the small payload are
done. Note the bar: `encoding/json` is *not* the fastest Go JSON
implementation (`json/v2`, `sonic` and `go-json` beat it substantially), so "parity with
encoding/json" is the floor for a format that wants to lead, not the finish line.

## Honest caveats

- **We do strictly more work than JSON decoding**: schema binding, per-member validation with
  designated error codes, and a value model that preserves decimals and bigints. Some gap is
  inherent — and the typed path now beats `encoding/json` anyway, while doing that extra work.
  The remaining 2.1× on a hoisted small payload is the part that plausibly *is* inherent; the
  4.9× before hoisting is not, and item 8 says so.
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
