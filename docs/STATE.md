# State of the Go implementation

**Updated 2026-09-03.** What exists, how it got here, and where the remaining scope is.
Read [PROGRESS.md](PROGRESS.md) first for the resume anchor; this document is the wider map.

---

## 1. The journey, and what changed at each turn

### 1.1 The 2025 tree (abandoned, preserved on `archive/2025-tokenizer`)

The original effort built a tokenizer and several experimental parsers — `ZeroParser`, a
pooled "fast parser", value-vs-pointer token slices — chasing allocation counts. It had
benchmarks and tests of its own and was, in isolation, reasonable work.

**Why it was replaced rather than repaired:** it predated both the frozen error codes and the
shared conformance corpus, and had never been run against either. Its interface was
tokenizer-shaped (`ParseString` returning an AST) rather than format-shaped, and its
correctness story was "the tests we wrote," not "the corpus every implementation shares."
Optimizing against no specification is how a port drifts.

### 1.2 The restart: corpus-first

The rewrite's **first commit was the conformance runner reporting 0/262**, before a line of
tokenizer existed. Every phase after that ended at 100% of one suite with all previous suites
still at 100%. The corpus is the definition of done; a missing corpus **fails** the build
rather than skipping.

That ordering is the single most consequential decision in the project. It is why the port
reached the full ladder — 1,572 corpus cases plus 262 bootstrap — and why the fuzzers later
had a trustworthy baseline to find *new* bugs against.

### 1.3 How the public interface evolved

| Stage | Interface | Why it changed |
| ----- | --------- | -------------- |
| 2025 tree | `ParseString(src) (*AST, error)`, parser objects, pools exposed | Tokenizer-shaped; leaked internals; no schema story |
| ADR 0002 | `Parse`, `ParseSchema`, `Stream` — pipeline moved under `internal/` | The format is portable, the API is not: `(value, error)` with accumulate-and-continue, `iter.Seq2` streaming, nothing transliterated from JavaScript |
| ADR 0003 | `Marshal`, `Unmarshal`, `Validate`, `SchemaFor` + `io`/`schema` struct tags | Go developers work with structs; schema derived from the type is the format's advantage made free |
| ADR 0004 D0 | Vocabulary law: **one verb per direction** | The surface said `Parse` in one place and `Unmarshal` in another with no stated rule, and the internals used a third pair (`Load`/`Write`). `Load`, `Read`, `Decode`, `Write` and any `…IO` suffix are now banned |
| ADR 0004 D5 | `…With(…, s *Schema)` — `ParseWith`, `UnmarshalWith`, `MarshalWith`, `ValidateWith`, `StreamOptions.Schema` | A schema may live in a registry, not in tags or the document. Compile once, apply anywhere |
| ADR 0005 | `Error{Code, Category, Path, RecordIndex, Line, Col}`, `io.ErrorItem`, `io.IsError`, `ErrorList` helpers | Errors reported a fabricated `1:1` at 13 sites; the corpus asserts codes only, so nothing gated positions anywhere |

**Current public surface** (the whole of it):

```go
func Parse(src string) (*Document, error)
func ParseWith(src string, s *Schema) (*Document, error)
func ParseSchema(def string) (*Schema, error)
func Unmarshal(src string, v any) error
func UnmarshalWith(src string, v any, s *Schema) error
func Marshal(v any) (string, error)
func MarshalWith(v any, s *Schema) (string, error)
func Validate(v any) error
func ValidateWith(v any, s *Schema) error
func SchemaFor[T any]() (*Schema, error)
func Stream(r io.Reader, opts *StreamOptions) iter.Seq2[StreamItem, error]
func IsError(v any) bool

type Document  // Value, Records, Errors, Schema, SchemaOf, String
type Schema    // MemberNames, Open, String
type Error, ErrorList, ErrorItem, StreamItem, StreamOptions
type Object, Member, Decimal, Temporal   // the value model
type MarshalError, UnmarshalError        // binding faults, distinct from wire faults
```

### 1.4 How performance was reached

Six passes, each driven by a profile rather than intuition. Full detail in
[reports/benchmarks.md](reports/benchmarks.md).

| | Start | Now | `encoding/json` |
| --- | ---: | ---: | ---: |
| Decode → struct | 5.92 ms · 40,830 allocs | **1.65 ms · 4,060** | 2.40 ms · 6,019 |
| Encode ← struct | 5.50 ms · 38,701 allocs | **0.37 ms · 22** | 0.42 ms · 2 |
| Dynamic parse | 5.19 ms · 31,073 allocs | 3.30 ms · 20,953 | 1.91 ms · 23,013 |

The two structural wins were the same idea applied in both directions: **stop building a
value tree nobody asked for.** Encode walks the struct straight into the output buffer;
decode frames records as token spans (ADR 0007) and binds each member straight into its Go
field, where a decoded string is a substring of the source and costs nothing.

Everything else was profile-led detail: sized token slices, append-style writing, no `regexp`
on hot paths, no per-record maps, no re-boxing of validated values, error paths built only on
error, field kinds compiled into the plan, and `strings.ContainsAny` (which rebuilds a
256-bit set per call) replaced by one table pass.

**Three optimizations were tried, measured worse, and reverted** — recorded so nobody retries
them: parent-pointer error paths (escape, +6,000 allocs), per-level lazy path joins (+9,500),
and reusing the parsed record as the validated one (builds reference cycles; the fuzzer found
two shapes within seconds).

---

## 2. Architecture

```
internal/tokenizer   scanner; 20-byte value tokens over the source; lazy decode
                     (a string is a substring — decoding allocates nothing)
internal/parser      tokens → Document (header defs, sections, records)
                     + raw.go: token-span framing for the lazy decode path
internal/schema      compile (fail-fast) + validate (the full check order)
internal/document    pipeline top: Parse/ParseWith, String (canonical writer),
                     Reader (streaming), ParseFramed (header + framed data)
internal/numfmt      ECMAScript Number::toString, with an append form
internal/value       the value model + IsScalar + corpus equality
internal/errs        designated codes + category classification
root package         Parse/Unmarshal/Marshal/Validate/Stream, struct plans,
                     the direct encode path, the lazy decode path
```

**Two paths, one specification.** Encode and decode each have a fast route and a general
route. The general route is the specification; the fast route is an optimization of it that
declines anything it is not certain of. `IO_NO_FAST_PATH=1` and `IO_NO_LAZY=1` force the
general routes, and differential fuzzers hold the pairs byte-identical (encode) and
value-and-code-identical (decode).

---

## 3. Schema handling and validation — current coverage

### 3.1 Where a schema can come from

1. **In the document** — the header (`~ $schema: {...}`, a bare schema expression, or a named
   `$Person` bound with `--- $Person`).
2. **Derived from a Go type** — field names and types, with `io` tags for naming and
   `schema` tags carrying constraint syntax verbatim.
3. **At runtime** — `ParseSchema(text)` from a registry or file, `SchemaFor[T]()`, or
   `doc.SchemaOf(name)` lifted from another document.

Precedence when more than one exists: **explicitly attached > document header > tag-derived**,
never merged. `io` tags remain the name-binding contract in every case.

### 3.2 Types and constraints implemented

| Family | Types | Constraints honored |
| ------ | ----- | ------------------- |
| string | `string`, `email`, `url` | `choices`, `pattern`, `flags`, `len`, `minLen`, `maxLen`, `format`, `escapeLines`, `encloser` |
| number | `number`, `int`, `uint`, `float`, `int8/16/32`, `uint8/16/32` | `choices`, `min`, `max`, `multipleOf`, `format`, plus intrinsic type bounds |
| bigint | `bigint` | `choices`, `min`, `max`, `multipleOf` |
| decimal | `decimal` | `choices`, `min`, `max`, `multipleOf`, `precision`, `scale` |
| bool | `bool` | — |
| temporal | `date`, `time`, `datetime` | `choices`, `min`, `max` |
| array | `array`, `[T]` | `of`, `len`, `minLen`, `maxLen` |
| object | `object`, `{…}`, `$Ref` | `schema` |
| any | `any` | `choices`, `anyOf`, `isSchema` |

Also implemented: optional (`?`) and nullable (`*`) markers and their long forms, `default`,
open schemas (`*`) and typed wildcards (`*: T`), nested and referenced schemas resolved
lazily (so recursive schemas work), `@variable` resolution with cycle detection, the
lone-object absorption rule, and the reserved types (`int64`, `uint64`, `float32`, `float64`)
rejected as the specification requires.

**Validation discipline**, matching the reference exactly and pinned by 538 corpus cases: a
bare record fails fast with only the prevailing fault; a `~`-collection accumulates one fault
per faulted record, in record order; membership faults (unknown, duplicate, unexpected
positional) prevail over member faults; per member the order is @var resolution → absence →
null → choices → type → declared bounds → intrinsic bounds → multipleOf.

### 3.3 Temporal values are native `time.Time`

`d"2024-03-20"` decodes to `io.Temporal{T time.Time, Kind TemporalKind}` — a **real
`time.Time`**, usable directly (`tm.T.Year()`, comparisons, arithmetic), with the kind kept
alongside because Go's `time.Time` cannot express "this is a date, not a midnight instant".

```go
d"2024-03-20"                 → Temporal{T: 2024-03-20T00:00:00Z, Kind: KindDate}
t"14:30:45.123"               → Temporal{T: 1900-01-01T14:30:45.123Z, Kind: KindTime}
dt"2024-03-20T14:30:45.123Z"  → Temporal{T: 2024-03-20T14:30:45.123Z, Kind: KindDateTime}
```

A `time.Time` struct field binds straight from the wire, and `io:",date"` / `io:",time"`
choose which literal it writes back as. A time-of-day uses 1900-01-01 as its date anchor —
the reference's convention, so instants compare across implementations.

**The kind is carried, never inferred** (PORTING-NOTES rules 15 and 18). Inferring it from
the instant — which the reference must do, because a JavaScript `Date` has no kind — silently
turned a midnight datetime into a date and a 1900-01-01 date into a time of day. This port
had that bug until 2026-09-03; `temporal_kind_test.go` is the gate, since no corpus case
covers it.

### 3.4 Known limits in the schema layer

- **The lazy decode path handles simple schemas only** — plain typed members, no constraints,
  defaults, choices, references or variables. Everything else falls back to the general path,
  correctly but at ~5× the allocations. Widening this is the largest remaining perf item.
- **`isSchema` and `anyOf`** compile and validate, but have thin corpus coverage upstream, so
  confidence rests on the reference's behavior rather than on cases.
- **No schema evolution story** — no versioning, no compatibility checking between two
  schemas. Nothing in the format defines one yet.

---

## 4. Verification

| Gate | Scope | State |
| ---- | ----- | ----- |
| Conformance corpus | 1,572 cases across 7 suites + 262 bootstrap | **all green** |
| Property fuzzer | value → write → re-parse → compare → idempotence | green at 240,000 documents |
| `FuzzParse` | arbitrary bytes: never panic; clean parse round-trips | green (63M execs cumulative) |
| `FuzzStream` | chunk boundaries are never semantic | green (57M) |
| `FuzzFramingAgreesWithParser` | framing vs parser | green (36M) |
| `FuzzLazyMatchesTreePath` | lazy vs tree decode: values AND codes | green (4.6M) |
| `FuzzFastPathMatchesTreePath` | direct vs tree encode: byte-identical | green (2.3M) |
| `FuzzScannersMatchRegexes` | hand scanners vs the regexes they replaced | green (10M) |
| `FuzzFastPathEqualsGeneral` | integer number spelling vs ECMAScript algorithm | green (13M) |
| Error-model tests | position, path, category, record index, marker identity | 9 tests — **the only gate for these anywhere** |

**The fuzzers have found roughly 25 real defects since the corpus went green**, including
several the reference implementation shares. That ratio is the argument for keeping every one
of them.

---

## 5. Where the remaining scope is

### 5.1 Not built, though designed (ADR 0004)

- **The four embeddable bases** — `io.Object` (record: `Set`/`Get`/`Validate`/`Marshal`),
  `io.Document`, `io.Collection[T]`, `io.Definitions` — plus `io.New[T]`/`Attach` and the
  package-level `io.Set`/`io.Get` twins. This is the "validate on mutation" story and is
  entirely unwritten.
- **Multi-section documents bound to struct fields.** `Unmarshal` handles one data section
  today; sections-as-fields is designed and untouched.
- **Dynamic navigation** — `doc.Section(name)`, `sec.Records()`, `doc.Var(name)`,
  `SectionAs[T]`, `StreamAs[T]`.
- **The `Object` → `Record` rename**, which frees the good name for the base.
- **`iogen` code generation** — ADR 0004 D6 is a charter, not a spec; the real spec is
  blocked on phase 1 settling the runtime primitives its setters would call.

### 5.2 Performance still on the table

| Item | Estimated | Note |
| ---- | --------- | ---- |
| Widen the lazy decode path (constraints, nested structs, `$refs`) | decode −40…60% on those shapes | Largest remaining win; the machinery and its gates already exist |
| Dynamic `Parse` still builds the full tree | −30% | ADR 0007 phase 3 |
| Column-wise decoding (ADR 0006 F2) | monomorphic inner loop | **JSON cannot do this** — each object re-declares its keys |
| Parallel record decoding (F3) | near-linear on large documents | Boundaries are already found by the framer |
| Lazy materialization (F4) | order of magnitude for "read 3 fields" | Needs the span machinery, which now exists |
| `[]byte` and `io.Writer` entry points (F5) | one full copy per call | API decision, not a tweak |
| Reusable `Decoder`/`Encoder` (F6) | per-request allocation in servers | API decision |
| `iogen` removing reflection | −30…50% on typed paths | Level 2 |

### 5.3 Correctness and ecosystem scope

- **Report the 20 upstream findings.** Per ADR 0007 upstream this output outranks the
  library, and it is still undone. Four are defects the reference shares (uncoded stack
  overflows, control characters written bare, an unspellable datetime, an unbounded bigint).
- **The corpus cannot gate error positions** (finding #16) — a one-column change upstream
  would hold every port to them.
- **The corpus pin has drifted again**: `internal/conformance` pins `e6f288c` while the
  sibling checkout is at `15e02ce`. Everything passes at both; the pin should move, and
  should become a `v1.0.0` tag when upstream cuts one.
- **No CI.** Every gate here is run by hand. A pipeline running the corpus, the fuzz quick
  gates and a `benchstat` allocation-count regression check is the single highest-value
  engineering item that is not code.
- **No cross-implementation benchmark.** "Fastest Internet Object implementation" is
  unmeasured; a shared payload in `io-test-cases` would make it a claim rather than a hope.
- **Documentation for users** — the examples and the package doc are good, but there is no
  guide-level document (getting started, schema authoring, error handling) aimed at someone
  who has never seen the format.

### 5.4 Things deliberately not done

SIMD scanning (the scanner is ~1% of decode time), JIT or pervasive `unsafe` (hostile to the
corpus-and-fuzz discipline that makes this port trustworthy), and hand-written assembly. Each
has a stated condition that would justify revisiting it — see ADR 0006.
