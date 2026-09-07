# SPEC 0002 — Decimal

- **Status:** IMPLEMENTED 2026-09-07. Landed green: corpus 1,572+262, -race, seven fuzzers,
  100% statement coverage on `internal/core/decimal.go`, hot benchmarks unmoved.
- **Decides:** the value semantics and API of `core.Decimal`. Sits under
  [ADR 0011](../decisions/0011-core-model-and-layout.md) and follows
  [SPEC 0001](core-value-model.md) §1's lowering rules: `core` depends on nothing.
- **Oracle:** io-js2 `src/core/decimal/decimal.ts` for *behaviour*; the format spec
  `io-specs/…/data-types/number/decimal.md` for *validation*. Where the two disagree, §6.

---

## 1. What exists, and why it is not enough

```go
type Decimal struct {
    Coef  *big.Int
    Scale int
}
func (d Decimal) String() string
```

That is the whole type. It can be parsed and written, and nothing else: no constructor, no
comparison, no arithmetic, no rounding, no way to ask its precision. Meanwhile the behaviour a
decimal needs *does* exist — `cmpDecimal`, `decimalMultiple`, `decimalDigits`, `pow10` — as
unexported helpers inside `internal/schema/type-decimal.go`, written where validation needed
them and reachable from nowhere else. That is the same defect SPEC 0001 found in `Object`:
the pipeline did the value's job for it.

Two traps follow from the bare struct and must be closed by this spec:

- **`==` is identity, not equality.** `Coef` is a pointer, so two decimals parsed from the
  same text compare unequal with `==`. Go cannot forbid `==` on the struct; the API must make
  reaching for it unnecessary and the doc must say so.
- **Aliasing.** Any method that returns a `Decimal` sharing `Coef` with its receiver makes a
  later in-place `big.Int` operation on one silently change the other. Every result is a fresh
  coefficient. `Coef` is never mutated after construction.

---

## 2. Invariants

1. **A `Decimal` is immutable.** Methods return new values. `Coef` is never written after
   construction.
2. **The zero value is zero.** `var d Decimal` behaves as `0m` in every method, never panics
   on the nil coefficient. Already true of `String`; becomes true of everything.
3. **Scale is part of the value.** `1.5m` and `1.50m` are different values with the same
   magnitude — the writer preserves scale end to end, and the corpus pins it. Which equality a
   caller wants is therefore a choice they make explicitly (§4).
4. **Precision is derived, never stored.** A literal declares no precision; it *has* one. §3.
5. **No hot-path change.** Parsing a literal stays `DecimalParts` → `Decimal{coef, scale}`.
   Nothing here adds an allocation to parsing, validation or writing.

---

## 3. Precision — the definition, and a divergence it exposes

The format defines `precision`/`scale` as SQL `DECIMAL(precision, scale)`: scale is the exact
fractional digit count; precision is the *total digit capacity*. In SQL, `0.05` needs
`DECIMAL(2,2)`: its scale alone is two digits, so its precision is at least two.

The reference computes exactly that (`initFromString`: integer digits with leading zeros
stripped, plus fractional digits). **io-go computes something else** — `decimalDigits` counts
the digits of the coefficient, so `0.05m` has coefficient `5` and precision **1**.

Probed 2026-09-07:

| schema | value | io-js2 | io-go |
| --- | --- | --- | --- |
| `{decimal, precision: 1}` | `0.05m` | `mismatched-precision` | **passes** |
| `{decimal, precision: 1}` | `0.5m` | passes | passes |
| `{decimal, precision: 2}` | `100m` | `mismatched-precision` | `mismatched-precision` |

The corpus has no case with a leading-zero fraction, so nothing caught it. The spec's prose
says "significant digits", which read mathematically gives io-go's answer — but the same
section defines the pair as SQL `DECIMAL(p, s)`, which gives the reference's, and the reference
is the oracle. **This port follows the oracle**, and the prose ambiguity is escalated as
finding #25 with a corpus case that would pin it.

**Definition.** `Precision() = max(len(digits(|Coef|)), Scale)`, with `0` having one digit.
This is the SQL number in one expression: every case in the table above, and `0.50m` → 2,
`0m` → 1, `0.0m` → 1, `123.45m` → 5.

---

## 4. Comparison — two equalities, both named

```go
func (d Decimal) Cmp(o Decimal) int      // numeric: scales aligned, -1 / 0 / +1
func (d Decimal) Equal(o Decimal) bool   // numeric: Cmp == 0;  1.5m.Equal(1.50m) == true
func (d Decimal) Same(o Decimal) bool    // structural: equal scale AND coefficient
func (d Decimal) Sign() int
func (d Decimal) IsZero() bool
```

`Cmp` is what validation's `min`/`max`/`multipleOf` already do (scale-aligning), promoted onto
the type; `type-decimal.go` calls it instead of carrying its own copy. `Same` is what
`core.Equal` already does for two decimals and stays what `choices` and the corpus adapter use
(§6.1). The reference's `compareTo` *throws* on unequal precision or scale; Go gets a total
order instead, and the validator's own scale-normalisation step disappears because `Cmp` does
it.

---

## 5. Arithmetic — `math/big` vocabulary, explicit scale where it matters

```go
func (d Decimal) Neg() Decimal
func (d Decimal) Abs() Decimal
func (d Decimal) Add(o Decimal) Decimal              // scale = max(d.Scale, o.Scale)
func (d Decimal) Sub(o Decimal) Decimal              // scale = max
func (d Decimal) Mul(o Decimal) Decimal              // scale = d.Scale + o.Scale
func (d Decimal) Quo(o Decimal, scale int) (Decimal, error)   // half-up at scale; zero divisor is an error
func (d Decimal) Rem(o Decimal) (Decimal, error)     // sign of dividend; scale = max
func (d Decimal) Round(scale int) Decimal            // half away from zero
func (d Decimal) Ceil(scale int) Decimal
func (d Decimal) Floor(scale int) Decimal
func (d Decimal) Rescale(scale int) (Decimal, bool)  // exact or false; never rounds silently
```

Scale rules for `Add`/`Sub`/`Mul`/`Rem` and half-up rounding are the reference's exactly
(`alignOperands`, `roundHalfUp`). Two deliberate differences:

- **`Quo` takes the result scale.** The reference's `div` uses the *divisor's* scale — `1.0m /
  3m` yields scale 0 — while its own RDBMS helper (`calculateDivisionResultPrecisionScale`,
  min scale 6) is never called by `div`. That is an inconsistency inside the reference, not a
  format rule (io-specs says nothing about arithmetic), and copying it would bake a surprise
  into a Go API. Explicit scale is what `big.Float`, `shopspring/decimal` and SQL all do.
  Recorded as a reference note in the findings, not escalated: arithmetic is outside the
  format's scope.
- **Division by zero is an `error`, not a panic.** A decimal is data; data divides by zero.

Rounding mode is **half away from zero** everywhere (`Round`, `Quo`) — the reference's
`roundHalfUp` on the absolute value with the sign restored, which is that mode. Named once,
implemented once, used by both.

Results carry only coefficient and scale (invariant 4). The reference also computes a
"result precision" per operation for later constraint checks; here `Precision()` of the result
answers that on demand.

---

## 6. Where this port and the reference disagree — the findings

### 6.1 `choices` on a decimal never matches in the reference — reference defect

`decimal.ts` calls `doCommonTypeCheck` **without** an `equalityComparator`, so choices fall
through to `val === choice` on two `Decimal` objects — identity — and never match. Probed
2026-09-07: `{decimal, choices: [1.5m, 2m]}` rejects `1.5m` itself with `mismatched-choice`.

io-go matches structurally (`Same`): `1.5m` passes, `1.50m` and `2.0m` are rejected. That is
the reading consistent with invariant 3 and with `core.Equal` as the corpus adapter uses it.
The corpus has no decimal `choices` case, so the reference's defect is invisible to it.
Escalated as finding #24 with a corpus case; io-go keeps `Same` until the format says which
equality `choices` means for decimals.

### 6.2 Precision of a leading-zero fraction — spec prose vs SQL definition

§3. Finding #25. **Closed:** io-go now computes `max(digits, scale)` and agrees with the
oracle on every probed case, pinned by `TestPrecisionMatchesTheOracle`.

### 6.3 `mul` silently loses the product in the reference — found during implementation

Probing the oracle to build the differential table (§8.1) turned up a data-loss defect nobody
was looking for. `decimal.ts` `mul()` computes the exact product at `scale1 + scale2` and then
rounds it down to `max(scale1, scale2)`:

| expression | reference | exact |
| --- | --- | --- |
| `0.01 * 0.01` | `0.00` | `0.0001` |
| `0.001 * 0.002` | `0.000` | `0.000002` |
| `1.5 * 1.5` | `2.3` | `2.25` |

The sum of the scales is exactly the scale at which a product is exact, so that rounding step
can only destroy information — and when the product is smaller than the operands' own scale it
destroys all of it. This is more serious than the `div` note above: division has no exact
answer so any scale is defensible, but multiplication always has one.

io-go's `Mul` is exact. The fuzzer asserts the property directly — *a product is zero only when
an operand is* — which the reference fails. Escalated as finding #26.

---

## 7. Construction and conversion

```go
func ParseDecimal(s string) (Decimal, error)      // "1.50", "-0.05"; the literal grammar minus the m
func NewDecimal(coef int64, scale int) Decimal    // 150, 2 → 1.50m
func DecimalFromBig(coef *big.Int, scale int) Decimal   // copies coef
func (d Decimal) Float64() float64                // lossy, documented as such
func (d Decimal) Precision() int
func (d Decimal) String() string                  // exists; unchanged
```

`ParseDecimal` reuses the tokenizer's literal rules through one shared function so a string
accepted here is a string the parser accepts, and vice versa — one grammar, one site.
Vocabulary follows ADR 0004 D0: `Parse`/`String` are the pair; no `Load`, `Read`, `Write`.

---

## 8. Test obligations

Beyond SPEC 0001 §7 (corpus, `-race`, fuzzers, benchmarks unchanged):

1. **Differential against the oracle.** A table of `Add`/`Sub`/`Mul`/`Rem`/`Round` cases with
   the reference's outputs captured by probe, asserted equal — scale included.
2. **Properties, fuzzed.** For arbitrary `Decimal` a, b: `ParseDecimal(a.String()) Same a`;
   `a.Cmp(b) == -b.Cmp(a)`; `a.Add(b).Sub(b).Cmp(a) == 0`; `a.Mul(one).Same(a)` when `one`
   has scale 0; `a.Round(s).Scale == s`; zero value participates in every operation.
3. **The two divergences pinned locally**: `0.05m` against `precision: 1` fails; `1.5m`
   against `choices: [1.5m]` passes. Both mirror the corpus cases proposed in the findings, so
   when the corpus gains them the local tests can be retired.
4. **Aliasing.** A test that takes `a.Add(b)`, then mutates `b.Coef` through the pointer, and
   asserts the sum is unchanged — the one way the invariant can be violated, checked directly.

---

## ▶ RESUME HERE

**Done.** `internal/core/decimal.go` holds the type; `decimal.go` exposes it as `io.Decimal`
with `ParseDecimal`, `NewDecimal`, `DecimalFromBig`, `ErrDivideByZero`. The validator's four
private helpers are gone — `internal/schema/type-decimal.go` now calls `Cmp`, `Precision` and
`IsMultipleOf` on the value itself.

Gates: 1,572+262 corpus, `-race`, `FuzzDecimalProperties` (15.9M execs) and `FuzzParseDecimal`
(13.7M) plus the five existing fuzzers, 100% statement coverage on the type, and the four hot
benchmarks unchanged to the allocation (§1 invariant 5 holds).

Findings #24 (decimal `choices` never match — still open, io-go keeps structural equality),
#25 (**closed**), #26 (`mul` data loss — new) in `io-js2/.private/docs/go-port/FINDINGS.md`.

**Next: SPEC 0001 §4.9** — the `Code` constants.
