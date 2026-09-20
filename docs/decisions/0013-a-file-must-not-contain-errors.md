# ADR 0013 — A projection may describe errors; a file must not contain them

- **Status:** Accepted, 2026-09-20. Implemented.
- **Context:** parsing **accumulates** faults and returns the document anyway (ADR 0005) — the
  format's promise that one bad record does not cost you the other nine hundred. That promise has a
  second half nobody had written down: the damaged document must not be able to become a damaged
  **file**.
- **The rule comes from the reference implementation**, which settled it first, having shipped the
  same bug: a collected error could become a corrupt file with nothing to signal it. io-go adopts
  the rule, not its spelling — and adds a refusal the reference does not have, for faults that
  leave no failed record at all.

## D1 — Writing refuses a document that carries any fault

`Document.Text(opts *TextOptions) (string, error)` returns `forbidden-error-node` when
`len(doc.Errors) > 0`. `TextOptions.SkipErrors` writes the survivors instead.

**What it was, measured 2026-09-17.** `doc.String()` dropped every failed record and said nothing:

```go
doc, _ := io.Parse("name: string, age: int\n---\n~ Alice, 30\n~ Bob, oops\n~ Carol, 40")
doc.String()   // "name: string, age: int\n---\n~ Alice, 30\n~ Carol, 40"   ← Bob is gone
```

Parse tolerantly, save, lose data — with no error, no option, and a doc comment that promised *"the
output always re-parses to the same value"*, which for this document it did not. **No test covered
writing a faulted document**, in either direction: the whole suite passed before and after the
refusal was added.

**The question is the FAULT LIST, not the shape of the records.** The first implementation scanned
for `core.ErrorNode` records, which is how the reference frames it — and review found that misses most of
the ways a document can be unwritable, because only a *validation* failure produces an error node:

| input | before | |
| --- | --- | --- |
| `~ a: d"2024-99-99"\n~ b: 1` | wrote `---\n~ a: \n~ b: 1` | a deferred malformed literal stays a normal record holding a `core.ErrorValue`, which `appendValue` cannot spell, so it emitted **nothing** |
| `~ meta: d"2024-99-99"\n---\n~ a: 1` | wrote `~ meta: \n---\n~ a: 1` | a header fault marks no record at all |
| `~ $draft: {title: nosuchtype}\n---\n~ a: 1` | wrote `---\n~ a: 1` | the whole definition vanished — and the result *looks* clean and re-parses |

The first two write **corrupt** text and report success, which is worse than the truncation this ADR
set out to fix. Keying on `doc.Errors` covers all of them, and cannot drift from the writer's own
skip logic, because it does not restate it.

Review round 2 found the same corruption surviving on the other branch: `SkipErrors` fell through
to the writer for exactly these faults, because they are not fatal and no record was removed.
See D4.

## D2 — `String` marks the refusal, and the mark is a syntax error

`fmt.Stringer` cannot fail, so `String()` renders the refusal instead of text:

```
{<internetobject: forbidden-error-node at $[1].age (4:8)>
```

The leading brace is never closed, which makes the note a **syntax error** — it cannot be written to
a file and mistaken for a document. That detail is load-bearing and was got wrong TWICE:

1. Without the brace, a fault carrying no position renders as `<internetobject: forbidden-error-node>`,
   and `key: value` is valid Internet Object, so it **parsed** — as a one-member record. Nine of
   eleven parse-recovery faults produced exactly that shape.
2. With the brace but an unsanitised message, a MEMBER NAME closes it. A name may be quoted, so it
   may carry a `}`, and a `#` comments away the position that would otherwise break the parse:
   `~ $schema: {"a} #": int}` produced a note that parsed. Six of seven reachable keys did this.

So the message is sanitised — `}`, `#` and newlines replaced — before it is embedded. The test
asserts unparseability across ten fault shapes plus five `}`-bearing member names, and asserts the
message cannot carry the closer.

The alternatives were worse: returning the survivors is the original bug; returning `""` loses
everything just as silently; panicking in a `Stringer` is not Go.

## D3 — The projections keep describing the failure

`Document.Value()` and `Document.JSON()` still include the failed record, and `Document.Errors()`
still lists the fault. A playground, a report, or an editor needs to SHOW what broke; only the write
to text refuses. This is what makes D1 safe rather than merely strict.

## D4 — `SkipErrors` skips records, and only records

`SkipErrors` can rescue a fault only when its record actually LEFT the document. Two kinds never do:

- **A fault whose record is still there**, holding a value nothing can spell — a deferred malformed
  literal. Without this, `SkipErrors` wrote `~ a: ` for `~ a: d"2024-99-99"` and reported success:
  the very corruption D1 describes, on the branch D1 did not cover.
- **A fault that abandoned the load** — a fatal parse, a bad header, a broken schema binding, a
  failed bare record. Sections after it were never validated, so writing verbatim re-emits the text
  that failed to read.

**One marker covers both.** `errs.Error.Recovered` is set where a fault's record is turned into an
error node — the only three places that know the record has LEFT the document (parse recovery, and
the validation and variable-resolution branches of `load`). `SkipErrors` refuses if any fault is not
marked. Every abandon-the-load path leaves such a fault, so it needs no separate treatment.

That last sentence is the whole lesson, and it took three attempts. A `Doc.Fatal` flag was set at
each `return` that abandoned the load, and review found it had **already missed two of five sites**.
Inverting it — fatal until the single normal exit clears it — fixed that. Then sabotage showed both
remaining `Fatal` mutations changed nothing: `Recovered` had subsumed it. `Doc.Fatal` was deleted.

Getting there exposed a third bug neither the flag nor the inversion would have caught: the
variable-resolution branch marked `Recovered` unconditionally, so a BARE record's fault — which
abandons the load — was counted as skippable. It now reads `Recovered = sec.Collection`, matching
the validation branch.

## D5 — The category is `general`, stated once

`forbidden-error-node` is raised by the library when writing — nothing in the source text or in a
schema check caused it — so `errs.CategoryOf` classifies it `general` through a `generalCodes`
table. The raise site deliberately does **not** set `Category` itself: that would be a second
statement of the same fact, free to drift. The first version did set it, which made the table dead
code — caught by sabotage, since removing the table then changed nothing.

## D6 — A fault inside a collection now names its record

Unrelated to the refusal in principle, but it is what the refusal reports. Parse recovery built its
`ErrorNode` as `core.ErrorNode{Code: …}` — no path, index or position — so a syntax fault inside a
collection could only be reported against the document. Validation had always located its own
faults; recovery now does the same: `~ 1\n~ }` reports `unexpected-token at $[1] (2:3)` where it
used to report `unexpected-token`. Deferred literal faults are located the same way.

This is a **public data change**, not only an internal one: `io.ErrorItem` is an alias of
`core.ErrorNode`, so a failed record reached through `Document.Value()` now carries a category,
path, index and position where it carried zeros. `core.Equal` compares error nodes by VALUE, so a
corpus row asserting a recovered node must spell the new fields or compare codes — noted at
`internal/core/equal.go`.

## Consequences

- `Document.Text` is the serialization entry point; `Document.String` is for display. `Document` also
  implements `encoding.TextMarshaler` by delegating to `Text(nil)`. The README and
  `examples/01-parse` were updated — the example now demonstrates the refusal and then asks for the
  survivors explicitly, which is the behaviour worth teaching.
- `forbidden-error-node` closes one of the four codes the reference had and io-go did not. No
  specification page defines it, and the format's own notes record it as an open question — is it a
  format rule or a library rule? — so this ADR states io-go's answer rather than claiming the
  specification's.
- The reference's message names its skip option as the way past. io-go's `Error` carries a code and a position
  only (codes are the contract, messages are informational), so the escape is documented on
  `ForbiddenErrorNode` and `TextOptions.SkipErrors` instead of in the error text.
- `TextOptions` pairs with `Text` as `JSONOptions` pairs with `JSON`, and is where the rest of the
  write surface belongs when it lands — `emitKeys`, `indent`, `includeTypes`, `sectionsFilter` are
  all still missing against the reference, tracked for SPEC 0008. Nil-able, zero value is the default,
  following `slog.HandlerOptions`.
