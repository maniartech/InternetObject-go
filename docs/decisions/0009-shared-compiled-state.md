# ADR 0009 — Compiled state is shared, so it must be read-only

- **Status:** Accepted, 2026-09-03. Implemented.
- **Context:** two optimizations independently arrived at the same requirement — stop rebuilding
  what is already built, and stop copying what is already correct. Both turn on the same
  property: **a compiled schema, and a validated record, are immutable after they are made.**
  One of them was not, and that was a shipped data race.

## D1 — The `pattern` regexp is compiled at compile time (a race fix)

`validateString` compiled a member's `pattern` regexp on first use and cached it on the
`*MemberDef`. That def is reachable from the global plan cache (`planCache`, ADR 0003 D7), so
two goroutines calling `io.Validate` on the same struct type wrote it concurrently.

**This was a real, shipped race**, not a theoretical one — confirmed by `go test -race`, which
reported the read at `validate.go`'s `md.re` against the write at the same site. It survived
because **no test and no corpus case used a `pattern` through a `schema` tag**: the existing
concurrency test validated a struct with no pattern, so it exercised everything except the one
field that was written.

The regexp is now built by `schema.compilePattern`, once, during compilation. Validation only
reads. An invalid pattern is deliberately **not** promoted to a compile error — the reference
reports it per value as `mismatched-pattern`, so the failure is recorded on the def (`reBad`)
and raised at exactly the moment it always was.

**The standing rule this establishes:** nothing may write to a compiled schema after `Compile`
returns. Everything below depends on it.

## D2 — The compiled header is memoized on the header text

Parsing and compiling the header on every `Unmarshal` is invisible at 1,000 records and
dominates at one: it measured as ~45% of the allocations of a 133-byte payload — the shape an
HTTP handler decodes all day.

`internal/document.headerFor` memoizes `(header, schema, framable?)` on the header text, in a
`sync.Map`, following the existing `planCache` pattern rather than inventing a second cache
idiom.

**Why the header text alone is a sufficient key**, on this path specifically: `ParseFramed` has
already declined any `--- $Name` selector, so no section binding can vary; the section it
compiles against is constructed locally with no name; and `Compile` deliberately does not
resolve `@`-references, so variable *values* never enter a compiled schema. Compilation is
therefore a pure function of that text.

**Deliberately not cached:** the `*docDefs`, which is mutable — it memoizes per-name
compilation — so every document still gets a fresh one. Caching it would race.

**Bounds, because header text can be attacker-controlled.** The cache never evicts, so it is
bounded twice: headers over 4 KB are not stored (they amortize their own compilation), and past
1,024 entries it stops storing and degrades to exactly the previous behavior. Cache *poisoning*
is not a risk — an entry is stored only after a pure compile of that exact text, so a wrong key
cannot return a right entry — but unbounded growth would have been a memory-exhaustion vector.

`IO_NO_HEADER_CACHE=1` forces the uncached path, joining `IO_NO_LAZY` and `IO_NO_FAST_PATH`, so
the two can be held identical by running the corpus both ways.

## D3 — The projection is copy-on-write, and therefore a view

Projecting a document does exactly three things: drop absent slots, number positional and empty
keys, recurse. A value with none of those projects **to itself** — and that is not a corner
case, it is every schema-validated record, because `assemble` emits one keyed, present member
per declared slot.

The old code deep-cloned an entire validated document to produce a value-identical copy. It was
the dynamic path's *third* full materialization of every record (the parser builds one,
validation assembles a second) and ~20% of its allocated bytes, on a path where the CPU profile
shows the GC accounting for roughly half the time.

**The cost is aliasing, and it is a public contract change.** `Document.Value()` and
`Document.Records()` now return the document's own objects, so mutating a projection can change
what `String()` writes. Leaf values (`[]byte`, `*big.Int`, `Decimal.Coef`) were always shared
this way; this widens it to containers. Both methods say so.

Renaming keys *in place* would have been the other way to avoid the clone, and it is wrong:
`String()` re-serializes those same records, so a positional record would start emitting
`"0": v`. The corpus would have caught it; recording it here so nobody tries.

## What it measured

Exact allocation counts (the metric this project gates on — ns/op on the bench machine swings
±30% with load):

| | before | after | `encoding/json` |
| --- | ---: | ---: | ---: |
| Small record (133 B) → struct | 7,728 B · 51 allocs | **2,144 B · 14** | 480 B · 11 |
| Dynamic parse (64 KB) | 1,824,815 B · 20,953 | **1,456,610 B · 17,952** | 728,785 B · 23,013 |
| Decode → struct (64 KB) | 1,150,107 B · 4,060 | **1,145,606 B · 4,024** | 390,810 B · 6,019 |

The small-payload path went from 4.6× `encoding/json`'s allocation count to **1.3×**, while
still doing schema binding and per-member validation that JSON does not do at all.

## Gates

The corpus ladder (1,572 + 262) and all seven fuzzers ran clean, under the default routes and
under `IO_NO_HEADER_CACHE=1`, `IO_NO_LAZY=1` and `IO_NO_FAST_PATH=1`; the whole suite also ran
under `-race`. Three new tests gate the cache (distinct headers must not collide, concurrent
use under `-race`, and correctness past the entry and size bounds) and one gates the race fix
(`TestConcurrentPatternValidation`) — the case that had no coverage at all.
