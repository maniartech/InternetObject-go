# ADR 0011 — The core value model, and the package layout

- **Status:** Accepted, 2026-09-07
- **Context:** a review against the TypeScript reference's own public surface
  (`io-js2/src/index.ts`) and against Go's structural conventions. Two findings, and they
  are the same finding seen from two sides.

## The finding

**io-go is a pipeline with a read-only view bolted on. io-js2 is an object model with a
pipeline attached.**

io-js2 exports nine structural types — `IODocument`, `IOHeader`, `IODefinitions`,
`IOCollection`, `IOObject`, `IOSection`, `IOSectionCollection`, `IOErrorItem`, `Decimal` —
each a real container you can construct, read, mutate, attach a schema to, and serialize.

io-go exports `Document`, `Section`, `Object`, `Member`, `Decimal`, `ErrorItem`. Of those,
`Document` and `Section` are read-only views over parser structures, and `Object` is a bare
struct with one method (`Find`). **There is no public constructor for any of them.** You can
go text → struct and struct → text, and you can look at a parsed document, but you cannot
build one, change one, or hand-assemble a header. That is why the port reads as patchwork:
each capability was added where it was needed by the pipeline, so the value model was never
required to stand on its own.

This is not a Go-vs-TS difference. A Go library can and should offer an ordered-map container
with a real API; `Object` having exported `Members []Member` and nothing else pushes every
caller into manipulating the representation directly, which is the opposite of what the rest
of this port does.

## D1. `internal/core` becomes the value model, with behaviour

The core containers get the API their job requires, in Go spelling — value semantics where
they fit, methods where TS uses getters, no fluent chaining for its own sake:

| io-js2 | io-go | notes |
| --- | --- | --- |
| `IOObject` | `core.Object` | `Get/Set/Delete/Has/Len/Keys/At/KeyAt/Push`, ordered, plus the existing `Find` |
| `IOCollection` | `core.Collection` | new; the item list a section holds, with `At/Set/Delete/Append/Len` |
| `IODefinitions` | `core.Definitions` | promoted out of `internal/document`, made public |
| `IOHeader` | `core.Header` | schema + definitions, `Merge` |
| `IOSection` | `core.Section` | name, schema name, data, errors |
| `IOSectionCollection` | `core.Sections` | `Get(name)`, `At(i)`, `Len` |
| `IOErrorItem` | `core.ErrorNode` | exists |

`map`/`filter`/`reduce`/`some`/`every`/`find` are **not** ported: Go 1.23 iterators plus the
standard `slices` package cover them, and a container that reimplements the standard library
is noise. `Collection` yields `iter.Seq2[int, any]` instead.

**Explicitly not ported:** `Revision`, `subscribe`, `version`, `proxyDocument`, `proxyValue`,
`IO_NODE`. These exist because JS can intercept property access; the Go answer to "watch a
document for changes" is a channel the caller owns, and inventing an observer protocol nobody
asked for is exactly the overengineering this project rejects.

## D2. The capability gaps that are real, and what closes each

Measured against `io-js2/src/index.ts`, excluding `loadInferred` (out of scope by decision):

| Gap | Status | Closes with |
| --- | --- | --- |
| No constructor for any core type | **missing** | D1 |
| `createStreamWriter` | **missing entirely** | a `StreamWriter` beside the existing reader |
| `parseDefinitions` / parse against preloaded defs | **missing** | `ParseDefs`, `ParseWithDefs` |
| `toObject({skipErrors})` | **missing** | `Value` gains an option form |
| `toJSON` | **missing** | JSON projection of a document |
| `stringifyHeader` | internal only | export the existing `document.SchemaText` |
| `ErrorCodes` | 46 codes, all unexported | export them |
| `validate` returning a *result* | error only | the error list already carries it; **no change** |
| `safeParse` | n/a | Go's `(value, error)` **is** safeParse; **no change** |
| tag functions, proxy, notify | n/a | D1 non-goals |

## D3. The layout follows the pipeline, and no file holds two jobs

The root package is 21 files of mixed concern and mixed naming (`document_struct.go` beside
`enc-kind.go` beside `marshal-error.go`). `internal/document` holds the loader, the framed
fast path, the definitions **and** a 1,235-line serializer. Both are fixed by the same rule
already applied to `internal/schema` and `internal/core`:

- **one package per stage**, named for the stage: `tokenizer` → `parser` → `schema` →
  `document` → `writer`;
- **one file per type or per coherent job**, named for it;
- **kebab-case file names throughout** (`enc-kind.go`, not `document_struct.go`), matching
  what `internal/core` and `internal/schema` already do.

Concretely: the 1,235-line `internal/document/write.go` becomes ten `write-*.go` files, one
per job — document, header, typedef, record, number, temporal, string-scan, string, key, and
the exported spellers. It stays in `internal/document` rather than becoming its own package:
every one of those functions is a method on `*Doc` reading its unexported state, so a package
boundary would mean exporting the loader's internals to serve a split that the file names
already achieve. One file per job is the goal; a package per job is ceremony.

The two string files are worth naming apart: `write-string-scan.go` only *decides* whether
text would read back as itself, and `write-string.go` only *emits* the spelling that decision
chose. They were interleaved before, and the question and the answer are different jobs.

Source files are kebab-case (`document-struct.go`, not `document_struct.go`); test files keep
Go's universal `_test.go` spelling.

## D4. Performance is a constraint on this work, not a casualty of it

The measured wins — the two-path architecture (ADR 0006/0007), the header cache (ADR 0009),
generated marshalers (ADR 0010) — are load-bearing and stay. The rule for D1 is therefore:

**the new containers may not enter the hot path.** Parsing keeps building `parser` structures
and `core.Object` exactly as it does today; `Collection`, `Header` and `Sections` are
construction and inspection APIs layered over what the parser already produces. Any change
here is gated by the existing benchmark suite, and a regression outside noise is a defect,
not a trade.

## The contract

This ADR decides; **[SPEC 0001](../specs/core-value-model.md) is the contract to build
against** - the lowering the containers must respect, each type's invariants, the API
signatures, and the test obligations every landing carries.

## ▶ RESUME HERE

Order of work, each landing green (corpus 1,572 + 262, `-race`, five fuzzers):

1. **D3 layout** — mechanical, no behaviour change, makes the rest navigable. ← *in progress*
2. **D1 core containers** + their tests.
3. **D2 gaps**, biggest first: stream writer, `ParseDefs`, `toJSON`/`skipErrors`, exported codes.
