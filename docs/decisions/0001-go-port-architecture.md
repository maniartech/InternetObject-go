# ADR 0001 — Go port: clean restart, layout, and conformance gating

- **Status:** Accepted, 2026-09-02
- **Upstream:** io-js2 ADR 0007 (porting architecture) · `io-test-cases/PORT-START-HERE.md` ·
  `io-test-cases/CONFORMANCE.md` · `io-test-cases/PORTING-NOTES.md`

## Context

The 2025 `io-go` tree (6,500 lines: tokenizer, AST, benchmark write-ups) predates the frozen
error-code grammar, the reduced numeric rules, and the conformance corpus entirely. Auditing it
against today's format costs more than rewriting, and imports assumptions nobody can enumerate.

## Decisions

### D1. Start over; archive the old tree

The old `master` is preserved as `archive/2025-tokenizer`. The new tree starts empty. The module
path stays `github.com/maniartech/InternetObject-go`.

### D2. The conformance runner is commit #1, and a missing corpus FAILS

Commit #1 wires `io-test-cases/bootstrap/tokenizer.csv` and reports 0 passing — the scoreboard
exists before the opinions do. The corpus is located as a **sibling checkout**
(`../io-test-cases`), overridable with `IO_CORPUS_DIR`. If it is absent the suite **fails**; it
never skips. A gate that can silently not-run is not a gate.

The corpus has no version tags yet (upstream ADR 0007 D5 is open), so the harness pins the corpus
**commit hash** in a constant, prints it with every report, and warns loudly when the sibling
checkout's actual HEAD differs from the pin. When the corpus is tagged `v1.0.0`, the pin moves to
the tag.

### D3. Phase order and the gate rule

Suites are adopted in the order fixed by `PORT-START-HERE.md` §3: tokenizer (bootstrap CSV) →
parser → schema → validation → serializer → document → streaming → regression. A phase ends at
100% **with every previous phase still at 100%**. Between phases nothing else lands — no
refactors, no API work, no performance work.

### D4. Layout: internal pipeline, public surface later

```
internal/tokenizer/     the tokenizer (phase 1)
internal/numfmt/        ECMAScript-compatible float formatting (shared: corpus compare, serializer)
internal/conformance/   the corpus harness (test-only)
```

Everything stays under `internal/` until the public API is designed — deliberately, so nothing is
committed to before the format work forces it. The public surface will be added at the module
root in a later ADR, **in Go's idiom**: `(value, error)` returns, an errors slice on results for
accumulate-and-continue, error **codes** and positions exposed via a typed error. Nothing from the
JavaScript API (`safeParse`, tag functions, proxies) is ported.

### D5. Token representation: compact, lazily decoded

A token is a small value struct — kind, sub-kind, error code (all one byte), byte offsets, and
line/column — with **no decoded payload**. Decoding (string unescaping, number parsing, base64)
happens on demand against the retained source text. The token stream is one slice; steady-state
tokenization allocates nothing per token. This serves the near-zero-allocation goal without
speculative machinery.

### D6. One decision, one site

The traps in `PORTING-NOTES.md` all reduce to *one decision implemented at many sites*. This port
makes each such decision exactly once, in one exported function, from day one:

- "is this value a record or a scalar?" — one predicate, value types listed explicitly;
- "how is a string written?" — one writer;
- number/marker classification (Rule 1 all-or-nothing, Rule 2 a-marker-is-a-claim) — one
  classifier in the tokenizer.

### Deferred (decided when a phase forces them)

- **Decimal representation** (scale is part of the value; a float is forbidden) — phase 2.
- **BigInt** — `math/big.Int` — phase 2.
- **Temporal kinds** (`date`/`time`/`datetime` stay distinct end to end) — phase 2 value model.
  **Superseded 2026-09-03 by [ADR 0008](0008-temporal-is-time-time.md):** the kind is
  presentational (validation treats the three as interchangeable, the corpus compares by
  instant), so a temporal is a plain `time.Time` and the kind is chosen on write.
- **Public API shape, recovery idiom, live-vs-JSON projection** — post-phase-2 ADR.
