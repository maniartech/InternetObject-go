# SPEC 0004 — A Go-native public API

- **Status:** DRAFT, 2026-09-14. §A is a list of **bugs** and is being fixed without waiting (the
  owner's standing bar: A+, "must never look like Java"). §B needs **owner decisions**, because
  each one contradicts a recorded ADR or breaks the public API.
- **Source:** an independent Go-idiom audit of the public surface, 2026-09-14, run as the review
  gate SPEC 0003 §7 asks for. Every §A item was reproduced by a probe program before being written
  here (`go run` / `go test -race` against this repository).
- **Bar:** what a Go reviewer expects from a codec next to `encoding/json`, `encoding/xml`,
  `gopkg.in/yaml.v3` and `BurntSushi/toml`.

## A. Bugs — fix now, no design question

| # | Defect (reproduced 2026-09-14) | Fix |
| --- | --- | --- |
| A1 | **Data race on a shared `*Document`.** `Document.SchemaOf` writes `Definitions`' memo maps unlocked; 8 goroutines under `-race` report DATA RACE. The README promises a parsed Document is safe to share. | Make the lookup race-free (guarded memo, or compiled at load), and a `-race` test that shares one Document. |
| A2 | **`Marshal(nil)` panics** (`reflect: call of reflect.Value.Type on zero Value`); likewise `MarshalWith`/`Validate`. | Return an error, as every other invalid input does. |
| A3 | **Silent corruption of opaque types.** A struct with no exported fields (`netip.Addr`, a generated guarded type) marshals as `{}`; a `[4]byte` with `MarshalText` as `[0, 0, 0, 0]`. | Refuse what cannot be represented with an error naming the type. Honouring `encoding.TextMarshaler` is a feature: §B2. |
| A4 | **Write errors lose their chain.** `StreamMarshaler` flattens a writer's error to `*MarshalError{Msg: err.Error()}`; `errors.Is(err, net.ErrClosed)` is false. | Keep the cause (`Unwrap`). |
| A5 | **`Error` breaks its documented contract** on four paths (`ParseSchema`, `Document.SchemaOf`, `ParseWith(nil)`, `Stream`'s fatal error): empty `Category`, `RecordIndex` 0 where the doc says -1, a fabricated 1:1. `Collection[T].Errors()` reports `$.people[0][0] (0:0)` and drops the Go cause. | One constructor for every `Error`, as ADR 0005 intends. |
| A6 | **Binding errors leak internal names:** `cannot store *core.Object in string`. | Name the wire kind (`object`), not the Go type behind it. |
| A7 | **Zero values panic:** `var b Builder; b.Section(…)` (nil map); a nil `*Section` from `doc.Section("missing")` panics on any method, while `Definitions` and `Collection` are nil-safe. | Useful zero `Builder`; nil-safe `*Section` like `*Definitions`. |
| A8 | **Schema lookups allocate a new `*Schema` per call** (`defs.Schema("p") != defs.Schema("p")`), discarding the per-schema header cache: `MarshalWith(v, defs.Schema("p"))` in a loop re-renders the header every time. | Intern the wrapper. (Also a SPEC 0003 performance item.) |
| A9 | **`Definitions.Stream` silently ignores the receiver** when `opts.Definitions` is set, and re-parses its own rendered text per stream. | Decide precedence explicitly, and pass the compiled form. |
| A10 | **Docs that are false:** `JSON` doc says temporals keep their kind (ADR 0008 removed that); `Definitions.String()` doc says no separator, returns `"---"` for nil; `marshal.go` cites a file that does not exist. | Correct the docs to the behaviour. |
| A11 | **iogen breaks its own guarantee:** a guarded type's `Tags()` returns, and `SetTags` stores, the caller's slice, so mutating it bypasses validation. | Clone on the way in and out; `sync.OnceValues` for the schema. |
| A12 | **No runnable `Example` functions**, so no doc snippet is compiled. | `ExampleMarshal`, `ExampleUnmarshal`, `ExampleParse`, `ExampleStream`, `ExampleCollection`, `ExampleBuilder`. |

Each lands with a black-box test that failed before the fix.

## B. Owner decisions — design, or breaking

Recommendation first; the audit's reasoning in one line each.

1. **The `io` import alias** (ADR 0002 D1) shadows the standard library's `io` — and this package's
   own API takes `io.Reader`/`io.Writer`, which is why its files already import `goio "io"`. Go
   reviewers treat shadowing a stdlib package as a hard no. **Recommend** documenting a
   non-colliding name everywhere (e.g. `iobj`), changing no code.
2. **Custom marshaling.** Honour `encoding.TextMarshaler`/`TextUnmarshaler` (every major Go codec
   does), and add `Marshaler`/`Unmarshaler` interfaces with format-suffixed methods
   (`MarshalIO`/`UnmarshalIO`, as `MarshalJSON`/`MarshalYAML`/`MarshalTOML` — the suffix exists
   because one type implements several codecs). ADR 0003 D7 deferred this; ADR 0004 D0 bans the
   `…IO` suffix. **Recommend** both, amending D0 for method names only.
3. **The streaming writer's name.** `StreamMarshaler` reads as an interface in Go (an `-er` name
   for a concrete struct), and `MarshalAs(v, name)` uses `As` against the package's own rule that
   `…As[T]` produces a T. The idiom is `NewEncoder(w, opts) *Encoder` / `Encode(v) error`, which
   ADR 0004 D0 bans. **Recommend** `Encoder`/`Encode`, `EncodeWith(v, schemaName)`, and splitting
   reader and writer options (`StreamOptions.DefaultSchema` is silently ignored by the writer).
4. **`[]byte`, append and `io.Writer` entry points** (ADR 0003 chose `string`; ADR 0006 F5 proposes
   them). **Recommend** adding `AppendMarshal(dst []byte, v any) ([]byte, error)` now; a `[]byte`
   Unmarshal only with a documented copy, since `string` input is what makes the lazy decoder's
   zero-copy substrings safe.
5. **Public value types instead of aliases to `internal/core`.** Godoc cannot show methods of
   `Decimal`, `Object`, `Member` (`go doc . Decimal.Equal` fails), and the aliases expose
   representation (`Decimal.Coef *big.Int` on an immutable type). **Recommend** defining them in
   the root package with unexported fields.
6. **`StreamItem.Err *Error`** is a typed-nil trap (`return item.Err` as `error` is non-nil).
   **Recommend** `Err error`.
7. **Lookup shapes.** `Document.SchemaOf(name) (*Schema, error)`, `Definitions.Schema(name) *Schema`,
   `Definitions.Var(name) (any, bool)`; `Section.SchemaName()` is `"p"` where `StreamItem.SchemaName`
   is `"$p"`. **Recommend** `(T, bool)` throughout and one spelling of a schema name.
8. **Sentinels:** `ErrSectionNotFound`, `ErrNilSchema`, and an `UnmarshalTypeError{Path, Value,
   Type}` (the `*UnsupportedTypeError` ADR 0003 D3 promised was never built). **Recommend** yes.
9. **Small ones:** `TimeAnchor` is a mutable exported copy (→ `func TimeAnchor() time.Time`);
   `type Category string`; `Code` implementing `error` so `errors.Is(err, MismatchedMin)` works;
   file names with underscores instead of hyphens; godoc that cites ADR numbers and io-js2 paths
   users cannot see; a mixed-case module path (`InternetObject-go`). **Recommend** all but the
   module path, which is the owner's brand call.

## Already idiomatic — do not churn

`Marshal`/`Unmarshal`/`Validate` mirroring `encoding/json`; the `io` tag using json's grammar and a
separate `schema` tag like `validate:`; the `…With(…, *Schema)` suffix; no `Get` prefixes; no
Manager/Factory/Impl types; generics where they earn it (`SchemaFor[T]`, `SectionAs[T]`,
`Collection[T]`); `iter.Seq2` for streaming; `ErrorList` with `Unwrap() []error`; typed `Code`
constants; nil-able options structs with zero-value defaults (the `slog.HandlerOptions` precedent);
an idempotent `Close` that leaves the underlying writer open.

## ▶ RESUME HERE

- **State:** DRAFT. §A fixed — A1 committed (`601d7fd`), A2–A12 in review. §B awaits the owner.
- **How each §A item landed** (every one with a black-box test shown to FAIL on the old code):
  - A1 `Definitions` resolves every named schema when built and never writes those maps again; the
    inline schema is published through atomic pointers. A mutex was built first and rejected on
    measurement (receiver escape → heap allocations on every MarshalWith).
  - A2 `topValue` — one statement of "unwrap a top-level value, nil at any level refused".
  - A3 refused: structs with no field to write (every field unexported or `io:"-"`; `Collection[T]`
    excepted) and types defining `encoding.TextMarshaler` on the value or the pointer (`time.Time`,
    `Decimal`, `big.Int` excepted) — one rule, `definesText`, asked of declared types at plan time and
    of values met at run time. A `Collection[T]` of such a T fails the load; it is not a row fault.
    **This is a deliberate break:** types that round-tripped exactly through their underlying kind
    — `slog.Level`, named string/int enums with `MarshalText`, `[16]byte` UUID types, generics over
    them — are now refused, for Marshal and Unmarshal alike. Refusing is chosen over keeping the old
    form because §B2, if it honours `TextMarshaler`, would otherwise change those documents'
    spelling silently.
  - A4 `MarshalError.Err` + `Unwrap`; `StreamMarshaler` keeps the writer's error.
  - A5 every public `Error` goes through `toError`; a Collection's section base path was built
    from a literal whose zero index meant "element 0". Found while writing A12's examples: an Error
    with no source position printed `(0:0)`; it now prints none, and `ParseWith(nil)` and a fatal
    `Stream` fault no longer claim 1:1. **Not done:** a Collection binding fault still drops its Go
    cause ("cannot store number in int") — keeping it needs a new field on `Error`, which is §B8.
  - A6 `core.KindName` names the format's kinds in binding errors. (A `time.Time` is always
    `datetime`: the value model does not keep a temporal's declared kind — ADR 0008.)
  - A7 the zero `Builder` works; every method of a nil `*Section` returns its zero result.
  - A8 `Definitions` hands out one `*Schema` per compiled schema, so an alias and its target are
    the same value; this is not promised in godoc, so §B7 stays free. (`Document.Schema`/`SchemaOf` still mint
    per call; a document is per-parse, so its header cache rarely matters — revisit with §B7.)
  - A9 `Definitions.Stream` passes its compiled header to the reader as a read-only parent instead
    of re-parsing rendered text, and no longer drops it when `opts.Definitions` is set.
  - A10 docs corrected; empty `Definitions.String()` is `""` as documented.
  - A11 generated types copy slices, pointers and `*big.Int` in and out; `sync.OnceValues`.
  - A12 `example_test.go`: ten runnable examples.
- **Review of A2–A12 (2026-09-14): changes required, all fixed** — a Collection absorbed A3's
  refusal; A11's generated code did not compile for members named `slices` or like a helper; the
  plan-time and run-time text-form rules disagreed; an in-stream bare schema expression lost to a
  preloaded `$schema` in `Stream` (now: the later layer's default wins, as in `Parse`; pinned by
  `TestStreamDefaultSchemaPrecedence`).
- **Confirmed** by the reviewer the same day (approved with nits, applied: a `Collection` of a
  non-struct type is an error, not a panic; a struct whose every field is `io:"-"` is refused too).
- **Next:** SPEC 0003 §5.2.
