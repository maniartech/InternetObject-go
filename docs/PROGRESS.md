# ▶ RESUME HERE — Go port progress

Read this first in a fresh session, then `docs/decisions/0001-go-port-architecture.md`, then the
recent `git log`. The upstream cold-start guide is `io-test-cases/PORT-START-HERE.md`; the traps
are `io-test-cases/PORTING-NOTES.md`.

## Current state — THE FULL CORPUS IS GREEN

Every suite passes, against `io-test-cases` commit `e6f288c` (still untagged upstream — ADR 0007
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

## Fuzzing — DONE, all layers green

Three layers, all clean after fixing what they found:

- **Property fuzzer** (`internal/document/fuzz_roundtrip_test.go`, PORTING-NOTES part 3):
  deterministic 8-seed value generator → write → re-parse → strict value compare →
  idempotence. Quick gate (8×400) runs in every `go test`; the PORTING-NOTES gate is
  `IO_FUZZ_SOAK=1` (8×3,000; `IO_FUZZ_ROUNDS` deepens it). **Green at 240,000 documents.**
- **Byte fuzzer** (`fuzz_test.go`): coverage-guided `go test -fuzz=FuzzParse` — never panic;
  a CLEAN parse's output re-parses cleanly and idempotently. **Green at 63M execs / 10 min.**
- **Stream fuzzer** (`FuzzStream`): never panic; whole-buffer vs per-byte chunking yield
  identical item sequences. **Green at 57M execs / 5 min.**
  Regression inputs live in `testdata/fuzz/`.

The fuzzers found ~15 real bugs AFTER the full corpus was green — writer enclosure at bare emit
sites, control-character and `\r` strings written bare, exponent-suffix quoting (writer now asks
the reader's own classifier), absent holes written as `N`, `*`-only headers dropped, deferred
literal errors masked in headers and under `any`, schema alias cycle stack overflow, bigint
exponent DoS, unspellable UTC datetimes, `schema: $ref` long form, `--- $$` section names,
surplus positional members skipping validation. Upstream-relevant ones are FINDINGS 10–13 plus
the "suggested corpus cases" list.

## Struct marshaling — DONE (ADR 0003)

`Marshal`/`Unmarshal` at the root package: schema derived from the struct type via the `io`
tag (json's grammar — rename, `-`, `omitempty`; pointer = nullable; `,date`/`,time` for
`time.Time` kinds), data written positionally through the fuzz-hardened canonical writer,
unmarshal validating against the embedded schema (ErrorList) and binding schema-less records
positionally. Plans cached in a `sync.Map`; concurrent-safe (tested). ~1.7µs marshal /
~1.9µs unmarshal per 100-field-record document row. The kitchen-sink round trip found and
fixed a validation bug: the `*` wildcard counted as a declared member in ISSUE-15 absorption
(`{*: int}` with keyed data absorbed instead of validating per member — oracle-pinned fix).

Constraints (ADR 0003 D5–D6): the `schema` struct tag holds the member's IO annotation
verbatim (`schema:"{int, min: 0, max: 130}"`, braces optional), compiled by the one compile
site at plan build (bad tag = designated code at first use). Marshal auto-validates whenever
the type carries constraints; `Validate(v)` runs the check on demand (the Go spelling of
"validate on mutation" — field assignment cannot be intercepted); `SchemaFor[T]()` +
`Schema.String()` expose the derived schema (String round-trips through ParseSchema).

## What's next

1. **Report upstream** — every entry in [FINDINGS.md](FINDINGS.md) (13 numbered + corpus-case
   suggestions) belongs in io-test-cases/io-specs/io-js2 issues. Per ADR 0007 this output
   outranks the library.
2. **Performance pass** — benchmarks, allocation audit, profile-guided tuning. The tokenizer is
   already zero-alloc per token.
3. Retrospective for the Rust port (definition of done, item 4).

## Standing rules (from upstream, non-negotiable)

- `io-specs` is the authority; io-js2 is the **oracle** (probe it via `npx tsx` scripts in
  io-js2 to derive exact behavior — this settled every ambiguity of this port).
- Error **codes** are the contract; messages are not. Never invent a code.
- Gaps found here go back into `io-test-cases`/`io-specs` — that output outranks the library.
