# Internet Object — Go implementation

A from-scratch, independent Go implementation of the
[Internet Object](https://internetobject.org) data-interchange format, built against the shared
conformance corpus (`io-test-cases`) with the specification (`io-specs`) as the sole authority.

**Status: the full conformance corpus passes** — all eight suites, 1,572 cases
(tokenizer 262 · parser 195 · schema 160 · validation 538 · serializer 148 · document 100 ·
streaming 118 · regression 51), against `io-test-cases` commit `15e02ce`. See
[docs/decisions/](docs/decisions/) for the architecture decisions and
[docs/reports/benchmarks.md](docs/reports/benchmarks.md) for the performance work.

## Usage

### Structs, the encoding/json way

```go
import io "github.com/maniartech/InternetObject-go"

type Person struct {
    Name  string   `io:"name"`
    Age   int      `io:"age"`
    Email string   `io:"email,omitempty"`
    Tags  []string `io:"tags,omitempty"`
}

text, err := io.Marshal([]Person{
    {Name: "Alice", Age: 30},
    {Name: "Bob", Age: 25, Email: "bob@x.io"},
})
// name: string, age: int, email?: string, tags?: [string]
// ---
// ~ Alice, 30
// ~ Bob, 25, bob@x.io

var people []Person
err = io.Unmarshal(text, &people)
```

`Marshal` derives the schema from the struct type and writes the data positionally — the
format's leanness for free. `Unmarshal` validates against the document's schema (faults come
back as the `ErrorList` with designated codes) and also binds schema-less records
(`io.Unmarshal("Alice, 30", &p)` works). A pointer field is nullable (`nil` ⇄ `N`),
`io:"-"` skips, `io:",omitempty"` makes the member optional, and `io:",date"` / `io:",time"`
pick a `time.Time` field's temporal kind.

Constraints go in a `schema` tag whose value is the format's own annotation syntax — no
second mini-language to learn, and the tag text is exactly what the marshaled header carries:

```go
type User struct {
    Name string `io:"name" schema:"{string, minLen: 2, maxLen: 50}"`
    Age  int    `io:"age"  schema:"int, min: 0, max: 130"`
    Role string `io:"role,omitempty" schema:"{string, choices: [admin, user]}"`
}

_, err := io.Marshal(User{Name: "A", Age: 300})
// ErrorList: mismatched-min-len; mismatched-max — Marshal can never emit a
// document that fails its own schema.

err = io.Validate(u)          // the same check on demand, after mutations
s, _ := io.SchemaFor[User]()  // the derived schema; s.String() renders it
```

See [ADR 0003](docs/decisions/0003-struct-marshal.md).

### The dynamic document model

```go
doc, err := io.Parse(`
~ $schema: {name: string, age: {int, min: 0}}
---
~ Alice, 30
~ Bob, 25
`)
// err lists every fault (stable kebab-case codes with positions) and the
// document still holds every record that survived — errors accumulate,
// they don't abort.

records := doc.Value().([]any)   // the live value model
text := doc.String()             // canonical IO text: re-parses to the same
                                 // value, and re-writing it changes nothing
```

Values decode precisely: numbers are `float64`, bigints `*big.Int`, decimals keep their scale
(`1.50m` ≠ `1.5m`), temporals are a native `time.Time`, binary is `[]byte`. A temporal's
`date` / `time` / `datetime` spelling is chosen on write — by the schema when the member
declares one, by the `io:",date"` / `io:",time"` tag on a struct field, and otherwise from the
instant, the same way the writer re-picks a string's open / raw / quoted form.

### Streaming

```go
for item, err := range io.Stream(reader, nil) {
    if err != nil { /* fatal: iteration is over */ }
    if item.Err != nil { /* one bad record; the stream continues */ }
    use(item.Value)
}
```

Chunk boundaries are never semantic — the same input split any way yields the identical item
sequence.

### Schemas

```go
s, err := io.ParseSchema("name: string, age?: {int, min: 0}, *")
```

## Conformance

The corpus is the definition of done. The suite prints per-suite numbers with the pinned corpus
commit, and **fails** (never skips) when the sibling `io-test-cases` checkout is missing
(`IO_CORPUS_DIR` overrides the location):

```bash
go test ./...
```

Divergences between the specification and the reference implementation found by this port are
reported to the format owners and handled through the shared escalation process — per the
corpus's porting guide, that output outranks the library. Where a divergence is deliberate and
still open, the code says so at the site.

## Design

- The **format** is portable; the **API** is not. The public surface is Go idiom —
  `(value, error)` returns with accumulate-and-continue, `iter.Seq2` streaming — designed in
  [ADR 0002](docs/decisions/0002-public-api-v0.md), not transliterated from the JavaScript
  reference.
- Near-zero-allocation tokenizer: compact 20-byte value tokens over the source text, decoded
  lazily; the token stream is a single slice.
- Every easy-to-duplicate decision (record-vs-scalar, string writing, undeclared-member
  defaults, numeric claim rules) lives at exactly one site — the central lesson of the
  reference implementation's bug history.

## History

The 2025 tokenizer/benchmark work predates the frozen error codes and the conformance corpus and
was never checked against either; it is preserved on the `archive/2025-tokenizer` branch. This
tree is a clean restart per upstream ADR 0007.

## License

MIT
