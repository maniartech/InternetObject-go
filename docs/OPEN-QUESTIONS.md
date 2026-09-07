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
