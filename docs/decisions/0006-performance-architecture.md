# ADR 0006 — Performance architecture: how we close the gap

- **Status:** Accepted (plan), 2026-09-02
- **Context:** [reports/benchmarks.md](../reports/benchmarks.md) measured us at 1.8×–8.7×
  slower than `encoding/json` after a first quick-win pass. The scanner is already fast
  (153 MB/s, 3 allocs/document); every remaining cost is in the layers above it. This ADR
  names the techniques, in the order they should be applied, each tied to a measured profile
  line rather than to general advice.

## Non-negotiables

1. **Correctness first, always.** Every step lands only with the corpus (1,572 + 262) green
   and all three fuzz layers re-run clean. A fast implementation that drifts from the spec is
   worthless to a format whose entire value is interoperability.
2. **No `unsafe` without a written justification and isolation.** Where it is ever used it
   goes behind one reviewed helper with a race/asan test, never sprinkled through the code.
   We are not copying `sonic`'s JIT-and-unsafe approach; it is unmaintainable for a
   specification implementation.
3. **Measure, don't believe.** Every claim in this ADR is a hypothesis until `benchstat`
   says otherwise. Estimates below are from allocation profiles.
4. **The public API does not change for performance.** ADR 0002/0004 shapes stay.

## The techniques, in priority order

### P1. Append-style single-buffer encoding — the big one

**Problem, measured:** `writeRecord` + `writeSection` account for **40% of encode
allocations**. Every value is formatted into its own `string`, collected into a `[]string`
per nesting level, and `strings.Join`ed — O(n) allocations and O(n) copies. `encoding/json`
does the same job in **2 allocations** because it never materializes an intermediate string.

**Technique — the standard-library pattern.** One `[]byte` buffer, threaded by pointer
through the whole writer, plus the `Append*` family that writes into a caller-owned buffer:

```go
// Before: every level allocates a []string and a joined string.
func (d *Doc) writeRecord(obj *value.Object, s *schema.Schema) string

// After: one buffer, appended in place. No intermediate strings anywhere.
func (d *Doc) appendRecord(dst []byte, obj *value.Object, s *schema.Schema) []byte
```

with `strconv.AppendFloat(dst, f, 'g', -1, 64)`, `strconv.AppendInt`, `strconv.AppendQuote`,
`append(dst, s...)` replacing every `+` and `fmt.Sprintf`. This is exactly how
`encoding/json`'s `encodeState`, `time.AppendFormat` and `net/http`'s header writer work; the
`Append*` convention exists in the standard library precisely for this.

**Buffer sizing:** one `make([]byte, 0, estimate)` up front from a cheap size estimate
(record count × observed average), so the buffer doubles at most once or twice.

**Expected:** encode allocations −80…90%, time −50…70% → **3.43 ms → ~1.0–1.5 ms**, i.e.
inside 2–3× of `encoding/json` instead of 8.7×. **Risk:** medium — it rewrites the
fuzz-critical writer, but the round-trip corpus and the property fuzzer gate it precisely.

### P2. Maps → slices for compiled-schema lookups

**Problem, measured:** `validateObject` allocates a `slots map[string]any` and a
`processed map[string]bool` **per record**; `assemble` then walks them. Map construction,
hashing and iteration dominate validation.

**Technique.** The schema is compiled once and its members have stable indices, so:

```go
// index the schema ONCE at compile time (Schema.Index map[string]int, built in Compile)
slots     := make([]any,  len(s.Names))   // one slice, indexed by member position
processed := make([]bool, len(s.Names))   // stack-friendly, no hashing
```

Map-to-slice on a bounded, statically-known key set is the single most reliable Go
micro-optimization; `encoding/json` does the same thing with its cached field index arrays.
For small member counts (< 8) even a linear scan over a `[]string` beats a map.

**Expected:** decode −15…25%. **Risk:** low — pure data-structure swap inside one function.

### P3. Delete redundant passes

**Problem, measured:** decoding a document runs **four passes** — tokenize → build tree →
validate into a *newly allocated* tree (`assemble`, 16% of decode allocations) → bind into Go
values; and the dynamic path adds a fifth, `Project()`, which **deep-clones the entire tree**
just to rename positional keys. `encoding/json` decodes in one pass, directly into the target.

**Technique.** Three separate removals, in increasing difficulty:

1. **Validate in place.** `validateObject` mutates the record it was given instead of
   assembling a replacement; the "one output object per record" allocation disappears.
2. **Project lazily.** Positional-key renaming becomes a view computed during traversal, not
   a materialized clone. (`Project()` exists for the dynamic API; `Unmarshal` should never
   call it — verify with a profile that it doesn't.)
3. **Bind during validation.** For the struct path, write into the Go value as each member
   validates, so the intermediate tree is never built at all. This is what a single-pass
   decoder does and it is the biggest of the three.

**Expected:** decode −20…35% combined. **Risk:** medium — (1) and (2) are contained; (3)
restructures the decode path and should be done last, behind the fuzzers.

### P4. Slab allocation for tree nodes

**Problem, measured:** `parser.addMember` is 20% of decode allocations — one `[]Member`
backing array per record, thousands per document.

**Technique — chunked slab (arena-style) allocation**, the standard approach in
high-performance parsers (simdjson's tape, `go/scanner`'s chunked token storage, protobuf's
arena):

```go
type memberSlab struct{ buf []value.Member } // one 1024-element chunk

func (s *memberSlab) alloc(n int) []value.Member {
    if len(s.buf)+n > cap(s.buf) { s.buf = make([]value.Member, 0, max(1024, n)) }
    out := s.buf[len(s.buf) : len(s.buf)+n : len(s.buf)+n] // capped slice: no cross-record aliasing
    s.buf = s.buf[:len(s.buf)+n]
    return out
}
```

The three-index slice expression is load-bearing: it caps capacity so one record's `append`
can never overwrite the next record's members. **Do not use `GOEXPERIMENT=arenas`** — it is
unreleased, unsupported, and explicitly not recommended for libraries.

**Expected:** decode allocation *count* −40…60% (bytes roughly unchanged; GC pressure is the
win). **Risk:** medium — lifetime bugs are the classic slab hazard, which the capped slice
and the fuzzers address.

### P5. Kill `regexp` from hot paths

**Problem, measured:** the writer compiles and runs **6 regexes** per string and key
(`reKeyNumeric`, `reKeyBareSafe`, `reDateLike`, …); `regexp.(*bitState).reset` showed up in
the first profile. Go's `regexp` is RE2 — safe, no backtracking, and *slow* relative to a
hand scanner.

**Technique.** Byte-class lookup tables plus hand-written scanners:

```go
var bareSafeKeyByte = [256]bool{ /* A-Za-z0-9_. -$ */ }   // one table, no allocation
func isBareSafeKey(s string) bool { … }                   // single pass, no state machine
```

This is how `net/http` validates tokens and how `strconv` classifies digits. The email/URL
regexes stay — they are ported verbatim from the reference and are not on the hot path.

**Expected:** encode −10…20%. **Risk:** low, but each replacement needs a table-driven test
proving equivalence to the regex it replaces on the corpus's strings.

### P6. Reuse transient buffers with `sync.Pool`

**Problem:** the token slice and the encoder buffer are allocated fresh per call and thrown
away — the classic `sync.Pool` shape (short-lived, uniformly-sized, high-churn).

**Technique — pooled with the standard guards:**

```go
var encPool = sync.Pool{New: func() any { return new([]byte) }}
// get, reset with buf = buf[:0], defer put — and DROP oversized buffers on put,
// so one 50 MB document does not pin 50 MB forever.
```

`encoding/json` (`encodeStatePool`) and `fmt` (`ppFree`) both do exactly this, including the
oversize guard. **Caveat to measure, not assume:** `sync.Pool` costs on single-threaded
workloads and is cleared every GC cycle; if `benchstat` shows no win, drop it.

**Expected:** −10…20% on repeated calls. **Risk:** low-medium — pooled buffers must never
escape into returned values (the fuzzers and `-race` catch this).

### P7. Reduce interface boxing in the value model

**Problem, measured on this machine:** storing a *varying* `float64` in an `any` costs
**1 allocation (8 B) and ~62 ns** versus a typed slice. Our value model stores every scalar
as `any`, so a 1,000-record × 6-member document boxes ~6,000 values on the way in — a
meaningful share of the 34,804 decode allocations.

**Technique.** The known answer is a **tagged union** instead of `any`:

```go
type Value struct {
    kind Kind      // number | string | bool | bigint | decimal | temporal | object | array | null
    num  float64   // numbers and bools, unboxed
    str  string    // strings, unboxed
    ref  any       // only the genuinely heap-shaped cases
}
```

`fastjson`, `gjson` and `reflect.Value` itself all use this shape. **But it is a breaking
change to the public value model** (`io.Object`/`Member.Value` are exported), so it is
deliberately *last*, and only if P1–P6 leave us short of the target. A cheaper partial win
available now: stop *re-boxing* during validation and projection — reuse the `any` the parser
already created instead of unwrapping and re-wrapping.

**Expected:** decode −15…25% if taken all the way. **Risk:** high (public API + every
consumer). Gate it behind a measured need.

### P8. Escape-analysis audit

**Technique.** `go build -gcflags='-m -m' ./internal/...` on the hot packages; hunt values
that escape only because they are returned through an interface or captured by a closure.
Typical fixes: return concrete types instead of interfaces from internal helpers, hoist
closures out of loops, pass a `*[]byte` rather than returning a fresh slice. Cheap to run,
frequently finds 5–10%.

### P9. Profile-guided optimization (PGO)

**Technique.** Go 1.21+ reads `default.pgo` from the main package and uses it for inlining
and devirtualization decisions; the toolchain reports **2–14%** typical gains for free. We
have representative workloads already (the comparative benchmarks), so:

```bash
go test -bench Compare -cpuprofile default.pgo -run '^$' .   # commit the profile
```

Do this **last**, once the code shape is stable — a profile taken against the current
allocation-heavy code would optimize for hot paths we are about to delete. Re-collect
whenever the hot set changes.

### P10. Eliminate reflection via codegen

ADR 0004 phase 3 (`iogen`) removes reflection entirely from the struct path — the same reason
`easyjson`/`ffjson` exist. It is the largest remaining win for typed workloads (−30…50% on
top of everything above) and it costs nothing in correctness because generated code is static
binding that delegates to the engine (ADR 0004 D6).

## Measurement discipline

Performance work without a gate regresses within a month.

1. **`benchstat`, never eyeballing.** `go test -bench Compare -count=10 -benchmem` before and
   after, then `benchstat old.txt new.txt`; report the delta with its confidence, and treat
   anything under ~5% on this machine as noise.
2. **Benchmarks live in the repo** (`bench_compare_test.go`) and cover the four operations
   users actually perform, at two payload sizes.
3. **CI regression gate:** run the benchmark set on a fixed runner and fail the build when a
   tracked metric regresses beyond a threshold. Allocation *counts* are the stable signal —
   they do not vary with machine load the way ns/op does, so gate on `allocs/op` first.
4. **Profile before and after every phase**, and put the profile line that motivated a change
   in the commit message. Every claim in this ADR is traceable that way.
5. **Cross-implementation numbers:** propose a shared benchmark payload in `io-test-cases` so
   "fastest Internet Object implementation" becomes a measured claim rather than a hope.

## Targets

| Phase | Work | Encode vs json | Decode vs json |
| ----- | ---- | -------------- | -------------- |
| today | (first quick-win pass, landed) | 8.7× | 2.0× |
| A | P1 + P5 | **~2–3×** | 2.0× |
| B | P2 + P3 + P4 | ~2–3× | **~1.1–1.3×** |
| C | P6 + P8 + P9 | **~1.5–2×** | **~parity** |
| D | P10 (`iogen`) for typed workloads | **beats `encoding/json`** | **beats it** |

The honest bar: `encoding/json` is *not* the fastest Go JSON library — `json/v2`, `go-json`
and `sonic` beat it substantially. Parity with `encoding/json` is the floor for a format that
intends to lead; phases C and D are what make "leading" a defensible claim, and only phase D
plausibly beats the fastest JSON libraries, because it removes reflection entirely while they
cannot remove JSON's per-record key parsing — which is exactly the structural advantage the
Internet Object format was designed to have.

## Beyond the roadmap — where the remaining headroom is

P1–P10 above take us to roughly parity with `encoding/json`. Parity is not leadership, and
the techniques that go past it are mostly **not** general Go tricks — they are things this
format can do *because* it is schema-first, which a JSON library cannot copy.

### The format's own advantages, unexploited so far

**F1. Compile the schema into a decode program.** For a bound section every record has the
same shape, so the schema can compile once into a flat instruction list — "read string into
field 0, read int into field 1, …" — executed with no reflection, no per-value type switch
and no map lookups. protobuf-go's fast path and `easyjson` work exactly this way. It applies
to the *dynamic* path too, not only to generated code, and it composes with P10 rather than
competing with it. **Biggest single remaining win; est. decode −40…60%.**

**F2. Decode a collection column-wise.** Every row shares one schema, so the type dispatch
can be hoisted out of the inner loop: decode all of member 0, then all of member 1. Branch
prediction stops thrashing and the hot loop becomes monomorphic. **JSON cannot do this** —
each object re-declares its own keys, so a JSON decoder must re-dispatch per value. This is
the clearest structural advantage the format has, and nothing in the ecosystem competes with
it.

**F3. Decode records in parallel.** A `~`-collection is embarrassingly parallel at record
granularity, and the record boundaries are trivially findable (the streaming framer already
does it). Split, decode on N goroutines, reassemble in order. Near-linear on multicore for
large documents; JSON's nesting makes safe splitting far harder. Gate it behind a size
threshold — for small documents the goroutine overhead loses.

**F4. Lazy materialization.** Tokenize, then materialize only the members the caller actually
touches (`gjson`'s model, but schema-aware so it is typed rather than string-scraping). For
"read three fields out of a large record" workloads this is an order of magnitude, not a
percentage.

### API-level provisions (they need new surface, hence ADR-level decisions)

**F5. `[]byte` and `io.Writer` entry points.** Today `Marshal` returns a `string`, which costs
one full copy of the document at the end, and `Parse` takes a `string`, which costs a copy for
any caller holding `[]byte`. Adding `AppendMarshal(dst []byte, v any) []byte` and an
`io.Writer`-based encoder removes both, and lets callers stream to a socket or file without
materializing the document at all. Same for a `[]byte` reader entry point.

**F6. A reusable `Decoder`/`Encoder` object.** P6 pools buffers per call; a caller-held
decoder can hold the token slice, the slab and the output buffer across *many* documents —
the right shape for servers, and the standard library's own answer (`json.Decoder`).

**F7. Zero-copy strings.** A decoded string that needs no unescaping (the common case) can be
a sub-slice of the source rather than a copy. It requires documenting that values alias the
input buffer and must not outlive it — a real API contract change, so it is a decision, not a
tweak.

### Deliberately not doing (and the condition that would change that)

- **SIMD scanning.** `simdjson` reaches multi-GB/s by vectorizing the scan. Our tokenizer is
  already **~1% of decode time**, so vectorizing it would buy nothing today. Revisit only if
  the scan ever exceeds ~15% of a profile.
- **JIT / pervasive `unsafe`** (the `sonic` approach). Fast, but unmaintainable for a
  specification implementation and hostile to the fuzz-and-corpus discipline that makes this
  port trustworthy. The isolated-helper rule in the non-negotiables stands.
- **Hand-written assembly.** Same reasoning, plus it would need per-architecture fallbacks.

### Operational, not code

**F8. GC tuning guidance in the docs** — allocation-heavy pipelines benefit from `GOGC` and
`GOMEMLIMIT` tuning; document it rather than guessing on the user's behalf.
**F9. Benchmark against the modern bar** — `json/v2`, `go-json`, `sonic`, plus CBOR and
MessagePack libraries, so "leading" is a measured claim about the field rather than about
`encoding/json` alone.

### Honest ceiling

Some gap is inherent: we validate against a schema, preserve decimals/bigints/temporal kinds,
and produce designated error codes — none of which a JSON decoder does. Against a *validating*
JSON stack (decode + JSON Schema) the comparison already favors us today. F1–F4 are what make
"fastest" defensible against a plain JSON decoder; F5–F7 are what make it true in real
services, where copies and per-request allocation dominate.

## Sequencing note

P1–P5 are independent of ADR 0005 (the error model) except in one place: adding positions to
`value.Member` (the error-model fix) touches the same struct P4 slab-allocates. **Do ADR 0005
first**, so the member struct settles before we optimize its allocation — otherwise P4 gets
rewritten.
