# ADR 0004 — The native API: a gradient, not a framework

- **Status:** Accepted (design), 2026-09-02. Implementation is phased; each phase lands only
  with the corpus and all three fuzz layers green.
- **Context:** ADR 0003 shipped tagged-struct Marshal/Unmarshal with `schema`-tag constraints,
  boundary auto-validation, `Validate`, `SchemaFor`. Discussion (2026-09-02) settled how the
  rest of the surface grows: mutation-time validation, documents/sections/collections/
  definitions, runtime schemas, and code generation.

## The governing principle: no mandatory framework types

Plain structs and slices with tags must always work through package functions. The core types
are **opt-in method sugar** layered on the same engine — never a requirement. This is the line
the Go community draws (embedding `gorm.Model` is normal; a mandatory base class is "Java").
Every capability therefore exists at every level of a gradient:

| Level | You write | You call |
| ----- | --------- | -------- |
| 0 — plain | structs, slices, `io`/`schema` tags | `io.Marshal/Unmarshal/Validate/Set/Get` |
| 1 — embedded | the same, plus an embedded base | methods: `emp.Set(…)`, `d.Load(…)` |
| 2 — generated | a `.io` schema file | `iogen` output: typed validating setters |

Same engine, same designated codes, same wire text at every level.

## D0. One verb per direction — the vocabulary law

Named after review feedback (2026-09-02) that the surface said `Parse` in one place and
`Unmarshal` in another without explaining which was which. The rule, which every name in the
module now follows:

| Verb pair | Converts | Precedent |
| --------- | -------- | --------- |
| `Marshal` / `Unmarshal` | Go value ⇄ IO text | `encoding/json` |
| `Parse` / `String` | IO text ⇄ this module's own types (`Document`, `Schema`) | `url.Parse` / `URL.String` |
| `Stream` | incremental reading, record by record | — |
| `Validate` | check a value, produce no text | — |

Two suffixes modify a verb without replacing it: `…With(…, s *Schema)` performs the same
operation against an explicitly supplied schema; `…As[T](…)` performs it producing Go values
of type `T`. So `ParseWith` and `UnmarshalWith` are the runtime-schema forms of exactly the
two base verbs, and `StreamAs[T]` is typed streaming.

**Banned from the public surface**: `Load`, `Read`, `Decode`, `Write`, `Get…`/`Set…`-style
alternatives to the four verbs, and any `…IO` suffix (the package name already says it).
Internals follow the same law — `document.Parse`/`ParseWith`/`ParseSchema` and `Doc.String()`
replaced the former `Load`/`LoadWith`/`CompileSchemaString`/`Write`.

Consequences for the proposed surface: the document base's method is `d.Unmarshal(text)`, not
`d.Load(text)`; generated types get `Marshal()`/`Unmarshal()` methods, not
`MarshalIO`/`UnmarshalIO`.

## D1. Four embeddable bases — and deliberately not five

`io.Object` (record), `io.Document` (multi-section), `io.Collection[T]` (rows + row faults),
`io.Definitions` (typed header). **`Section` gets no base**: a section has no standalone
identity — the struct field + tag IS the section (`Joinees []Employee `io:"joinees"``);
dynamic access goes through `doc.Section(name)`. Adding a base there would be symmetry for
its own sake.

```go
type Employee struct {
    io.Object
    Name string `io:"name" schema:"{string, minLen: 2}"`
    Age  int    `io:"age"  schema:"int, min: 0, max: 130"`
}

type Dashboard struct {
    io.Document
    Defs    AppDefs                 `io:"header"`
    Joinees io.Collection[Employee] `io:"joinees"`
    Stats   Stats                   `io:"stats"`
}
```

## D2. Attachment: the one systemic cost, and its mitigations

Go embedding has no back-pointer: a promoted method receives the base, not the outer struct.
Therefore:

- `io.New[T]()` constructs AND attaches (recursively — values the binder creates inside
  collections are attached automatically; a user must never see "not attached" on a value the
  library built).
- Literal construction attaches once: `emp := &Employee{…}; emp.Attach(emp)`.
- An unattached method call fails loudly with guidance, never misbehaves.
- **Every method has a package-function twin that needs no attach**: `io.Set(&e, "age", 50)`,
  `io.Unmarshal(text, &d)`. Zero-value usability is preserved through the twins.
- Bases are invisible to data: anonymous fields never become members, never marshal, and hold
  only the attach pointer + cached plan.
- Known limit, documented: direct field writes (`emp.Age = -1`) bypass `Set`; the boundary
  (`Marshal`/`Validate`) is the backstop. The unbypassable version is Level 2.

## D3. `optional` vs `omitempty`

Two distinct facts, two spellings: `io:",optional"` marks the member `?` (may be absent; the
zero value is still written); `io:",omitempty"` implies optional AND leaves the zero value
off the wire (json muscle-memory). Implemented with ADR 0003.

## D4. Documents, sections, collections, definitions

- Section ↔ field by `io` tag name; the default unnamed section binds to `io:"data"`.
  Unknown sections are ignored (json's rule); unmatched fields stay zero.
- Marshal writes `~ $name: {…}` schema definitions and binds `--- name: $name`, in field
  order.
- `[]T` is strict (any row fault fails the load); `io.Collection[T]` is tolerant — the
  format's accumulate-and-continue at row level: `Items()`, `Errors()`, `Len()`, `All()`
  (iter.Seq), and `Add(v)` validating on insert.
- `io.Definitions` binds the header: `io:"@name"` fields are typed @variables, plain-named
  fields are plain definitions. Reusable across documents and as `StreamOptions.Definitions`.
- Dynamic navigation on the parsed document: `doc.Section(name)`, `doc.Sections()`,
  `sec.Objects()` (iter.Seq2 of `*io.Object` + row error — a section holds a COLLECTION,
  so its items are objects), `doc.Var`, `doc.SchemaOf`;
  bridges `io.SectionAs[T](sec)` and `io.StreamAs[T](r, opts)`.
- **~~Rename `io.Object` to `io.Record`~~ — WITHDRAWN 2026-09-07.** It had the two words the
  wrong way round. The format's own vocabulary is:
  **an OBJECT is the item in a collection; a RECORD is the item in a stream.**
  So `io.Object` already carries the right name for the value model and keeps it, and `Record`
  belongs to the streaming surface — where io-go currently says `StreamItem`.
  The base therefore needs a name that is not `Object`; that choice is still open, and the
  freeing-up this bullet was written to justify is not needed.

## D5. Runtime schemas — tags are one source, not the source

Tags are design-time. A schema may equally live elsewhere (a registry, a file, a remote
service). The engine already treats a schema as data, so:

- **Works today, zero new API**: a fetched schema is header text — prepend it
  (`io.Parse(schemaText + "\n---\n" + data)`) or hand it to streaming
  (`StreamOptions{Definitions: schemaText}`); validation happens in the engine as always.
- **Planned typed surface** (phase 2): `io.UnmarshalWith(text, &v, s)`,
  `io.ValidateWith(v, s)`, `io.MarshalWith(v, s)` taking a compiled `*io.Schema` (from
  `ParseSchema`, `SchemaFor`, or a fetched document); on the bases, `AttachSchema(s)` makes
  `Set`/`Validate` use it.
- **Precedence**: explicitly attached schema > document header schema > tag-derived schema.
  `io` tags remain the NAME-binding contract in all cases (they say which field is which
  member); an attached schema replaces the constraint/type layer entirely — no merging, so
  there is exactly one authority per load.
- **The three combinations, spelled out** (`schema` tags are always optional):

  | `schema` tags | runtime schema | who validates |
  | ------------- | -------------- | ------------- |
  | none | none | types only (derived from the Go field types) |
  | none | attached/fetched | **the runtime schema** — the expected common case: name-only tags, constraints live in a registry/file/service (examples/05) |
  | present | none | the tag constraints (design-time) |
  | present | attached | the runtime schema wins outright; tag constraints are ignored for that load — never merged |

## D6. Code generation (`iogen`) — Level 2, schema-first

A `.io` schema file is the source of truth; `go:generate iogen` emits a guarded type:
unexported fields, `NewUser(…)` constructor, getters, typed `SetAge(v) error` setters,
static `MarshalIO`/`UnmarshalIO`, and the schema constant embedding the verbatim source
text. **Generated code contains zero semantic logic** — it is static binding that delegates
to the engine (the 1-1 rule); a fully inlined fast path may only ever enter behind
byte-for-byte differential gates against the canonical writer plus the corpus and fuzzers.
Generated output ships with generated differential tests (`MarshalIO ≡ io.Marshal`, setter
verdicts ≡ `io.Validate`).

## Phases

1. `io.Object` base + `Attach`/`New[T]` + package twins `io.Set`/`io.Get` + the
   (the withdrawn `Object`→`Record` rename is no longer part of it).
2. Multi-section binding, `io.Document`/`io.Collection[T]`/`io.Definitions`, dynamic
   navigation, `SectionAs`/`StreamAs`, the `With` functions and `AttachSchema`.
3. `iogen`.

Each phase: gofmt/vet clean, corpus 1,572+262 green, property soak green, byte fuzzers
re-run, examples updated, then commit.
