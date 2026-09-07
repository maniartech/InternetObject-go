# Internet Object for Go

A complete, conformant implementation of [Internet Object](https://internetobject.org) —
a schema-first data format that carries the same records as JSON in roughly half the bytes,
with types, constraints and validation built into the wire format rather than bolted on beside it.

```go
import io "github.com/maniartech/InternetObject-go"
```

```
~ $employee: {name: string, age: {int, min: 0, max: 130}}
--- employees: $employee
~ Alice, 30
~ Bob, 41
```

That document is self-describing: the header declares the shape once, every row after it is
positional, and a reader validates as it goes. The equivalent JSON repeats `"name"` and
`"age"` on every record and checks nothing.

**Status:** feature-complete and conformant. 1,572 corpus cases plus 262 tokenizer cases pass,
along with 12 fuzzers and a `-race` build. Faster than `encoding/json` in both directions on
the typed path. See [Conformance](#conformance) and [Performance](#performance).

---

## Contents

- [Install](#install) · [Five minutes](#five-minutes)
- [The four verbs](#the-four-verbs)
- [Working with structs](#working-with-structs) — tags, constraints, sections
- [Working with documents](#working-with-documents) — dynamic reading
- [Building documents](#building-documents) — when no Go type fits
- [Tolerant reading](#tolerant-reading) — `Collection[T]`
- [Streaming](#streaming) — both directions
- [Shared schemas](#shared-schemas) — `Definitions`
- [Errors](#errors) · [Decimals](#decimals) · [JSON](#json)
- [Performance](#performance) · [Conformance](#conformance)
- [Design](#design) · [Status and limits](#status-and-limits)

---

## Install

```bash
go get github.com/maniartech/InternetObject-go
```

Requires **Go 1.24** (the API uses range-over-func iterators). No dependencies outside the
standard library.

## Five minutes

```go
package main

import (
	"fmt"
	io "github.com/maniartech/InternetObject-go"
)

type Employee struct {
	Name string `io:"name"`
	Age  int    `io:"age" schema:"{int, min: 0, max: 130}"`
}

func main() {
	const doc = `~ $employee: {name: string, age: {int, min: 0, max: 130}}
--- $employee
~ Alice, 30
~ Bob, 41`

	var staff []Employee
	if err := io.Unmarshal(doc, &staff); err != nil {
		panic(err)
	}
	fmt.Println(staff) // [{Alice 30} {Bob 41}]

	text, _ := io.Marshal(staff)
	fmt.Println(text)  // the schema header, then the two rows

	// Validation is native: the `schema` tag is enforced without a document.
	fmt.Println(io.Validate(Employee{"X", -5})) // mismatched-min at $.age
}
```

---

## The four verbs

The whole API is four verb pairs. There is no fifth spelling of anything.

| Verb | Converts | Precedent |
| --- | --- | --- |
| `Marshal` / `Unmarshal` | Go value ⇄ IO text | `encoding/json` |
| `Parse` / `String` | IO text ⇄ this package's own types | `url.Parse` / `URL.String` |
| `Stream` | incremental reading, record by record | — |
| `Validate` | check a value, produce no text | — |

Two suffixes modify a verb without replacing it:

- **`…With(…, s *Schema)`** performs the same operation against an explicitly supplied schema:
  `ParseWith`, `UnmarshalWith`, `MarshalWith`, `ValidateWith`.
- **`…As[T](…)`** performs it producing Go values of type `T`: `SectionAs[T]`.

## Working with structs

### Tags

```go
type Employee struct {
	Name  string   `io:"name"`
	Age   int      `io:"age"      schema:"{int, min: 0, max: 130}"`
	Email string   `io:"email"    schema:"email"`
	Tags  []string `io:"tags,optional"`
	Notes string   `io:"notes,omitempty"`
}
```

- **`io:"name"`** binds the field to a member. This is the name contract, always.
- **`schema:"…"`** adds type constraints, in the format's own memberdef syntax. Enforced by
  `Marshal`, `Unmarshal` and `Validate` alike.
- **`,optional`** marks the member `?` — it may be absent, and the zero value is still written.
- **`,omitempty`** implies optional *and* leaves the zero value off the wire (JSON muscle memory).

`optional` and `omitempty` are deliberately two spellings, because they are two different
facts: "may be absent" and "do not write it when empty".

### Deriving a schema from a type

```go
s, err := io.SchemaFor[Employee]()   // compiled from the struct's tags
fmt.Println(s)                        // name: string, age: {int, min:0, max:130}
```

### Multi-section documents

One document can carry several entity types — the thing JSON has no answer for. Bind the whole
document to a struct whose fields name the sections:

```go
type Dashboard struct {
	Employees []Employee `io:"employees"`
	Alerts    []Alert    `io:"alerts"`
	Stats     []Stat     `io:"stats"`
}

var d Dashboard
err := io.Unmarshal(payload, &d)   // each section validated against its own schema
```

Or take one at a time:

```go
employees, err := io.SectionAs[Employee](doc, "employees")
```

A section the sender did not send is an **error**, not an empty slice — "not sent" and "sent
none" are different facts. And a multi-section document cannot be flattened into a single
slice by accident; that is refused with a message naming the sections it found.

## Working with documents

When the shape is not known at compile time:

```go
doc, err := io.Parse(text)

doc.Value()          // the projected value: *Object, []any, or a scalar
doc.Records()        // the records, failed ones carrying an ErrorItem marker
doc.Sections()       // []*Section
doc.Section("alerts")
doc.Schema()         // the bound schema, if any
doc.SchemaOf("employee")
doc.Var("region")    // a header @variable
doc.String()         // canonical text; re-parses to the same value, idempotent
```

An `Object` is an ordered record with a real API:

```go
rec := doc.Records()[0].(*io.Object)

rec.Get("name")      // (any, bool)
rec.Has("age")
rec.At(0)            // by position — how a positional record reads
rec.KeyAt(0)
rec.Keys()
rec.Len()
for key, val := range rec.All() { … }   // "" is the key of a positional member
```

> **A parsed `Document` is immutable and safe to share across goroutines.** `Value()` and
> `Records()` return **views** of its own objects — mutating what they return changes the
> document. To edit one, clone it with [`NewBuilderFrom`](#building-documents).

## Building documents

For the case no Go type can express — a gateway assembling sections whose shape is decided at
runtime:

```go
b := io.NewBuilder()
b.Define("employee", empSchema).Define("alert", alertSchema)

emp := b.Section("employees", "employee")
emp.Add(map[string]any{"name": "Alice", "age": 30})
emp.Add(io.NewObject(2).Append("name", "Bob").Append("age", 41))

if err := emp.Add(map[string]any{"name": "X", "age": -5}); err != nil {
	// mismatched-min — reported HERE, at the call that caused it
}

doc, err := b.Document()
text := doc.String()
```

A record may be a **struct, a map with string keys, or an `*Object`**. It is validated when it
is added, so a builder can never produce a document its own parser would reject — a property
asserted by a fuzzer over millions of mutations.

Definitions **freeze once a record exists**: `Define` and `Var` after the first `Add` fail the
builder rather than silently changing the header under rows already validated against it.

`NewBuilderFrom(doc)` clones a parsed document so it can be edited without touching the original.

## Tolerant reading

`[]T` is strict — one bad row fails the whole load. That is right for a config file and wrong
for a feed of a million events.

```go
var d struct {
	Events io.Collection[Event] `io:"events"`
}
io.Unmarshal(text, &d)

d.Events.Items()      // the rows that bound
d.Events.Errors()     // the faults of the ones that did not
d.Events.Len()        // rows ATTEMPTED — the two above always sum to it
d.Events.Add(e)       // validates against the section's own schema

for i, e := range d.Events.All() { … }   // i is the DOCUMENT row index
```

Indices in `All()` and `Errors()` are positions **in the document**, not among the survivors,
so a fault and its row always line up.

Absorption is decided **per section**: a fault in a section bound to a plain slice still fails
the load, and a header fault is never absorbed, because it is not a row fault at all.

## Streaming

Reading, record by record, with a schema resolved from the stream's own header:

```go
for item, err := range io.Stream(conn, nil) {
	if err != nil {
		break // fatal: the stream itself failed
	}
	if item.Err != nil {
		log.Printf("record %d: %s", item.Index, item.Err.Code) // recoverable
		continue
	}
	use(item.Value)
}
```

And writing — the other half of the link:

```go
sm, err := io.NewStreamMarshaler(conn, &io.StreamOptions{Schema: s})
sm.Marshal(rec)                      // validated, then framed
sm.MarshalAs(alert, "$alert")        // a heterogeneous stream
sm.Close()
```

A record that fails validation is **not written**, so a stream never carries a row its own
reader would reject. The header is emitted once, before the first record, and the `---`
terminator is always emitted. A schema switch appears only where the effective schema changes.

> `StreamMarshaler` is **not safe for concurrent use**, by protocol: the specification states
> that writer calls must be issued sequentially. Hold your own lock if you fan in from several
> goroutines, exactly as with `bufio.Writer`.

## Shared schemas

The deployment mode the format is built for: a schema agreed out of band, so the wire carries
only data.

```go
defs, err := io.ParseDefinitions(headerText)   // compiled ONCE

doc, err := defs.Parse(payload)                 // payload has no header of its own
for item, err := range defs.Stream(conn, nil) { … }

defs.Schema("employee")
defs.Default()
defs.Var("region")
defs.Names()
defs.String()                                   // render the header back
```

A `*Definitions` is **immutable and safe to share across goroutines** — every definition is
compiled up front, so a header that cannot compile fails here rather than on the thousandth
payload.

Precedence follows the specification: a document's own header wins. A name it defines shadows
the same name here, its own `$schema` replaces this default, and where it declares neither,
these stay in force.

## Errors

A document is an **error collector** as much as a value: it holds the records that survived
beside the ones that did not.

```go
doc, err := io.Parse(text)     // err is an ErrorList; doc is still usable

for _, e := range doc.Errors() {
	fmt.Println(e.Code, e.Path, e.Line, e.Col, e.RecordIndex)
}

for _, sec := range doc.Sections() {
	if sec.HasErrors() { … }    // which SECTION a fault came from
}

for i, rec := range doc.Records() {
	if io.IsError(rec) { … }    // this row failed; the ones around it are fine
}
```

`Error.Code` is a designated, kebab-case `Code` — the conformance contract, identical across
every implementation of the format. Use the constants:

```go
if list.Has(io.MismatchedMin) { … }      // checked at compile time
if err.Code == "mismatched-min" { … }    // still compiles; no caller breaks
```

`io.IsError` is a **type check, not a property check**: a schema may legitimately declare a
member called `__error`, so data can never impersonate a failure.

`errors.Is` and `errors.As` both work, and `ErrorList` gives `Codes()`, `Has(Code)` and
`Unwrap() []error`.

## Decimals

`io.Decimal` is exact fixed-point — for money, and anything a float would quietly corrupt.

```go
price, _ := io.ParseDecimal("19.99")
total := price.Mul(io.DecimalFromInt(3))     // 59.97, exact
tax := total.Mul(rate).Round(2)
q, err := total.Quo(divisor, 4)              // scale is explicit; zero divisor is an error
```

**Scale is part of the value**, so there are two equalities and they are named apart:
`Equal` compares magnitude (`1.5 == 1.50`), `Same` compares magnitude *and* scale. Never use
`==`: the coefficient is a pointer, so `==` is identity.

Multiplication is **exact** — the result carries the sum of the operand scales, because that
is the scale at which a product is exact. `Quo` takes the result scale explicitly, because
division has no exact answer to inherit one from.

In and out of Go's own types:

```go
io.DecimalFromInt(42)                  // exact, any integer type
io.DecimalFromFloat(19.99, 2)          // scale required — a float has no honest one
d.Int64()                              // (int64, bool) — exact or refuses
d.Float64()                            // lossy, and documented as such
d.String()                             // exact, always
```

It implements `json.Marshaler`, `json.Unmarshaler`, `encoding.TextMarshaler` and
`encoding.TextUnmarshaler`, so it survives `encoding/json`, XML, YAML and database drivers —
carried as a **string**, since a JSON number is a double by convention.

## JSON

```go
b, err := doc.JSON(&io.JSONOptions{Indent: "  ", SkipErrors: false})
```

The format carries values JSON has no spelling for, so the mapping is a decision and is stated
rather than discovered:

| IO | JSON |
| --- | --- |
| object | object, **member order preserved** |
| positional member | its index as the key |
| bigint | a number when it fits `int64`, else an exact string |
| decimal | a string — scale and every digit survive |
| datetime / date / time | RFC 3339 / `YYYY-MM-DD` / `HH:MM:SS[.fff]` |
| binary | base64 |
| failed row | `null`, or omitted with `SkipErrors` |

`SkipErrors` drops failed rows **without renumbering** the survivors, because an index carried
by an `Error` refers to the document.

`Document` implements `json.Marshaler`, so it drops into an existing `encoding/json` response.

## Performance

1,000 records × 6 members; 64 KB of IO text against 114 KB of equivalent JSON, decoded into
the same Go structs.

| Operation | io-go | `encoding/json` | |
| --- | ---: | ---: | --- |
| Unmarshal → struct | **1.33 ms** · 4,024 allocs | 1.92 ms · 6,019 | **faster** |
| Marshal ← struct | **0.33 ms** · 22 allocs | 0.39 ms · 2 | **faster** |
| Parse → dynamic | 3.01 ms · 17,952 | 1.83 ms · 23,013 | fewer allocations, more time |
| Validate | 1.37 ms | — | JSON has no equivalent |

This is achieved without giving up validation: every member is type-checked and
constraint-checked on the way in, which `encoding/json` does not do at all.

**Allocation counts are the gate**, not nanoseconds — they are exact and load-independent, and
every change is measured against them. Details and the full history:
[`docs/reports/benchmarks.md`](docs/reports/benchmarks.md).

## Conformance

The definition of done is the shared corpus, [`io-test-cases`](https://github.com/maniartech/InternetObject-test-cases),
which every implementation of the format runs.

```
tokenizer   262/262      validation  538/538      document     100/100
parser      195/195      serializer  148/148      regression    51/51
schema      160/160      streaming   118/118
```

**1,572 + 262 cases, all passing.** A missing corpus **fails** the suite — it never skips.

Beyond the corpus:

- **12 fuzzers**, including differential ones that hold the fast and general paths identical
  (`IO_NO_LAZY`, `IO_NO_FAST_PATH`, `IO_NO_HEADER_CACHE`), and round-trip fuzzers for the
  builder, the stream marshaler and the JSON projection.
- **The playground samples** as a second corpus: every sample parses, round-trips and is
  idempotent, and they seed the fuzzers.
- `-race` clean, `go vet` clean, 8 runnable examples.

## Design

The port has a written design record. If you are reading the code, read these first:

| | |
| --- | --- |
| [`docs/specs/core-value-model.md`](docs/specs/core-value-model.md) | the public surface, and why it is shaped this way |
| [`docs/specs/decimal.md`](docs/specs/decimal.md) | the decimal contract |
| [`docs/decisions/`](docs/decisions/) | 11 ADRs — architecture, API, errors, performance, code generation |
| [`docs/reports/benchmarks.md`](docs/reports/benchmarks.md) | every optimisation pass, with numbers |
| [`docs/OPEN-QUESTIONS.md`](docs/OPEN-QUESTIONS.md) | what is undecided, and what was assumed meanwhile |

Two principles run through all of it:

**One decision, one site.** Every rule — how a string must be spelled so it reads back, what a
bare `---` binds to, which section a fault came from — is stated exactly once. Most bugs found
late in this port were a second copy of a rule that had drifted from the first.

**A writer must never emit text its own reader cannot read back as the same value.** The
round-trip fuzzers exist to enforce that, and they have caught real defects the corpus could
not see.

## Status and limits

**Feature-complete.** Everything the TypeScript reference exports is delivered, except
`loadInferred` (excluded by decision) and the JavaScript-only proxy/notify/tag-function
surfaces, which have no Go equivalent.

Known limits, stated plainly:

- **Dynamic parse is slower than `encoding/json`** in wall-clock time (1.65×), though it
  allocates less. The typed path — the common one — is faster in both directions.
- **`iogen`, the code generator, is experimental.** It handles records; collections, nested
  objects and whole documents are not done. See [ADR 0010](docs/decisions/0010-code-generation.md).
- **CI is not yet running.** The conformance corpus lives in a private repository; the workflow
  exists and needs a token.
- **Six defects are open against the reference implementation**, reported through the project's
  escalation process and awaiting confirmation. None affects this port's correctness; what they
  cost is enforcement, since no corpus case pins those behaviours yet.

## License

**Not yet declared.** This repository carries no `LICENSE` file, which for a public repository
means default copyright — nobody may use, copy or distribute it. That needs deciding before
any release.
