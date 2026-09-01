# ▶ RESUME HERE — Go port progress

Read this first in a fresh session, then `docs/decisions/0001-go-port-architecture.md`, then the
recent `git log`. The upstream cold-start guide is `io-test-cases/PORT-START-HERE.md`; the traps
are `io-test-cases/PORTING-NOTES.md`.

## Current state — THE FULL CORPUS IS GREEN

Every suite passes, against `io-test-cases` commit `0fc0af8` (still untagged upstream — ADR 0007
D5 remains open; when `v1.0.0` lands, move the pin in
`internal/conformance/corpus.go` and re-run):

| Phase | Suite | Result |
| ----: | ----- | ------ |
| 1 | Tokenizer (bootstrap CSV) | **262/262** ✅ |
| 2 | Parser | **195/195** ✅ (valid 123, invalid 72) |
| 3 | Schema | **160/160** ✅ (valid 120, invalid 40) |
| 4 | Validation | **538/538** ✅ (valid 311, invalid 227) |
| 5 | Serializer (round-trip ×3 properties) | **148/148** ✅ |
| 6 | Document | **100/100** ✅ (valid 77, invalid 23) |
| 7 | Streaming (×3 chunkings) | **118/118** ✅ |
| 8 | Regression | **51/51** ✅ (valid 29, invalid 22) |

Run `go test ./...` — the suite prints these numbers with the corpus pin, and fails (never
skips) when the sibling corpus checkout is missing.

## Architecture (all under `internal/` until the public API ADR)

```
internal/tokenizer     zero-alloc-per-token scanner; lazy decode; ERROR tokens, never throws
internal/parser        syntax → Document (header defs, sections, records); accumulate+recover
internal/schema        compile (fail-fast) + validate (the full check order, one site per rule)
internal/document      the pipeline top: Load (bind/validate/project), Write (canonical),
                       Reader (streaming)
internal/numfmt        ECMAScript-compatible float formatting
internal/value         the value model + IsScalar predicate + corpus equality
internal/conformance   the corpus harness (all six comparators)
```

## What's next

1. **Public API ADR + surface** — design the Go-idiomatic public package at the module root
   (`Parse`/`Load` returning `(value, error)`, errors slice for accumulate-and-continue, the
   live-vs-JSON projection pair, streaming via `iter.Seq`). The format work is done; nothing
   above `internal/` exists yet.
2. **Report upstream** — every entry in [FINDINGS.md](FINDINGS.md) (now 12 items) belongs in
   io-test-cases/io-specs/io-js2 issues. Per ADR 0007 this output outranks the library.
3. **Property fuzzer** (PORTING-NOTES part 3) — round-trip properties over generated documents;
   the corpus is the floor, not the ceiling.
4. **Performance pass** — now allowed (phases are done): benchmarks, allocation audit,
   profile-guided tuning. The tokenizer is already zero-alloc per token.
5. Retrospective for the Rust port (definition of done, item 4).

## Standing rules (from upstream, non-negotiable)

- `io-specs` is the authority; io-js2 is the **oracle** (probe it via `npx tsx` scripts in
  io-js2 to derive exact behavior — this settled every ambiguity of this port).
- Error **codes** are the contract; messages are not. Never invent a code.
- Gaps found here go back into `io-test-cases`/`io-specs` — that output outranks the library.
