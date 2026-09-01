# Internet Object — Go implementation

A from-scratch, independent Go implementation of the
[Internet Object](https://internetobject.org) data-interchange format, built against the shared
conformance corpus (`io-test-cases`) with the specification (`io-specs`) as the sole authority.

**Status: in development.** Progress is measured by the corpus, phase by phase — see
[docs/PROGRESS.md](docs/PROGRESS.md) for the live scoreboard and
[docs/decisions/](docs/decisions/) for the architecture decisions.

## Conformance

The corpus is the definition of done. Every phase is gated by a number the test suite prints:

```bash
go test ./...
```

The conformance harness requires a sibling checkout of
[`io-test-cases`](https://github.com/maniartech/InternetObject-test-cases) (or the
`IO_CORPUS_DIR` environment variable pointing at one). A missing corpus **fails** the run — it
never skips.

## Design

- The **format** is portable; the **API** is not. The public surface is designed for Go —
  `(value, error)` returns, error codes exposed via a typed error — not transliterated from the
  JavaScript reference. See [ADR 0001](docs/decisions/0001-go-port-architecture.md).
- Near-zero-allocation tokenizer: tokens are compact value structs over the source text, decoded
  lazily.

## History

The 2025 tokenizer/benchmark work predates the frozen error codes and the conformance corpus and
was never checked against either; it is preserved on the `archive/2025-tokenizer` branch. This
tree is a clean restart per upstream ADR 0007.

## License

MIT
