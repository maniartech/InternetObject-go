# ADR 0003 — Native struct Marshal/Unmarshal

- **Status:** Accepted, 2026-09-02
- **Context:** the corpus and three fuzz layers are green (PROGRESS.md); the public API v0
  (ADR 0002) exposes Parse/ParseSchema/Stream over the dynamic value model. Requested: working
  with plain Go structs the way `encoding/json` does — tags included — "super easy, intuitive
  to a Go developer".

## Decisions

### D1. The shape is encoding/json's, the format's strengths included

```go
func Marshal(v any) (string, error)      // struct, *struct, []T, map, scalar
func Unmarshal(src string, v any) error  // into *T, *[]T, *map[string]any, *any
```

`string`, not `[]byte`: Internet Object is a text format and everything else in this package
speaks `string` (`Parse(src string)`, `Document.String()`). One conversion at the boundary
beats two inside every call.

**Marshal derives a schema from the struct type and writes it as the header**, then writes the
data positionally — this is the format's whole advantage over JSON, and a Go developer gets it
for free:

```go
type Person struct {
    Name   string   `io:"name"`
    Age    int      `io:"age"`
    Email  string   `io:"email,omitempty"`
    Tags   []string `io:"tags,omitempty"`
}
io.Marshal([]Person{{Name: "Alice", Age, 30}, …})
// name: string, age: int, email?: string, tags?: [string]
// ---
// ~ Alice, 30
// ~ Bob, 25, bob@x.io
```

**Unmarshal accepts both worlds**: a document WITH a header validates against it as usual
(designated codes, accumulate semantics — an invalid document returns the `ErrorList`); a
SCHEMA-LESS record binds positionally by field order and by key for named members, so
`io.Unmarshal("Alice, 30", &p)` just works.

### D2. Tags: the `io` key, json's grammar

- `io:"-"` — skipped.
- `io:"name"` — member name (default: the exact field name; no case munging).
- `io:",omitempty"` — the member compiles as OPTIONAL (`name?`) and is omitted from output
  when the value is the type's zero value.
- A **pointer field** is the nullable marker: it compiles with `*` (null allowed), `nil`
  marshals as `N`, and `N` unmarshals to `nil`.
- `io:",date"` / `io:",time"` on a `time.Time` field selects the temporal kind
  (default `datetime`).
- Embedded structs follow `reflect.VisibleFields` — flattened and shadowed exactly as a Go
  developer expects from encoding/json.

### D3. Type mapping (one table, both directions)

| Go | IO schema type | notes |
| -- | -------------- | ----- |
| `string` | `string` | |
| `bool` | `bool` | |
| `int`, `intN`, `uint`, `uintN` | `int` | unmarshal checks integrality and range |
| `float32`, `float64` | `number` | |
| `*big.Int` | `bigint` | every digit survives |
| `Decimal` (= value.Decimal) | `decimal` | scale is part of the value |
| `time.Time` | `datetime` (`date`/`time` by tag) | UTC on the wire |
| `[]byte` | `bytes` | base64 on the wire |
| `[]T` | `[T]` | |
| `map[string]T` | open object `{*: T}` | key order sorted for determinism |
| nested struct | inline object schema | recursion guarded |
| `any` | `any` (null allowed) | decodes to the dynamic model |

Anything else (channels, funcs, complex) is an `*UnsupportedTypeError`, like json.

### D4. Errors are Go errors, not wire codes

Wire-level faults keep their designated codes (they arrive as the `ErrorList` from the parse
step). Binding faults — a type that cannot hold the value, an unsupported field type — are
ordinary Go errors carrying the field path (`people[2].age: cannot store 3.5 in int`). The
conformance contract governs the wire, not Go's reflect layer.

### D5. Implementation notes

- Marshal builds the SHAPE (`*value.Object` trees) and reuses `schema.Compile` and the
  canonical document writer — the fuzz-hardened round-trip guarantees apply to marshaled
  output by construction, no second writer.
- Per-type field plans are computed once and cached in a `sync.Map`; Marshal/Unmarshal are
  safe for concurrent use.
- Deferred: custom `Marshaler`/`Unmarshaler` interfaces, streaming struct decode
  (`Stream` + bind), `omitzero`. Add when a real need appears (KISS).
