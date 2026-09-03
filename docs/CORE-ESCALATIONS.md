# Core-level escalations — things this port cannot decide alone

**Status:** open · **Raised by:** io-go · **Owner:** the format (io-specs / io-test-cases / io-js2)
· **Created:** 2026-09-03

## Why this file exists

Building this port surfaced defects and gaps that are **not Go problems**. Some are in the
spec, some in the reference implementation (io-js2), some are simply missing from the shared
corpus. A port can do one of three things with each, and today there is **no agreed rule for
choosing**:

1. match the reference and stay bug-compatible,
2. follow the spec and diverge from the reference,
3. stop and escalate.

Each port has been choosing case by case, on the judgement of whoever was building it. That is
the actual gap this file records. **The process for handling a core-level defect does not exist
yet** — this file is the interim substitute and the input to designing one.

Until that process exists, io-go's interim rule is: **follow the spec, record the divergence
here and in [FINDINGS.md](FINDINGS.md), and never diverge silently.**

### What a process would need to answer

- Who decides, and how is a decision recorded so five other ports can find it?
- When the spec and io-js2 disagree, which wins **by default** — and does the answer differ for
  a data-loss bug versus a cosmetic one?
- What must land before a decision is "done"? (spec text · corpus case · reference fix · each
  port · a note in every port's findings file)
- How does a port discover that a decision was made after its own release?
- Who may declare a deliberate, temporary divergence, and how is it tracked to closure?

## A. Decided, but not yet landed at the core

### A1. Temporal `min`/`max` compare the declared part, not the whole instant

- **Decision (2026-09-03, format owner):** a bound compares the part the declared type governs
  — dates against dates, clocks against clocks, whole instants only under `datetime`.
- **Why:** the format had already decided the annotation governs precision (any temporal is
  permitted under any annotation, and the writer truncates to the declared kind). Comparison
  was the one place that rule was not applied, so a value could be **rejected on a component
  the same schema discards on output**. Two concrete outcomes, both probed against io-js2:
  `{date, max: d"2024-03-20"}` rejected `dt"2024-03-20T14:30:45.123Z"`, whose date IS the
  bound; and `{time, max: t"15:00:00"}` rejected a 2024 datetime of 14:30 — because a bare
  time-of-day is anchored at 1900-01-01 and 2024 is after 1900, which compares nothing
  meaningful.
- **Done:** io-go (`schema.compareAs`, pinned by `TestTemporalBoundsCompareDeclaredPart`).
- **Still owed:** spec text · corpus cases (none exist — see FINDINGS #22 for three suggested)
  · io-js2 · io-rust · io-python · io-php · io-java · io-dart.
- **Blast radius:** this **changes which documents validate**. Any port not yet updated will
  reject documents io-go accepts. That is the strongest argument for having a process.

## B. Open — need a core-level decision

### B1. A `time` serializes without its milliseconds (reference defect, data loss)

- **The defect:** io-js2 writes `t"14:30:45"` for `t"14:30:45.999"`. The value is intact in
  memory; only serialization loses it — so `parse → toString → parse` **silently changes the
  instant**. `datetime` is unaffected.
- **The spec** (`the-structure/values/date-and-time.md`) gives the canonical Time form as
  `HH:mm:ss.SSS`, so the reference contradicts it.
- **io-go's interim position:** follows the spec, keeps the milliseconds, still elides `.000`.
  **io-go and io-js2 therefore disagree on the same input today.**
- **Why it went unnoticed:** the only corpus case, `serializer/scalars.io :: time_value`, uses
  `t"14:30:45.000"` — a **zero** field. It cannot distinguish "elides a zero" from "drops the
  field", so all six ports could disagree here invisibly. **Suggested case:**
  `~ time_millis, 't"14:30:45.999"', '---\nt"14:30:45.999"'`.
- **Needs:** confirmation that the reference is wrong, then the fix, the corpus case, and a
  check of the other four ports. Related: FINDINGS #1 — the reference also rejects
  `t"143045.123"` on input, so it disregards fractional seconds in **both** directions.

### B2. Twenty other findings, unreported

[FINDINGS.md](FINDINGS.md) holds 22 numbered entries plus a list of suggested corpus cases.
Two are handled above; the rest have never been filed against io-specs, io-test-cases or
io-js2. Per [ADR 0007](decisions/0007-lazy-decode.md) this output **outranks the library** — a
port that finds a format bug and keeps it in its own notes has done half the work.

They fall into three groups, and the process should probably treat them differently:

| Group | Examples | Likely handling |
| --- | --- | --- |
| **Reference defects** | #14 (a self-absorbing schema `$P: {A: $P}` stack-overflows io-js2; io-go reports `unknown-member`) | Fix io-js2 + corpus case |
| **Corpus gaps** — behavior nothing pins, so ports can drift undetected | malformed literal in a header; surplus positionals under an open schema; deferred error under an `any` subtree; headerless stream with preloaded definitions | Add cases; no spec change |
| **Spec silence** — the spec simply does not say | #22 above was one of these | Spec text + case |

**Not yet filed anywhere**, deliberately: the escalation process is being designed first, and
filing 22 issues without an agreed handling rule would just move the backlog.

## C. Recorded here so the reasoning is not lost

- **`Marshal`/`Unmarshal` are safe for concurrent use** (ADR 0003 D7) — but a `pattern`
  constraint made that false in io-go until 2026-09-03, because the compiled regexp was cached
  onto a shared schema. **Worth checking in every port that memoizes anything onto a compiled
  schema**, and worth a corpus-adjacent note that compiled schemas are shared state. No test or
  corpus case anywhere uses a `pattern` through a struct tag.
- **PORTING-NOTES rule 15** (keep the three temporal kinds distinct end to end) presumes a
  *kinded host*. Go's standard library has one temporal type, so io-go uses native `time.Time`
  and decides the kind at write time ([ADR 0008](decisions/0008-temporal-is-time-time.md)).
  Recorded as a deliberate, argued divergence rather than absorbed silently — and an example of
  a rule that should say what a non-kinded host does.
- **No cross-implementation benchmark exists.** No port publishes comparable numbers, so
  "fastest Internet Object implementation" is unmeasured in every port that claims it. A shared
  benchmark payload in io-test-cases would fix that.

## Next step

Agree the process in section "What a process would need to answer", then work B1 and B2 through
it — starting with B1, which is the smallest complete example of the problem: a spec, a
reference, a corpus gap and six ports, all disagreeing about one millisecond.
