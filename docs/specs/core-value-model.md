# SPEC 0001 — The core value model

- **Status:** Draft for implementation, 2026-09-07
- **Decides:** nothing. [ADR 0011](../decisions/0011-core-model-and-layout.md) decides;
  this is the contract to build against.
- **Scope:** the container types, their invariants, where each one lives, and what the
  public surface projects. Everything here is gated by the existing corpus, `-race` and
  the fuzzers; nothing here may regress a benchmark.

---

## 1. The lowering, and why the reference's class list does not survive it

io-js2 has one flat set of core classes because JavaScript has one representation. This port
**lowers** a document through five representations, each strictly cheaper to work with and
strictly less able to answer questions about the source:

```
  text                                    string
    |  tokenizer                          - lexical decisions, once
  tokens                                  []Token over the original string
    |  parser                             - structure, no meaning
  parse tree                              *parser.Document - Header, Sections, core.Object
    |  document (bind + validate)         - meaning, via schema
  validated tree                          *document.Doc - same nodes, checked values
    |  root package                       - projection
  Go values                               structs, maps, []any, *io.Object
```

The rule that keeps this honest, and which the measured dependency graph already obeys:

> **A stage may depend only on stages below it.** `core`, `errs`, `numfmt` and `tokenizer`
> depend on nothing; `parser` and `schema` on those; `document` on those; `streaming` on
> `document`; the root package on all of them.

That graph is a DAG today and must stay one. It is why `schema` can compile a typedef without
knowing what a parser is, and why the streaming reader can reuse the document's binding rules
without the document knowing a stream exists.

**The consequence for this work:** the reference's nine core classes are not nine Go types in
one package. They are capabilities, and each belongs at the stage that owns the information it
needs. Cloning the flat list would drag `parser` and `schema` into `core` and collapse the
lowering into a ball of mud.

| io-js2 class | io-go home | why there |
| --- | --- | --- |
| `IOObject` | `core.Object` | a pure value; needs nothing |
| `IOCollection` | `core.Collection` | a pure value; needs nothing |
| `IOErrorItem` | `core.ErrorNode` | a pure value |
| `Decimal` | `core.Decimal` | a pure value |
| `IOHeader` | `parser.Header` | holds unresolved definition *shapes*; parse-stage |
| `IODefinitions` | `document.Definitions` | **a service, not a value** — see §2 |
| `IOSection` | `parser.Section` + `io.Section` | parse-stage data, public projection |
| `IOSectionCollection` | `io.Document.Sections()` | a slice is the Go spelling |
| `IODocument` | `parser.Document` → `document.Doc` → `io.Document` | one per stage |

### 1.1 SOLID, as it applies here

- **SRP.** One type, one job, one file. The two live violations are being fixed: the writer
  held ten jobs in one file (fixed), and `core.Object` held *representation* with no behaviour
  so every caller did its job for it (fixed).
- **OCP.** New *types* extend `internal/schema` by adding a `type-<name>.go` carrying its
  memberdef schema and validator, plus one line in `typedefSchemas`. Nothing else changes.
  This is already true and must stay true.
- **LSP.** There is one substitution relationship in the port and it is load-bearing: the
  **fast path must be indistinguishable from the general path**. It is enforced by
  differential fuzzers under `IO_NO_LAZY`, `IO_NO_FAST_PATH` and `IO_NO_HEADER_CACHE`, not by
  types. Any new fast path inherits that obligation.
- **ISP.** No container implements a wide interface. Go's `error`, `fmt.Stringer` and
  `iter.Seq2` are the only ones anything satisfies. Do not invent a `Container` interface to
  unify Object and Collection — nothing would consume it.
- **DIP.** `core` depends on nothing, which is what makes it the bottom. It must not learn
  about `errs`: a value model that can report errors invites validation to migrate into it.
  Containers report failure with Go's comma-ok or `error`, never `errs.Error`.

---

## 2. `Definitions` is a service, and that decides its API

`document.Definitions` holds a `*parser.Header` and memoizes compiled schemas in two mutable
maps. It is **not a value**: it has identity, it caches, and it is *not safe for concurrent
use*. This is why `framed.go` deliberately caches the compiled `*schema.Schema` across
goroutines but gives every document a fresh `Definitions`.

**Contract.** A `*Definitions` belongs to exactly one document, on one goroutine. Compiled
`*schema.Schema` values are immutable once returned and may be shared freely — a standing
obligation on `internal/schema`, and the `pattern` regexp that was once compiled during
validation was a real data race for exactly this reason (ADR 0009).

**Public surface.** `Definitions` is exposed read-only, as a lookup, not as the reference's
mutable namespace:

```go
func (d *Document) SchemaOf(name string) (*Schema, error)   // exists
func (d *Document) Var(name string) (any, bool)             // to add
func (d *Document) DefinitionNames() []string               // to add
```

Mutation (`set`, `delete`, `push`, `merge`) is **not ported**. A definition is only meaningful
against the records that were validated with it, so changing one after the fact would leave a
document whose records no longer match its own header. To build a document with different
definitions, build the document.

---

## 3. `core.Object` — the record

Delivered. Restated because the rest of the spec depends on its invariants.

**Invariants.**

1. Member order is preserved end to end. Nothing reorders members; the writer emits them in
   the order they are held.
2. A **positional** member has no key and is never returned by a keyed lookup. `Find`, `Get`,
   `Has`, `Delete` and `Keys` skip them.
3. An **absent** member (an empty comma slot, `~ a, , c`) is always positional. Writing to one
   via `Set`/`SetAt` clears `Absent` — a hole that receives a value is no longer a hole.
4. Lookup is O(n). Justified: a key-to-index map would be one allocation per record on the
   hottest path, to index the handful of members a record has.
5. `Clone` separates structure and shares values. Both halves are contract.

**`Members` stays exported.** The pipeline walks it directly and must not pay for accessors.
Callers should use the methods; the type doc says so.

---

## 4. `core.Collection` — the records of a section

Not yet built. It is the one genuinely missing *value*: today a section's records are a bare
`[]any` on `parser.Section`, so nothing carries the fact that they are a collection except a
sibling `Collection bool`.

**The design constraint that dominates everything else:** the parser produces `[]any` on the
hot path and must keep doing so. `core.Collection` therefore **wraps a `[]any`, it does not
replace it**, and conversion in either direction is free:

```go
type Collection struct{ Items []any }
```

**API.**

```go
func NewCollection(cap int) *Collection
func (c *Collection) Len() int
func (c *Collection) At(i int) (any, bool)          // reported, never panicked
func (c *Collection) SetAt(i int, v any) bool
func (c *Collection) Append(v ...any) *Collection
func (c *Collection) DeleteAt(i int) bool
func (c *Collection) All() iter.Seq2[int, any]
func (c *Collection) Clone() *Collection            // structure copied, values shared
```

**Deliberately absent:** `map`, `filter`, `reduce`, `some`, `every`, `find`, `findIndex`,
`join`, `includes`, `indexOf`, `lastIndexOf`. `slices` and range-over-func cover every one, and
a container that reimplements the standard library is noise a reviewer has to read past.

**Error reading.** The reference's `IOCollection.getErrors()` walks its items looking for error
markers. This port does not: errors are attributed **where the fault is raised**
(ADR 0005 D7), so `Section.Errors()` already answers this and re-deriving would double-count
every validation fault. `Collection` gets no error API.

---

## 5. What the public surface gains

Ordered by value. Each lands separately, green.

### 5.1 Building a document without a Go type — the headline gap

Today a document can be built only from a Go struct. The dynamic case — a gateway assembling
sections whose shape is known at runtime — has no answer. After §3/§4 it does:

```go
doc := io.NewDocument()
doc.Define("Employee", empSchema)                       // header definition
sec := doc.AddSection("employees", "Employee")          // named, schema-bound
sec.Add(io.NewObject(2).Append("name", "Alice").Append("age", 30))
text, err := doc.String()                               // validates on the way out
```

**Contract.** `String` validates every record against the section's bound schema and returns
an `ErrorList` if any fails — a builder must not be able to emit a document its own parser
rejects. A section with no schema is written header-less and validates nothing.

### 5.2 `StreamWriter` — the missing half of streaming

`Stream` reads. Nothing writes. The reference has `createStreamWriter`, and a port that can
consume a stream but not produce one cannot be used on both ends of a link.

```go
func NewStreamWriter(w io.Writer, s *Schema) (*StreamWriter, error)
func (w *StreamWriter) Write(v any) error   // one record, validated, then flushed
func (w *StreamWriter) Close() error
```

**Contract.** The header is written once, before the first record. `Write` validates against
the schema and emits nothing if validation fails, so a stream never carries a record its
reader would reject. Safe for concurrent `Write` — a writer is exactly the thing several
goroutines hand records to. *(This is the one place in the port that needs a mutex; §1's LSP
note does not apply, there is no second path.)*

### 5.3 Parsing against preloaded definitions

The reference has `parseDefinitions` plus `parse(data, defs)`, so a fixed header can be
compiled once and reused across many payloads — the streaming and RPC case.

```go
func ParseDefs(src string) (*Defs, error)
func ParseWithDefs(src string, d *Defs) (*Document, error)
```

**Contract.** A `*Defs` is immutable once returned and safe to share across goroutines. This
is the same guarantee `headerFor` already relies on internally, promoted to public API — which
means it is already implemented and already fuzzed, it just has no name yet.

### 5.4 Projection options

```go
func (d *Document) JSON() ([]byte, error)          // toJSON
func (d *Document) ValueSkippingErrors() any       // toObject({skipErrors:true})
```

`skipErrors` **drops failed rows**; it does not renumber the survivors' record indices — an
index in an error refers to the document, not to the filtered projection.

### 5.5 Exported error codes

46 designated codes live unexported in `internal/errs`. The reference exports `ErrorCodes`,
and a caller comparing `err.Code` against a string literal today gets no compile-time check at
all. Export them as typed constants.

---

### 5.6 `Decimal` — comparison, arithmetic, precision

`core.Decimal` is a coefficient and a scale with a `String()`. No constructor, no comparison,
no arithmetic; the behaviour lives as unexported helpers inside `internal/schema`. Specified
separately in **[SPEC 0002](decimal.md)**, which also records two divergences from the
reference the analysis exposed (precision of `0.05m`; `choices` on decimals).

---

## 6. What is not ported, and why

| Reference | Verdict |
| --- | --- |
| `Revision`, `subscribe`, `version`, `touch` | JS intercepts property access; Go's answer is a channel the caller owns. Inventing an observer protocol nobody asked for is the overengineering this project rejects. |
| `proxyDocument`, `proxyValue`, `IO_NODE` | Needs `Proxy`. No Go equivalent, no demand. |
| `ioDocument` / `ioObject` / tag functions | Template-literal tags. No Go equivalent. |
| `safeParse` | Go's `(value, error)` **is** safeParse. A second spelling would be the ceremony this API exists to avoid. |
| `loadInferred` | Out of scope by the project owner's decision. |
| `IOCollection.map/filter/reduce/...` | `slices` plus range-over-func. §4. |
| `Definitions` mutation | §2. |

---

## 7. Test obligations

Nothing in §5 lands without all of:

1. **The corpus stays green** — 1,572 conformance cases plus 262 generated-code cases. A
   missing corpus case FAILS the run; it never skips.
2. **`-race` clean**, including the new `StreamWriter`, which must have a test that writes
   from several goroutines at once.
3. **The five fuzzers stay clean**, and anything with two paths gains a differential fuzzer
   asserting they agree.
4. **Round-trip.** Anything that can be built must survive `String` → `Parse` → equal, and a
   second `String` must be byte-identical. The builder joins `FuzzParse`'s property set.
5. **Benchmarks.** ADR 0011 D4: none of this may enter the hot path. `Parse`, `Unmarshal`,
   `Marshal` and the small-payload benchmarks run before and after each landing, and a
   regression outside noise is a defect, not a trade.

---

---

## Review 2026-09-07 — open items, not yet resolved

A critical pass against the code found these. They are recorded here so the spec is not built
against as written; each is resolved by revising the section named, not by implementing around it.

1. **§5.2 `StreamWriter.Write` is banned vocabulary** (ADR 0004 D0 lists `Write`). Reshape
   as a function like the reader's `Stream`.
2. **§4 `core.Collection` contradicts ADR 0004 D4**, which already specifies `io.Collection[T]`
   with a job (tolerant row binding: `Items/Errors/Len/All/Add`). §4's type has no consumer.
   Drop §4; build ADR 0004's.
3. **Ownership is unstated.** `Value()`/`Records()` return views; `Object` is now mutable; so
   editing a projected record rewrites the parsed document and races if shared. Needs a rule:
   a parsed `Document` is immutable and shareable; mutation belongs to the builder.
4. **§5.1 `String() (string, error)` collides with `Document.String() string`.** Make the
   builder its own type and validate at `Add` (also what the reference does).
5. **§5.1 `Define` is definitions mutation, which §2 forbids.** Resolve: allowed before any
   record is validated (builder), never on a parsed document.
6. **§1.1 OCP claim is false** — adding a type also touches `family.go`, `validate.go`,
   `write-typedef.go`, `gen/types.go`, `enc-kind.go`. Fix the code or drop the claim.
7. **`Object` doc says the scan was "measured"; it was not.** Reword or measure.
8. **§5.3 overclaims** — `headerFor` caches the default schema only and declines variables
   and `$Name` selectors; a shareable `Defs` is not "already implemented".
9. **§7.5 benchmarks were not run for the last two landings.** Allocation counts unchanged
   on the three hot benchmarks, but the rule was broken by its author.
10. **§5.4 `JSON()` underspecified**: Decimal/bigint precision, temporal form, positional
    keys, error rows, key order. **§5.5** typed codes change `Error.Code`'s type — breaking.
    **§5.2** per-record flush is a syscall per record; buffer, flush on `Close`.

## ▶ RESUME HERE

- §3 `core.Object` — **done**.
- Marshal record dispatch — **done** (three defects fixed; `isRecordType`).
- **Next:** resolve the review items above (revise §4, §5.1, §5.2, add an ownership section),
  then SPEC 0002 Decimal, then §5.1 the builder, then the stream writer, then §5.3–5.5.
