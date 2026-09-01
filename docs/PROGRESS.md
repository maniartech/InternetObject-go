# ▶ RESUME HERE — Go port progress

Read this first in a fresh session, then `docs/decisions/0001-go-port-architecture.md`, then the
recent `git log`. The upstream cold-start guide is `io-test-cases/PORT-START-HERE.md`; the traps
are `io-test-cases/PORTING-NOTES.md`.

## Current state

- **Phase 0 in progress** — clean restart (old tree on `archive/2025-tokenizer`), conformance
  harness being wired against `bootstrap/tokenizer.csv`.
- Corpus pin: commit `0fc0af8` of `io-test-cases` (no tags exist upstream yet; see ADR 0001 D2).
- The bootstrap CSV currently holds **262** cases (the upstream docs' "255" predates the corpus's
  latest commit; the harness reports what it measures).

## Scoreboard

| Phase | Suite | Cases | Status |
| ----: | ----- | ----: | ------ |
| 1 | Tokenizer (bootstrap CSV) | 262 | **0/262** — harness wired, no tokenizer |
| 2 | Parser | 195 | not started |
| 3 | Schema | 160 | not started |
| 4 | Validation | 538 | not started |
| 5 | Serializer | 148 | not started |
| 6 | Document | 100 | not started |
| 7 | Streaming | 118 | not started |
| 8 | Regression | 51 | not started |

Run `go test ./...` — the suite prints the live numbers with the corpus pin.

## What's next

1. Phase 1: implement `internal/tokenizer` to 262/262.
2. Phase 2: parser + value model (Decimal, BigInt, temporal kinds) — the self-hosting step; the
   port then reads the `.io` corpus with its own parser.

## Standing rules (from upstream, non-negotiable)

- `io-specs` is the authority; io-js2 is the **oracle** (run it to derive exact values, never to
  decide). A spec/implementation divergence is a defect to close upstream, not to encode here.
- A phase ends at 100% with every previous phase still 100%; nothing else lands between phases.
- Error **codes** are the contract; messages are not. Never invent a code.
- Gaps found here go back into `io-test-cases`/`io-specs` — that output outranks the library.
