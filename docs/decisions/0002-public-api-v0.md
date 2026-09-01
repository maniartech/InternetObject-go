# ADR 0002 — The public Go API, v0

- **Status:** Accepted, 2026-09-02
- **Context:** the full conformance corpus passes (see PROGRESS.md); the pipeline lives under
  `internal/`. Upstream ADR 0007 D3: the format is portable, the API is not — nothing from the
  JavaScript surface (`safeParse`, tag functions, proxies) is ported.

## Decisions

### D1. One root package, `internetobject`

`import io "github.com/maniartech/InternetObject-go"` — package name `internetobject`, commonly
aliased. The pipeline stays internal; the root package is a deliberate, small facade.

### D2. `(value, error)` with accumulate-and-continue

```go
doc, err := internetobject.Parse(src)
```

`err` is non-nil whenever the document carries faults. It is an `*ErrorList` (an `error` holding
the ordered `[]Error`, each with `Code`, `Line`, `Col`), and the DOCUMENT IS STILL RETURNED —
records that survived record-level recovery are present, faulted ones appear as error markers.
This is the format's accumulate-and-continue promise in Go's idiom: inspect `err` for the
faults, use `doc` for what loaded. There is no "safe" variant; Go has two return slots.

Error codes are the conformance contract; `Error.Code` is the designated kebab-case code.

### D3. The two projections

- `Document.Value() any` — the **live** value model: `*Object` (ordered members), `[]any`,
  `string`, `float64`, `*big.Int`, `Decimal`, `[]byte`, `Temporal`, `bool`, `nil`. Decimal
  scale and temporal kind survive.
- `Document.String() string` — canonical Internet Object text, round-trip safe (the corpus's
  three writer properties).
- A JSON-safe projection (`MarshalJSON`) is **deferred**: its spellings for decimal, bigint,
  binary and temporals must be oracle-derived first, not guessed.

Value types are exported as aliases of the internal model (`Object`, `Member`, `Decimal`,
`Temporal`), so the pipeline and the public surface cannot drift.

### D4. Streaming is an iterator

```go
for item, err := range internetobject.Stream(r, nil) { … }
```

`iter.Seq2[StreamItem, error]` over an `io.Reader`. A recoverable record error arrives as an
item whose `Err` is set (iteration continues); a fatal error ends iteration with a non-nil
`err` on the final pair. Chunk boundaries are never semantic.

### D5. Schemas compile explicitly

`ParseSchema(def string) (*Schema, error)` compiles a schema definition string — the same stage
the corpus's `schemaDef` suite pins. v0 exposes compilation only; validating **native Go
values** against a schema (the "load route") is planned, and when it lands it must run the
validation corpus through that entry point too (upstream CONFORMANCE §7.1 — two ways into
validation must give one answer).

### Deferred, deliberately

- JSON projection (D3), the native-value load route (D5).
- Writer options (indentation, key-emission modes) — the canonical form only, for now.
- A `Decimal` arithmetic API — it is a value carrier today.
