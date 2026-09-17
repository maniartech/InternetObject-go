# io-go — roadmap and document index

One entry point for the Go port. **▶ RESUME HERE is at the bottom**; it always answers where we are,
what is next, and why. Everything else on this page is the map.

## The owner's standing directives

These outrank any recommendation in a spec. Recorded 2026-09-17.

1. **Beat `encoding/json`. Period.** Not "on the typed path", not "within noise" — every comparable
   operation. Where io-go does work the standard library does not (constraint validation, source
   positions, error collection), say so plainly in the report *and* win anyway. Tracked by SPEC 0007.
2. **Custom type serialization is required.** User types must be able to override how they are
   written and read (marshal-overriding methods). Tracked by SPEC 0006, which is SPEC 0004 §B2's
   detailed design.
3. **The TypeScript implementation (io-js2) is the reference to match.** It is the most competent
   implementation so far. Take inspiration for both tests and implementation — *what* it achieves,
   not *how* it spells it: everything must be Go-native and must never read as Java. Its test suite
   in particular is the bar. **Non-goal: JSON inference** (deriving a schema from JSON) — excluded
   deliberately. Tracked by SPEC 0008.

Two rules that predate these and still hold:

- **Spec first.** A non-trivial change gets an in-depth spec, reviewed, before any code. Claims in a
  spec are measured or marked as estimates.
- **An independent reviewer signs off every landing before its commit** (SPEC 0003 §7: performance,
  quality, Go-nativity). Every review round so far has found a real defect.

## Specifications

| Spec | Subject | Status |
| --- | --- | --- |
| [SPEC 0001](specs/core-value-model.md) | The public surface and the core model | Complete (2026-09-07) |
| [SPEC 0002](specs/decimal.md) | Decimal | Implemented (2026-09-07) |
| [SPEC 0003](specs/fast-paths.md) | Fast paths for the schemas people actually write | Approved; 5.6, 5.1, 5.2, 5.3 landed; 5.4 and 5.5 open |
| [SPEC 0004](specs/go-native-api.md) | A Go-native public API | §A fixed and committed; §B2 approved → SPEC 0006; rest of §B awaits the owner |
| [SPEC 0005](specs/dynamic-parse.md) | The dynamic parse builds one tree | Draft; §5 awaits the owner |
| SPEC 0006 | Custom type serialization | Being written (directive 2) |
| SPEC 0007 | Beating `encoding/json` everywhere | Being written (directive 1) |
| SPEC 0008 | Test parity with the TypeScript reference | Being written (directive 3) |

## Architecture decisions

[0001](decisions/0001-go-port-architecture.md) port layout and conformance gating ·
[0002](decisions/0002-public-api-v0.md) the public API ·
[0003](decisions/0003-struct-marshal.md) struct Marshal/Unmarshal ·
[0004](decisions/0004-native-api-design.md) the native API gradient ·
[0005](decisions/0005-error-model.md) the error model ·
[0006](decisions/0006-performance-architecture.md) performance architecture ·
[0007](decisions/0007-lazy-decode.md) lazy decoding ·
[0008](decisions/0008-temporal-is-time-time.md) a temporal is a `time.Time` ·
[0009](decisions/0009-shared-compiled-state.md) compiled state is read-only ·
[0010](decisions/0010-code-generation.md) code generation ·
[0011](decisions/0011-core-model-and-layout.md) the core value model and layout

Also: [OPEN-QUESTIONS.md](OPEN-QUESTIONS.md) (including findings escalated upstream) and
[reports/benchmarks.md](reports/benchmarks.md) (every measured pass, dated).

## Where the code stands

Branch `revamp`, pushed as `origin/finalization`. Gates green on the last run: the pinned conformance
corpus, `-race`, `gofmt`, `vet`, the full suite on Go 1.26 and 1.24, the three forced routes
(`IO_NO_LAZY`, `IO_NO_FAST_PATH`, `IO_NO_HEADER_CACHE`), and the generated-code corpus.

Not ready for `master`: CI has never run (needs a `CORPUS_TOKEN` secret only the owner can create);
private artifacts are still in `finalization`'s history; a stray remote `revamp` branch; review
branches in io-test-cases and io-js2 hold uncommitted changes.

## ▶ RESUME HERE

- **State (2026-09-17):** SPEC 0003's fast-path work is landed through §5.3 and pushed. The owner has
  just issued the three directives above, which open three new work streams.
- **In flight:** SPEC 0006, 0007 and 0008 are being written from three parallel audits — the
  remaining performance gaps root-caused and re-measured, the TS test suite diffed against the Go
  suite, and the custom-marshaling design surface mapped. No code changes until those specs are
  reviewed.
- **Then, in order:** SPEC 0006 (custom marshaling — it changes how types are written, so it lands
  before anything depends on the spelling), SPEC 0007 (performance), SPEC 0008 (test parity).
  SPEC 0003 §5.4 folds into SPEC 0007; §5.5 still needs the owner's remaining §B decisions.
