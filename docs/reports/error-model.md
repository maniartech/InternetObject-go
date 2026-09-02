# Error model report — io-go vs the TypeScript reference

**Date:** 2026-09-02 · **Scope:** how faults are collected, what each one carries, and how a
caller reaches them. Sources: `io-js2/src/**`, `io-go/**`, `io-specs/streaming/error-model.md`,
`io-test-cases/CONFORMANCE.md`. Every claim below was verified against the code.

## Verdict

**The collection *mechanism* is sound and matches the reference: we accumulate rather than
fail fast, in the right order, with the right per-record discipline — and that is the one
dimension the conformance corpus actually gates, which we pass. The *content* of each error
is far thinner than the reference's, and four of the gaps are severe enough to call defects
rather than missing features.**

The root cause is structural and worth stating plainly: **error positions are asserted by
zero corpus cases**, so nothing ever forced them to be right. Thirteen sites in this port
construct errors with a hardcoded `Line: 1, Col: 1`, including the two `panic` helpers that
govern *the entire validation and schema-compile surface*. The corpus is the floor, not the
ceiling — exactly as PORTING-NOTES warns.

## How each side works

**TypeScript — an explicit sink in slot three.** Every entry point takes an optional
`ErrorSink = Error[] | ((e: Error) => void)`: `parse(source, defs?, sink?, options?)`,
`validateObject(data, defs?, sink?, options?)`, `load…` likewise. The sink is also the
**fail-fast switch**: with no sink the first error is *thrown*, with the whole bag attached as
`.errors`; with a sink every recoverable fault is reported and good records survive
(`facade/error-sink.ts:37-83`). On top of that there are five more routes to the same
information — `safeParse` result objects, an in-projection `IOErrorItem` replacing each failed
record, `getErrors()` on every container (document, section, collection, object), streaming
items, and a `forbidden-error-node` refusal when a document holding a failure is serialized.
A `reconcileErrors` pass exists specifically to keep the sink and the container-attached
errors reporting the same set.

**Go — the return value is the sink.** Go has two return slots, so no sink parameter is
needed: `Parse` returns `(doc, err)` where `err` is an `ErrorList` carrying every fault *and*
the document still holds the records that survived; `doc.Errors()` returns the same list;
`Validate`/`Marshal`/`Unmarshal` accumulate identically. **This part I would not change** — it
is the idiomatic Go expression of the same contract, and it removes the reference's dual
sink/container reconciliation problem by construction.

## What an error carries

| | TypeScript | Go |
| --- | --- | --- |
| stable code | ✅ `errorCode` | ✅ `Code` |
| position | ✅ real token position | ⚠️ **always `1:1`** except tokenizer/parser faults |
| end position | ✅ `endPosition` | ❌ |
| category (`syntax`/`validation`/`stream`/`general`) | ✅ derived from the error class | ⚠️ computed internally, **dropped at the public boundary** |
| collection index | ✅ `collectionIndex` | ❌ |
| member name / path | ⚠️ inside the message string only | ❌ (not even in the message) |
| human message | ✅ code + fact + position | ⚠️ `"code at L:C"` |
| in-projection marker | ✅ `IOErrorItem`, public, `instanceof`-safe | ⚠️ `value.ErrorNode`, **not exported** |
| predicate | ✅ `io.isError(v)` | ❌ |

## The defects, ranked

**P0 — positions are fabricated for every validation and schema error.** Both `vfail`
([validate.go:45](../../internal/schema/validate.go)) and `fail`
([schema.go:149](../../internal/schema/schema.go)) hardcode `Line: 1, Col: 1`. Measured
side by side on the same document, the reference reports `mismatched-min` at `4:8` (the
offending token) and we report `1:1`. In a 10,000-row document that is the difference between
a usable diagnostic and none. Eleven further sites do the same for resolution errors
(`undefined-schema`, `undefined-variable`, `invalid-definition`), `duplicate-section-name`,
and every streaming item.

**P0 — we discard a correct position we already computed.** A deferred malformed literal
(`value.ErrorValue`) carries true `Line`/`Col` from the tokenizer. When validation surfaces
it, `validate.go:408` calls `vfail(ev.Code)` and **throws the position away**, replacing it
with `1:1`. This one is a regression of data already in hand, not a missing feature.

**P0 — the streaming category is computed and then dropped.** `internal/document/stream.go`
classifies every fault into `syntax`/`validation`/`stream`/`general` — correctly, and the
conformance suite exercises it — but the public `StreamItem.Err` is an `Error`, which has no
`Category` field, so `stream.go:59-63` reads the code and discards the category.
`io-specs/streaming/error-model.md:21-27` makes carrying it a **MUST**.

**P0 — the failed-record marker is unnameable, and our two projections disagree.**
`doc.Value()` puts a `value.ErrorNode` in place of a faulted record — correct, and structurally
what the reference does — but `value.ErrorNode` is not aliased in the public package, so a
caller **cannot type-assert it**; it arrives as an opaque value inside a `[]any` and looks
like data. Meanwhile `doc.Records()` maps that same row to `nil`, losing even the code, while
its own doc comment claims "faulted ones included (as their error markers)". The reference has
both a public type and a deliberately forgery-proof `io.isError()` predicate (`instanceof`,
never a `__error` property check, because a schema may legitimately declare such a member).

**P1 — no collection index, no member path.** The reference stamps `collectionIndex` onto each
error at the collection boundary; we iterate with the index in hand
(`document.go:87-100`) and never record it. And although `MemberDef.Name`/`.Path` are already
populated during validation, no error carries them — so `mismatched-min` arrives with no
indication of *which member* of *which record* failed. This is the gap users will feel first.

## The single highest-leverage fix

Four of the gaps collapse into one plumbing change:

1. add `Line`/`Col` to `value.Member` (`internal/value/value.go`);
2. stamp them in `parser.addMember`, which **already receives the token** and currently uses
   it only to position a `duplicate-member` error;
3. thread the member position plus `md.Path` into `vfail`.

That single change converts the whole validation surface from `1:1` to real positions *with*
member paths. Everything else — category on the public `Error`, `RecordIndex`, exporting the
marker type, an `IsError` predicate, making `Value()`/`Records()` agree — is small and
independent.

## Why this drifted, and what that implies

The conformance corpus asserts **error codes only**. Verified: a regex for position keys
across every live `.io` case returns exactly one hit, and it is a false positive (a schema
member literally named `at`). `CONFORMANCE.md:261` is explicit — *"asserting codes only for
errors"* — and the sole position affordance in the repo (`at: {line, col}`) lives in a stale
YAML section, is marked optional, and **is used by no case**. Our own runner reads neither
`Line` nor `Col`.

So position correctness was never gated in either direction. Any fix must therefore come with
**its own tests**, because passing the corpus will not tell us we got it right — and would not
have told us we got it wrong.

What the corpus *does* gate, we pass: error **order and count** are load-bearing (13 cases
assert multi-code lists; two depend on ordering), and the fail-fast-vs-accumulate discipline
is asserted per record shape. Our runner compares the code list index by index.

## Findings that belong upstream

1. **The spec requires stream-absolute positions** (`streaming/error-model.md:104-109`:
   positions "MUST be stream-absolute… MUST NOT report record-relative positions") — and
   **neither implementation does this**. The reference reports frame-relative positions; we
   report `1:1`. A shared gap, not a port gap.
2. **The corpus cannot gate positions at all.** Adding one optional column to the error case
   tables (and a §8 rule) would let every port be held to it. Without that, every port is free
   to do what this one did.
3. **`CONFORMANCE.md` §2 and §5 describe an abandoned YAML layout** — camelCase codes and
   `message:` assertions, both forbidden by the live contract. §8/§9 describe the real `.io`
   layout. Two sections of the contract document are stale.
4. **§5 and §8 contradict each other on ordering**: §5 says multi-code expectations are
   "order-independent unless `ordered: true`", §8 says "in this order". `ordered: true` is
   implemented nowhere and used by no case; the data follows §8.
5. **The reference has no structured member path either** — it interpolates the path into the
   message string, which no conformance rule can assert. A structured field would serve every
   port.

## Recommendation

Land this as **ADR 0005 — the error model**, with:

```go
type Error struct {
    Code        string // designated, the contract
    Category    string // syntax | validation | stream | general
    Path        string // "$[2].age" — record index and member, structured
    Line, Col   int    // real positions, stream-absolute where applicable
}
```

plus the exported failed-record marker (`io.ErrorItem`), an `io.IsError(v)` predicate,
`Value()`/`Records()` agreement, and `ErrorList` helpers (`Codes()`, `ByCode()`, `errors.Is`
support). Sequencing: this is independent of ADR 0004's phases and touches the parser's value
model, so it is cleanest to do **before** phase 1 rather than after — the embedded bases and
`iogen` both surface errors and would otherwise be built against the thin model.
