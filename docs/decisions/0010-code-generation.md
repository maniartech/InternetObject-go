# ADR 0010 — Code generation (`iogen`)

- **Status:** Accepted, 2026-09-05. **Implemented in its delegating form.** The inlined form is
  deliberately deferred — see D4, which names exactly what it needs and why it is not built yet.
- **Implements:** [ADR 0004](0004-native-api-design.md) D6, whose text this ADR supersedes on two
  points (naming, and what "zero semantic logic" costs).
- **Does NOT depend on** ADR 0004 phases 1-2. `iogen` is the *Level 2* alternative to the
  embeddable base, not a layer on top of it: generated types are self-contained.

## D1. What is generated, and why a guarded type is the point

`//go:generate go run .../cmd/iogen -schema person.io -type Person` emits a type whose fields are
**unexported**, plus a constructor, getters, typed setters, an embedded verbatim schema constant,
`Validate`, `Marshal` and `Unmarshal` — and a generated test file.

The guarantee is the whole reason it exists: **a value of this type that exists is one the schema
accepted.** The tagged-struct API cannot give that, because nothing stops `p.Age = -1`. A setter
revalidates the whole record, because a guarded type's invariant is that it is valid, not that each
field was valid alone.

**Naming follows [ADR 0004](0004-native-api-design.md) D0, which already corrected D6's own text:**
generated types get `Marshal()`/`Unmarshal()`, never `MarshalIO`/`UnmarshalIO` — the `…IO` suffix is
banned from the surface because the package name already says it.

## D2. It declines what it cannot bind exactly

`$`-references, `anyOf`, nested object schemas, open schemas, arrays with no element type, member
names that cannot be spelled as Go identifiers, and members colliding with the type name all fail
**loudly at generation time**. Emitting code that binds a shape approximately would be worse than
emitting none: the failure would surface as wrong data at runtime instead of an error at build time.

Measured against the corpus: 698 rows carry a schema and `iogen` accepts **542 (78%)**.

## D3. Generated code faces the corpus

Generated types are a **third path** alongside the tree and the framed one, and every path here is
held to the shared corpus — a path checked only by its own hand-written tests is a path that drifts.

`internal/gen`'s corpus gate generates a type per corpus schema, compiles them into one package, and
asserts for **442 cases** that

```
generated:  v.Unmarshal(input)          then v.Marshal()
engine:     io.UnmarshalWith(input, &twin, s) then io.MarshalWith(twin, s)
```

agree on the error **and byte-for-byte** on the output. `IO_GEN_CORPUS=1`, ~12s, wired into CI.

Today the generated path delegates, so agreement is near-tautological. **That is the point:** the
gate is green *before* anything is inlined, so the day a generated writer stops delegating, this is
what catches the divergence. D6 permits an inlined path only behind such a gate; the gate therefore
comes first.

It earned its keep on its first run, finding three name-collision bugs the hand-written example could
never have hit — all of them user-facing:

- package-level vars named from the one-letter **receiver**, so `User` and `Upload` in one package
  would not compile;
- the constructor's local *was* the receiver, colliding with its own parameter when a member shared
  that name;
- the generated tests' local shadowed `*testing.T` for any type starting with `T`.

Fixed at the root: every generated identifier is picked against the set of field names plus the
reserved `v` and `t`. A member name is **arbitrary user input** and can no longer collide with
generated scaffolding.

## D4. Delegation now; inlining deferred, with the reason measured

Generated code delegates every wire crossing to the engine. That is correct and it is slow:

| single record | ns/op | B/op | allocs |
| --- | ---: | ---: | ---: |
| generated `Marshal`, as first written | 5,108 | 2,992 | 57 |
| **generated `Marshal`, now** | **~2,300** | **1,312** | **17** |
| hand-tagged struct via `io.Marshal` | ~1,900 | 1,040 | 21 |
| `encoding/json` | ~450 | 192 | 2 |
| **inlined prototype (not shipped)** | **~250** | **176** | **1** |

Two library changes got 57 → 17 allocations with no public API change and no new semantics — both
recorded in [reports/benchmarks.md](../reports/benchmarks.md): the schema header is rendered once per
`*Schema` instead of on every call, and `MarshalWith` stopped building document scaffolding and a
validated tree it discarded. **Generated code now allocates less than the hand-tagged struct.**

**The inlined form can beat `encoding/json`** — the prototype does, ~250 ns against ~450, while
emitting a *self-describing* document (174 bytes to JSON's 97). It gets there by making the header a
constant, writing fields straight into one buffer, and compiling constraints into `if`s.

### Why it is not built yet

Reaching that number requires generated code to make **format spelling decisions** — when a string
may be written bare, how a number or temporal is spelled. Those decisions already exist, exactly
once, in `internal/document`. Generated code lives in the **user's package** and cannot import an
internal one, so an inlined generator must either call exported helpers or *re-derive* them in every
generated file.

Re-deriving them is not a theoretical risk. The first version of the prototype's string speller
**quoted `alice@example.com`**: it treated `@` as always-significant, when `@` introduces a variable
reference only at the *start* of a value. One realistic email address was enough to prove a copied
format decision wrong, and the committed prototype is merely *closer* — "closer" is not a standard.

**So the decision is not "inline it" but "export the speller, then inline it."** That is a public API
expansion — append-style spelling helpers, plus a way to write records without a header — and it
belongs to whoever owns the surface, not to this ADR.

### The trigger for revisiting

When the encoder primitives are exported. At that point the work is: teach the generator to emit the
inlined form, keep the delegating form as the fallback for shapes it declines, gate the pair with
`IO_NO_INLINE_GEN=1` the way `IO_NO_LAZY` and `IO_NO_FAST_PATH` gate the other two fast paths, and
let the D3 corpus gate hold them byte-identical. **The gate already exists**, which is the single
most useful thing this ADR leaves behind.

## D5. What is still on the table, in order

1. **The intermediate encode tree** (~37% of what remains). `Marshal` calls `marshalFast` and skips
   it; `MarshalWith` never does, so it builds a tree the other entry point has avoided since pass 4.
   Not a drop-in: `marshalFast` derives its schema from struct tags, and `MarshalWith` must use the
   supplied one and emit members in **its** order. No public API needed.
2. **Exported spellers + the inlined writer** (D4). The step that crosses `encoding/json`.
3. **Collections.** Generated types are single records today; `[]Person` goes through the engine.
4. **Nested object schemas**, which would generate nested types and lift the largest decline in D2.
