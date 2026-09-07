# ADR 0005 — The error model

- **Status:** Accepted, 2026-09-02
- **Context:** an internal error-model report measured this port against
  the TypeScript reference. The collection *mechanism* is right (accumulate-and-continue, in
  the correct order — the one dimension the corpus gates, which we pass); the *content* of
  each error is far thinner, and four of the gaps are defects rather than missing features.
  Nothing in the corpus asserts positions, so nothing gated them and 13 sites drifted to a
  hardcoded `1:1`.

## D1. What an error carries

```go
type Error struct {
    Code        string // designated kebab-case code — the conformance contract
    Category    string // syntax | validation | stream | general (io-specs error-model)
    Path        string // "$[2].age" — record index and member, structured
    RecordIndex int    // 0-based index within its collection; -1 outside one
    Line, Col   int    // 1-based position of the offending value
}
```

`Code` stays the contract; everything else is diagnostics. `Category` is **derived from the
error's origin, never from the code's spelling** (`io-specs/streaming/error-model.md` requires
this), so it is computed by one exported function in `internal/errs` and shared by the
document and streaming paths — the streaming reader already computed it correctly and then
dropped it at the public boundary.

## D2. Positions come from the value, and the value model must carry them

The reference positions a validation fault on the **AST node of the offending value** (for a
scalar, the token itself), and an absence fault on the **parent record**. Our value model
carries no positions at all, which is why `vfail` had nothing to report.

Therefore `value.Member` and `value.Object` each gain `Line, Col int32`, stamped by the
parser from tokens it **already has in hand** (`addMember` receives the token and used it only
to position a duplicate-member error). This is the single change that converts the whole
validation surface from `1:1` to real positions, and it is a prerequisite for ADR 0006 P4
(slab allocation of members), which is why this ADR lands first.

Cost: 8 bytes per member. Measured against the alternative — a side table keyed by member
identity — the inline fields are cheaper and simpler; a member is already 6 words.

## D3. Paths are structured, not interpolated into a message

The reference puts the member path inside the message *string*, and messages are explicitly
non-normative, so no conformance rule can assert which member failed. We carry `Path` as a
field: `$` for the document root, `$[2]` for the third record of a collection, `.age` for a
member, `[0]` for an array element. This is upstream finding #19.

## D4. A failed record is a value the caller can name

`value.ErrorNode` becomes exported as `io.ErrorItem` — a projected marker carrying the same
fields as `Error` — and `io.IsError(v) bool` is the predicate. Following the reference's
reasoning, `IsError` is a **type check, not a property check**: a schema may legitimately
declare a member called `__error`, so data must never be able to impersonate a failure.

`Document.Value()` and `Document.Records()` must agree: both keep the faulted row in place
carrying its marker. `Records()` returning `nil` (losing the code entirely, while its own doc
comment promised the marker) is fixed.

## D5. `ErrorList` gains the obvious helpers

`Codes() []string`, `Has(code) bool`, and `errors.Is`/`errors.As` support so
`errors.As(err, &list)` and comparison against a sentinel both work. No filtering DSL — KISS.

## D6. Correctness of positions is gated by our OWN tests

The corpus asserts codes only, in every implementation, so it cannot catch a position
regression in either direction (finding #16). Position, path, category and record index
therefore ship with dedicated tests in this repo. Until the corpus gains a position column,
these tests are the only gate that exists anywhere.

## D7. A section owns its errors; a document owns all of them

A document is an **error collector** as much as a value: it routinely holds valid records
beside the ones that failed. `Document.Errors()` is the flat list, in document order — but a
flat list cannot say *which* section a fault came from, because two sections both report
`$[1]` for their second record and nothing in the error itself distinguishes them.

`Section.Errors()` is therefore the attribution, with `Section.HasErrors()` as the predicate.
This mirrors the reference, whose `IOSection.errors` exists for the same reason — and whose
errors carry no path at all, only a position, so per-section reading is the *only* attribution
it has. We keep both: richer paths **and** the section.

Two consequences follow from *"one decision, one site"*:

- **Attribution is recorded where the fault is raised**, by `parser.Document.AddSectionError`,
  which appends to the flat list and the section's list together. It is never re-derived
  afterwards by scanning records for `ErrorItem` markers — that would count every validation
  fault twice, once as the error and once as its marker. All four routes into a section's
  errors (parse recovery, variable resolution, schema validation, deferred literals) go
  through that one call, and a test asserts the totals reconcile: the section lists account
  for exactly the document's faults, no more and no less.
- **Faults that belong to no section stay unattributed.** A header fault or a broken schema
  binding is the *document's*, and it is also fatal — the sections it would have described do
  not exist. `sum(section errors) == len(Document.Errors())` therefore holds for every
  document that got as far as having sections at all.

### On the reference's `IOCollection.errors`

The reference splits this three ways — `IOCollection.errors`, `IOSection.errors` (which
aggregates the collection's plus its child objects'), and `Document.getErrors()` — because
there a collection is a standalone container that can be built and schema-attached on its own.

In io-go a **section is the collection**: `Section.Records()` *is* the item list, and there is
no separate `Collection` type to carry a second error list (ADR 0004 D4). So `Section.Errors()`
covers exactly what the reference splits between `IOCollection.errors` and `IOSection.errors`,
and the aggregation the reference performs at read time we perform at write time. Should a
standalone `Collection` ever ship, it inherits this rule: it accumulates at the point of
failure, and the section it is attached to sees those errors because they were recorded
through the same call — not because a getter walked its items.

## Non-goals

- **End positions / ranges.** The reference has `endPosition`; nothing consumes it here yet.
  Add when a consumer appears.
- **Human messages.** `Error()` stays `code at line:col` plus the path — messages are
  non-normative and inventing prose per code is upstream's call, not a port's.
- **Stream-absolute positions.** Required by the spec, implemented by neither port
  (finding #15). Track upstream rather than diverge unilaterally.
