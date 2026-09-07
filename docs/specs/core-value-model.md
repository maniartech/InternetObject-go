# SPEC 0001 — The public surface and the core model (rev 2)

- **Status:** Draft for implementation, 2026-09-07. **Supersedes rev 1** (same file, earlier
  commits), whose critical review found ten defects; §9 says how each is resolved here.
- **Decides:** nothing new. [ADR 0004](../decisions/0004-native-api-design.md) (the native API
  gradient and the vocabulary law) and [ADR 0011](../decisions/0011-core-model-and-layout.md)
  (core model, layout) decide; this is the contract to build against.
- **Governing principle, from the project owner:** *the Go library delivers everything the
  TypeScript library delivers (except `loadInferred`), and each does so in its own native
  style.* io-js2 is the inspiration for **what** must be covered; Go decides **how it is
  spelled and where it lives**. Nothing here is named after a TS class.

---

## 1. Principles

### 1.1 Delivery parity, measured against `io-js2/src/index.ts`

Every exported capability of the reference has a row in §5 with its Go spelling and status.
"Not ported" is allowed only with a reason that survives review (§6). The definition of
delivered is not "an equivalent name exists" but "the corpus, the playground samples and the
tests prove the behaviour".

### 1.2 Go-native, not translated

These rules are checkable, and a reviewer should check them:

| Rule | Consequence |
| --- | --- |
| Package functions first, types second (ADR 0004) | Every capability works on plain structs, maps and slices through `io.Marshal`/`Unmarshal`/`Parse`/`String`/`Stream`/`Validate`. Types are opt-in sugar over the same engine. |
| The vocabulary law (ADR 0004 D0) | Four verb pairs; `…With(…, *Schema)` and `…As[T]` suffixes; **banned**: `Load`, `Read`, `Decode`, `Write`, `…IO`. |
| No `IO`/`Io` prefix or suffix anywhere | The package name already says it. `io.Document`, not `IODocument`. |
| Accessors are nouns, not `Get…` | `sec.Name()`, `obj.Keys()`. `Get` is reserved for keyed lookup with comma-ok: `obj.Get(key) (any, bool)`. |
| Failure is a return value | `(T, bool)` for absence, `error` for faults. Nothing exported panics on data. |
| Iteration is `iter.Seq`/`iter.Seq2` | Not `map`/`filter`/`forEach` methods; `slices` and range-over-func are the standard library's job. |
| Options are structs, not positional flags | `StreamOptions`, `JSONOptions`. Zero value is the default. |
| Generics where the type is the point | `Collection[T]`, `SectionAs[T]`, `StreamAs[T]`. Never as a substitute for `any`. |
| Immutable after construction unless the type exists to be built | A parsed `Document` is a value to read and share; the `Builder` is the thing you mutate (§4.6). |
| One type per file, kebab-case file names, file named for the type | §2. |

### 1.3 The lowering, and where each capability lives

This port lowers a document through five representations, and the measured dependency graph
is a strict DAG over them:

```
  text ──tokenizer──▶ tokens ──parser──▶ parse tree ──document──▶ validated tree ──root──▶ Go values
```

> **A stage may depend only on stages below it.** `core`, `errs`, `numfmt`, `tokenizer` depend
> on nothing; `parser` and `schema` on those; `document` on those; `streaming` on `document`;
> the root package on all of them.

The reference has one flat set of classes because JavaScript has one representation. Here,
each capability lives at the stage that owns the information it needs — so `Definitions`
(which holds parser output and compiles schemas) cannot live in `core`, and `core` may never
import `errs`. This is not a limitation to work around; it is what keeps `schema` ignorant of
the parser and the stream reader ignorant of documents.

### 1.4 SOLID, as it actually applies

- **SRP** — one type, one job, one file. Enforced by §2's layout.
- **OCP** — *as it stands, adding a scalar type touches six sites* (`type-<name>.go`,
  `typedefSchemas`, `family.go`, `validate.go`, `write-typedef.go`, `gen/types.go`,
  `enc-kind.go`). Rev 1 claimed one; that was false. The obligation this rev takes on is
  honest: a new type's *validation* is one file, and the remaining sites are a documented
  checklist in `internal/schema/typedef.go` until a later ADR collapses them.
- **LSP** — the one substitution that matters is **fast path ≡ general path**, enforced by
  differential fuzzers under `IO_NO_LAZY`, `IO_NO_FAST_PATH`, `IO_NO_HEADER_CACHE`. Every new
  fast path inherits that gate.
- **ISP** — no wide interfaces. `error`, `fmt.Stringer`, `iter.Seq2` are the only ones anything
  satisfies. No `Container` interface unifying `Object` and `Collection`: nothing would consume it.
- **DIP** — `core` is the bottom and depends on nothing.

---

## 2. The root package, file by file

The public package is where a Go reader forms their opinion of the library. It must read as a
designed surface, not a collection of files added when needed. Target layout — one exported
type or one job per file, kebab-case, named for what it holds:

```
io.go                 package doc; the four verbs' entry points; type aliases to core
document.go           Document: Parse, ParseWith, String, Value, Records, Errors, Sections, Section
section.go            Section: Name, SchemaName, Schema, IsCollection, Len, Records, Value, Errors, HasErrors
object.go             Object = core.Object; NewObject; the doc a caller reads first
collection.go         Collection[T]: tolerant row binding (ADR 0004 D4)
definitions.go        Definitions: ParseDefinitions; Schema, Default, Var, Names; Parse, Stream
builder.go            Builder: NewBuilder, NewBuilderFrom; Define, Var, Section; Document, String
builder-section.go    SectionBuilder: Add
schema.go             Schema: ParseSchema, SchemaFor, String, MemberNames, Open
marshal.go            Marshal, MarshalWith (struct/map/Object → text)
marshal-fast.go       the fast path (declines; differential-fuzzed)
marshal-error.go      MarshalError
unmarshal.go          Unmarshal, UnmarshalWith
unmarshal-lazy.go     the lazy path (declines; differential-fuzzed)
unmarshal-error.go    UnmarshalError
validate.go           Validate, ValidateWith
stream.go             Stream, StreamAs[T]
stream-item.go        StreamItem (a record in a stream)
stream-options.go     StreamOptions
stream-marshaler.go   StreamMarshaler: NewStreamMarshaler; Marshal, MarshalAs, Flush, Close
json.go               Document.JSON, JSONOptions
error.go              Error
error-code.go         Code and the designated constants
error-list.go         ErrorList
error-item.go         ErrorItem = core.ErrorNode; IsError
decimal.go            Decimal = core.Decimal; ParseDecimal, NewDecimal (SPEC 0002)
path.go, enc-kind.go, field-plan.go, struct-plan.go   unexported machinery, unchanged
```

Two files exist today under other names and are renamed to match (`with.go` → folds into the
files of the verbs it modifies; `document-struct.go` → `unmarshal-sections.go`, since that is
what it does). Test files keep Go's `_test.go` spelling.

---

## 3. The format's vocabulary, spelled in Go

io-specs is the authority on nouns, and its nouns are what the exported names use:

| io-specs says | means | Go name |
| --- | --- | --- |
| **object** | the value: ordered members, keyed or positional | `Object` |
| **record** | a row — the position an object occupies in a collection *or* a stream | `Records()`, `StreamItem`, `RecordIndex` |
| **collection** | an ordered sequence of records in a section | `Section.IsCollection()`, `Collection[T]` |
| **section** | one `---` block: a name, an optional schema binding, one object or a collection | `Section` |
| **document** | header + sections | `Document` |
| **definitions** | the header's namespace: `$schemas`, `@variables`, plain metadata | `Definitions` |
| **stream item** | the envelope a reader emits per record | `StreamItem` |

`Object` is the *value*; `record` is the *row*. Both words are correct and mean different
things, which is why `Section.Records()` returns objects.

---

## 4. Types and their contracts

### 4.1 `Object` — delivered

`core.Object`, aliased as `io.Object`. Invariants (tested): member order preserved end to end;
positional members never answer keyed lookups; an absent slot is positional and a value assigned
to it through `SetAt` clears `Absent`; lookup is O(n) *by decision* (a key index would be an
allocation per record on the hottest path — the doc comment must say "by decision", not
"measured", which rev 1's implementation wrongly claims); `Clone` copies structure and shares
values. `Members` stays exported for the pipeline.

### 4.2 `Document` — immutable once parsed; the ownership rule

A `*Document` returned by `Parse`/`ParseWith` is **read-only and safe to share across
goroutines**. `Value()` and `Records()` return *views* of the document's own objects (ADR 0009):
mutating what they return is a programming error, and the doc comment says so in the first
sentence. A caller who wants to change a document goes through `NewBuilderFrom(doc)` (§4.6),
which clones. Read API, unchanged: `String`, `Value`, `Records`, `Errors`, `Sections`,
`Section(name)`, `Schema`, `SchemaOf(name)`; added: `Var(name) (any, bool)`,
`Definitions() *Definitions` (a read-only view, §4.5).

### 4.3 `Section` — delivered

`Name`, `SchemaName`, `Schema`, `IsCollection`, `Len`, `Records`, `Value`, `Errors`,
`HasErrors`. Errors are attributed where the fault is raised (ADR 0005 D7), never re-derived by
scanning rows — so there is no separate collection-level error list, by design.

### 4.4 `Collection[T]` — the tolerant row container (ADR 0004 D4)

Rev 1 specified an untyped `core.Collection` wrapper with no consumer; that is withdrawn. The
collection type this port needs is the one ADR 0004 already decided:

```go
type Collection[T any] struct { /* unexported */ }
func (c *Collection[T]) Items() []T                 // the rows that bound
func (c *Collection[T]) Errors() []Error            // the rows that did not, with paths
func (c *Collection[T]) Len() int                   // rows attempted, both kinds
func (c *Collection[T]) All() iter.Seq2[int, T]     // index is the document row index
func (c *Collection[T]) Add(v T) error              // validates against the bound schema
```

**Job:** `[]T` is strict — any row fault fails the whole `Unmarshal`. `Collection[T]` is the
format's accumulate-and-continue at row level: a struct field of this type receives every row
that bound *and* the faults of those that did not. It is also what `SectionAs[T]` returns in
its tolerant form, `SectionCollectionAs[T]`. `Add` validates on insert, which is the one place
mutation-time validation exists at Level 0.

### 4.5 `Definitions` — a compiled header, shareable

Two things share the name in the reference; Go separates them by mutability:

- **Read-only view on a parsed document**: `doc.Definitions()` with `Schema(name)`,
  `Default()`, `Var(name)`, `Names()`. Backed by the document's own resolver. No mutation —
  a definition is only meaningful against the records validated with it.
- **Preloaded, compiled once, shared**: `ParseDefinitions(src string) (*Definitions, error)`
  compiles a header eagerly (every named schema, every variable) and returns an **immutable**
  value safe for concurrent use. It is *used* through methods, which keeps the vocabulary law
  intact without a fifth verb:

  ```go
  defs, _ := io.ParseDefinitions(headerText)
  doc, err := defs.Parse(data)                     // like url.URL.Parse, template.Template.Parse
  for item, err := range defs.Stream(r, opts) {…}  // preloaded definitions for a stream
  ```

  Precedence follows io-specs `streaming/schema-and-state.md`: in-stream definitions override
  preloaded keys; an in-stream `$schema` overrides the fallback default; otherwise the fallback
  stays active. `StreamOptions.Definitions string` stays for out-of-band header *text*;
  `*Definitions` is its compiled form.

### 4.6 `Builder` — the only mutable document

Rev 1 put a `(string, error)` `String()` on `Document`, colliding with `fmt.Stringer`, and
validated at the end. Both wrong. The builder is its own type and **validates at `Add`**, so a
fault is reported at the call that caused it — which is also what the reference does.

```go
b := io.NewBuilder()
b.Define("Employee", empSchema)         // *Schema; a header definition
b.Var("region", "apac")                 // an @variable
emp := b.Section("employees", "Employee")   // named, bound; "" name = the default section
if err := emp.Add(map[string]any{"name": "Alice", "age": 30}); err != nil {…}  // struct, map or *Object
doc, err := b.Document()                // immutable from here on
text := doc.String()
```

`NewBuilderFrom(doc)` clones a parsed document so it can be edited. A section with no schema
binding accepts any record and writes header-less. `Define` after a section has rows is refused
(`error`): definitions may not change under records already validated against them — §4.5's
rule, stated for the one place mutation exists.

### 4.7 `StreamMarshaler` — the writer, shaped by io-specs

Rev 1 named this `StreamWriter.Write`, which ADR 0004 bans. The verb for Go value → IO text is
`Marshal`, and the precedent for an incremental marshaler is `json.NewEncoder(w).Encode(v)`:

```go
sm, err := io.NewStreamMarshaler(w, &io.StreamOptions{Schema: s})
err = sm.Marshal(rec)                   // one record, validated, buffered
err = sm.MarshalAs(rec, "$Alert")       // explicit schema switch for a heterogeneous stream
err = sm.Flush()
err = sm.Close()                        // flushes; the header is emitted even if nothing was
```

Contract, each line traceable to `io-specs/streaming/readers-and-writers.md` §Writer:

- Serializes through the canonical writer — the same code `Document.String` uses; no
  stream-only spellings.
- The header is emitted **at most once**, before the first record, and the `---` terminator
  is **always** emitted, even for an empty header. The legacy header-less form is never produced.
- A schema switch is emitted only when the effective schema changes.
- A record that fails validation is **not emitted** and the error is returned; the stream never
  carries a record its reader would reject.
- **Sequential, by protocol.** io-specs: "Writer calls MUST be issued sequentially". The type
  is therefore *not* safe for concurrent use and says so, like `bufio.Writer`; a caller who
  fans in from goroutines owns the lock. (Rev 1 promised a mutex — that would have hidden a
  protocol rule behind an API guarantee.)
- Buffered. `Flush` is explicit; `Close` flushes. No syscall per record.

Conformance: no corpus pins a writer directly. The gate is **`Stream(NewStreamMarshaler(…))`
round-trip** — every record marshaled is read back identical by this port's own reader, over
the streaming corpus inputs and under a differential fuzzer — plus the serializer corpus, which
already pins the canonical spelling.

### 4.8 Projection — `Value`, `JSON`

`Value()` is the live view (§4.2). Added:

```go
func (d *Document) JSON(opts *JSONOptions) ([]byte, error)
type JSONOptions struct { SkipErrors bool; Indent string }
```

Decided mappings, so they are testable rather than discovered:

| IO value | JSON |
| --- | --- |
| object | object, **member order preserved** (never through `map`) |
| positional member | key is its index as a string, as `Value()` does |
| number | number |
| bigint | number if it fits `int64`, else string (exact) |
| decimal | string (exact; scale preserved) |
| datetime / date / time | RFC 3339 string; date `YYYY-MM-DD`; time `HH:MM:SS[.fff]` |
| bytes | base64 string |
| failed row | `null`; omitted when `SkipErrors` — indices are *not* renumbered |
| multi-section document | object keyed by section name, as `Value()` does |

A test probes the reference's `toJSON` on the playground samples and records every difference
as a finding — the mapping above is a decision, and where it diverges from the reference that
must be visible, not silent.

### 4.9 Errors

**Delivered.** `Code` is `core.Code`, defined in the value model because `ErrorNode` carries
one — a failed record is a value, so its code belongs to the value model too, and that is what
lets the public package name the codes without `core` learning about anything above it.

`Error.Code`, `ErrorItem.Code`, `ErrorValue.Code` and the stream item's code are all the same
type, so a caller never converts between two spellings. `ErrorList.Has(Code)` and
`Codes() []Code`. Comparison against a bare literal still compiles (untyped constant), so the
change breaks no caller.

A code does **not** determine a `Category`: io-specs requires the category to be derived from
where the fault arose, so `Code` deliberately has no `Category()` method.

The catalogue cannot drift: `TestPublicCodesCoverEveryInternalCode` reads the two files where
codes are declared and fails in **both** directions — an internal code with no constant, and a
constant nothing raises. Verified by breaking it deliberately.

### 4.10 `Decimal`

[SPEC 0002](decimal.md).

---

## 5. Delivery parity — `io-js2/src/index.ts` → Go

| Reference export | Go spelling | Status |
| --- | --- | --- |
| `IOObject` | `Object` | ✅ |
| `IOCollection` | `Collection[T]` (§4.4) | ✅ |
| `IODocument` | `Document` (read) + `Builder` (build) | ✅ |
| `IOSection`, `IOSectionCollection` | `Section`, `Document.Sections()` | ✅ |
| `IOHeader`, `IODefinitions` | `Definitions` (§4.5) | ✅ |
| `IOErrorItem` | `ErrorItem`, `IsError` | ✅ |
| `Decimal` | `Decimal` (SPEC 0002) | ✅ |
| `IOError`, `IOSyntaxError`, `IOValidationError` | `Error` with `Category` (syntax/validation/stream) — one type, a field, not a hierarchy | ✅ |
| `ErrorCodes` | `Code` constants (§4.9) | ✅ |
| `IOSchema`, `parseSchema` | `Schema`, `ParseSchema`, `SchemaFor[T]` | ✅ |
| `parse`, `parseDocument`, `safeParse*` | `Parse`, `ParseWith` — `(v, err)` **is** safeParse | ✅ |
| `parseDefinitions` + `parse(data, defs)` | `ParseDefinitions`, `defs.Parse` | ✅ |
| `load`, `loadObject`, `loadCollection` | `Unmarshal`, `UnmarshalWith`, `SectionAs[T]` | ✅ |
| `loadInferred` | — | out of scope by decision |
| `stringify`, `stringifyDocument` | `Marshal`, `Document.String` | ✅ |
| `stringifyHeader` | `Schema.String`, `Definitions.String` | ✅ |
| `toObject`, `toJSON` | `Value`, `JSON` (§4.8) | JSON ❌ |
| `validate`, `validateObject`, `validateCollection` | `Validate`, `ValidateWith` (structs, slices; maps ❌) | ⚠️ |
| `createStreamReader`, `IOStreamReader` | `Stream`, `StreamAs[T]` | ✅ |
| `createStreamWriter` | `StreamMarshaler` (§4.7) | ✅ |
| `createPushSource`, `BufferTransport` | an `io.Reader` — `io.Pipe` is the push source, `bufio` the transport | ✅ by the standard library |
| `IOStreamError`, `StreamErrorCode` | `Error` with `Category: "stream"`, codes in `Code` | ✅ |
| `proxyDocument`, `proxyValue`, `subscribe`, `version`, tag functions | — | not ported, §6 |

Delivered: 22. Missing: 0. Partial: 1.

---

## 6. Not ported, and why

| Reference | Verdict |
| --- | --- |
| `Revision`, `subscribe`, `version`, `touch` | Exist because JS can intercept property access. Go's answer to "watch a document" is a channel the caller owns; an observer protocol nobody asked for is the overengineering this project rejects. |
| `proxyDocument`, `proxyValue`, `IO_NODE` | Need `Proxy`. No Go equivalent, no demand. |
| `ioDocument`/`ioObject`/… tag functions | Template-literal tags. No Go equivalent. |
| `safeParse` as a second spelling | `(v, error)` already is it. |
| `IOCollection.map/filter/reduce/…` | `slices` + range-over-func. |
| Definitions mutation after parse | §4.5; the `Builder` is where definitions are set. |
| `loadInferred` | Project owner's decision. |

---

## 7. Performance budget

The owner's bar is *better than `encoding/json`*. Current standing (1,000 records, 64 KB IO vs
114 KB JSON, `docs/reports/benchmarks.md` pass 7):

| Operation | io-go | `encoding/json` | standing |
| --- | --- | --- | --- |
| Unmarshal → struct | 1.33 ms · 4,024 allocs | 1.92 ms · 6,019 | ✅ faster |
| Marshal ← struct | 0.33 ms · 22 | 0.39 ms · 2 | ✅ faster |
| Small record decode | 2,144 B · 14 | 480 B · 11 | ⚠️ 1.3× allocs, doing validation JSON does not |
| Parse → dynamic | 1,456 KB · 17,952 | 729 KB · 23,013 | ✅ fewer allocs; ⚠️ 1.65× time |

Rules this spec adds:

1. **Nothing in §4 enters the hot path.** `Builder`, `Collection[T]`, `Definitions`,
   `StreamMarshaler`, `JSON` are layered over what the parser already produces.
2. **Allocation counts are the gate**, per landing, recorded in the benchmarks report. They are
   exact and load-independent; nanoseconds are noise unless the alloc count moved.
3. **`StreamMarshaler.Marshal` of a struct must cost what `Marshal` of that struct costs** —
   it reuses the fast path and the cached header, and must not re-render the header.
4. Dynamic parse remains the one operation slower than JSON in time; ADR 0009's
   copy-on-write projection is the standing plan and this spec does not regress it.

---

## 8. Test obligations — every landing, no exceptions

1. **Corpus**: 1,572 conformance + 262 generated-code cases green. A missing corpus **fails**.
   The serializer suite pins three properties per case — output, re-parse, idempotence — and is
   the gate for everything that writes, including the builder and the stream marshaler.
2. **Playground**: every sample in `io-playground` parses, round-trips and is idempotent (the
   21 pinned today, plus any added). The two intentional-error samples must still report errors.
   Samples seed the fuzzers.
3. **`-race`** clean, including a test that shares one parsed `Document` and one `*Definitions`
   across goroutines while reading.
4. **Fuzzers**: the five existing stay clean; added — builder round-trip
   (`Builder → String → Parse ≡`), stream round-trip (`StreamMarshaler → Stream ≡`),
   `Object` operation sequences (order preserved, positional/keyed separation),
   `Collection[T]` partial binding (`len(Items)+len(Errors) == Len()`).
5. **Coverage**: black-box behaviour plus white-box branches; the module's coverage number is
   reported in the benchmarks report next to the alloc counts, and may not fall.
6. **Benchmarks** run before and after each landing with the numbers recorded — the rule rev 1
   stated and its author broke twice.
7. **Oracle probes**: where this spec *decides* something the corpus does not pin (§4.8's JSON
   mapping, SPEC 0002's divergences), a test probes io-js2 and records disagreement as a
   finding rather than letting it pass silently.

---

## 9. Resolution of the rev-1 review

| # | Rev-1 defect | Resolved by |
| --- | --- | --- |
| 1 | `StreamWriter.Write` — banned vocabulary | §4.7 `StreamMarshaler.Marshal` |
| 2 | `core.Collection` contradicted ADR 0004 D4, had no consumer | §4.4 `Collection[T]` per ADR 0004 |
| 3 | No ownership rule for mutable `Object` vs `Value()` views | §4.2 immutable document; §4.6 builder |
| 4 | Builder `String() (string, error)` collided with `Stringer` | §4.6 `Builder.Document()`; validate at `Add` |
| 5 | `Define` contradicted "no definitions mutation" | §4.5/§4.6: mutation only in the builder, refused once rows exist |
| 6 | OCP claim false | §1.4 states the six sites and the obligation |
| 7 | `Object` doc claims a measurement that was not made | §4.1: "by decision"; code comment to be corrected |
| 8 | `ParseDefs` "already implemented" overclaimed | §4.5 eager compile of all definitions, immutable, specified as new work |
| 9 | Benchmarks not run per landing | §8.6 |
| 10 | `JSON` underspecified; typed codes breaking; per-record flush | §4.8 table; §4.9 compatibility note; §4.7 buffered |

---

## ▶ RESUME HERE

- Delivered: `Object` (§4.1), `Section` errors (§4.3), marshal record dispatch, writer split,
  **`Decimal` (§4.10 / SPEC 0002)**, **`Code` constants (§4.9)**, **`Definitions` (§4.5)**, **`Builder` (§4.6)**, **`Collection[T]` (§4.4)**, **`StreamMarshaler` (§4.7)**.
- **Next, in order, each landing green under §8:** (1) §4.8 `JSON`; (2) §2 file renames, last,
  so history stays readable; (3) hygiene — the five dead functions.
- Open decision for the owner: none. The base-type name for the ADR 0004 Level-1 embedded
  object base is still unchosen (ADR 0004 D4 note) but nothing in this spec depends on it.
