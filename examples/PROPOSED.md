# Proposed surface — ADR 0004, by example

> **Status: design, not yet implemented.** This file shows the API agreed in
> [ADR 0004](../docs/decisions/0004-native-api-design.md) as user code, so the target
> experience is reviewable before it lands. Phases: (1) `io.Object` base, (2) documents/
> sections/collections/definitions + runtime-schema functions, (3) `iogen`.
> Everything below delegates to the same corpus-hardened engine — no second implementation.

## The gradient

Every capability exists at three levels; the higher ones are optional sugar:

| Level | You write | You call |
| ----- | --------- | -------- |
| 0 — plain (ships today for records) | structs + tags | `io.Marshal/Unmarshal/Validate` |
| 1 — embedded | + an embedded base | methods on your own types |
| 2 — generated | a `.io` schema file | typed, unbypassable setters |

## Level 1: the four embeddable bases

```go
type Employee struct {
    io.Object                        // record base: Set/Get/Validate/Marshal
    Name string `io:"name" schema:"{string, minLen: 2}"`
    Age  int    `io:"age"  schema:"int, min: 0, max: 130"`
}

emp := io.New[Employee]()            // constructs AND attaches
err := emp.Set("age", -1)            // ErrorList{mismatched-min}; field untouched
err  = emp.Set("age", 50)            // nil; emp.Age is now 50
age, _ := emp.Get("age")
err  = emp.Validate()
text, _ := emp.Marshal()

// Literal construction attaches once; package twins never need attaching:
e2 := &Employee{Name: "Bo", Age: 1}; e2.Attach(e2)
err = io.Set(&e2, "age", 50)         // same engine, zero ceremony
```

```go
type Dashboard struct {
    io.Document                      // document base: Load/Marshal/Header/Var
    Defs    AppDefs                 `io:"header"`
    Joinees io.Collection[Employee] `io:"joinees"`   // rows + row faults
    Stats   Stats                   `io:"stats"`     // single-record section
}

d := io.New[Dashboard]()
err := d.Load(text)                  // multi-section binding, ErrorList on faults
for _, e := range d.Joinees.Items() {}   // the good rows
for _, re := range d.Joinees.Errors() {} // row 7: expected-integer (accumulate-and-continue)
d.Joinees.Add(emp)                   // validates on insert
out, _ := d.Marshal()
```

```go
type AppDefs struct {
    io.Definitions                   // typed header, reusable across documents
    AdminEmail string `io:"@adminEmail"`   // @variable
    MaxRetries int    `io:"@maxRetries"`
    Env        string `io:"env"`           // plain header definition
}
```

**No `io.Section` base, on purpose** — a section is a named slot in a document; the field +
tag expresses it completely. Dynamic access: `doc.Section("joinees")`.

## Level 0 twins of the same examples

```go
type Dashboard struct {                       // zero framework types
    Joinees []Employee `io:"joinees"`         // strict: any row fault fails the load
    Stats   Stats      `io:"stats"`
}
var d Dashboard
err := io.Unmarshal(text, &d)
text, err := io.Marshal(d)

// io.Collection[Employee] as a FIELD is still Level 0 (like time.Time) — reach
// for it only when you want good-rows-plus-row-errors instead of all-or-nothing.
```

## Dynamic navigation (no structs at all)

```go
doc, _ := io.Parse(text)
sec, ok := doc.Section("joinees")
for rec, rowErr := range sec.Records() {     // iter.Seq2[*io.Record, error]
    age, _ := rec.Get("age")
}
v, _ := doc.Var("adminEmail")
emps, err := io.SectionAs[Employee](sec)     // bridge one section into types
for e, err := range io.StreamAs[Employee](r, nil) {} // typed streaming
```

(`io.Record` is the renamed dynamic object — today's `io.Object` alias; the base takes the
`Object` name.)

## Runtime schemas — SHIPPED, see [05-runtime-schema](05-runtime-schema/main.go)

`ParseSchema` / `SchemaFor[T]` / `doc.SchemaOf(name)` produce a compiled `*io.Schema` that
drives `UnmarshalWith`, `ValidateWith`, `MarshalWith`, `ParseWith` and
`StreamOptions{Schema:}`. Still proposed here: `emp.AttachSchema(s)` on the bases, so
`Set`/`Validate` use it.

Precedence: attached > document header > tag-derived. `io` tags always remain the
name-binding contract.

## Level 2: generated (`iogen`)

```go
//go:generate iogen -in user.io -pkg model
u, err := model.NewUser("Alice", 30)         // cannot construct an invalid value
err  = u.SetAge(131)                         // typed, compile-checked name, engine-checked value
text, _ := u.MarshalIO()                     // static binding, no reflection
```

Generated code contains zero semantic logic (the 1-1 rule) and ships with generated
differential tests: `MarshalIO ≡ io.Marshal`, setter verdicts ≡ `io.Validate`.
