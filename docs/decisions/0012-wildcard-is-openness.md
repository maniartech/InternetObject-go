# ADR 0012 — The wildcard is openness, not a member

- **Status:** Accepted, 2026-09-18. Implemented.
- **Context:** the format spells two different things with `*` in a schema. A **bare** `*` (or a
  typed `*: T`) is the wildcard: "and any other members". A **quoted** `"*"` is an ordinary member
  that happens to be named `*`, which is a key JSON data uses freely (`{"rules": {"*": "allow"}}`).
- **This is not a Go decision.** It is decision **D1** of the format's own decision log, settled by
  the owner and implemented in the reference implementation on 2026-08-19. This ADR records how
  io-go implements it, and why the port got it wrong first.

## D1 — the wildcard lives on `Open` alone

`schema.Schema` publishes the wildcard on `Open` (`nil` = closed, `OpenAny` = bare `*`, a
`*MemberDef` = typed `*: T`) and **never** enters it into `Names`, `Defs` or `Index`. A quoted
`"*"` is compiled like any other member and does enter them. Declaring both collides on
`duplicate-member` — raised by the parser's duplicate-key check, and restated in `compileObject`
so that neither reader depends on the other layer — so the two can never coexist under one key.
That is what makes "is this `*` the wildcard?" a question no code has to ask.

**The port originally stored the typed wildcard twice** — on `Open` *and* under `Defs["*"]` — which
is what the reference did before D1. The decision log is explicit that questions like this one had
to be settled BEFORE any port was written, precisely so that a second implementation would not
reproduce an accident of the first as though it were the design. That is exactly what happened
here.

**What the duplicate cost, measured 2026-09-17:**

- A member named `*` parsed and validated correctly, then **vanished when written**. Four sites
  meant "skip the wildcard" and asked the name instead of the schema: `writeSchemaBody` and
  `nestedSchemaAnnotation` (`internal/document/write-typedef.go`), the record writer's schema-order
  loop and its already-written test (`internal/document/write-record.go`), and the public
  `Schema.MemberNames`. `doc.String()` emitted a header without the member, and re-parsing that
  output failed `unknown-member` — round-trip data loss.
- A **data** key spelled `*` was special-cased in the absorption test (`validate.go`), diverging
  from the reference on three inputs that differ only in how a key is spelled:
  `{a: {*}}` with `~ *: 5` reported `unknown-member`, and `{a: {*}, *}` and `{a: {*}, *: int}`
  reported `missing-value`, where the reference binds all three to `[{"a":{"*":5}}]`.

A first fix kept the duplicate and added a predicate (`IsWildcard`) that every read site had to
remember to call. That was rejected in review: it leaves the trap armed for the next site, and it
had already missed one. Removing the duplicate deletes the question instead, and with it every
call site, the `declared--` corrections, and the hand-forced "treat this as an extra" branch.

**The corpus asserts the wildcard twice, and that is deliberate.** The neutral schema projection
lists a typed wildcard under `open` *and* as the final entry of `members`.
`internal/conformance/schemadef.go` therefore **synthesizes** that entry from `Open` rather than
reading it from `Names` — the reference runner appends it the same way, for the same reason. A bare
`*` sets `open: true` and adds no member.

## D2 — Known deviation: a Go struct field may not be named `*`

`struct-plan.go` refuses an `io:"*"` field tag as "reserved for an open schema". Under D1 the name
is not reserved, so this is a **deliberate, temporary Go-only deviation**, not a reading of the
format: schema-less `Marshal(map[string]any{"*": 42})` already writes the member, and a document
carrying one now round-trips, so only the struct tag cannot express it. Lifting it is a separate
change with its own fast-path surface to check, and is tracked in `docs/OPEN-QUESTIONS.md`.

## Consequences

- `Names`, `Defs` and `Index` hold real members only, so any code iterating them is correct by
  construction. There is no predicate to remember.
- `Schema.MemberNames()` now returns a member named `*` when one is declared. It never returns the
  wildcard, which is reported by `Schema.Open()`. A schema with no members returns an empty slice,
  never nil — `slices.Clone` was rejected for exactly that reason.
- `Schema.fastFor` now accepts a typed `*: T` schema, which used to fail its name-count check
  because the wildcard padded `Names`. A struct has no members beyond its fields, so the wildcard
  never applies and the direct encoder agrees with the tree byte-for-byte; this was verified over
  five schemas and three value shapes, and a typed-wildcard seed was added to
  `FuzzMarshalWithMatchesTreePath` to keep it covered.
- Writers quote the member (`isBareSafeKey` rejects `*` as a first byte), so `"*": int` and the
  wildcard `*: int` stay distinguishable in text. `wildcard_test.go` pins the exact bytes, because
  the two spellings project identical data and a value-only comparison cannot tell them apart.
