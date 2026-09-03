# ADR 0008 — A temporal is a native `time.Time`

- **Status:** Accepted, 2026-09-03. Implemented; supersedes the `Temporal` carrier introduced
  by [ADR 0001](0001-go-port-architecture.md) §"Temporal kinds" and listed in the public
  surface of [ADR 0002](0002-public-api-v0.md).
- **Context:** the value model had exactly two carrier types — `Decimal` and
  `Temporal{time.Time; Kind}`. `Decimal` is forced by the format (scale is part of the value,
  and Go has no decimal). `Temporal` was not: it existed to hold a `date` / `time` / `datetime`
  tag alongside an instant Go can already represent.

## The question

Does the temporal **kind** belong to the value, or to the way the value is written?

If it belongs to the value, a carrier is required and `time.Time` is insufficient. If it is
presentational, the carrier is an invented type standing between the user and a stdlib type
they already know, and the kind belongs at the writer.

## The evidence — from the reference, not from convenience

Two facts settle it, and neither is an implementation preference:

1. **Validation treats the kinds as interchangeable.** A `date` value satisfies a `datetime`
   member and vice versa; `internal/schema/validate.go` says so in as many words. If the kind
   were semantic, the type check is exactly where it would be enforced.
2. **The corpus comparator ignores the kind.** Temporals compare **by instant**. The shared
   definition of "these two implementations agree" has no opinion on the kind.

Nothing in the format's semantics can observe the difference between `d"2024-03-20"` and
`dt"2024-03-20T00:00:00.000Z"`. That is the definition of presentational.

**The precedent already in the model:** a string may be written open, raw or quoted. Three
spellings, one value, and no host keeps the spelling on the value — the writer re-picks the
leanest form on output. The temporal kind is the same kind of fact, and now gets the same
treatment.

## Decision

`d"…"`, `t"…"` and `dt"…"` all decode to `time.Time`. The kind is decided on **write**:

| the member is | spelling comes from | lossless? |
| -- | -- | -- |
| declared `date`/`time`/`datetime` in a schema | the schema | **yes** — the normal case in a schema-first format |
| a `time.Time` field tagged `io:",date"` / `io:",time"` | the tag | **yes** |
| undeclared (schemaless) | `document.InferTemporalKind`, from the instant | normalized, as a string's spelling is |

`value.TimeAnchor` (1900-01-01 UTC) is the date a time-of-day carries — the reference's
convention, so instants compare across implementations.

**The declared kind truncates on write, and only on write.** A `date` member writes the date
and drops the clock; a `time` member writes the clock and drops the date; a `datetime` member
widens. All six cross-kind combinations were probed against io-js2 on 2026-09-03 and match,
with one deliberate exception: a `time` carrying non-zero milliseconds writes them
(`t"23:59:59.999"`) where the reference drops them, because the spec's canonical Time form is
`HH:mm:ss.SSS` and dropping a non-zero field is data loss ([FINDINGS](../FINDINGS.md) #21).

**The value is never truncated.** `validation/temporal-depth.io` pins both directions — a date
under `time` keeps its 2024 date, a time under `date` keeps its 12:00 clock — under the heading
"the three annotations are not interchangeable". A validator that truncates to the declared
precision fails exactly those two cases; measured, then reverted, with the reason recorded at
`validateTemporal`. So the annotation governs the spelling and never the instant, which is the
same split this ADR makes everywhere else.

One consequence worth stating plainly: because the value keeps its full instant and the writer
truncates, `parse → String → parse` can lose a component that `String → parse → String` never
does. That asymmetry is the reference's too, and the round-trip property the corpus asserts is
the second one.

Only the third row normalizes, and it has nothing to normalize *from*: a schemaless document
never stated which of three interchangeable spellings it meant. This is the one case the
earlier design was protecting, and it is protecting a distinction the format does not make.

## Consequence for PORTING-NOTES rule 15

Rule 15 asks a host to keep the three kinds distinct end to end. It scopes itself to a
**kinded host** and names the types it expects there — Rust's `Temporal`, Python's
`date`/`time`/`datetime`. Go's standard library has one temporal type, so Go is not such a
host, and honoring rule 15 here would mean *inventing* the kinded host the rule presumes.
Recorded as a deliberate divergence in [FINDINGS.md](../FINDINGS.md) rather than absorbed
silently.

## What it costs and what it buys

- **Removed from the public surface:** `Temporal`, `TemporalKind`, `KindDate`, `KindTime`,
  `KindDateTime`, `NewDate`, `NewTimeOfDay`, `NewDateTime`. Added: `TimeAnchor`.
- **For callers:** `v.(time.Time)` instead of `v.(io.Temporal).Time` — no wrapper to learn, no
  conversion at the boundary, every `time` package function directly applicable.
- **Performance: unchanged.** Both types box into an `any` identically; the benchmarks did not
  move. This bought API surface, not speed — claimed here only because it was measured.
- **Breaking**, for anything reading `.Kind` off a value. Pre-1.0, and the replacement (declare
  the type in the schema) is what a schema-first format asks for anyway.

## Gates

`temporal_test.go` pins the four behaviors: a temporal decodes to `time.Time`; a declared type
fixes the spelling (including the two shapes an instant cannot distinguish — a midnight
datetime and a 1900-01-01 date); the struct tags select the spelling; an undeclared temporal is
inferred and preserves its instant. The full corpus ladder (1,572 + 262), the 24k-document
round-trip soak, and all five fuzzers ran clean on the change.
