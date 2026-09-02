# ADR 0007 — Lazy, token-backed decoding

- **Status:** Proposed, 2026-09-02 — awaiting a decision before implementation.
- **Context:** decode allocates ~22 objects per record where `encoding/json` allocates ~6
  ([reports/benchmarks.md](../reports/benchmarks.md)). The gap is not tuning: we materialize
  the document as a boxed tree, copy it during validation, then bind it into the caller's
  struct. This ADR removes the tree from the typed decode path *without* introducing a second
  grammar.

## The measurement that decides the design

```
BenchmarkDecodeTokenToString    15.45 ns/op    0 B/op   0 allocs/op
BenchmarkDecodeTokenToNumber    25.91 ns/op    0 B/op   0 allocs/op
BenchmarkBoxDecodedString       32.19 ns/op   16 B/op   1 allocs/op   ← the same decode, boxed
```

The tokenizer already decodes lazily — tokens carry positions and values are produced on
demand — and a decoded string is a **substring of the source**, which in Go shares the
backing array and allocates nothing. **Every allocation on the decode path comes from boxing
those values into `any` to build a tree that is walked once and discarded.**

## Where the ~22 allocations per record go today

| Stage | per record | necessary? |
| ----- | ---------: | ---------- |
| boxing each scalar into `any` | ~6 | no |
| parser `Object` + `Members` | ~2 | no |
| `assemble` — a second record | ~2 | no |
| validation `slots` | ~1 | no |
| array `[]any` + boxed elements | ~4 | the `[]string` is real; the `[]any` is not |
| decoded strings + the target slice | ~5 | **yes — this is the data** |
| reflection binding | ~2 | yes |

## D1. The parser records SPANS, not values

```go
// internal/parser — what framing produces instead of a value tree.
type RawMember struct {
    Key  string // "" when positional; a substring of the source, so free
    Tok  int32  // index of this member's first token in the stream
    End  int32  // one past its last token (a scalar spans exactly one)
    Kind uint8  // rawScalar | rawArray | rawObject
}

type RawRecord struct {
    Members []RawMember // a window into ONE per-document arena, not its own slice
}
```

Nothing is decoded here and nothing is boxed. The member descriptors for a whole document
live in one growing arena, so a record costs **zero** allocations to frame.

## D2. Binding decodes straight into the field

```go
func bindRaw(rv reflect.Value, rec RawRecord, s *tokenizer.Stream,
             plan *structPlan, sch *schema.Schema) error {
    for i, m := range rec.Members {
        f := plan.fields[i]                    // schema order == plan order
        field := rv.FieldByIndex(f.index)
        tok := s.Tokens[m.Tok]

        switch field.Kind() {
        case reflect.String:
            if tok.Kind != tokenizer.KindString {
                return fault(errs.ExpectedString, tok)   // position is right there
            }
            field.SetString(s.StringValue(tok))          // 0 allocations
        case reflect.Int, reflect.Int64:
            if tok.Kind != tokenizer.KindNumber {
                return fault(errs.ExpectedInteger, tok)
            }
            field.SetInt(int64(s.Number(tok)))           // 0 allocations
        // …bool, float, bigint, decimal, temporal, bytes, slices
        }
    }
    return nil
}
```

The **type check and the decode are the same step** — if the token is not a number and the
field is an `int`, that *is* `expected-integer`, reported at the token's own line and column.

Constraints (`min`, `choices`, `pattern`, …) run in this loop against the typed local. A
value is boxed **only when a constraint needs the generic checker** — so a plain member never
pays for one.

## D3. Worked example

```
name: string, age: int, email: string, active: bool, score: number, tags: [string]
---
~ Alice, 42, alice@example.com, T, 99.5, [admin, ops]
~ Bob,   25, bob@example.com,   F, 80.0, [user]
```

**Today**, per record: `Object`(1) + `Members`(1) + six boxes(6) + `[]any` for tags(1) +
two boxed tag strings(2) + `slots`(1) + `assemble` object and members(2) + the `[]string`(1)
≈ **17**, plus reflection.

**Proposed**, per record:

| member | work | allocations |
| ------ | ---- | ----------: |
| `name` | `SetString(src[7:12])` | 0 |
| `age` | `SetInt(Number(tok))` | 0 |
| `email` | `SetString(...)` | 0 |
| `active` | `SetBool(...)` | 0 |
| `score` | `SetFloat(...)` | 0 |
| `tags` | one `[]string` + two aliased substrings | **1** |

**≈ 1 allocation per record** — the `[]string` that must exist. `encoding/json` allocates ~6
for the same record because it copies every string out of its `[]byte` input; our source is a
Go `string`, which is immutable, so substrings are safe to alias forever. **On this shape we
would decode with fewer allocations than `encoding/json`.**

## D4. What does NOT change — the reason this is safe

- **One scanner.** The tokenizer is untouched.
- **One grammar.** Framing reuses the parser's existing structure walk (braces, brackets,
  commas, escapes, sections, recovery); it records a span where it currently builds a value.
  There is no second parser — that is the distinction from a "fast decoder", and it is the
  whole point.
- **One validation algorithm.** Member order, absence, null, choices, type checks and their
  designated codes stay exactly where they are; they read a typed local instead of an `any`.
- **The dynamic path is unaffected.** `io.Parse` still materializes a `*Document`, built from
  the same spans, so its behavior — and the corpus that pins it — does not move.

## D5. How it is gated

1. **The corpus already targets this.** 195 parser, 538 validation, 100 document and 118
   streaming cases exercise the grammar; a framing divergence fails them.
2. **A differential mode**, as the encode fast path has: `IO_NO_LAZY=1` forces the tree, and a
   test asserts both routes produce identical values *and* identical error codes for every
   corpus document.
3. **The three fuzzers**, plus a new differential fuzz target over arbitrary documents.
4. Positions improve rather than regress — the token is in hand at the point of failure.

## D6. Phasing

1. Framing produces spans **alongside** the current tree; differential-test the two.
2. `Unmarshal` binds from spans for eligible types; anything else falls back to the tree.
3. Once green, skip building the tree when nobody asks for the dynamic value.

Each phase lands only with the corpus and all fuzzers green, as every phase in this port has.

## Expected outcome

Decode from ~22 to ~2–4 allocations per record, i.e. **at or below `encoding/json`**, with
better error positions. This is also the prerequisite for ADR 0006 F2 (column-wise decoding)
and F4 (materializing only the members a caller touches), which are the two places the format
can beat JSON structurally rather than merely match it.

## Cost and risk, stated plainly

The largest single change in the port so far — it touches the parser's record path and the
decode path. The mitigation is that it adds no new *rules*: framing and binding are mechanical,
the semantics stay where they are, and both are held to the existing route by a differential
test. The honest alternative is to accept ~1.5× on decode and spend the effort on the
developer-experience work instead; that is a legitimate choice, and the reason this ADR is
Proposed rather than Accepted.
