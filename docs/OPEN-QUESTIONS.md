# Open questions

Recorded rather than blocked on, per the owner's instruction (2026-09-07): when a decision is
genuinely the owner's, write it here and move to the next task that can be finished
independently. Each entry states what was assumed so the work could continue, and what would
change if the answer differs.

## 1. A corpus gap: no streaming case uses a bare-expression header

**Found** 2026-09-07 while building the `StreamMarshaler`.

`Parse` and `Stream` disagreed about what a bare `---` binds to when the header is a bare
schema EXPRESSION (`name: string, age: int`) rather than a `$schema` definition. `Parse` bound
the members by name; the stream reader read them positionally, because its `defaultSchema()`
carried a partial copy of the resolution that had forgotten the expression form.

**Fixed here** — both now go through `document.DefaultSchemaOf`, one statement of the rule.

**The question for the corpus:** nothing pinned this. All 118 streaming cases use `$schema:` or
a named `$P`, so a port can get the expression form wrong and stay green. A case would close it:

```
~ inline_expression_header, "name: string, age: int
---
~ Alice, 30",
  { items: [{ kind: record, recordIndex: 0, value: { name: "Alice", age: 30 } }] }
```

**Assumed meanwhile:** the stream must agree with `Parse`, since the streaming spec's
equivalence rule says a stream and a document carrying the same records mean the same thing.

## 2. `Document.JSON` mapping is decided, not derived

SPEC 0001 §4.8 fixes a mapping per value type (decimal → string, bigint → number-or-string,
temporals → RFC 3339, positional members → index keys). The reference's `toJSON` was not
probed case by case before this was written down.

**Assumed:** the table in §4.8. Where a probe later shows the reference differing, that is a
finding to record, not a silent change — the mapping is a decision this port is entitled to
make, since io-specs does not cover JSON projection.

## 3. Is ADR 0004's Level-1 embedded surface still the plan? — **for the owner**

ADR 0004 describes a gradient: Level 0 (plain structs plus package functions), Level 1 (an
embedded base giving `emp.Set(…)` methods), Level 2 (`iogen`). **Level 1 has never been
built** — no object base, no `New[T]`, no `Attach`, no `io.Set`/`io.Get` package twins,
no `StreamAs[T]`.

**Why this is a question and not a task.** SPEC 0001 has now delivered every capability the
reference exports without it. Level 1 buys mutation-time validation on a struct
(`emp.SetAge(-1)` failing at the call), and ADR 0004 D2 is candid that it costs an attachment
step Go embedding cannot do for you — `io.New[T]()` or `emp.Attach(emp)` — plus a documented
hole where a direct field write bypasses it. The same guarantee already exists twice over
without any of that: `Builder.Add`, `Collection[T].Add` and `StreamMarshaler.Marshal` all
validate at the call, and `iogen` (Level 2) gives the unbypassable version.

**Assumed meanwhile:** not building it. Nothing in SPEC 0001 depends on it, and the parity
table is complete without it.

**What would change the answer:** a caller who wants a *plain struct* to reject a bad field
write in place. If that case is real, Level 1 is worth its cost; if the answer is "use the
generated type", ADR 0004's phase list should be amended to say so.

Two related loose ends ADR 0004 D4 leaves open, both blocked on this: the name for the
embedded base (the `Object`→`Record` rename was withdrawn 2026-09-07), and `StreamAs[T]`,
which is a small typed wrapper over `Stream` and could ship independently.
